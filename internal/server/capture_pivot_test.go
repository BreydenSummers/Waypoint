package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestCapturePropagatesOrderedPivotsThroughAuditAndAPI(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceDir := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceDir)

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actionSourceID := "44444444-4444-4444-8444-444444444444"
	stdout := []byte("pivoted")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	actorToken := "human-test-token"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(actorToken))

	envelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa9",
		"sourceAgent": map[string]any{
			"id":       actionSourceID,
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "attacks",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nc",
		"argv":        []string{"nc", "-vz", "10.0.0.5", "443"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "host", "value": "10.0.0.5"},
		"timing": map[string]any{
			"startedAt":  "2025-01-15T10:00:00.000Z",
			"endedAt":    "2025-01-15T10:00:01.000Z",
			"durationMs": 1000,
		},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost": map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":   map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{
				map[string]any{"type": "ssh", "host": "jump.example", "port": 22, "label": "first hop"},
				map[string]any{"type": "socks5", "host": "pivot.internal", "port": 1080, "label": "second hop"},
			},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": hex.EncodeToString(stdoutSum[:])},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": hex.EncodeToString(stderrSum[:])},
		},
		"parsing": map[string]any{"status": "raw"},
	}

	resp := doCaptureRequest(t, HandlerWithDB(db), actorToken, "pivot-request", envelope, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("capture status = %d, want %d", resp.Code, http.StatusCreated)
	}

	var data string
	if err := db.QueryRowContext(ctx, `SELECT data::text FROM audit_event WHERE engagement_id = $1 AND type = 'capture.accepted' ORDER BY id DESC LIMIT 1`, engagementID).Scan(&data); err != nil {
		t.Fatalf("load audit event: %v", err)
	}
	var auditParsed struct {
		Network struct {
			PivotChain []struct {
				Type  string `json:"type"`
				Host  string `json:"host"`
				Port  int    `json:"port"`
				Label string `json:"label"`
			} `json:"pivotChain"`
		} `json:"network"`
	}
	if err := json.Unmarshal([]byte(data), &auditParsed); err != nil {
		t.Fatalf("decode audit event data: %v", err)
	}
	wantChain := []struct {
		Type  string
		Host  string
		Port  int
		Label string
	}{
		{"ssh", "jump.example", 22, "first hop"},
		{"socks5", "pivot.internal", 1080, "second hop"},
	}
	if len(auditParsed.Network.PivotChain) != len(wantChain) {
		t.Fatalf("audit event missing ordered pivots: %s", data)
	}
	for i, want := range wantChain {
		got := auditParsed.Network.PivotChain[i]
		if got.Type != want.Type || got.Host != want.Host || got.Port != want.Port || got.Label != want.Label {
			t.Fatalf("audit event pivot[%d] = %+v, want %+v (data=%s)", i, got, want, data)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/actions", nil)
	req.Header.Set("Authorization", "Bearer "+actorToken)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	rr := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("actions status = %d, want %d", rr.Code, http.StatusOK)
	}
	var actions actionPageResponse
	if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&actions); err != nil {
		t.Fatalf("decode actions: %v", err)
	}
	if len(actions.Items) != 1 || len(actions.Items[0].Capture.Network.PivotChain) != 2 || actions.Items[0].Capture.Network.PivotChain[0].Host != "jump.example" || actions.Items[0].Capture.Network.PivotChain[1].Host != "pivot.internal" {
		t.Fatalf("actions pivot chain = %#v", actions)
	}
}

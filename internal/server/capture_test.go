package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

type captureAckResponse struct {
	ContractVersion  string `json:"contractVersion"`
	ActionID         string `json:"actionId"`
	CaptureID        string `json:"captureId"`
	AuditEventCursor string `json:"auditEventCursor"`
	ReceivedAt       string `json:"receivedAt"`
	Idempotency      string `json:"idempotency"`
	ClockSkew        *struct {
		Status   string `json:"status"`
		OffsetMs int64  `json:"offsetMs"`
	} `json:"clockSkew"`
}

type problemResponse struct {
	Code        string `json:"code"`
	Status      int    `json:"status"`
	RequestID   string `json:"requestId"`
	ExistingID  string `json:"existingActionId"`
	FieldErrors []struct {
		Pointer string `json:"pointer"`
		Code    string `json:"code"`
	} `json:"fieldErrors"`
}

func TestCaptureIngestCreatesReplaysAndRejectsChangedPayload(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actionSourceID := "44444444-4444-4444-8444-444444444444"
	stdout := []byte("abc")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	stdoutHash := hex.EncodeToString(stdoutSum[:])
	stderrHash := hex.EncodeToString(stderrSum[:])
	actorToken := "human-test-token"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(actorToken))

	baseEnvelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent": map[string]any{
			"id":       actionSourceID,
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nmap",
		"argv":        []string{"nmap", "-sV", "192.0.2.10"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "ip", "value": "192.0.2.10"},
		"timing": map[string]any{
			"startedAt":  "2025-01-15T10:00:00.000Z",
			"endedAt":    "2025-01-15T10:00:01.000Z",
			"durationMs": 1000,
		},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": stdoutHash},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": stderrHash},
		},
		"parsing": map[string]any{"status": "raw"},
	}

	resp := doCaptureRequest(t, HandlerWithDB(db), actorToken, "aaaa1111", baseEnvelope, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", resp.Code, http.StatusCreated)
	}
	var ack captureAckResponse
	decodeResponse(t, resp, &ack)
	if ack.Idempotency != "created" || ack.CaptureID != baseEnvelope["captureId"].(string) {
		t.Fatalf("ack = %#v", ack)
	}
	if ack.AuditEventCursor == "" || ack.ActionID == "" {
		t.Fatalf("ack missing ids: %#v", ack)
	}

	resp = doCaptureRequest(t, HandlerWithDB(db), actorToken, "aaaa1111", baseEnvelope, stdout, stderr)
	if resp.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want %d", resp.Code, http.StatusOK)
	}
	decodeResponse(t, resp, &ack)
	if ack.Idempotency != "replayed" {
		t.Fatalf("replay ack = %#v", ack)
	}

	mutated := cloneMap(baseEnvelope)
	mutated["command"] = "/usr/bin/other"
	resp = doCaptureRequest(t, HandlerWithDB(db), actorToken, "aaaa1111", mutated, stdout, stderr)
	if resp.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d", resp.Code, http.StatusConflict)
	}
	var prob problemResponse
	decodeResponse(t, resp, &prob)
	if prob.Code != "idempotency_conflict" || prob.ExistingID != ack.ActionID {
		t.Fatalf("problem = %#v", prob)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1`, engagementID).Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count != 2 {
		t.Fatalf("audit_event count = %d, want 2", count)
	}

	var eventType, data string
	if err := db.QueryRowContext(ctx, `SELECT type, data::text FROM audit_event WHERE subject_id = $1 AND type = 'capture.accepted' ORDER BY id DESC LIMIT 1`, ack.ActionID).Scan(&eventType, &data); err != nil {
		t.Fatalf("load accepted audit event: %v", err)
	}
	if eventType != "capture.accepted" || !bytes.Contains([]byte(data), []byte(`"egressStatus":"disabled"`)) {
		t.Fatalf("accepted audit event unexpected: type=%s data=%s", eventType, data)
	}
}

func TestCapturePersistsEvidenceAndRecoversOrphans(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tempEvidenceDir := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", tempEvidenceDir)

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actionSourceID := "44444444-4444-4444-8444-444444444444"
	stdout := []byte("abc")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	stdoutHash := hex.EncodeToString(stdoutSum[:])
	stderrHash := hex.EncodeToString(stderrSum[:])
	actorToken := "human-test-token"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(actorToken))

	orphanPath := filepath.Join(tempEvidenceDir, "captures", strings.Repeat("f", 64), "stdout")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o750); err != nil {
		t.Fatalf("mkdir orphan dir: %v", err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o600); err != nil {
		t.Fatalf("write orphan evidence: %v", err)
	}

	envelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "cccccccc-cccc-4ccc-8ccc-ccccccccccc1",
		"sourceAgent": map[string]any{
			"id":       actionSourceID,
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nmap",
		"argv":        []string{"nmap", "-sV", "192.0.2.10"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "ip", "value": "192.0.2.10"},
		"timing": map[string]any{
			"startedAt":  "2025-01-15T10:00:00.000Z",
			"endedAt":    "2025-01-15T10:00:01.000Z",
			"durationMs": 1000,
		},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": stdoutHash},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": stderrHash},
		},
		"parsing": map[string]any{"status": "raw"},
	}

	resp := doCaptureRequest(t, HandlerWithDB(db), actorToken, "cccccccc-cccc-4ccc-8ccc-ccccccccccc1", envelope, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", resp.Code, http.StatusCreated)
	}

	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan evidence still present after recovery: %v", err)
	}
	for _, tc := range []struct {
		path string
		want []byte
	}{
		{path: filepath.Join(tempEvidenceDir, "captures", stdoutHash, "stdout"), want: stdout},
		{path: filepath.Join(tempEvidenceDir, "captures", stderrHash, "stderr"), want: stderr},
	} {
		got, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read stored evidence %s: %v", tc.path, err)
		}
		if !bytes.Equal(got, tc.want) {
			t.Fatalf("stored evidence %s = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestCaptureAcceptsAIInitiationWithDecisionContext(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	aiID := "33333333-3333-4333-8333-333333333333"
	humanToken := "human-ai-approver"
	aiToken := "ai-operator-token"
	stdout := []byte("abc")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	stdoutHash := hex.EncodeToString(stdoutSum[:])
	stderrHash := hex.EncodeToString(stderrSum[:])

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Synthetic Field Agent', 'synthetic', '2025.01', $4)`, aiID, engagementID, hashHex(aiToken), humanID)

	envelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1",
		"sourceAgent": map[string]any{
			"id":       "55555555-5555-4555-8555-555555555555",
			"kind":     "remote_agent",
			"name":     "waypoint-agent",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "macos", "arch": "arm64"},
		},
		"phase":           "recon",
		"initiatedBy":     "ai",
		"decisionContext": map[string]any{"rationale": "Confirm the scope.", "promptReference": "audit-1"},
		"command":         "/opt/homebrew/bin/nmap",
		"argv":            []string{"nmap", "-sV", "192.0.2.30"},
		"cwd":             "/Users/agent/engagement",
		"target":          map[string]any{"kind": "ip", "value": "192.0.2.30"},
		"timing":          map[string]any{"startedAt": "2025-01-15T10:10:00.000Z", "endedAt": "2025-01-15T10:10:01.000Z", "durationMs": 1000},
		"execution":       map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.14", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "auto", "status": "observed", "address": "198.51.100.25", "observedAt": "2025-01-15T10:09:55.000Z"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": stdoutHash},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": stderrHash},
		},
		"parsing": map[string]any{"status": "raw"},
	}
	resp := doCaptureRequest(t, HandlerWithDB(db), aiToken, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1", envelope, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("ai capture status = %d, want %d", resp.Code, http.StatusCreated)
	}
	var ack captureAckResponse
	decodeResponse(t, resp, &ack)
	if ack.Idempotency != "created" {
		t.Fatalf("ai ack = %#v", ack)
	}
}

func TestCapturePersistsStructuredResultsAndRollsBackInvalidOutput(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	humanToken := "human-test-token"
	stdout := []byte("abc")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	stdoutHash := hex.EncodeToString(stdoutSum[:])
	stderrHash := hex.EncodeToString(stderrSum[:])

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))

	valid := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent": map[string]any{
			"id":       "44444444-4444-4444-8444-444444444444",
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nmap",
		"argv":        []string{"nmap", "-sV", "192.0.2.10"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "ip", "value": "192.0.2.10"},
		"timing": map[string]any{
			"startedAt":  "2025-01-15T10:00:00.000Z",
			"endedAt":    "2025-01-15T10:00:01.000Z",
			"durationMs": 1000,
		},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": stdoutHash},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": stderrHash},
		},
		"parsing": map[string]any{
			"status": "parsed",
			"plugin": map[string]any{
				"id":              "waypoint.nmap",
				"version":         "1.0.0",
				"artifactSha256":  strings.Repeat("a", 64),
				"contractVersion": "1.0.0",
				"match": map[string]any{
					"binary":      "nmap",
					"reason":      "binary name and service-version arguments matched",
					"specificity": 20,
				},
			},
			"result": map[string]any{
				"schemaId":      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
				"schemaVersion": "1.0.0",
				"extracted":     map[string]any{"hostsUp": 1, "services": []any{}},
				"entities": []any{
					map[string]any{
						"kind": "host",
						"identifiers": []any{
							map[string]any{"type": "fqdn", "value": "demo.local"},
							map[string]any{"type": "ip", "value": "192.0.2.10"},
						},
						"attributes": map[string]any{"state": "up"},
					},
				},
			},
		},
	}

	resp := doCaptureRequest(t, HandlerWithDB(db), humanToken, "aaaa1111", valid, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", resp.Code, http.StatusCreated)
	}
	var ack captureAckResponse
	decodeResponse(t, resp, &ack)

	var resultID, pluginID, schemaID, schemaVersion string
	if err := db.QueryRowContext(ctx, `SELECT id, plugin_id, schema_id, schema_version FROM result WHERE action_id = $1`, ack.ActionID).Scan(&resultID, &pluginID, &schemaID, &schemaVersion); err != nil {
		t.Fatalf("load result: %v", err)
	}
	if pluginID != "waypoint.nmap" || schemaID != "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json" || schemaVersion != "1.0.0" {
		t.Fatalf("result row = %s %s %s", pluginID, schemaID, schemaVersion)
	}

	var obsCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation WHERE result_id = $1`, resultID).Scan(&obsCount); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if obsCount != 1 {
		t.Fatalf("observation count = %d, want 1", obsCount)
	}

	var keyType, keyValue, kind string
	if err := db.QueryRowContext(ctx, `SELECT e.key_type, e.key_value, e.kind FROM observation o JOIN entity e ON e.id = o.entity_id WHERE o.result_id = $1`, resultID).Scan(&keyType, &keyValue, &kind); err != nil {
		t.Fatalf("load entity link: %v", err)
	}
	if keyType != "fqdn" || keyValue != "demo.local" || kind != "host" {
		t.Fatalf("entity link = %s %s %s", keyType, keyValue, kind)
	}

	invalid := cloneMap(valid)
	invalid["captureId"] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
	parsing := cloneMap(valid["parsing"].(map[string]any))
	result := cloneMap(parsing["result"].(map[string]any))
	result["schemaId"] = "https://schemas.waypoint.security/plugins/nmap/1.0.0/other.schema.json"
	parsing["result"] = result
	invalid["parsing"] = parsing

	resp = doCaptureRequest(t, HandlerWithDB(db), humanToken, "aaaa2222", invalid, stdout, stderr)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid parse status = %d, want %d", resp.Code, http.StatusBadRequest)
	}

	for _, query := range []struct {
		name string
		sql  string
	}{
		{name: "action", sql: `SELECT COUNT(*) FROM action WHERE engagement_id = $1`},
		{name: "result", sql: `SELECT COUNT(*) FROM result WHERE engagement_id = $1`},
		{name: "observation", sql: `SELECT COUNT(*) FROM observation WHERE engagement_id = $1`},
		{name: "entity", sql: `SELECT COUNT(*) FROM entity WHERE engagement_id = $1`},
	} {
		var count int
		if err := db.QueryRowContext(ctx, query.sql, engagementID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", query.name, err)
		}
		if count != 1 {
			t.Fatalf("count %s = %d, want 1", query.name, count)
		}
	}

	rollbackEnvelope := cloneMap(valid)
	rollbackEnvelope["captureId"] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"
	rollbackEnvelope["parsing"] = map[string]any{"status": "raw"}
	resp = doCaptureRequest(t, HandlerWithDB(db), humanToken, "aaaa3333", rollbackEnvelope, stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("raw fallback status = %d, want %d", resp.Code, http.StatusCreated)
	}
}

func TestCaptureRejectsConflictingStableEntityKinds(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	humanToken := "human-test-token"
	stdout := []byte("abc")
	stderr := []byte{}
	stdoutSum := sha256.Sum256(stdout)
	stderrSum := sha256.Sum256(stderr)
	stdoutHash := hex.EncodeToString(stdoutSum[:])
	stderrHash := hex.EncodeToString(stderrSum[:])

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))

	makeEnvelope := func(captureID, kind string) map[string]any {
		return map[string]any{
			"contractVersion": "1.0.0",
			"captureId":       captureID,
			"sourceAgent": map[string]any{
				"id":       "44444444-4444-4444-8444-444444444444",
				"kind":     "operator_wrapper",
				"name":     "waypoint-wrapper",
				"version":  "1.0.0",
				"platform": map[string]any{"os": "linux", "arch": "amd64"},
			},
			"phase":       "recon",
			"initiatedBy": "manual",
			"command":     "/usr/bin/nmap",
			"argv":        []string{"nmap", "-sV", "demo.local"},
			"cwd":         "/home/operator/engagement",
			"target":      map[string]any{"kind": "hostname", "value": "demo.local"},
			"timing": map[string]any{
				"startedAt":  "2025-01-15T10:00:00.000Z",
				"endedAt":    "2025-01-15T10:00:01.000Z",
				"durationMs": 1000,
			},
			"execution": map[string]any{"status": "exited", "exitCode": 0},
			"network": map[string]any{
				"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
				"egress":     map[string]any{"mode": "off", "status": "disabled"},
				"pivotChain": []any{},
			},
			"evidence": map[string]any{
				"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": stdoutHash},
				"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": stderrHash},
			},
			"parsing": map[string]any{
				"status": "parsed",
				"plugin": map[string]any{
					"id":              "waypoint.nmap",
					"version":         "1.0.0",
					"artifactSha256":  strings.Repeat("a", 64),
					"contractVersion": "1.0.0",
					"match": map[string]any{
						"binary":      "nmap",
						"reason":      "binary name and service-version arguments matched",
						"specificity": 20,
					},
				},
				"result": map[string]any{
					"schemaId":      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
					"schemaVersion": "1.0.0",
					"extracted":     map[string]any{"hostsUp": 1, "services": []any{}},
					"entities": []any{
						map[string]any{
							"kind": kind,
							"identifiers": []any{
								map[string]any{"type": "fqdn", "value": "demo.local."},
							},
							"attributes": map[string]any{"state": "up"},
						},
					},
				},
			},
		}
	}

	resp := doCaptureRequest(t, HandlerWithDB(db), humanToken, "aaaa4444", makeEnvelope("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb4", "host"), stdout, stderr)
	if resp.Code != http.StatusCreated {
		t.Fatalf("seed status = %d, want %d", resp.Code, http.StatusCreated)
	}

	resp = doCaptureRequest(t, HandlerWithDB(db), humanToken, "aaaa5555", makeEnvelope("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb5", "service"), stdout, stderr)
	if resp.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d", resp.Code, http.StatusConflict)
	}
	var prob problemResponse
	decodeResponse(t, resp, &prob)
	if prob.Code != "entity_conflict" {
		t.Fatalf("conflict problem = %#v", prob)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity WHERE engagement_id = $1`, engagementID).Scan(&count); err != nil {
		t.Fatalf("count entities: %v", err)
	}
	if count != 1 {
		t.Fatalf("entity count = %d, want 1", count)
	}
}

func TestEntityIdentityNormalizationPrecedenceAndConcurrency(t *testing.T) {
	cases := []struct {
		name     string
		ids      []captureEntityIdentifier
		wantType string
		wantVal  string
	}{
		{
			name: "sid wins and trims",
			ids: []captureEntityIdentifier{
				{Type: "mac", Value: "aa-bb-cc-dd-ee-ff"},
				{Type: "ad_sid", Value: " S-1-5-21-42 "},
				{Type: "fqdn", Value: "demo.local."},
			},
			wantType: "ad_sid",
			wantVal:  "S-1-5-21-42",
		},
		{
			name:     "mac normalizes",
			ids:      []captureEntityIdentifier{{Type: "mac", Value: "AA-BB-CC-DD-EE-FF"}},
			wantType: "mac",
			wantVal:  "aa:bb:cc:dd:ee:ff",
		},
		{
			name:     "fqdn trims dot",
			ids:      []captureEntityIdentifier{{Type: "fqdn", Value: " Demo.Local. "}},
			wantType: "fqdn",
			wantVal:  "demo.local",
		},
		{
			name:     "hostname ip pair normalizes",
			ids:      []captureEntityIdentifier{{Type: "hostname", Value: " HostOne. "}, {Type: "ip", Value: " 2001:0db8:0:0:0:0:0:1 "}},
			wantType: "hostname_ip",
			wantVal:  "hostname=hostone|ip=2001:db8::1",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotVal, gotOK := entityIdentity(tc.ids)
			if !gotOK || gotType != tc.wantType || gotVal != tc.wantVal {
				t.Fatalf("entityIdentity() = %q %q %v, want %q %q true", gotType, gotVal, gotOK, tc.wantType, tc.wantVal)
			}

			const workers = 32
			var wg sync.WaitGroup
			errCh := make(chan string, workers)
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					concurrentType, concurrentVal, concurrentOK := entityIdentity(tc.ids)
					if concurrentType != tc.wantType || concurrentVal != tc.wantVal || !concurrentOK {
						errCh <- "concurrent normalization drifted"
					}
				}()
			}
			wg.Wait()
			close(errCh)
			for err := range errCh {
				if err != "" {
					t.Fatal(err)
				}
			}
		})
	}
}

func doCaptureRequest(t *testing.T, h http.Handler, token, requestID string, envelope map[string]any, stdout, stderr []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	enc, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := mw.WriteField("envelope", string(enc)); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := mw.WriteField("stdout", string(stdout)); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := mw.WriteField("stderr", string(stderr)); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/captures", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", envelope["captureId"].(string))
	req.Header.Set("X-Request-ID", requestID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func hashHex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WAYPOINT_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func resetPublicSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, stmt := range []string{`DROP SCHEMA IF EXISTS public CASCADE`, `CREATE SCHEMA public`, `GRANT ALL ON SCHEMA public TO public`} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("reset schema with %q: %v", stmt, err)
		}
	}
}

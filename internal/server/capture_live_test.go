package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestLiveMultiActorRESTCaptureJourneys(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	evidenceDir := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceDir)

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	aiID := "33333333-3333-4333-8333-333333333333"
	humanToken := "human-live-token"
	aiToken := "ai-live-token"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Synthetic Field Agent', 'synthetic', '2025.01', $4)`, aiID, engagementID, hashHex(aiToken), humanID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	baseSourceAgent := map[string]any{
		"id":      "44444444-4444-4444-8444-444444444444",
		"kind":    "operator_wrapper",
		"name":    "waypoint-wrapper",
		"version": "1.0.0",
		"platform": map[string]any{
			"os":   "linux",
			"arch": "amd64",
		},
	}
	baseNetwork := map[string]any{
		"execHost": map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
		"egress":   map[string]any{"mode": "off", "status": "disabled"},
		"pivotChain": []any{
			map[string]any{"type": "ssh_jump", "host": "pivot.internal", "port": 22, "label": "jumpbox"},
		},
	}
	endedAt := time.Now().UTC().Add(-6 * time.Second)
	startedAt := endedAt.Add(-1 * time.Second)
	baseTiming := map[string]any{"startedAt": startedAt.Format(time.RFC3339Nano), "endedAt": endedAt.Format(time.RFC3339Nano), "durationMs": 1000}

	humanKnown := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent":     baseSourceAgent,
		"phase":           "recon",
		"initiatedBy":     "manual",
		"command":         "/usr/bin/nmap",
		"argv":            []string{"nmap", "-sV", "192.0.2.10"},
		"cwd":             "/home/operator/engagement",
		"target":          map[string]any{"kind": "ip", "value": "192.0.2.10"},
		"timing":          baseTiming,
		"execution":       map[string]any{"status": "exited", "exitCode": 0},
		"network":         baseNetwork,
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("nmap ok\n"), "sha256": hashHex("nmap ok\n")},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": 0, "sha256": hashHex("")},
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

	humanResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanToken, "req-human-known", humanKnown, []byte("nmap ok\n"), nil)
	if humanResp.status != http.StatusCreated {
		t.Fatalf("human capture status = %d, want %d: %s", humanResp.status, http.StatusCreated, humanResp.body)
	}
	var humanAck captureAckResponse
	decodeBody(t, humanResp.body, &humanAck)
	if humanAck.Idempotency != "created" || humanAck.ActionID == "" {
		t.Fatalf("human ack = %#v", humanAck)
	}
	if humanAck.ClockSkew == nil || humanAck.ClockSkew.Status != "outside_tolerance" || humanAck.ClockSkew.OffsetMs == 0 {
		t.Fatalf("human clock skew = %#v", humanAck.ClockSkew)
	}
	assertActionSnapshot(t, ctx, db, humanAck.ActionID, humanID, "human", "manual", "parsed", "waypoint.nmap", false)
	assertEvidenceLinked(t, ctx, db, evidenceDir, humanAck.ActionID, "nmap ok\n", "")
	assertSingleResultAndObservation(t, ctx, db, humanAck.ActionID, 1, 1)
	var execHostIP, egressIP, pivotChain string
	var actionStartedAt, actionEndedAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT exec_host_ip::text, COALESCE(egress_public_ip::text, ''), pivot_chain::text, started_at, ended_at FROM action WHERE id = $1`, humanAck.ActionID).Scan(&execHostIP, &egressIP, &pivotChain, &actionStartedAt, &actionEndedAt); err != nil {
		t.Fatalf("load human action metadata: %v", err)
	}
	if execHostIP != "10.10.0.12" || egressIP != "" || !strings.Contains(pivotChain, `"ssh_jump"`) || !actionEndedAt.After(actionStartedAt) {
		t.Fatalf("human action metadata unexpected: execHost=%q egress=%q pivot=%q started=%s ended=%s", execHostIP, egressIP, pivotChain, actionStartedAt, actionEndedAt)
	}

	aiUnknown := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1",
		"sourceAgent": map[string]any{
			"id":       "55555555-5555-4555-8555-555555555555",
			"kind":     "remote_agent",
			"name":     "waypoint-agent",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "macos", "arch": "arm64"},
		},
		"phase":       "attacks",
		"initiatedBy": "ai",
		"decisionContext": map[string]any{
			"rationale":       "Need to validate a suspicious endpoint.",
			"promptReference": "live-rest-01",
		},
		"command":   "/opt/tools/oddscan",
		"argv":      []string{"oddscan", "--probe", "192.0.2.30"},
		"cwd":       "/Users/agent/engagement",
		"target":    map[string]any{"kind": "hostname", "value": "demo.local"},
		"timing":    map[string]any{"startedAt": "2025-01-15T10:10:00.000Z", "endedAt": "2025-01-15T10:10:01.000Z", "durationMs": 1000},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network":   baseNetwork,
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("oddscan output\n"), "sha256": hashHex("oddscan output\n")},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": 0, "sha256": hashHex("")},
		},
		"parsing": map[string]any{"status": "needs-plugin"},
	}

	aiResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", aiToken, "req-ai-unknown", aiUnknown, []byte("oddscan output\n"), nil)
	if aiResp.status != http.StatusCreated {
		t.Fatalf("ai capture status = %d, want %d: %s", aiResp.status, http.StatusCreated, aiResp.body)
	}
	var aiAck captureAckResponse
	decodeBody(t, aiResp.body, &aiAck)
	if aiAck.Idempotency != "created" || aiAck.ActionID == "" {
		t.Fatalf("ai ack = %#v", aiAck)
	}
	assertActionSnapshot(t, ctx, db, aiAck.ActionID, aiID, "ai_agent", "ai", "needs-plugin", "", true)
	assertEvidenceLinked(t, ctx, db, evidenceDir, aiAck.ActionID, "oddscan output\n", "")
	assertSingleResultAndObservation(t, ctx, db, aiAck.ActionID, 0, 0)

	rawEnvelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "cccccccc-cccc-4ccc-8ccc-ccccccccccc1",
		"sourceAgent":     baseSourceAgent,
		"phase":           "recon",
		"initiatedBy":     "manual",
		"command":         "/usr/bin/whoami",
		"argv":            []string{"whoami"},
		"cwd":             "/home/operator/engagement",
		"target":          map[string]any{"kind": "host", "value": "jumpbox"},
		"timing":          baseTiming,
		"execution":       map[string]any{"status": "exited", "exitCode": 0},
		"network":         baseNetwork,
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("operator\n"), "sha256": hashHex("operator\n")},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("warning\n"), "sha256": hashHex("warning\n")},
		},
		"parsing": map[string]any{"status": "raw"},
	}
	rawResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanToken, "req-human-raw", rawEnvelope, []byte("operator\n"), []byte("warning\n"))
	if rawResp.status != http.StatusCreated {
		t.Fatalf("raw capture status = %d, want %d: %s", rawResp.status, http.StatusCreated, rawResp.body)
	}
	var rawAck captureAckResponse
	decodeBody(t, rawResp.body, &rawAck)
	assertActionSnapshot(t, ctx, db, rawAck.ActionID, humanID, "human", "manual", "raw", "", false)
	assertEvidenceLinked(t, ctx, db, evidenceDir, rawAck.ActionID, "operator\n", "warning\n")
	assertEvidenceFiles(t, evidenceDir, "operator\n", "warning\n")

	replay := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanToken, "req-human-known", humanKnown, []byte("nmap ok\n"), nil)
	if replay.status != http.StatusOK {
		t.Fatalf("replay status = %d, want %d: %s", replay.status, http.StatusOK, replay.body)
	}
	var replayAck captureAckResponse
	decodeBody(t, replay.body, &replayAck)
	if replayAck.Idempotency != "replayed" || replayAck.ActionID != humanAck.ActionID || replayAck.AuditEventCursor != humanAck.AuditEventCursor {
		t.Fatalf("replay ack = %#v", replayAck)
	}

	mutated := cloneMap(humanKnown)
	mutated["command"] = "/usr/bin/changed"
	conflict := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanToken, "req-human-known", mutated, []byte("nmap ok\n"), nil)
	if conflict.status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d: %s", conflict.status, http.StatusConflict, conflict.body)
	}
	var prob problemResponse
	decodeBody(t, conflict.body, &prob)
	if prob.Code != "idempotency_conflict" || prob.ExistingID != humanAck.ActionID {
		t.Fatalf("conflict problem = %#v", prob)
	}
}

func doLiveCaptureRequest(t *testing.T, client *http.Client, url, token, requestID string, envelope map[string]any, stdout, stderr []byte) struct {
	status int
	body   []byte
} {
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

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body.Bytes()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", envelope["captureId"].(string))
	req.Header.Set("X-Request-ID", requestID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post capture: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return struct {
		status int
		body   []byte
	}{status: resp.StatusCode, body: data}
}

func decodeBody(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
}

func assertActionSnapshot(t *testing.T, ctx context.Context, db *sql.DB, actionID, wantActorID, wantActorKind, wantInitiatedBy, wantParseStatus, wantPluginID string, wantDecisionContext bool) {
	t.Helper()
	var actorID, actorKind, initiatedBy, parseStatus, pluginID sql.NullString
	var decisionContext sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT actor_id::text, a.kind, initiated_by, parse_status, COALESCE(plugin_id, ''), COALESCE(decision_context::text, '') FROM action act JOIN actor a ON a.id = act.actor_id WHERE act.id = $1`, actionID).Scan(&actorID, &actorKind, &initiatedBy, &parseStatus, &pluginID, &decisionContext); err != nil {
		t.Fatalf("load action snapshot: %v", err)
	}
	if actorID.String != wantActorID || actorKind.String != wantActorKind || initiatedBy.String != wantInitiatedBy || parseStatus.String != wantParseStatus {
		t.Fatalf("action snapshot = actor=%s kind=%s initiatedBy=%s parse=%s", actorID.String, actorKind.String, initiatedBy.String, parseStatus.String)
	}
	if wantPluginID == "" {
		if pluginID.String != "" {
			t.Fatalf("plugin id = %q, want empty", pluginID.String)
		}
	} else if pluginID.String != wantPluginID {
		t.Fatalf("plugin id = %q, want %q", pluginID.String, wantPluginID)
	}
	if wantDecisionContext != (decisionContext.String != "") {
		t.Fatalf("decisionContext present = %t, want %t", decisionContext.String != "", wantDecisionContext)
	}
}

func assertSingleResultAndObservation(t *testing.T, ctx context.Context, db *sql.DB, actionID string, wantResults, wantObservations int) {
	t.Helper()
	var resultCount, obsCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM result WHERE action_id = $1`, actionID).Scan(&resultCount); err != nil {
		t.Fatalf("count results: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation WHERE action_id = $1`, actionID).Scan(&obsCount); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if resultCount != wantResults || obsCount != wantObservations {
		t.Fatalf("result/observation counts = %d/%d, want %d/%d", resultCount, obsCount, wantResults, wantObservations)
	}
}

func assertEvidenceLinked(t *testing.T, ctx context.Context, db *sql.DB, evidenceDir, actionID, wantStdout, wantStderr string) {
	t.Helper()
	var stdoutKey, stderrKey string
	if err := db.QueryRowContext(ctx, `SELECT se.storage_key, ee.storage_key FROM action a JOIN evidence se ON se.id = a.stdout_evidence_id JOIN evidence ee ON ee.id = a.stderr_evidence_id WHERE a.id = $1`, actionID).Scan(&stdoutKey, &stderrKey); err != nil {
		t.Fatalf("load evidence links: %v", err)
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: stdoutKey, want: wantStdout},
		{path: stderrKey, want: wantStderr},
	} {
		data, err := os.ReadFile(filepath.Join(evidenceDir, filepath.FromSlash(tc.path)))
		if err != nil {
			t.Fatalf("read evidence %s: %v", tc.path, err)
		}
		if string(data) != tc.want {
			t.Fatalf("evidence %s = %q, want %q", tc.path, data, tc.want)
		}
	}
}

func assertEvidenceFiles(t *testing.T, evidenceDir, wantStdout, wantStderr string) {
	t.Helper()
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "stdout", want: wantStdout},
		{name: "stderr", want: wantStderr},
	} {
		sha := hashHex(tc.want)
		path := filepath.Join(evidenceDir, "captures", sha, tc.name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read evidence file %s: %v", path, err)
		}
		if string(data) != tc.want {
			t.Fatalf("evidence file %s = %q, want %q", path, data, tc.want)
		}
	}
}

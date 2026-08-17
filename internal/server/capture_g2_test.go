package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

type gateFrameExpectation struct {
	actionID      string
	captureID     string
	actorID       string
	actorKind     string
	handle        string
	authorizedBy  string
	originKind    string
	parseStatus   string
	egressStatus  string
	initiatedBy   string
	sourceAgentID string
	command       string
}

type gateActionExpectation struct {
	sourceAgentID   string
	execHostIP      string
	egressIP        string
	pivotType       string
	wantDecisionCtx bool
}

func TestCaptureRoundTripGateG2Transcript(t *testing.T) {
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
	humanAID := "22222222-2222-4222-8222-222222222222"
	humanBID := "33333333-3333-4333-8333-333333333333"
	aiID := "44444444-4444-4444-8444-444444444444"
	humanAToken := "gate-human-a-token"
	humanBToken := "gate-human-b-token"
	aiToken := "gate-ai-token"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanAID, engagementID, hashHex(humanAToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'beatrice.operator', $3, 'operator')`, humanBID, engagementID, hashHex(humanBToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Synthetic Field Agent', 'synthetic', '2025.01', $4)`, aiID, engagementID, hashHex(aiToken), humanAID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events", humanAToken, "req-g2-sse", "", "")
	defer sseResp.Body.Close()

	known := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent": map[string]any{
			"id":       "55555555-5555-4555-8555-555555555555",
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
		"timing":      map[string]any{"startedAt": "2025-01-15T10:00:00.000Z", "endedAt": "2025-01-15T10:00:01.250Z", "durationMs": 1250},
		"execution":   map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed", "interface": "eth0"},
			"egress":     map[string]any{"mode": "manual", "status": "declared", "address": "198.51.100.24", "observedAt": "2025-01-15T09:59:55.000Z"},
			"pivotChain": []any{map[string]any{"type": "ssh_jump", "host": "pivot.manual.local", "port": 22, "label": "jumpbox"}},
		},
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
				"extracted": map[string]any{
					"hostsUp":  1,
					"services": []any{map[string]any{"port": 443, "transport": "tcp", "name": "https"}},
				},
				"entities": []any{map[string]any{
					"kind":        "host",
					"identifiers": []any{map[string]any{"type": "ip", "value": "192.0.2.10"}},
					"attributes":  map[string]any{"state": "up"},
				}},
			},
		},
	}

	knownResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanAToken, "req-g2-known", known, []byte("nmap ok\n"), nil)
	if knownResp.status != http.StatusCreated {
		t.Fatalf("known capture status = %d, body=%s", knownResp.status, knownResp.body)
	}
	var knownAck captureAckResponse
	decodeBody(t, knownResp.body, &knownAck)
	if knownAck.Idempotency != "created" || knownAck.ActionID == "" {
		t.Fatalf("known ack = %#v", knownAck)
	}
	knownFrame := readSSEFrame(t, sseResp.Body)
	assertGateSSEFrame(t, knownFrame, gateFrameExpectation{
		actionID:      knownAck.ActionID,
		captureID:     known["captureId"].(string),
		actorID:       humanAID,
		actorKind:     "human",
		handle:        "alex.operator",
		originKind:    "rest",
		parseStatus:   "parsed",
		egressStatus:  "declared",
		initiatedBy:   "manual",
		sourceAgentID: "55555555-5555-4555-8555-555555555555",
		command:       "/usr/bin/nmap",
	})
	assertActionSnapshot(t, ctx, db, knownAck.ActionID, humanAID, "human", "manual", "parsed", "waypoint.nmap", false)
	assertActionNetworkFields(t, ctx, db, knownAck.ActionID, gateActionExpectation{
		sourceAgentID:   "55555555-5555-4555-8555-555555555555",
		execHostIP:      "10.10.0.12",
		egressIP:        "198.51.100.24",
		pivotType:       "ssh_jump",
		wantDecisionCtx: false,
	})
	assertSingleResultAndObservation(t, ctx, db, knownAck.ActionID, 1, 1)
	assertEvidenceLinked(t, ctx, db, evidenceDir, knownAck.ActionID, "nmap ok\n", "")

	unknown := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
		"sourceAgent": map[string]any{
			"id":       "66666666-6666-4666-8666-666666666666",
			"kind":     "mcp_client",
			"name":     "waypoint-mcp-client",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "arm64"},
		},
		"phase":       "attacks",
		"initiatedBy": "manual",
		"command":     "/opt/tools/mystery-scan.exe",
		"argv":        []string{"mystery-scan.exe", "--target", "192.0.2.20"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "ip", "value": "192.0.2.20"},
		"timing":      map[string]any{"startedAt": "2025-01-15T10:05:00.000Z", "endedAt": "2025-01-15T10:05:00.500Z", "durationMs": 500},
		"execution":   map[string]any{"status": "exited", "exitCode": 1},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.13", "method": "interface_selection", "confidence": "inferred"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{map[string]any{"type": "socks_proxy", "host": "127.0.0.1", "port": 1080, "label": "assessment pivot"}},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "application/octet-stream", "byteLength": len("mystery\n"), "sha256": hashHex("mystery\n")},
			"stderr": map[string]any{"mediaType": "application/octet-stream", "byteLength": len("warning\n"), "sha256": hashHex("warning\n")},
		},
		"parsing": map[string]any{"status": "needs-plugin"},
	}

	unknownResp := doLiveMCPRequest(t, ts.Client(), ts.URL+"/mcp", humanBToken, "req-g2-unknown", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "waypoint_ingest_capture",
			"arguments": map[string]any{
				"idempotencyKey": unknown["captureId"],
				"envelope":       unknown,
				"stdoutBase64":   "bXlzdGVyeQo=",
				"stderrBase64":   "d2FybmluZwo=",
			},
		},
	})
	if unknownResp.status != http.StatusOK {
		t.Fatalf("unknown capture status = %d, body=%s", unknownResp.status, unknownResp.body)
	}
	var unknownAck struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Ack struct {
					ActionID    string `json:"actionId"`
					Idempotency string `json:"idempotency"`
				} `json:"ack"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeBody(t, unknownResp.body, &unknownAck)
	if unknownAck.Result.IsError || unknownAck.Result.StructuredContent.Ack.Idempotency != "created" || unknownAck.Result.StructuredContent.Ack.ActionID == "" {
		t.Fatalf("unknown ack = %#v", unknownAck.Result)
	}
	unknownFrame := readSSEFrame(t, sseResp.Body)
	assertGateSSEFrame(t, unknownFrame, gateFrameExpectation{
		actionID:      unknownAck.Result.StructuredContent.Ack.ActionID,
		captureID:     unknown["captureId"].(string),
		actorID:       humanBID,
		actorKind:     "human",
		handle:        "beatrice.operator",
		originKind:    "mcp",
		parseStatus:   "needs-plugin",
		egressStatus:  "disabled",
		initiatedBy:   "manual",
		sourceAgentID: "66666666-6666-4666-8666-666666666666",
		command:       "/opt/tools/mystery-scan.exe",
	})
	assertActionSnapshot(t, ctx, db, unknownAck.Result.StructuredContent.Ack.ActionID, humanBID, "human", "manual", "needs-plugin", "", false)
	assertActionNetworkFields(t, ctx, db, unknownAck.Result.StructuredContent.Ack.ActionID, gateActionExpectation{
		sourceAgentID:   "66666666-6666-4666-8666-666666666666",
		execHostIP:      "10.10.0.13",
		egressIP:        "",
		pivotType:       "socks_proxy",
		wantDecisionCtx: false,
	})
	assertSingleResultAndObservation(t, ctx, db, unknownAck.Result.StructuredContent.Ack.ActionID, 0, 0)
	assertEvidenceLinked(t, ctx, db, evidenceDir, unknownAck.Result.StructuredContent.Ack.ActionID, "mystery\n", "warning\n")

	parserFailed := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3",
		"sourceAgent": map[string]any{
			"id":       "77777777-7777-4777-8777-777777777777",
			"kind":     "remote_agent",
			"name":     "waypoint-agent",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "macos", "arch": "arm64"},
		},
		"phase":           "recon",
		"initiatedBy":     "ai",
		"decisionContext": map[string]any{"rationale": "Validate the pinned parser crash path.", "promptReference": "gate-g2-parser-failed"},
		"command":         "/opt/homebrew/bin/nmap",
		"argv":            []string{"nmap", "-sV", "192.0.2.30"},
		"cwd":             "/Users/agent/engagement",
		"target":          map[string]any{"kind": "ip", "value": "192.0.2.30"},
		"timing":          map[string]any{"startedAt": "2025-01-15T10:10:00.000Z", "endedAt": "2025-01-15T10:10:01.000Z", "durationMs": 1000},
		"execution":       map[string]any{"status": "exited", "exitCode": 2},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.14", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "manual", "status": "declared", "address": "198.51.100.25", "observedAt": "2025-01-15T10:09:55.000Z"},
			"pivotChain": []any{map[string]any{"type": "ssh_jump", "host": "pivot.manual.ai", "port": 22, "label": "ai jump"}},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("parser crash\n"), "sha256": hashHex("parser crash\n")},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len("traceback\n"), "sha256": hashHex("traceback\n")},
		},
		"parsing": map[string]any{
			"status": "parse-failed",
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
			"failure": map[string]any{"code": "crash", "message": "Pinned parser process exited before returning a result."},
		},
	}

	failedResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", aiToken, "req-g2-failed", parserFailed, []byte("parser crash\n"), []byte("traceback\n"))
	if failedResp.status != http.StatusCreated {
		t.Fatalf("parse-failed capture status = %d, body=%s", failedResp.status, failedResp.body)
	}
	var failedAck captureAckResponse
	decodeBody(t, failedResp.body, &failedAck)
	if failedAck.Idempotency != "created" || failedAck.ActionID == "" {
		t.Fatalf("parse-failed ack = %#v", failedAck)
	}
	failedFrame := readSSEFrame(t, sseResp.Body)
	assertGateSSEFrame(t, failedFrame, gateFrameExpectation{
		actionID:      failedAck.ActionID,
		captureID:     parserFailed["captureId"].(string),
		actorID:       aiID,
		actorKind:     "ai_agent",
		handle:        "field-agent-7",
		authorizedBy:  humanAID,
		originKind:    "rest",
		parseStatus:   "parse-failed",
		egressStatus:  "declared",
		initiatedBy:   "ai",
		sourceAgentID: "77777777-7777-4777-8777-777777777777",
		command:       "/opt/homebrew/bin/nmap",
	})
	assertActionSnapshot(t, ctx, db, failedAck.ActionID, aiID, "ai_agent", "ai", "parse-failed", "waypoint.nmap", true)
	assertActionNetworkFields(t, ctx, db, failedAck.ActionID, gateActionExpectation{
		sourceAgentID:   "77777777-7777-4777-8777-777777777777",
		execHostIP:      "10.10.0.14",
		egressIP:        "198.51.100.25",
		pivotType:       "ssh_jump",
		wantDecisionCtx: true,
	})
	assertSingleResultAndObservation(t, ctx, db, failedAck.ActionID, 0, 0)
	assertEvidenceLinked(t, ctx, db, evidenceDir, failedAck.ActionID, "parser crash\n", "traceback\n")
	_ = sseResp.Body.Close()

	replay := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanAToken, "req-g2-known-replay", known, []byte("nmap ok\n"), nil)
	if replay.status != http.StatusOK {
		t.Fatalf("replay status = %d, body=%s", replay.status, replay.body)
	}
	var replayAck captureAckResponse
	decodeBody(t, replay.body, &replayAck)
	if replayAck.Idempotency != "replayed" || replayAck.ActionID != knownAck.ActionID || replayAck.AuditEventCursor != knownAck.AuditEventCursor {
		t.Fatalf("replay ack = %#v", replayAck)
	}

	mutated := cloneMap(known)
	mutated["command"] = "/usr/bin/changed"
	conflict := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", humanAToken, "req-g2-known-conflict", mutated, []byte("nmap ok\n"), nil)
	if conflict.status != http.StatusConflict {
		t.Fatalf("conflict status = %d, body=%s", conflict.status, conflict.body)
	}

	var acceptedCount, conflictCount, actionCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = 'capture.accepted'`, engagementID).Scan(&acceptedCount); err != nil {
		t.Fatalf("count accepted events: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = 'capture.conflict'`, engagementID).Scan(&conflictCount); err != nil {
		t.Fatalf("count conflict events: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action WHERE engagement_id = $1`, engagementID).Scan(&actionCount); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	if acceptedCount != 3 || conflictCount != 1 || actionCount != 3 {
		t.Fatalf("counts = accepted:%d conflict:%d action:%d", acceptedCount, conflictCount, actionCount)
	}

	for _, path := range []string{
		filepath.Join(evidenceDir, "captures", hashHex("nmap ok\n"), "stdout"),
		filepath.Join(evidenceDir, "captures", hashHex("mystery\n"), "stdout"),
		filepath.Join(evidenceDir, "captures", hashHex("parser crash\n"), "stdout"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("evidence file missing %s: %v", path, err)
		}
	}
}

func assertGateSSEFrame(t *testing.T, frame map[string]string, want gateFrameExpectation) {
	t.Helper()
	if frame["event"] != "capture.accepted" {
		t.Fatalf("sse event = %q, want capture.accepted", frame["event"])
	}
	if frame["id"] == "" {
		t.Fatal("sse frame missing id")
	}
	var event struct {
		Actor struct {
			ID           string `json:"id"`
			Kind         string `json:"kind"`
			Handle       string `json:"handle"`
			Role         string `json:"role"`
			AuthorizedBy string `json:"authorizedBy,omitempty"`
		} `json:"actor"`
		Origin struct {
			Kind string `json:"kind"`
		} `json:"origin"`
		Data struct {
			ActionID      string `json:"actionId"`
			CaptureID     string `json:"captureId"`
			SourceAgentID string `json:"sourceAgentId"`
			Phase         string `json:"phase"`
			InitiatedBy   string `json:"initiatedBy"`
			Command       string `json:"command"`
			ParseStatus   string `json:"parseStatus"`
			EgressStatus  string `json:"egressStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(frame["data"]), &event); err != nil {
		t.Fatalf("decode sse data: %v", err)
	}
	if event.Actor.ID != want.actorID || event.Actor.Kind != want.actorKind || event.Actor.Handle != want.handle || event.Actor.AuthorizedBy != want.authorizedBy || event.Origin.Kind != want.originKind {
		t.Fatalf("sse actor/origin = %#v %#v", event.Actor, event.Origin)
	}
	if event.Data.ActionID != want.actionID || event.Data.CaptureID != want.captureID || event.Data.InitiatedBy != want.initiatedBy || event.Data.ParseStatus != want.parseStatus || event.Data.EgressStatus != want.egressStatus || event.Data.SourceAgentID != want.sourceAgentID || event.Data.Command != want.command {
		t.Fatalf("sse data = %#v", event.Data)
	}
}

func assertActionNetworkFields(t *testing.T, ctx context.Context, db *sql.DB, actionID string, want gateActionExpectation) {
	t.Helper()
	var sourceAgentID, execHostIP, egressIP, pivotChain, decisionContext string
	if err := db.QueryRowContext(ctx, `SELECT source_agent_id::text, exec_host_ip::text, COALESCE(egress_public_ip::text, ''), pivot_chain::text, COALESCE(decision_context::text, '') FROM action WHERE id = $1`, actionID).Scan(&sourceAgentID, &execHostIP, &egressIP, &pivotChain, &decisionContext); err != nil {
		t.Fatalf("load action network fields: %v", err)
	}
	if sourceAgentID != want.sourceAgentID || execHostIP != want.execHostIP || egressIP != want.egressIP {
		t.Fatalf("action network fields = source_agent_id=%q exec_host_ip=%q egress_public_ip=%q", sourceAgentID, execHostIP, egressIP)
	}
	if !strings.Contains(pivotChain, want.pivotType) {
		t.Fatalf("pivot chain = %s, want %s", pivotChain, want.pivotType)
	}
	if want.wantDecisionCtx != (decisionContext != "") {
		t.Fatalf("decision_context present = %t, want %t", decisionContext != "", want.wantDecisionCtx)
	}
}

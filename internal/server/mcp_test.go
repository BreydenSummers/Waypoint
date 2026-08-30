package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestMCPStandardFlowReusesCaptureService(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Setenv("WAYPOINT_EVIDENCE_DIR", t.TempDir())

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	token := "mcp-capture-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(token))

	fixture := loadMCPFixture(t)
	requestID := "req-mcp"

	initResp := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "synthetic-agent", "version": "1.0.0"},
		},
	})
	if initResp.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, body=%s", initResp.Code, initResp.Body.String())
	}
	var initBody struct {
		Result struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
		} `json:"result"`
	}
	decodeResponse(t, initResp, &initBody)
	if initBody.Result.ProtocolVersion != "2025-03-26" {
		t.Fatalf("initialize protocolVersion = %q", initBody.Result.ProtocolVersion)
	}
	if _, ok := initBody.Result.Capabilities["tools"]; !ok {
		t.Fatalf("initialize capabilities = %#v", initBody.Result.Capabilities)
	}

	notification := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	if notification.Code != http.StatusAccepted {
		t.Fatalf("initialized notification status = %d, body=%s", notification.Code, notification.Body.String())
	}
	if notification.Body.Len() != 0 {
		t.Fatalf("initialized notification body = %q, want empty", notification.Body.String())
	}

	toolsResp := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if toolsResp.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body=%s", toolsResp.Code, toolsResp.Body.String())
	}
	var toolsBody struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	decodeResponse(t, toolsResp, &toolsBody)
	if len(toolsBody.Result.Tools) != 2 || toolsBody.Result.Tools[0].Name != "waypoint_ingest_capture" || toolsBody.Result.Tools[1].Name != "waypoint_capture_status" {
		t.Fatalf("tools/list = %#v", toolsBody.Result.Tools)
	}

	ingestArgs := fixture.IngestArguments
	ingestResp := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "waypoint_ingest_capture",
			"arguments": ingestArgs,
		},
	})
	if ingestResp.Code != http.StatusOK {
		t.Fatalf("tools/call ingest status = %d, body=%s", ingestResp.Code, ingestResp.Body.String())
	}
	var ingestBody struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Ack struct {
					ActionID         string `json:"actionId"`
					CaptureID        string `json:"captureId"`
					Idempotency      string `json:"idempotency"`
					AuditEventCursor string `json:"auditEventCursor"`
				} `json:"ack"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeResponse(t, ingestResp, &ingestBody)
	if ingestBody.Result.IsError || ingestBody.Result.StructuredContent.Ack.ActionID == "" {
		t.Fatalf("ingest result = %#v", ingestBody.Result)
	}
	if ingestBody.Result.StructuredContent.Ack.CaptureID != ingestArgs["envelope"].(map[string]any)["captureId"] {
		t.Fatalf("ingest captureId = %q, want %v", ingestBody.Result.StructuredContent.Ack.CaptureID, ingestArgs["envelope"].(map[string]any)["captureId"])
	}
	if ingestBody.Result.StructuredContent.Ack.Idempotency != "created" {
		t.Fatalf("ingest idempotency = %q", ingestBody.Result.StructuredContent.Ack.Idempotency)
	}
	if len(ingestBody.Result.Content) == 0 || ingestBody.Result.Content[0].Text != "Capture durably accepted." {
		t.Fatalf("ingest content = %#v", ingestBody.Result.Content)
	}

	var originKind string
	if err := db.QueryRowContext(ctx, `SELECT origin_kind FROM audit_event WHERE subject_id = $1 AND type = 'capture.accepted' ORDER BY id DESC LIMIT 1`, ingestBody.Result.StructuredContent.Ack.ActionID).Scan(&originKind); err != nil {
		t.Fatalf("load capture origin: %v", err)
	}
	if originKind != "mcp" {
		t.Fatalf("capture origin_kind = %q, want mcp", originKind)
	}

	replayResp := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "waypoint_ingest_capture",
			"arguments": ingestArgs,
		},
	})
	if replayResp.Code != http.StatusOK {
		t.Fatalf("tools/call replay status = %d, body=%s", replayResp.Code, replayResp.Body.String())
	}
	var replayBody struct {
		Result struct {
			StructuredContent struct {
				Ack struct {
					Idempotency string `json:"idempotency"`
				} `json:"ack"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeResponse(t, replayResp, &replayBody)
	if replayBody.Result.StructuredContent.Ack.Idempotency != "replayed" {
		t.Fatalf("ingest replay idempotency = %q", replayBody.Result.StructuredContent.Ack.Idempotency)
	}

	statusResp := doMCPRequest(t, HandlerWithDB(db), token, requestID, "1", map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "waypoint_capture_status",
			"arguments": map[string]any{
				"captureId":     ingestArgs["envelope"].(map[string]any)["captureId"],
				"sourceAgentId": ingestArgs["envelope"].(map[string]any)["sourceAgent"].(map[string]any)["id"],
			},
		},
	})
	if statusResp.Code != http.StatusOK {
		t.Fatalf("tools/call status = %d, body=%s", statusResp.Code, statusResp.Body.String())
	}
	var statusBody struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Status   string `json:"status"`
				ActionID string `json:"actionId"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeResponse(t, statusResp, &statusBody)
	if statusBody.Result.IsError || statusBody.Result.StructuredContent.Status != "captured" || statusBody.Result.StructuredContent.ActionID == "" {
		t.Fatalf("status result = %#v", statusBody.Result)
	}

	aliasResp := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(aliasResp, httptest.NewRequest(http.MethodPost, "/mcp/capture", nil))
	if aliasResp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("/mcp/capture status = %d, want 405", aliasResp.Code)
	}
}

func loadMCPFixture(t *testing.T) struct {
	IngestArguments map[string]any `json:"ingestArguments"`
} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "v1", "fixtures", "mcp.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var out struct {
		IngestArguments map[string]any `json:"ingestArguments"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return out
}

func doMCPRequest(t *testing.T, h http.Handler, token, requestID, sessionID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	if sessionID != "" {
		req.Header.Set("MCP-Session-Id", sessionID)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

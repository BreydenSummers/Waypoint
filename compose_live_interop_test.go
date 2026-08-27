package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type captureTranscriptFixture struct {
	Actor struct {
		ID           string `json:"id"`
		Kind         string `json:"kind"`
		Handle       string `json:"handle"`
		Role         string `json:"role"`
		AuthorizedBy string `json:"authorizedBy,omitempty"`
	} `json:"actor"`
	Request  map[string]any `json:"request"`
	RawParts struct {
		StdoutBase64 string `json:"stdoutBase64"`
		StderrBase64 string `json:"stderrBase64"`
	} `json:"rawParts"`
}

type mcpTranscriptFixture struct {
	IngestArguments map[string]any `json:"ingestArguments"`
}

type captureAckResponse struct {
	ActionID         string `json:"actionId"`
	AuditEventCursor string `json:"auditEventCursor"`
	CaptureID        string `json:"captureId"`
	Idempotency      string `json:"idempotency"`
}

type liveResponse struct {
	Status  int
	Body    []byte
	Headers http.Header
}

func TestComposeLiveCollectorInteropTranscripts(t *testing.T) {
	if _, err := execLookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := execDockerInfo(); err != nil {
		t.Skipf("docker daemon unavailable: %v\n%s", err, out)
	}

	project := fmt.Sprintf("waypoint-live-%d-%d", os.Getpid(), time.Now().UnixNano())
	overridePath := filepath.Join(t.TempDir(), "compose.override.yml")
	override := []byte("services:\n  waypoint:\n    ports:\n      - target: 8080\n        published: 0\n        protocol: tcp\n")
	if err := os.WriteFile(overridePath, override, 0o600); err != nil {
		t.Fatalf("write compose override: %v", err)
	}

	prepareComposeDockerConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), composeTestTimeout)
	defer cancel()

	runCompose := func(args ...string) string {
		t.Helper()
		out, err := composeOutput(ctx, project, overridePath, args...)
		if err != nil {
			t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), composeCleanupTimeout)
		defer cleanupCancel()
		_, _ = composeOutput(cleanupCtx, project, overridePath, "down", "-v", "--remove-orphans")
	})

	buildOut := mustComposeBuildNoCache(t, ctx, project, overridePath)
	t.Logf("docker compose build --no-cache:\n%s", strings.TrimSpace(buildOut))
	upOut := runCompose("up", "-d", "--wait")
	t.Logf("docker compose up --wait:\n%s", strings.TrimSpace(upOut))
	port := waitForComposePort(t, ctx, project, overridePath)
	baseURL := "http://127.0.0.1:" + port

	waitForHTTP(t, baseURL+"/readyz", http.StatusOK)
	assertHTTPBodyContains(t, baseURL+"/engagements/demo", http.StatusOK, `id="root"`)

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanAID := "22222222-2222-4222-8222-222222222222"
	humanBID := "33333333-3333-4333-8333-333333333333"
	aiID := "44444444-4444-4444-8444-444444444444"
	humanAToken := "compose-live-human-a-token"
	humanBToken := "compose-live-human-b-token"
	aiToken := "compose-live-ai-token"

	runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-v", "ON_ERROR_STOP=1", "-c", fmt.Sprintf(`INSERT INTO engagement (id, name, client, scope, status) VALUES ('%s', 'Demo', 'Client', 'Scope', 'active'); INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ('%s', '%s', 'human', 'alex.operator', '%s', 'operator'); INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ('%s', '%s', 'human', 'beatrice.operator', '%s', 'operator'); INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ('%s', '%s', 'ai_agent', 'field-agent-7', '%s', 'operator', 'Synthetic Field Agent', 'synthetic-offline-model', '2025.01', '%s');`, engagementID, humanAID, engagementID, sha256Hex(humanAToken), humanBID, engagementID, sha256Hex(humanBToken), aiID, engagementID, sha256Hex(aiToken), humanAID))

	sseResp := doLiveAuditRequest(t, &http.Client{Timeout: 5 * time.Second}, baseURL+"/events", humanAToken, "req-compose-sse", "", "")
	defer sseResp.Body.Close()

	humanKnown := loadCaptureTranscriptFixture(t, "valid/human-known-tool.json")
	humanUnknown := loadCaptureTranscriptFixture(t, "valid/human-unknown-tool.json")
	humanParseFailed := loadCaptureTranscriptFixture(t, "valid/human-known-parser-failed.json")
	aiKnown := loadCaptureTranscriptFixture(t, "valid/ai-known-tool.json")

	knownResp := doLiveCaptureRequestWithID(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/api/v1/captures", humanAToken, "req-compose-known-rest", humanKnown.Request, fixtureBytes(t, humanKnown.RawParts.StdoutBase64), fixtureBytes(t, humanKnown.RawParts.StderrBase64))
	if knownResp.Status != http.StatusCreated {
		t.Fatalf("known REST status = %d, want %d: %s", knownResp.Status, http.StatusCreated, knownResp.Body)
	}
	var knownAck captureAckResponse
	decodeBody(t, knownResp.Body, &knownAck)
	if knownAck.Idempotency != "created" || knownAck.ActionID == "" || knownAck.AuditEventCursor == "" {
		t.Fatalf("known REST ack = %#v", knownAck)
	}
	assertCaptureRecordedViaCompose(t, runCompose, knownAck.ActionID, humanAID, "human", "alex.operator", "", "parsed", "rest")
	assertEvidenceBytes(t, runCompose, filepath.Join("/var/lib/waypoint/evidence", "captures", sha256Hex(string(fixtureBytes(t, humanKnown.RawParts.StdoutBase64))), "stdout"), fixtureBytes(t, humanKnown.RawParts.StdoutBase64))
	knownReplay := doLiveCaptureRequestWithID(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/api/v1/captures", humanAToken, "req-compose-known-rest-replay", humanKnown.Request, fixtureBytes(t, humanKnown.RawParts.StdoutBase64), fixtureBytes(t, humanKnown.RawParts.StderrBase64))
	if knownReplay.Status != http.StatusOK {
		t.Fatalf("known REST replay status = %d, want %d: %s", knownReplay.Status, http.StatusOK, knownReplay.Body)
	}
	var knownReplayAck captureAckResponse
	decodeBody(t, knownReplay.Body, &knownReplayAck)
	if knownReplayAck.Idempotency != "replayed" || knownReplayAck.ActionID != knownAck.ActionID || knownReplayAck.AuditEventCursor != knownAck.AuditEventCursor {
		t.Fatalf("known REST replay ack = %#v", knownReplayAck)
	}
	frame1 := readSSEFrame(t, sseResp.Body)
	assertLiveSSECaptureAccepted(t, frame1, knownAck.ActionID, humanKnown.Request["captureId"].(string), "rest", "parsed")

	unknownResp := doLiveCaptureRequestWithID(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/api/v1/captures", humanBToken, "req-compose-unknown-rest", humanUnknown.Request, fixtureBytes(t, humanUnknown.RawParts.StdoutBase64), fixtureBytes(t, humanUnknown.RawParts.StderrBase64))
	if unknownResp.Status != http.StatusCreated {
		t.Fatalf("unknown REST status = %d, want %d: %s", unknownResp.Status, http.StatusCreated, unknownResp.Body)
	}
	var unknownAck captureAckResponse
	decodeBody(t, unknownResp.Body, &unknownAck)
	if unknownAck.Idempotency != "created" || unknownAck.ActionID == "" {
		t.Fatalf("unknown REST ack = %#v", unknownAck)
	}
	assertCaptureRecordedViaCompose(t, runCompose, unknownAck.ActionID, humanBID, "human", "beatrice.operator", "", "needs-plugin", "rest")
	frame2 := readSSEFrame(t, sseResp.Body)
	assertLiveSSECaptureAccepted(t, frame2, unknownAck.ActionID, humanUnknown.Request["captureId"].(string), "rest", "needs-plugin")

	failedResp := doLiveCaptureRequestWithID(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/api/v1/captures", humanAToken, "req-compose-failed-rest", humanParseFailed.Request, fixtureBytes(t, humanParseFailed.RawParts.StdoutBase64), fixtureBytes(t, humanParseFailed.RawParts.StderrBase64))
	if failedResp.Status != http.StatusCreated {
		t.Fatalf("parse-failed REST status = %d, want %d: %s", failedResp.Status, http.StatusCreated, failedResp.Body)
	}
	var failedAck captureAckResponse
	decodeBody(t, failedResp.Body, &failedAck)
	if failedAck.Idempotency != "created" || failedAck.ActionID == "" {
		t.Fatalf("parse-failed REST ack = %#v", failedAck)
	}
	assertCaptureRecordedViaCompose(t, runCompose, failedAck.ActionID, humanAID, "human", "alex.operator", "", "parse-failed", "rest")
	frame3 := readSSEFrame(t, sseResp.Body)
	assertLiveSSECaptureAccepted(t, frame3, failedAck.ActionID, humanParseFailed.Request["captureId"].(string), "rest", "parse-failed")

	aiResp := doLiveCaptureRequestWithID(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/api/v1/captures", aiToken, "req-compose-ai-rest", aiKnown.Request, fixtureBytes(t, aiKnown.RawParts.StdoutBase64), fixtureBytes(t, aiKnown.RawParts.StderrBase64))
	if aiResp.Status != http.StatusCreated {
		t.Fatalf("AI REST status = %d, want %d: %s", aiResp.Status, http.StatusCreated, aiResp.Body)
	}
	var aiAck captureAckResponse
	decodeBody(t, aiResp.Body, &aiAck)
	if aiAck.Idempotency != "created" || aiAck.ActionID == "" {
		t.Fatalf("AI REST ack = %#v", aiAck)
	}
	assertCaptureRecordedViaCompose(t, runCompose, aiAck.ActionID, aiID, "ai_agent", "field-agent-7", humanAID, "parsed", "rest")
	frame4 := readSSEFrame(t, sseResp.Body)
	assertLiveSSECaptureAccepted(t, frame4, aiAck.ActionID, aiKnown.Request["captureId"].(string), "rest", "parsed")

	mcpKnown := loadMCPTranscriptFixture(t)
	initResp := doLiveMCPRequest(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/mcp", humanAToken, "req-compose-mcp-init", "", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "compose-live-client", "version": "1.0.0"},
		},
	})
	if initResp.Status != http.StatusOK {
		t.Fatalf("MCP initialize status = %d, body=%s", initResp.Status, initResp.Body)
	}
	var initBody struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	decodeBody(t, initResp.Body, &initBody)
	if initBody.Result.ProtocolVersion != "2025-03-26" {
		t.Fatalf("MCP protocolVersion = %q", initBody.Result.ProtocolVersion)
	}
	sessionID := strings.TrimSpace(initResp.Headers.Get("MCP-Session-Id"))
	if sessionID == "" {
		t.Fatal("MCP initialize response missing session id")
	}
	listResp := doLiveMCPRequest(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/mcp", humanAToken, "req-compose-mcp-list", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if listResp.Status != http.StatusOK {
		t.Fatalf("MCP tools/list status = %d, body=%s", listResp.Status, listResp.Body)
	}
	var listBody struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	decodeBody(t, listResp.Body, &listBody)
	if len(listBody.Result.Tools) != 2 || listBody.Result.Tools[0].Name != "waypoint_ingest_capture" || listBody.Result.Tools[1].Name != "waypoint_capture_status" {
		t.Fatalf("MCP tools/list = %#v", listBody.Result.Tools)
	}
	mcpResp := doLiveMCPRequest(t, &http.Client{Timeout: 15 * time.Second}, baseURL+"/mcp", humanAToken, "req-compose-mcp-call", sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "waypoint_ingest_capture",
			"arguments": mcpKnown.IngestArguments,
		},
	})
	if mcpResp.Status != http.StatusOK {
		t.Fatalf("MCP tools/call status = %d, body=%s", mcpResp.Status, mcpResp.Body)
	}
	var mcpBody struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Ack struct {
					ActionID         string `json:"actionId"`
					AuditEventCursor string `json:"auditEventCursor"`
					Idempotency      string `json:"idempotency"`
				} `json:"ack"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeBody(t, mcpResp.Body, &mcpBody)
	if mcpBody.Result.IsError || mcpBody.Result.StructuredContent.Ack.Idempotency != "replayed" || mcpBody.Result.StructuredContent.Ack.ActionID != knownAck.ActionID || mcpBody.Result.StructuredContent.Ack.AuditEventCursor != knownAck.AuditEventCursor {
		t.Fatalf("MCP replay ack = %#v", mcpBody.Result)
	}
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM action WHERE engagement_id = '%s'", engagementID))); got != "4" {
		t.Fatalf("action count = %s, want 4", got)
	}
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM audit_event WHERE engagement_id = '%s' AND type = 'capture.accepted'", engagementID))); got != "4" {
		t.Fatalf("accepted audit event count = %s, want 4", got)
	}
}

func TestLoadCaptureTranscriptFixtureResolvesMutations(t *testing.T) {
	parserFailed := loadCaptureTranscriptFixture(t, "valid/human-known-parser-failed.json")
	if got := parserFailed.Request["parsing"].(map[string]any)["status"]; got != "parse-failed" {
		t.Fatalf("parser-failed status = %v, want parse-failed", got)
	}
	if _, ok := parserFailed.Request["parsing"].(map[string]any)["failure"]; !ok {
		t.Fatal("parser-failed fixture missing failure payload")
	}

	aiManual := loadCaptureTranscriptFixture(t, "invalid/ai-manual-initiation.json")
	if got := aiManual.Request["initiatedBy"]; got != "manual" {
		t.Fatalf("ai-manual initiatedBy = %v, want manual", got)
	}
	if _, ok := aiManual.Request["decisionContext"]; ok {
		t.Fatal("ai-manual fixture unexpectedly retained decisionContext")
	}
}

func loadCaptureTranscriptFixture(t *testing.T, rel string) captureTranscriptFixture {
	t.Helper()
	var out captureTranscriptFixture
	materialized := loadCaptureFixtureDocument(t, filepath.Join("contracts", "v1", "fixtures", "captures", rel), map[string]bool{})
	data, err := json.Marshal(materialized)
	if err != nil {
		t.Fatalf("marshal %s: %v", rel, err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode %s: %v", rel, err)
	}
	return out
}

func loadCaptureFixtureDocument(t *testing.T, path string, seen map[string]bool) map[string]any {
	t.Helper()
	var out map[string]any
	decodeJSONFile(t, path, &out)
	baseRel, ok := out["base"].(string)
	if !ok {
		return out
	}
	basePath := filepath.Clean(filepath.Join(filepath.Dir(path), baseRel))
	if seen[basePath] {
		t.Fatalf("fixture base cycle detected at %s", path)
	}
	seen[basePath] = true
	defer delete(seen, basePath)

	base := loadCaptureFixtureDocument(t, basePath, seen)
	materialized, err := applyCaptureFixtureMutations(base, out["mutations"])
	if err != nil {
		t.Fatalf("materialize %s: %v", path, err)
	}
	if name, ok := out["name"].(string); ok {
		materialized["name"] = name
	}
	if description, ok := out["description"].(string); ok {
		materialized["description"] = description
	}
	if expected, ok := out["expected"]; ok {
		materialized["expected"] = expected
	}
	return materialized
}

func applyCaptureFixtureMutations(document map[string]any, mutations any) (map[string]any, error) {
	if mutations == nil {
		return document, nil
	}
	rawMutations, ok := mutations.([]any)
	if !ok {
		return nil, fmt.Errorf("fixture mutations must be an array, got %T", mutations)
	}
	for _, rawMutation := range rawMutations {
		mutation, ok := rawMutation.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("fixture mutation must be an object, got %T", rawMutation)
		}
		op, _ := mutation["op"].(string)
		path, _ := mutation["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("fixture mutation missing path: %v", mutation)
		}
		if op != "add" && op != "remove" && op != "replace" {
			return nil, fmt.Errorf("unsupported fixture mutation op %q", op)
		}
		updated, err := applyCaptureFixtureMutation(document, path, op, mutation["value"])
		if err != nil {
			return nil, err
		}
		document = updated.(map[string]any)
	}
	return document, nil
}

func applyCaptureFixtureMutation(node any, pointer string, op string, value any) (any, error) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("fixture mutation is not a JSON Pointer: %q", pointer)
	}
	return applyCaptureFixtureMutationTokens(node, strings.Split(pointer[1:], "/"), op, value)
}

func applyCaptureFixtureMutationTokens(node any, rawTokens []string, op string, value any) (any, error) {
	if len(rawTokens) == 0 {
		return nil, fmt.Errorf("fixture mutation has an empty path")
	}
	key := strings.NewReplacer("~1", "/", "~0", "~").Replace(rawTokens[0])
	if len(rawTokens) == 1 {
		switch container := node.(type) {
		case map[string]any:
			switch op {
			case "remove":
				if _, ok := container[key]; !ok {
					return nil, fmt.Errorf("fixture mutation path does not exist: /%s", strings.Join(rawTokens, "/"))
				}
				delete(container, key)
			default:
				container[key] = value
			}
			return container, nil
		case []any:
			idx, err := fixtureMutationIndex(key, len(container), op == "add")
			if err != nil {
				return nil, err
			}
			switch op {
			case "remove":
				return append(container[:idx], container[idx+1:]...), nil
			case "replace":
				container[idx] = value
				return container, nil
			case "add":
				container = append(container, nil)
				copy(container[idx+1:], container[idx:])
				container[idx] = value
				return container, nil
			default:
				return nil, fmt.Errorf("unsupported fixture mutation op %q", op)
			}
		default:
			return nil, fmt.Errorf("fixture mutation parent is not a container")
		}
	}

	switch container := node.(type) {
	case map[string]any:
		child, ok := container[key]
		if !ok {
			return nil, fmt.Errorf("fixture mutation path does not exist: /%s", strings.Join(rawTokens, "/"))
		}
		updated, err := applyCaptureFixtureMutationTokens(child, rawTokens[1:], op, value)
		if err != nil {
			return nil, err
		}
		container[key] = updated
		return container, nil
	case []any:
		idx, err := fixtureMutationIndex(key, len(container), false)
		if err != nil {
			return nil, err
		}
		updated, err := applyCaptureFixtureMutationTokens(container[idx], rawTokens[1:], op, value)
		if err != nil {
			return nil, err
		}
		container[idx] = updated
		return container, nil
	default:
		return nil, fmt.Errorf("fixture mutation parent is not a container")
	}
}

func fixtureMutationIndex(token string, length int, allowAppend bool) (int, error) {
	if allowAppend && token == "-" {
		return length, nil
	}
	idx, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("fixture mutation array index %q is not numeric", token)
	}
	if idx < 0 || idx > length || (!allowAppend && idx == length) {
		return 0, fmt.Errorf("fixture mutation array index %d out of range", idx)
	}
	return idx, nil
}

func loadMCPTranscriptFixture(t *testing.T) mcpTranscriptFixture {
	t.Helper()
	var out mcpTranscriptFixture
	decodeJSONFile(t, filepath.Join("contracts", "v1", "fixtures", "mcp.json"), &out)
	return out
}

func decodeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func fixtureBytes(t *testing.T, v string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	return data
}

func doLiveCaptureRequestWithID(t *testing.T, client *http.Client, url, token, requestID string, envelope map[string]any, stdout, stderr []byte) liveResponse {
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
	return liveResponse{Status: resp.StatusCode, Body: data, Headers: resp.Header.Clone()}
}

func doLiveAuditRequest(t *testing.T, client *http.Client, rawURL, token, requestID, after, lastEventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	q := req.URL.Query()
	if after != "" {
		q.Set("after", after)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func doLiveMCPRequest(t *testing.T, client *http.Client, url, token, requestID, sessionID string, body map[string]any) liveResponse {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	if sessionID != "" {
		req.Header.Set("MCP-Session-Id", sessionID)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post mcp: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return liveResponse{Status: resp.StatusCode, Body: out, Headers: resp.Header.Clone()}
}

func decodeBody(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
}

func readSSEFrame(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	frame := map[string]string{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		frame[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan sse frame: %v", err)
	}
	return frame
}

func assertLiveSSECaptureAccepted(t *testing.T, frame map[string]string, wantActionID, wantCaptureID, wantOriginKind, wantParseStatus string) {
	t.Helper()
	if frame["event"] != "capture.accepted" {
		t.Fatalf("sse event = %q, want capture.accepted", frame["event"])
	}
	var event struct {
		Origin struct {
			Kind string `json:"kind"`
		} `json:"origin"`
		Data struct {
			ActionID    string `json:"actionId"`
			CaptureID   string `json:"captureId"`
			ParseStatus string `json:"parseStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(frame["data"]), &event); err != nil {
		t.Fatalf("decode sse data: %v", err)
	}
	if event.Origin.Kind != wantOriginKind || event.Data.ActionID != wantActionID || event.Data.CaptureID != wantCaptureID || event.Data.ParseStatus != wantParseStatus {
		t.Fatalf("sse event = origin:%#v data:%#v", event.Origin, event.Data)
	}
}

func assertCaptureRecordedViaCompose(t *testing.T, runCompose func(args ...string) string, actionID, wantActorID, wantActorKind, wantActorHandle, wantAuthorizedBy, wantParseStatus, wantOriginKind string) {
	t.Helper()
	query := fmt.Sprintf(`SELECT a.id::text, a.kind, a.handle, COALESCE(a.authorized_by::text, ''), act.parse_status FROM action act JOIN actor a ON a.id = act.actor_id WHERE act.id = '%s'`, actionID)
	out := runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-A", "-F", "|", "-tAc", query)
	got := strings.Split(strings.TrimSpace(out), "|")
	if len(got) != 5 {
		t.Fatalf("action metadata query returned %v", got)
	}
	if got[0] != wantActorID || got[1] != wantActorKind || got[2] != wantActorHandle || got[3] != wantAuthorizedBy || got[4] != wantParseStatus {
		t.Fatalf("action metadata = %v, want actor=%s/%s/%s auth=%s parse=%s", got, wantActorID, wantActorKind, wantActorHandle, wantAuthorizedBy, wantParseStatus)
	}
	originQuery := fmt.Sprintf(`SELECT origin_kind FROM audit_event WHERE subject_id = '%s' AND type = 'capture.accepted' ORDER BY id DESC LIMIT 1`, actionID)
	if gotOrigin := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", originQuery)); gotOrigin != wantOriginKind {
		t.Fatalf("origin kind = %q, want %q", gotOrigin, wantOriginKind)
	}
}

func assertEvidenceBytes(t *testing.T, runCompose func(args ...string) string, path string, want []byte) {
	t.Helper()
	got := runCompose("exec", "-T", "waypoint", "sh", "-lc", fmt.Sprintf("cat %s", shellQuote(path)))
	if got != string(want) {
		t.Fatalf("evidence %s = %q, want %q", path, got, want)
	}
}

func execLookPath(bin string) (string, error) { return exec.LookPath(bin) }

func execDockerInfo() (string, error) {
	out, err := exec.Command("docker", "info").CombinedOutput()
	return string(out), err
}

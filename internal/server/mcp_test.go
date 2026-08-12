package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestMCPCaptureStatusAndReviewReuseRESTServices(t *testing.T) {
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
	captureSourceID := "44444444-4444-4444-8444-444444444444"
	claimID := "55555555-5555-4555-8555-555555555555"
	captureToken := "mcp-capture-token"
	reviewToken := "mcp-review-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(captureToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'bob.operator', $3, 'operator')`, aiID, engagementID, hashHex(reviewToken))

	stdout := []byte("mcp ok\n")
	stderr := []byte{}
	stdoutHash := sha256.Sum256(stdout)
	stderrHash := sha256.Sum256(stderr)
	captureBody := bytes.NewBuffer(nil)
	mw := multipart.NewWriter(captureBody)
	writeMultipartJSON(t, mw, "envelope", map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent": map[string]any{
			"id":       captureSourceID,
			"kind":     "mcp_client",
			"name":     "waypoint-mcp-client",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nmap",
		"argv":        []string{"nmap", "-sn", "192.0.2.10"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "ip", "value": "192.0.2.10"},
		"timing":      map[string]any{"startedAt": "2025-01-15T10:00:00.000Z", "endedAt": "2025-01-15T10:00:01.000Z", "durationMs": 1000},
		"execution":   map[string]any{"status": "exited", "exitCode": 0},
		"network":     map[string]any{"execHost": map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"}, "egress": map[string]any{"mode": "off", "status": "disabled"}, "pivotChain": []any{}},
		"evidence":    map[string]any{"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": hex.EncodeToString(stdoutHash[:])}, "stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": hex.EncodeToString(stderrHash[:])}},
		"parsing":     map[string]any{"status": "raw"},
	})
	writeMultipartBlob(t, mw, "stdout", "stdout.txt", stdout)
	writeMultipartBlob(t, mw, "stderr", "stderr.txt", stderr)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/capture", captureBody)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+captureToken)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1")
	req.Header.Set("X-Request-ID", "req-mcp-capture")
	rr := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("mcp capture status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var ack captureAckResponse
	decodeResponse(t, rr, &ack)
	if ack.Idempotency != "created" || ack.ActionID == "" {
		t.Fatalf("mcp capture ack = %#v", ack)
	}

	var originKind string
	if err := db.QueryRowContext(ctx, `SELECT origin_kind FROM audit_event WHERE subject_id = $1 AND type = 'capture.accepted' ORDER BY id DESC LIMIT 1`, ack.ActionID).Scan(&originKind); err != nil {
		t.Fatalf("load capture origin: %v", err)
	}
	if originKind != "mcp" {
		t.Fatalf("capture origin_kind = %q, want mcp", originKind)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/capture", bytes.NewReader(captureBody.Bytes()))
	replayReq.Header.Set("Content-Type", mw.FormDataContentType())
	replayReq.Header.Set("Authorization", "Bearer "+captureToken)
	replayReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	replayReq.Header.Set("Idempotency-Key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1")
	replayReq.Header.Set("X-Request-ID", "req-mcp-capture-replay")
	captureReplayRR := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(captureReplayRR, replayReq)
	if captureReplayRR.Code != http.StatusOK {
		t.Fatalf("mcp capture replay = %d, body=%s", captureReplayRR.Code, captureReplayRR.Body.String())
	}
	decodeResponse(t, captureReplayRR, &ack)
	if ack.Idempotency != "replayed" {
		t.Fatalf("mcp capture replay ack = %#v", ack)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/status", nil)
	statusReq.Header.Set("Authorization", "Bearer "+captureToken)
	statusReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	statusReq.Header.Set("X-Request-ID", "req-mcp-status")
	statusRR := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(statusRR, statusReq)
	if statusRR.Code != http.StatusOK {
		t.Fatalf("mcp status = %d, body=%s", statusRR.Code, statusRR.Body.String())
	}
	var statusBody map[string]any
	decodeResponse(t, statusRR, &statusBody)
	if statusBody["status"] != "ready" {
		t.Fatalf("mcp status body = %#v", statusBody)
	}

	reviewReqBody := map[string]any{
		"claimId":        claimID,
		"claimKind":      "entity",
		"sourceActionId": ack.ActionID,
		"resolution":     "linked",
		"resolvedAt":     "2025-01-15T10:30:00.000Z",
		"notes":          "Linked after operator review.",
	}
	reviewRR := doReviewRequest(t, HandlerWithDB(db), "/api/v1/out-of-band/review", reviewToken, "req-review", claimID, reviewReqBody)
	if reviewRR.Code != http.StatusCreated {
		t.Fatalf("review status = %d, body=%s", reviewRR.Code, reviewRR.Body.String())
	}
	var reviewResp outOfBandReviewResponse
	decodeResponse(t, reviewRR, &reviewResp)
	if reviewResp.Idempotency != "created" || reviewResp.ClaimID != claimID {
		t.Fatalf("review resp = %#v", reviewResp)
	}

	replayRR := doReviewRequest(t, HandlerWithDB(db), "/api/v1/out-of-band/review", reviewToken, "req-review", claimID, reviewReqBody)
	if replayRR.Code != http.StatusOK {
		t.Fatalf("review replay status = %d, body=%s", replayRR.Code, replayRR.Body.String())
	}
	decodeResponse(t, replayRR, &reviewResp)
	if reviewResp.Idempotency != "replayed" {
		t.Fatalf("review replay resp = %#v", reviewResp)
	}

	var reviewCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE subject_id = $1 AND type = 'out-of-band.resolved'`, claimID).Scan(&reviewCount); err != nil {
		t.Fatalf("count review events: %v", err)
	}
	if reviewCount != 1 {
		t.Fatalf("review count = %d, want 1", reviewCount)
	}

	missingSourceClaimID := "66666666-6666-4666-8666-666666666666"
	missingSourceReview := doReviewRequest(t, HandlerWithDB(db), "/api/v1/out-of-band/review", reviewToken, "req-review-missing-source", missingSourceClaimID, map[string]any{
		"claimId":        missingSourceClaimID,
		"claimKind":      "result",
		"sourceActionId": "77777777-7777-4777-8777-777777777777",
		"resolution":     "linked",
		"resolvedAt":     "2025-01-15T10:35:00.000Z",
	})
	if missingSourceReview.Code != http.StatusConflict {
		t.Fatalf("missing source review status = %d, body=%s", missingSourceReview.Code, missingSourceReview.Body.String())
	}
	if !strings.Contains(missingSourceReview.Body.String(), "invalid_source_capture") {
		t.Fatalf("missing source review body = %s", missingSourceReview.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE subject_id = $1 AND type = 'out-of-band.resolved'`, missingSourceClaimID).Scan(&reviewCount); err != nil {
		t.Fatalf("count missing-source review events: %v", err)
	}
	if reviewCount != 0 {
		t.Fatalf("missing-source review count = %d, want 0", reviewCount)
	}
}

func writeMultipartJSON(t *testing.T, mw *multipart.Writer, name string, v any) {
	t.Helper()
	part, err := mw.CreateFormField(name)
	if err != nil {
		t.Fatalf("create part %s: %v", name, err)
	}
	if err := json.NewEncoder(part).Encode(v); err != nil {
		t.Fatalf("encode part %s: %v", name, err)
	}
}

func writeMultipartBlob(t *testing.T, mw *multipart.Writer, name, filename string, blob []byte) {
	t.Helper()
	part, err := mw.CreateFormFile(name, filename)
	if err != nil {
		t.Fatalf("create file part %s: %v", name, err)
	}
	if _, err := part.Write(blob); err != nil {
		t.Fatalf("write blob %s: %v", name, err)
	}
}

func doReviewRequest(t *testing.T, h http.Handler, path, token, requestID, idempotencyKey string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("X-Request-ID", requestID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestBuildOutOfBandClaimPendingAndLinked(t *testing.T) {
	claimID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
	actor := auditEventActor{ID: "22222222-2222-4222-8222-222222222222", Kind: "human", Handle: "alex.operator", Role: "operator"}
	observedAt := time.Date(2025, 1, 15, 10, 25, 0, 0, time.UTC)
	rows := []outOfBandClaimEventRow{{
		EventID:    106,
		ClaimID:    claimID,
		Type:       outOfBandClaimTypeFlagged,
		Revision:   1,
		OccurredAt: observedAt,
		Actor:      actor,
		Data: mustJSONClaim(t, outOfBandClaimObservedData{
			ClaimID:           claimID,
			ClaimKind:         "entity",
			ClaimedSubjectID:  "66666666-6666-4666-8666-666666666666",
			DetectionBoundary: "best_effort",
			Reason:            "missing_captured_source_action",
			ObservedAt:        observedAt,
		}),
	}}

	pending, err := buildOutOfBandClaim("11111111-1111-4111-8111-111111111111", rows)
	if err != nil {
		t.Fatalf("build pending claim: %v", err)
	}
	if pending.Status != outOfBandClaimStatusPending || pending.Revision != 1 || pending.ResolvedAt != nil {
		t.Fatalf("pending claim = %#v", pending)
	}

	resolvedAt := observedAt.Add(5 * time.Minute)
	sourceActionID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"
	rows = append(rows, outOfBandClaimEventRow{
		EventID:    107,
		ClaimID:    claimID,
		Type:       outOfBandClaimTypeResolved,
		Revision:   2,
		OccurredAt: resolvedAt,
		Actor:      actor,
		Data: mustJSONClaim(t, outOfBandClaimResolvedData{
			ClaimID:        claimID,
			ClaimKind:      "entity",
			SourceActionID: &sourceActionID,
			Resolution:     outOfBandClaimStatusLinked,
			ResolvedAt:     resolvedAt,
			Notes:          "linked to a captured action",
		}),
	})

	linked, err := buildOutOfBandClaim("11111111-1111-4111-8111-111111111111", rows)
	if err != nil {
		t.Fatalf("build linked claim: %v", err)
	}
	if linked.Status != outOfBandClaimStatusLinked || linked.Revision != 2 || linked.ResolvedAt == nil || linked.ResolvedBy == nil {
		t.Fatalf("linked claim = %#v", linked)
	}
	if linked.SourceActionID == nil || *linked.SourceActionID != sourceActionID {
		t.Fatalf("linked sourceActionId = %#v", linked.SourceActionID)
	}
}

func TestOutOfBandClaimRoutesWithoutDatabaseAreUnavailable(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/api/v1/out-of-band-claims", "/api/v1/out-of-band-claims/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "/api/v1/out-of-band-claims/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2/resolve", "/out-of-band-claims", "/out-of-band-claims/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "/out-of-band-claims/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2/resolve"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if strings.HasSuffix(path, "/resolve") {
			req.Method = http.MethodPost
			req.Body = http.NoBody
		}
		req.Header.Set("Authorization", "Bearer demo-token")
		req.Header.Set("Waypoint-Contract-Version", "1.0.0")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("path %s status = %d, want %d", path, rr.Code, http.StatusServiceUnavailable)
		}
	}
}

func TestOutOfBandReviewRoutesWithoutDatabaseAreUnavailable(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/api/v1/out-of-band-claims/review", "/out-of-band-claims/review", "/api/v1/out-of-band-claims/reviews", "/out-of-band-claims/reviews"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"claimId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2","claimKind":"entity","sourceActionId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3","resolution":"linked","resolvedAt":"2025-01-15T10:30:00Z"}`))
		req.Header.Set("Authorization", "Bearer demo-token")
		req.Header.Set("Waypoint-Contract-Version", "1.0.0")
		req.Header.Set("Idempotency-Key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("path %s status = %d, want %d", path, rr.Code, http.StatusServiceUnavailable)
		}
	}
}

func TestOutOfBandClaimLifecycleThroughPostgreSQL(t *testing.T) {
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
	sourceActorID := "22222222-2222-4222-8222-222222222222"
	reviewerActorID := "33333333-3333-4333-8333-333333333333"
	sourceToken := "oob-source-token"
	reviewerToken := "oob-review-token"
	claimObservedAt := time.Date(2025, 1, 15, 10, 25, 0, 0, time.UTC)
	claimResolvedAt := claimObservedAt.Add(5 * time.Minute)
	claimedSubjectID := "66666666-6666-4666-8666-666666666666"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, sourceActorID, engagementID, hashHex(sourceToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'beatrice.operator', $3, 'operator')`, reviewerActorID, engagementID, hashHex(reviewerToken))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	sourceEnvelope := rawCaptureEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", []byte("whoami\n"), []byte(""))
	sourceResp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", sourceToken, "req-oob-source", sourceEnvelope, []byte("whoami\n"), nil)
	if sourceResp.status != http.StatusCreated {
		t.Fatalf("source capture status = %d, body=%s", sourceResp.status, sourceResp.body)
	}
	var sourceAck captureAckResponse
	decodeBody(t, sourceResp.body, &sourceAck)
	if sourceAck.Idempotency != "created" || sourceAck.ActionID == "" {
		t.Fatalf("source ack = %#v", sourceAck)
	}
	assertActionSnapshot(t, ctx, db, sourceAck.ActionID, sourceActorID, "human", "manual", "raw", "", false)
	assertActionNetworkFields(t, ctx, db, sourceAck.ActionID, gateActionExpectation{
		sourceAgentID:   "44444444-4444-4444-8444-444444444444",
		execHostIP:      "10.10.0.12",
		egressIP:        "",
		pivotType:       "",
		wantDecisionCtx: false,
	})
	assertSingleResultAndObservation(t, ctx, db, sourceAck.ActionID, 0, 0)
	assertEvidenceLinked(t, ctx, db, evidenceDir, sourceAck.ActionID, "whoami\n", "")

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events", reviewerToken, "req-oob-sse", "", "")
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status = %d, body=%s", sseResp.StatusCode, readBody(t, sseResp.Body))
	}

	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/out-of-band-claims", reviewerToken, "req-oob-create", outOfBandClaimCreateRequest{
		ClaimKind:        "entity",
		ClaimedSubjectID: claimedSubjectID,
		ObservedAt:       claimObservedAt,
	}, "")
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", createResp.StatusCode, readBody(t, createResp.Body))
	}
	var created outOfBandClaimItem
	decodeHTTPResp(t, createResp, &created)
	if created.Status != outOfBandClaimStatusPending || created.Revision != 1 || created.SourceActionID != nil {
		t.Fatalf("created claim = %#v", created)
	}
	if created.ObservedBy.ID != reviewerActorID || created.ObservedBy.Handle != "beatrice.operator" {
		t.Fatalf("created observedBy = %#v", created.ObservedBy)
	}

	flaggedFrame := readSSEFrame(t, sseResp.Body)
	var flaggedEvent auditEventItem
	if err := json.Unmarshal([]byte(flaggedFrame["data"]), &flaggedEvent); err != nil {
		t.Fatalf("decode flagged event: %v", err)
	}
	if flaggedFrame["event"] != outOfBandClaimTypeFlagged || flaggedEvent.Type != outOfBandClaimTypeFlagged || flaggedEvent.Subject.ID != created.ID || flaggedEvent.Subject.Type != "out_of_band_claim" {
		t.Fatalf("flagged event = frame=%#v item=%#v", flaggedFrame, flaggedEvent)
	}
	if !strings.Contains(string(flaggedEvent.Data), `"detectionBoundary":"best_effort"`) || !strings.Contains(string(flaggedEvent.Data), `"reason":"missing_captured_source_action"`) {
		t.Fatalf("flagged data = %s", flaggedEvent.Data)
	}

	pageResp := doJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/out-of-band-claims?limit=10", reviewerToken, "req-oob-page-1", nil, "")
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d, body=%s", pageResp.StatusCode, readBody(t, pageResp.Body))
	}
	var page outOfBandClaimPageResponse
	decodeHTTPResp(t, pageResp, &page)
	if len(page.Items) != 1 || page.Items[0].ID != created.ID || page.Items[0].Status != outOfBandClaimStatusPending {
		t.Fatalf("claim page = %#v", page)
	}

	reviewResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/out-of-band-claims/review", reviewerToken, "req-oob-review", outOfBandReviewRequest{
		ClaimID:    created.ID,
		ClaimKind:  created.ClaimKind,
		Resolution: outOfBandClaimStatusDismissed,
		ResolvedAt: claimResolvedAt,
		Notes:      "Operator confirmed this observation has no captured source action.",
	}, created.ID)
	if reviewResp.StatusCode != http.StatusCreated {
		t.Fatalf("review status = %d, body=%s", reviewResp.StatusCode, readBody(t, reviewResp.Body))
	}
	var reviewed outOfBandReviewResponse
	decodeHTTPResp(t, reviewResp, &reviewed)
	if reviewed.Idempotency != "created" || reviewed.ClaimID != created.ID || reviewed.AuditEventCursor == "" {
		t.Fatalf("review response = %#v", reviewed)
	}
	if !reviewed.ResolvedAt.Equal(claimResolvedAt) {
		t.Fatalf("review resolvedAt = %s, want %s", reviewed.ResolvedAt, claimResolvedAt)
	}

	resolvedFrame := readSSEFrame(t, sseResp.Body)
	var resolvedEvent auditEventItem
	if err := json.Unmarshal([]byte(resolvedFrame["data"]), &resolvedEvent); err != nil {
		t.Fatalf("decode resolved event: %v", err)
	}
	if resolvedFrame["event"] != outOfBandClaimTypeResolved || resolvedEvent.Type != outOfBandClaimTypeResolved || resolvedEvent.Subject.ID != created.ID || resolvedEvent.Subject.Type != "out_of_band_claim" {
		t.Fatalf("resolved event = frame=%#v item=%#v", resolvedFrame, resolvedEvent)
	}
	if !strings.Contains(string(resolvedEvent.Data), `"resolution":"dismissed"`) || !strings.Contains(string(resolvedEvent.Data), `"notes":"Operator confirmed this observation has no captured source action."`) {
		t.Fatalf("resolved data = %s", resolvedEvent.Data)
	}

	replayResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/out-of-band-claims/review", reviewerToken, "req-oob-review-replay", outOfBandReviewRequest{
		ClaimID:    created.ID,
		ClaimKind:  created.ClaimKind,
		Resolution: outOfBandClaimStatusDismissed,
		ResolvedAt: claimResolvedAt,
		Notes:      "Operator confirmed this observation has no captured source action.",
	}, created.ID)
	if replayResp.StatusCode != http.StatusOK {
		t.Fatalf("review replay status = %d, body=%s", replayResp.StatusCode, readBody(t, replayResp.Body))
	}
	var replayed outOfBandReviewResponse
	decodeHTTPResp(t, replayResp, &replayed)
	if replayed.Idempotency != "replayed" || replayed.AuditEventCursor != reviewed.AuditEventCursor {
		t.Fatalf("review replay = %#v", replayed)
	}

	var flaggedCount, resolvedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = $2 AND subject_type = 'out_of_band_claim' AND subject_id = $3`, engagementID, outOfBandClaimTypeFlagged, created.ID).Scan(&flaggedCount); err != nil {
		t.Fatalf("count flagged events: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = $2 AND subject_type = 'out_of_band_claim' AND subject_id = $3`, engagementID, outOfBandClaimTypeResolved, created.ID).Scan(&resolvedCount); err != nil {
		t.Fatalf("count resolved events: %v", err)
	}
	if flaggedCount != 1 || resolvedCount != 1 {
		t.Fatalf("claim audit counts = flagged:%d resolved:%d", flaggedCount, resolvedCount)
	}

	claimResp := doJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/out-of-band-claims/"+created.ID, reviewerToken, "req-oob-claim", nil, "")
	if claimResp.StatusCode != http.StatusOK {
		t.Fatalf("claim get status = %d, body=%s", claimResp.StatusCode, readBody(t, claimResp.Body))
	}
	var resolved outOfBandClaimItem
	decodeHTTPResp(t, claimResp, &resolved)
	if resolved.Status != outOfBandClaimStatusDismissed || resolved.Revision != 2 || resolved.ResolvedBy == nil || resolved.ResolvedBy.ID != reviewerActorID {
		t.Fatalf("resolved claim = %#v", resolved)
	}

	pageResp = doJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/out-of-band-claims?limit=10", reviewerToken, "req-oob-page-2", nil, "")
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("page after review status = %d, body=%s", pageResp.StatusCode, readBody(t, pageResp.Body))
	}
	decodeHTTPResp(t, pageResp, &page)
	if len(page.Items) != 1 || page.Items[0].Status != outOfBandClaimStatusDismissed || page.Items[0].Revision != 2 {
		t.Fatalf("page after review = %#v", page)
	}
}

func doJSONRequest(t *testing.T, client *http.Client, method, rawURL, token, requestID string, body any, idempotencyKey string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeHTTPResp(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readBody(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func mustJSONClaim(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

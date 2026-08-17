package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func mustJSONClaim(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

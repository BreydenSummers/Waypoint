package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestEntityTierOverride(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	operatorID := "22222222-2222-4222-8222-222222222222"
	viewerID := "23232323-2323-4232-8232-232323232323"
	operatorToken := "tier-op-token"
	viewerToken := "tier-viewer-token"
	entityID := "44444444-4444-4444-8444-444444444444"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'op', $3, 'operator')`, operatorID, engagementID, hashHex(operatorToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'view', $3, 'viewer')`, viewerID, engagementID, hashHex(viewerToken))
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'ws-01.local')`, entityID, engagementID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()
	url := ts.URL + "/api/v1/entities/" + entityID

	// viewer cannot override
	resp := doFindingRequest(t, ts.Client(), url, viewerToken, "req-tier-viewer", http.MethodPatch, map[string]any{"tierOverride": 0})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer override status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// out-of-range rejected
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-tier-range", http.MethodPatch, map[string]any{"tierOverride": 5})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("range override status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// operator sets an override
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-tier-set", http.MethodPatch, map[string]any{"tierOverride": 1})
	var set entityReadResponse
	decodeHTTPResponse(t, resp, &set)
	resp.Body.Close()
	if set.TierOverride == nil || *set.TierOverride != 1 {
		t.Fatalf("tierOverride = %v, want 1", set.TierOverride)
	}
	if set.Revision != 2 {
		t.Fatalf("revision after set = %d, want 2", set.Revision)
	}

	// stale expectedRevision conflicts
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-tier-conflict", http.MethodPatch, map[string]any{"tierOverride": 2, "expectedRevision": 1})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale revision status = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// clearing the override (null) reverts to derived
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-tier-clear", http.MethodPatch, map[string]any{"tierOverride": nil, "expectedRevision": 2})
	var cleared entityReadResponse
	decodeHTTPResponse(t, resp, &cleared)
	resp.Body.Close()
	if cleared.TierOverride != nil {
		t.Fatalf("tierOverride after clear = %v, want nil", cleared.TierOverride)
	}
	if cleared.Revision != 3 {
		t.Fatalf("revision after clear = %d, want 3", cleared.Revision)
	}
}

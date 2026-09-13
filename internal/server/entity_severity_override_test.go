package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestEntitySeverityOverride(t *testing.T) {
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
	operatorToken := "sev-op-token"
	viewerToken := "sev-viewer-token"
	entityID := "44444444-4444-4444-8444-444444444444"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'op', $3, 'operator')`, operatorID, engagementID, hashHex(operatorToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'view', $3, 'viewer')`, viewerID, engagementID, hashHex(viewerToken))
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'ws-01.local')`, entityID, engagementID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()
	url := ts.URL + "/api/v1/entities/" + entityID

	// viewer cannot override
	resp := doFindingRequest(t, ts.Client(), url, viewerToken, "req-sev-viewer", http.MethodPatch, map[string]any{"severityOverride": "high"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer override status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// invalid severity value rejected
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-sev-bad", http.MethodPatch, map[string]any{"severityOverride": "spicy"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid value status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// both overrides at once rejected (exactly one required)
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-sev-both", http.MethodPatch, map[string]any{"severityOverride": "high", "tierOverride": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("both-overrides status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// operator sets an override
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-sev-set", http.MethodPatch, map[string]any{"severityOverride": "critical"})
	var set entityReadResponse
	decodeHTTPResponse(t, resp, &set)
	resp.Body.Close()
	if set.SeverityOverride == nil || *set.SeverityOverride != "critical" {
		t.Fatalf("severityOverride = %v, want critical", set.SeverityOverride)
	}
	if set.Revision != 2 {
		t.Fatalf("revision after set = %d, want 2", set.Revision)
	}

	// stale expectedRevision conflicts
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-sev-conflict", http.MethodPatch, map[string]any{"severityOverride": "low", "expectedRevision": 1})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale revision status = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// clearing the override (null) reverts to derived
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-sev-clear", http.MethodPatch, map[string]any{"severityOverride": nil, "expectedRevision": 2})
	var cleared entityReadResponse
	decodeHTTPResponse(t, resp, &cleared)
	resp.Body.Close()
	if cleared.SeverityOverride != nil {
		t.Fatalf("severityOverride after clear = %v, want nil", cleared.SeverityOverride)
	}
	if cleared.Revision != 3 {
		t.Fatalf("revision after clear = %d, want 3", cleared.Revision)
	}
}

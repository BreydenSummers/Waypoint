package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestEntityDeclareSubnet(t *testing.T) {
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
	operatorToken := "declare-op-token1"
	viewerToken := "declare-viewer-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', '10.4.0.0/16 campus', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'op', $3, 'operator')`, operatorID, engagementID, hashHex(operatorToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'view', $3, 'viewer')`, viewerID, engagementID, hashHex(viewerToken))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()
	url := ts.URL + "/api/v1/entities"

	// viewer cannot declare
	resp := doFindingRequest(t, ts.Client(), url, viewerToken, "req-declare-viewer", http.MethodPost, map[string]any{"kind": "subnet", "cidr": "10.9.1.0/24"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer declare status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// only subnet kind is allowed
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-declare-kind", http.MethodPost, map[string]any{"kind": "host", "cidr": "10.9.1.0/24"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("host declare status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// invalid cidr rejected
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-declare-bad", http.MethodPost, map[string]any{"kind": "subnet", "cidr": "10.9.1.0/40"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad cidr status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// IPv6 rejected for now (the UI's subnet model is IPv4-only)
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-declare-v6", http.MethodPost, map[string]any{"kind": "subnet", "cidr": "fd00::/64"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("v6 cidr status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// operator declares a subnet; the cidr is normalized to its masked form
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-declare-ok", http.MethodPost, map[string]any{"kind": "subnet", "cidr": "10.9.1.77/24", "label": "Branch office"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("declare status = %d, want 201", resp.StatusCode)
	}
	var created entityReadResponse
	decodeHTTPResponse(t, resp, &created)
	resp.Body.Close()
	if created.Kind != "subnet" {
		t.Fatalf("kind = %q, want subnet", created.Kind)
	}
	var attrs map[string]any
	if err := json.Unmarshal(created.Attributes, &attrs); err != nil {
		t.Fatalf("attributes: %v", err)
	}
	if attrs["cidr"] != "10.9.1.0/24" {
		t.Fatalf("cidr attr = %v, want 10.9.1.0/24", attrs["cidr"])
	}
	if attrs["label"] != "Branch office" {
		t.Fatalf("label attr = %v, want Branch office", attrs["label"])
	}
	if attrs["declaredBy"] != "op" {
		t.Fatalf("declaredBy attr = %v, want op", attrs["declaredBy"])
	}

	// declaring the same subnet again upserts rather than duplicating
	resp = doFindingRequest(t, ts.Client(), url, operatorToken, "req-declare-again", http.MethodPost, map[string]any{"kind": "subnet", "cidr": "10.9.1.0/24"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("re-declare status = %d, want 201", resp.StatusCode)
	}
	var again entityReadResponse
	decodeHTTPResponse(t, resp, &again)
	resp.Body.Close()
	if again.ID != created.ID {
		t.Fatalf("re-declare id = %s, want %s", again.ID, created.ID)
	}

	// the audit trail records the declaration
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_event WHERE engagement_id = $1 AND type = 'entity.subnet-declared'`, engagementID).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("audit events = %d, want 2", auditCount)
	}
}

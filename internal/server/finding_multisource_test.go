package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

// TestFindingMultiSourceEvidence covers promoting a finding with more than one
// corroborating capture and appending further evidence afterwards (append-only).
func TestFindingMultiSourceEvidence(t *testing.T) {
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
	humanToken := "ms-human-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'op', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))

	entityID := "44444444-4444-4444-8444-444444444444"
	stdoutID := "55555555-5555-4555-8555-555555555551"
	stderrID := "55555555-5555-4555-8555-555555555552"
	attackID := "66666666-6666-4666-8666-666666666661"
	reconID := "66666666-6666-4666-8666-666666666662"
	reconID2 := "66666666-6666-4666-8666-666666666663"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'dc.local')`, entityID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', repeat('a', 64), 0, 'text/plain', 'stdout/ms')`, stdoutID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', repeat('b', 64), 0, 'text/plain', 'stderr/ms')`, stderrID, engagementID)
	insertAction := func(id, phase string) {
		mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', $4, 'cmd', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'dc.local', now(), now(), 0, $5, $6, 'raw')`, id, engagementID, humanID, phase, stdoutID, stderrID)
	}
	insertAction(attackID, "attacks")
	insertAction(reconID, "recon")
	insertAction(reconID2, "recon")
	resultID := "77777777-7777-4777-8777-777777777771"
	mustExec(t, db, `INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted) VALUES ($1, $2, $3, 'plugin.demo', 'https://schemas.waypoint.security/demo', '1.0.0', '{}'::jsonb)`, resultID, engagementID, attackID)
	mustExec(t, db, `INSERT INTO observation (id, engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes) VALUES ($1, $2, $3, $4, $5, 'host', '[]'::jsonb, '{}'::jsonb)`, "88888888-8888-4888-8888-888888888881", engagementID, attackID, resultID, entityID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	// Promote with one corroborating recon capture beyond the attack action.
	promoteReq := map[string]any{
		"sourceActionId":    attackID,
		"evidenceActionIds": []string{reconID},
		"title":             "Kerberoastable service account",
		"severity":          "critical",
		"remediation":       "Rotate the service account and require AES.",
		"status":            "open",
	}
	resp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/promote", humanToken, "req-ms-promote", http.MethodPost, promoteReq)
	var promoted findingItem
	decodeHTTPResponse(t, resp, &promoted)
	resp.Body.Close()
	if len(promoted.EvidenceActionIDs) != 2 || promoted.EvidenceActionIDs[0] != attackID || promoted.EvidenceActionIDs[1] != reconID {
		t.Fatalf("promoted evidence = %v, want [%s %s]", promoted.EvidenceActionIDs, attackID, reconID)
	}

	// Append a further corroborating capture.
	patch := map[string]any{"expectedRevision": promoted.Revision, "addEvidenceActionIds": []string{reconID2}}
	resp = doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID, humanToken, "req-ms-append", http.MethodPatch, patch)
	var appended findingItem
	decodeHTTPResponse(t, resp, &appended)
	resp.Body.Close()
	if len(appended.EvidenceActionIDs) != 3 || appended.EvidenceActionIDs[2] != reconID2 {
		t.Fatalf("appended evidence = %v, want 3 ending in %s", appended.EvidenceActionIDs, reconID2)
	}
	if appended.Revision != promoted.Revision+1 {
		t.Fatalf("append revision = %d, want %d", appended.Revision, promoted.Revision+1)
	}

	// Appending an unknown action is rejected.
	badPatch := map[string]any{"expectedRevision": appended.Revision, "addEvidenceActionIds": []string{"99999999-9999-4999-8999-999999999999"}}
	resp = doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID, humanToken, "req-ms-badappend", http.MethodPatch, badPatch)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown evidence append status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// The append-only DB guard forbids shrinking/rewriting evidence links.
	if _, err := db.ExecContext(ctx, `UPDATE finding SET evidence_action_ids = ARRAY[$2]::uuid[] WHERE id = $1`, promoted.ID, reconID); err == nil {
		t.Fatalf("expected append-only guard to reject removing an evidence link")
	}
}

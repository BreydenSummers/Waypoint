package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestFindingPromotionRevisionsAndOperatorOnlyPromotion(t *testing.T) {
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
	humanToken := "finding-human-token"
	aiToken := "finding-ai-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Waypoint', 'gpt-4.1', '1.0', $4)`, aiID, engagementID, hashHex(aiToken), humanID)

	entityID := "44444444-4444-4444-8444-444444444444"
	evidenceStdoutID := "55555555-5555-4555-8555-555555555551"
	evidenceStderrID := "55555555-5555-4555-8555-555555555552"
	actionID := "66666666-6666-4666-8666-666666666666"
	resultID := "77777777-7777-4777-8777-777777777777"
	observationID := "88888888-8888-4888-8888-888888888888"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'fileserver.local')`, entityID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', repeat('a', 64), 0, 'text/plain', 'stdout/finding')`, evidenceStdoutID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', repeat('b', 64), 0, 'text/plain', 'stderr/finding')`, evidenceStderrID, engagementID)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status, decision_context) VALUES ($1, $2, $3, $3, 'ai', 'attacks', 'smbclient', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'fileserver.local', now(), now(), 0, $4, $5, 'raw', '{"rationale":"verify share access"}'::jsonb)`, actionID, engagementID, aiID, evidenceStdoutID, evidenceStderrID)
	mustExec(t, db, `INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted) VALUES ($1, $2, $3, 'plugin.demo', 'https://schemas.waypoint.security/demo', '1.0.0', '{}'::jsonb)`, resultID, engagementID, actionID)
	mustExec(t, db, `INSERT INTO observation (id, engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes) VALUES ($1, $2, $3, $4, $5, 'host', '[]'::jsonb, '{}'::jsonb)`, observationID, engagementID, actionID, resultID, entityID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	promoteReq := map[string]any{
		"sourceActionId": actionID,
		"title":          "Anonymous shares exposed",
		"severity":       "high",
		"remediation":    "Restrict share access and review group membership.",
		"status":         "open",
	}
	promoteResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/promote", humanToken, "req-finding-promote", http.MethodPost, promoteReq)
	defer promoteResp.Body.Close()
	if promoteResp.StatusCode != http.StatusCreated {
		t.Fatalf("promote status = %d, want %d", promoteResp.StatusCode, http.StatusCreated)
	}
	var promoted findingItem
	decodeHTTPResponse(t, promoteResp, &promoted)
	if promoted.Revision != 1 || promoted.PromotedBy != humanID || promoted.Status != "open" || len(promoted.EvidenceActionIDs) != 1 || promoted.EvidenceActionIDs[0] != actionID {
		t.Fatalf("promoted finding = %#v", promoted)
	}
	if len(promoted.AffectedEntityIDs) != 1 || promoted.AffectedEntityIDs[0] != entityID {
		t.Fatalf("promoted finding affected entities = %#v", promoted.AffectedEntityIDs)
	}

	var storedRevision int
	var promotedBy, affectedJSON, evidenceJSON string
	if err := db.QueryRowContext(ctx, `SELECT revision, promoted_by::text, COALESCE(array_to_json(affected_entity_ids)::text, '[]'), COALESCE(array_to_json(evidence_action_ids)::text, '[]') FROM finding WHERE id = $1`, promoted.ID).Scan(&storedRevision, &promotedBy, &affectedJSON, &evidenceJSON); err != nil {
		t.Fatalf("load finding row: %v", err)
	}
	var affected, evidence []string
	if err := json.Unmarshal([]byte(affectedJSON), &affected); err != nil {
		t.Fatalf("decode affected entities: %v", err)
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
		t.Fatalf("decode evidence actions: %v", err)
	}
	if storedRevision != 1 || promotedBy != humanID || len(evidence) != 1 || evidence[0] != actionID {
		t.Fatalf("finding row = rev:%d promotedBy:%s evidence:%v", storedRevision, promotedBy, evidence)
	}
	if len(affected) != 1 || affected[0] != entityID {
		t.Fatalf("finding affected entities = %v", affected)
	}

	var typ, actorKind, actorHandle, actorRole string
	var subjectRevision int
	var data string
	if err := db.QueryRowContext(ctx, `SELECT type, actor_kind, actor_handle, actor_role, subject_revision, data::text FROM audit_event WHERE subject_type = 'finding' AND subject_id = $1 ORDER BY id ASC LIMIT 1`, promoted.ID).Scan(&typ, &actorKind, &actorHandle, &actorRole, &subjectRevision, &data); err != nil {
		t.Fatalf("load finding promotion audit event: %v", err)
	}
	if typ != "finding.promoted" || actorKind != "human" || actorHandle != "alex.operator" || actorRole != "operator" || subjectRevision != 1 {
		t.Fatalf("promotion audit event = %s %s %s %s rev=%d", typ, actorKind, actorHandle, actorRole, subjectRevision)
	}
	if !bytes.Contains([]byte(data), []byte(`"sourceActionId":"`+actionID+`"`)) {
		t.Fatalf("promotion audit data = %s", data)
	}

	aiResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/promote", aiToken, "req-finding-ai", http.MethodPost, promoteReq)
	defer aiResp.Body.Close()
	if aiResp.StatusCode != http.StatusForbidden {
		t.Fatalf("ai promote status = %d, want %d", aiResp.StatusCode, http.StatusForbidden)
	}

	updateReq := map[string]any{
		"expectedRevision": promoted.Revision,
		"status":           "triage",
		"remediation":      "Restrict share access and review group membership immediately.",
	}
	updateResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID, humanToken, "req-finding-update", http.MethodPatch, updateReq)
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateResp.StatusCode, http.StatusOK)
	}
	var updated findingItem
	decodeHTTPResponse(t, updateResp, &updated)
	if updated.Revision != 2 || updated.Status != "triage" {
		t.Fatalf("updated finding = %#v", updated)
	}

	staleResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID, humanToken, "req-finding-stale", http.MethodPatch, updateReq)
	defer staleResp.Body.Close()
	if staleResp.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status = %d, want %d", staleResp.StatusCode, http.StatusConflict)
	}

	revisionsResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID+"/revisions", humanToken, "req-finding-revisions", http.MethodGet, nil)
	defer revisionsResp.Body.Close()
	if revisionsResp.StatusCode != http.StatusOK {
		t.Fatalf("revisions status = %d, want %d", revisionsResp.StatusCode, http.StatusOK)
	}
	var revisions findingRevisionsResponse
	decodeHTTPResponse(t, revisionsResp, &revisions)
	if len(revisions.Items) != 2 || revisions.Items[0].Actor.Kind != "human" || revisions.Items[1].Actor.Kind != "human" {
		t.Fatalf("finding revisions = %#v", revisions)
	}
	if revisions.Items[0].Type != "finding.promoted" || revisions.Items[1].Type != "finding.status-changed" {
		t.Fatalf("finding revision event types = %#v", revisions.Items)
	}
}

func doFindingRequest(t *testing.T, client *http.Client, url, token, requestID, method string, payload any) *http.Response {
	t.Helper()
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal finding request: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do finding request: %v", err)
	}
	return resp
}

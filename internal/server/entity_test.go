package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestEntityMergeSplitPreviewUndoAndProvenance(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "77777777-7777-4777-8777-777777777777"
	actorID := "88888888-8888-4888-8888-888888888888"
	token := "entity-merge-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	sourceID := "99999999-9999-4999-8999-999999999991"
	targetID := "99999999-9999-4999-8999-999999999992"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'alpha.local', '{"role":"source"}'::jsonb)`, sourceID, engagementID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'beta.local', '{"role":"target"}'::jsonb)`, targetID, engagementID)
	observationID := seedObservation(t, db, engagementID, actorID, sourceID, "alpha.local")

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	preview := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "req-merge-preview", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID, Preview: true})
	defer preview.Body.Close()
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("merge preview status = %d, want %d", preview.StatusCode, http.StatusOK)
	}
	var previewResp entityMutationResponse
	decodeHTTPResponse(t, preview, &previewResp)
	if !previewResp.Preview || previewResp.Applied {
		t.Fatalf("merge preview response = %#v", previewResp)
	}
	if previewResp.Source.MergedIntoEntityID == nil || *previewResp.Source.MergedIntoEntityID != targetID {
		t.Fatalf("merge preview source = %#v", previewResp.Source)
	}

	var mergedInto sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT merged_into_entity_id::text FROM entity WHERE id = $1`, sourceID).Scan(&mergedInto); err != nil {
		t.Fatalf("load preview merge state: %v", err)
	}
	if mergedInto.Valid {
		t.Fatalf("merge preview mutated source entity")
	}

	merged := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "req-merge", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID})
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d, want %d", merged.StatusCode, http.StatusOK)
	}
	var mergedResp entityMutationResponse
	decodeHTTPResponse(t, merged, &mergedResp)
	if !mergedResp.Applied || mergedResp.Source.MergedIntoEntityID == nil || *mergedResp.Source.MergedIntoEntityID != targetID {
		t.Fatalf("merge response = %#v", mergedResp)
	}

	var typ, actorKind, actorHandle, actorRole, requestID string
	var data string
	if err := db.QueryRowContext(ctx, `SELECT type, actor_kind, actor_handle, actor_role, request_id, data::text FROM audit_event WHERE subject_id = $1 ORDER BY id DESC LIMIT 1`, sourceID).Scan(&typ, &actorKind, &actorHandle, &actorRole, &requestID, &data); err != nil {
		t.Fatalf("load merge audit event: %v", err)
	}
	if typ != "entity.merged" || actorKind != "human" || actorHandle != "alex.operator" || actorRole != "operator" || requestID != "req-merge" {
		t.Fatalf("merge audit event = %s %s %s %s %s", typ, actorKind, actorHandle, actorRole, requestID)
	}
	if !bytes.Contains([]byte(data), []byte(`"sourceEntityId":"`+sourceID+`"`)) || !bytes.Contains([]byte(data), []byte(`"targetEntityId":"`+targetID+`"`)) {
		t.Fatalf("merge audit data = %s", data)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin provenance tx: %v", err)
	}
	resolvedID, err := upsertEntity(ctx, tx, engagementID, "host", "fqdn", "alpha.local", map[string]any{"role": "source", "state": "updated"})
	if err != nil {
		t.Fatalf("upsert merged entity: %v", err)
	}
	if resolvedID != targetID {
		t.Fatalf("merged source routed to %q, want %q", resolvedID, targetID)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit provenance tx: %v", err)
	}

	var currentSourceRev, currentTargetRev int
	if err := db.QueryRowContext(ctx, `SELECT s.revision, t.revision FROM entity s, entity t WHERE s.id = $1 AND t.id = $2`, sourceID, targetID).Scan(&currentSourceRev, &currentTargetRev); err != nil {
		t.Fatalf("load revisions: %v", err)
	}

	splitPreview := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/split", token, "req-split-preview", entitySplitRequest{EntityID: sourceID, ObservationID: observationID, Preview: true, ExpectedSourceRevision: intPtr(currentSourceRev), ExpectedTargetRevision: intPtr(currentTargetRev)})
	defer splitPreview.Body.Close()
	if splitPreview.StatusCode != http.StatusOK {
		t.Fatalf("split preview status = %d, want %d", splitPreview.StatusCode, http.StatusOK)
	}
	var splitPreviewResp entityMutationResponse
	decodeHTTPResponse(t, splitPreview, &splitPreviewResp)
	if !splitPreviewResp.Preview || splitPreviewResp.Source.MergedIntoEntityID != nil {
		t.Fatalf("split preview response = %#v", splitPreviewResp)
	}

	split := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/split", token, "req-split", entitySplitRequest{EntityID: sourceID, ObservationID: observationID, ExpectedSourceRevision: intPtr(currentSourceRev), ExpectedTargetRevision: intPtr(currentTargetRev)})
	defer split.Body.Close()
	if split.StatusCode != http.StatusOK {
		t.Fatalf("split status = %d, want %d", split.StatusCode, http.StatusOK)
	}
	var splitResp entityMutationResponse
	decodeHTTPResponse(t, split, &splitResp)
	if !splitResp.Applied || splitResp.Source.MergedIntoEntityID != nil {
		t.Fatalf("split response = %#v", splitResp)
	}

	if err := db.QueryRowContext(ctx, `SELECT merged_into_entity_id::text FROM entity WHERE id = $1`, sourceID).Scan(&mergedInto); err != nil {
		t.Fatalf("load split state: %v", err)
	}
	if mergedInto.Valid {
		t.Fatalf("split did not clear merge lineage")
	}

	if err := db.QueryRowContext(ctx, `SELECT type, actor_kind, actor_handle, actor_role, request_id, data::text FROM audit_event WHERE subject_id = $1 ORDER BY id DESC LIMIT 1`, sourceID).Scan(&typ, &actorKind, &actorHandle, &actorRole, &requestID, &data); err != nil {
		t.Fatalf("load split audit event: %v", err)
	}
	if typ != "entity.split" || requestID != "req-split" {
		t.Fatalf("split audit event = %s %s", typ, requestID)
	}
	if !bytes.Contains([]byte(data), []byte(`"observationId":"`+observationID+`"`)) {
		t.Fatalf("split audit data = %s", data)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin undo provenance tx: %v", err)
	}
	resolvedID, err = upsertEntity(ctx, tx, engagementID, "host", "fqdn", "alpha.local", map[string]any{"role": "source", "state": "restored"})
	if err != nil {
		t.Fatalf("upsert restored entity: %v", err)
	}
	if resolvedID != sourceID {
		t.Fatalf("restored source routed to %q, want %q", resolvedID, sourceID)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit undo provenance tx: %v", err)
	}

	if err := db.QueryRowContext(ctx, `SELECT entity_id FROM observation WHERE id = $1`, observationID).Scan(&resolvedID); err != nil {
		t.Fatalf("load provenance observation: %v", err)
	}
	if resolvedID != sourceID {
		t.Fatalf("observation entity = %q, want %q", resolvedID, sourceID)
	}
}

func TestEntityMergeConflictIsOptimisticUnderConcurrency(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	actorID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	token := "entity-concurrency-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	sourceID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	targetID := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'gamma.local')`, sourceID, engagementID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'delta.local')`, targetID, engagementID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	var wg sync.WaitGroup
	errCh := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "req-concurrent", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID, ExpectedSourceRevision: intPtr(1), ExpectedTargetRevision: intPtr(1)})
			defer resp.Body.Close()
			errCh <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(errCh)

	var okCount, conflictCount int
	for code := range errCh {
		switch code {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected merge status = %d", code)
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("merge concurrency results = ok:%d conflict:%d", okCount, conflictCount)
	}
}

func decodeHTTPResponse(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func doEntityMutationRequest(t *testing.T, client *http.Client, url, token, requestID string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal entity request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do entity request: %v", err)
	}
	return resp
}

func seedObservation(t *testing.T, db *sql.DB, engagementID, actorID, entityID, fqdn string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stdoutID, stderrID, actionID, resultID, observationID string
	if err := db.QueryRowContext(ctx, `INSERT INTO evidence (engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, 'stdout', repeat('1', 64), 0, 'text/plain', 'stdout/seed') RETURNING id`, engagementID).Scan(&stdoutID); err != nil {
		t.Fatalf("seed stdout evidence: %v", err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO evidence (engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, 'stderr', repeat('2', 64), 0, 'text/plain', 'stderr/seed') RETURNING id`, engagementID).Scan(&stderrID); err != nil {
		t.Fatalf("seed stderr evidence: %v", err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO action (engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, gen_random_uuid(), 'manual', 'recon', 'whoami', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', $3, now(), now(), 0, $4, $5, 'raw') RETURNING id`, engagementID, actorID, fqdn, stdoutID, stderrID).Scan(&actionID); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO result (engagement_id, action_id, plugin_id, schema_id, schema_version, extracted) VALUES ($1, $2, 'plugin.demo', 'https://schemas.waypoint.security/demo', '1.0.0', '{}'::jsonb) RETURNING id`, engagementID, actionID).Scan(&resultID); err != nil {
		t.Fatalf("seed result: %v", err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO observation (engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes) VALUES ($1, $2, $3, $4, 'host', jsonb_build_array(jsonb_build_object('type', 'fqdn', 'value', $5)), '{"note":"seed"}'::jsonb) RETURNING id`, engagementID, actionID, resultID, entityID, fqdn).Scan(&observationID); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	return observationID
}

func intPtr(v int) *int { return &v }

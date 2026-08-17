package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
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

	var resolvedID string
	if err := db.QueryRowContext(ctx, `SELECT entity_id FROM observation WHERE id = $1`, observationID).Scan(&resolvedID); err != nil {
		t.Fatalf("load merged provenance observation: %v", err)
	}
	if resolvedID != sourceID {
		t.Fatalf("merged observation entity = %q, want %q", resolvedID, sourceID)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin provenance tx: %v", err)
	}
	resolvedID, err = upsertEntity(ctx, tx, engagementID, "host", "fqdn", "alpha.local", map[string]any{"role": "source", "state": "updated"})
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
	if typ != "entity.split" || actorKind != "human" || actorHandle != "alex.operator" || actorRole != "operator" || requestID != "req-split" {
		t.Fatalf("split audit event = %s %s %s %s %s", typ, actorKind, actorHandle, actorRole, requestID)
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

func TestEntityIdentityNormalizationConflictAndConcurrentDeduplication(t *testing.T) {
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
	token := "entity-dedup-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	for _, tc := range []struct {
		name    string
		ids     []captureEntityIdentifier
		wantKey string
		wantVal string
	}{
		{name: "sid precedence", ids: []captureEntityIdentifier{{Type: "hostname", Value: "workstation"}, {Type: "ad_sid", Value: "  S-1-5-21-123  "}, {Type: "mac", Value: "00:11:22:33:44:55"}}, wantKey: "ad_sid", wantVal: "S-1-5-21-123"},
		{name: "mac normalization", ids: []captureEntityIdentifier{{Type: "mac", Value: "00-11-22-33-44-55"}}, wantKey: "mac", wantVal: "00:11:22:33:44:55"},
		{name: "fqdn precedence", ids: []captureEntityIdentifier{{Type: "hostname", Value: "Workstation.EXAMPLE.LOCAL."}, {Type: "ip", Value: "10.0.0.7"}, {Type: "fqdn", Value: "  App.EXAMPLE.local. "}}, wantKey: "fqdn", wantVal: "app.example.local"},
		{name: "hostname ip fallback", ids: []captureEntityIdentifier{{Type: "hostname", Value: "Host01.EXAMPLE.LOCAL."}, {Type: "ip", Value: "10.0.0.7"}}, wantKey: "hostname_ip", wantVal: "hostname=host01.example.local|ip=10.0.0.7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotVal, ok := entityIdentity(tc.ids)
			if !ok || gotKey != tc.wantKey || gotVal != tc.wantVal {
				t.Fatalf("entityIdentity() = %q %q %v, want %q %q true", gotKey, gotVal, ok, tc.wantKey, tc.wantVal)
			}
		})
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin conflict tx: %v", err)
	}
	if _, err := upsertEntity(ctx, tx, engagementID, "host", "fqdn", "conflict.local", map[string]any{"stage": "seed"}); err != nil {
		t.Fatalf("seed conflict entity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit conflict seed: %v", err)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin conflicting tx: %v", err)
	}
	if _, err := upsertEntity(ctx, tx, engagementID, "service", "fqdn", "conflict.local", map[string]any{"stage": "conflict"}); err != errEntityKindConflict {
		t.Fatalf("conflicting upsert error = %v, want %v", err, errEntityKindConflict)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback conflicting tx: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback()
			<-start
			id, err := upsertEntity(ctx, tx, engagementID, "host", "fqdn", "dedup.local", map[string]any{"worker": worker})
			if err != nil {
				errs <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errs <- err
				return
			}
			ids <- id
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent upsert error: %v", err)
		}
	}
	var gotIDs []string
	for id := range ids {
		gotIDs = append(gotIDs, id)
	}
	if len(gotIDs) != 2 || gotIDs[0] != gotIDs[1] {
		t.Fatalf("concurrent upsert ids = %v, want the same entity twice", gotIDs)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity WHERE engagement_id = $1 AND key_type = 'fqdn' AND key_value = 'dedup.local'`, engagementID).Scan(&count); err != nil {
		t.Fatalf("count dedup entity: %v", err)
	}
	if count != 1 {
		t.Fatalf("dedup entity count = %d, want 1", count)
	}
}

func TestEntityReadProvenanceTracksCanonicalLineageOnRealPostgreSQL(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "99999999-9999-4999-8999-999999999999"
	actorID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	token := "entity-read-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	sourceID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'alpha.local', '{"role":"source"}'::jsonb)`, sourceID, engagementID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'beta.local', '{"role":"target"}'::jsonb)`, targetID, engagementID)
	observationID := seedObservation(t, db, engagementID, actorID, sourceID, "alpha.local")

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	before := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID, token, "req-entity-before")
	defer before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("entity before merge status = %d", before.StatusCode)
	}
	var beforeResp entityReadResponse
	decodeHTTPResponse(t, before, &beforeResp)
	if beforeResp.ID != sourceID || len(beforeResp.Observations) != 1 || beforeResp.Observations[0].ID != observationID {
		t.Fatalf("entity before merge = %#v", beforeResp)
	}

	merged := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "req-entity-merge", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID})
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d", merged.StatusCode)
	}

	afterMerge := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID, token, "req-entity-after-merge")
	defer afterMerge.Body.Close()
	if afterMerge.StatusCode != http.StatusOK {
		t.Fatalf("entity after merge status = %d", afterMerge.StatusCode)
	}
	var afterMergeResp entityReadResponse
	decodeHTTPResponse(t, afterMerge, &afterMergeResp)
	if afterMergeResp.ID != targetID || len(afterMergeResp.Observations) != 1 || afterMergeResp.Observations[0].ID != observationID {
		t.Fatalf("entity after merge = %#v", afterMergeResp)
	}

	page := doEntityPageRequest(t, ts.Client(), ts.URL+"/api/v1/entities?limit=10", token, "req-entity-page")
	if page.Page.HasMore || len(page.Items) != 1 || page.Items[0].ID != targetID {
		t.Fatalf("entity page after merge = %#v", page)
	}

	var currentSourceRev, currentTargetRev int
	if err := db.QueryRowContext(ctx, `SELECT s.revision, t.revision FROM entity s, entity t WHERE s.id = $1 AND t.id = $2`, sourceID, targetID).Scan(&currentSourceRev, &currentTargetRev); err != nil {
		t.Fatalf("load revisions: %v", err)
	}
	resplit := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/split", token, "req-entity-split", entitySplitRequest{EntityID: sourceID, ObservationID: observationID, ExpectedSourceRevision: intPtr(currentSourceRev), ExpectedTargetRevision: intPtr(currentTargetRev)})
	defer resplit.Body.Close()
	if resplit.StatusCode != http.StatusOK {
		t.Fatalf("split status = %d", resplit.StatusCode)
	}

	afterSplit := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID, token, "req-entity-after-split")
	defer afterSplit.Body.Close()
	if afterSplit.StatusCode != http.StatusOK {
		t.Fatalf("entity after split status = %d", afterSplit.StatusCode)
	}
	var afterSplitResp entityReadResponse
	decodeHTTPResponse(t, afterSplit, &afterSplitResp)
	if afterSplitResp.ID != sourceID || len(afterSplitResp.Observations) != 1 || afterSplitResp.Observations[0].ID != observationID {
		t.Fatalf("entity after split = %#v", afterSplitResp)
	}

	page = doEntityPageRequest(t, ts.Client(), ts.URL+"/api/v1/entities?limit=10", token, "req-entity-page-after-split")
	if page.Page.HasMore || len(page.Items) != 2 {
		t.Fatalf("entity page after split = %#v", page)
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

func doEntityReadRequest(t *testing.T, client *http.Client, url, token, requestID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new read request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do read request: %v", err)
	}
	return resp
}

func doEntityPageRequest(t *testing.T, client *http.Client, url, token, requestID string) entityPageResponse {
	t.Helper()
	resp := doEntityReadRequest(t, client, url, token, requestID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("entity page status = %d, body=%s", resp.StatusCode, body)
	}
	var page entityPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode entity page: %v", err)
	}
	return page
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

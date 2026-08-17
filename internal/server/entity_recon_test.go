package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestReconReadApisAreKeysetPaginatedAndEngagementIsolated(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementA := "11111111-1111-4111-8111-111111111111"
	engagementB := "22222222-2222-4222-8222-222222222222"
	actorA := "33333333-3333-4333-8333-333333333333"
	actorB := "44444444-4444-4444-8444-444444444444"
	tokenA := "recon-read-token-a"
	tokenB := "recon-read-token-b"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo A', 'Client', 'Scope', 'active')`, engagementA)
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo B', 'Client', 'Scope', 'active')`, engagementB)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.a', $3, 'operator')`, actorA, engagementA, hashHex(tokenA))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.b', $3, 'operator')`, actorB, engagementB, hashHex(tokenB))

	alphaID := "55555555-5555-4555-8555-555555555551"
	betaID := "55555555-5555-4555-8555-555555555552"
	gammaID := "55555555-5555-4555-8555-555555555553"
	deltaID := "55555555-5555-4555-8555-555555555554"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, first_seen, last_seen) VALUES ($1, $2, 'host', 'fqdn', 'alpha.local', $3, $3)`, alphaID, engagementA, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC))
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, first_seen, last_seen) VALUES ($1, $2, 'host', 'hostname_ip', 'hostname=beta-host|ip=10.0.0.5', $3, $3)`, betaID, engagementA, time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC))
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, first_seen, last_seen) VALUES ($1, $2, 'host', 'fqdn', 'gamma.local', $3, $3)`, gammaID, engagementA, time.Date(2024, 1, 1, 0, 0, 3, 0, time.UTC))
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, first_seen, last_seen) VALUES ($1, $2, 'host', 'fqdn', 'delta.other', $3, $3)`, deltaID, engagementB, time.Date(2024, 1, 1, 0, 0, 4, 0, time.UTC))

	alphaObs1 := seedObservationAt(t, db, engagementA, actorA, alphaID, "alpha.local", time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC))
	alphaObs2 := seedObservationAt(t, db, engagementA, actorA, alphaID, "alpha-alt.local", time.Date(2024, 1, 1, 0, 2, 0, 0, time.UTC))
	_ = seedObservationAt(t, db, engagementA, actorA, betaID, "beta-host.local", time.Date(2024, 1, 1, 0, 3, 0, 0, time.UTC))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	page := doEntityPageRequest(t, ts.Client(), ts.URL+"/api/v1/entities?limit=2", tokenA, "req-recon-entities-1")
	if len(page.Items) != 2 || !page.Page.HasMore || page.Page.NextCursor == "" {
		t.Fatalf("entity page 1 = %#v", page)
	}
	if page.Items[0].ID != alphaID || page.Items[1].ID != betaID {
		t.Fatalf("entity page 1 ordering = %#v", page.Items)
	}
	page2 := doEntityPageRequest(t, ts.Client(), ts.URL+"/api/v1/entities?limit=2&after="+page.Page.NextCursor, tokenA, "req-recon-entities-2")
	if len(page2.Items) != 1 || page2.Page.HasMore || page2.Items[0].ID != gammaID {
		t.Fatalf("entity page 2 = %#v", page2)
	}

	isolated := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+deltaID, tokenA, "req-recon-isolation")
	defer isolated.Body.Close()
	if isolated.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-engagement entity status = %d, want %d", isolated.StatusCode, http.StatusNotFound)
	}

	obsResp := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+alphaID+"/observations?limit=1", tokenA, "req-recon-observations")
	defer obsResp.Body.Close()
	if obsResp.StatusCode != http.StatusOK {
		t.Fatalf("observation page status = %d", obsResp.StatusCode)
	}
	var obsPage entityObservationPageResponse
	decodeHTTPResponse(t, obsResp, &obsPage)
	if len(obsPage.Items) != 1 || !obsPage.Page.HasMore || obsPage.Page.NextCursor == "" || obsPage.Items[0].ID != alphaObs1 {
		t.Fatalf("observation page 1 = %#v", obsPage)
	}
	obsResp2 := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+alphaID+"/observations?limit=1&after="+obsPage.Page.NextCursor, tokenA, "req-recon-observations-2")
	defer obsResp2.Body.Close()
	var obsPage2 entityObservationPageResponse
	decodeHTTPResponse(t, obsResp2, &obsPage2)
	if len(obsPage2.Items) != 1 || obsPage2.Page.HasMore || obsPage2.Items[0].ID != alphaObs2 {
		t.Fatalf("observation page 2 = %#v", obsPage2)
	}

	idsResp := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+betaID+"/identifiers?limit=10", tokenA, "req-recon-identifiers")
	defer idsResp.Body.Close()
	if idsResp.StatusCode != http.StatusOK {
		t.Fatalf("identifier page status = %d", idsResp.StatusCode)
	}
	var idsPage entityIdentifierPageResponse
	decodeHTTPResponse(t, idsResp, &idsPage)
	gotTypes := make([]string, 0, len(idsPage.Items))
	for _, item := range idsPage.Items {
		gotTypes = append(gotTypes, item.Type)
	}
	sort.Strings(gotTypes)
	wantTypes := []string{"fqdn", "hostname", "ip"}
	if len(gotTypes) != len(wantTypes) {
		t.Fatalf("identifier types = %v, want %v", gotTypes, wantTypes)
	}
	for i := range wantTypes {
		if gotTypes[i] != wantTypes[i] {
			t.Fatalf("identifier types = %v, want %v", gotTypes, wantTypes)
		}
	}

	merged := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", tokenA, "req-recon-merge", entityMergeRequest{SourceEntityID: alphaID, TargetEntityID: gammaID})
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d", merged.StatusCode)
	}

	lineageResp := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+alphaID+"/lineage?limit=1", tokenA, "req-recon-lineage")
	defer lineageResp.Body.Close()
	if lineageResp.StatusCode != http.StatusOK {
		t.Fatalf("lineage page status = %d", lineageResp.StatusCode)
	}
	var lineagePage entityLineagePageResponse
	decodeHTTPResponse(t, lineageResp, &lineagePage)
	if len(lineagePage.Items) != 1 || !lineagePage.Page.HasMore || lineagePage.Page.NextCursor == "" {
		t.Fatalf("lineage page 1 = %#v", lineagePage)
	}
	lineageResp2 := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+alphaID+"/lineage?limit=1&after="+lineagePage.Page.NextCursor, tokenA, "req-recon-lineage-2")
	defer lineageResp2.Body.Close()
	var lineagePage2 entityLineagePageResponse
	decodeHTTPResponse(t, lineageResp2, &lineagePage2)
	if len(lineagePage2.Items) != 1 || lineagePage2.Page.HasMore {
		t.Fatalf("lineage page 2 = %#v", lineagePage2)
	}
}

func TestReconPreviewAndSplitProvenanceReadsFollowCanonicalLineage(t *testing.T) {
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
	token := "recon-preview-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	sourceID := "99999999-9999-4999-8999-999999999991"
	targetID := "99999999-9999-4999-8999-999999999992"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'alpha.local')`, sourceID, engagementID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'beta.local')`, targetID, engagementID)
	observationID := seedObservationAt(t, db, engagementID, actorID, sourceID, "alpha.local", time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	mergePreview := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID+"/merge-preview?targetEntityId="+targetID, token, "req-recon-merge-preview")
	defer mergePreview.Body.Close()
	if mergePreview.StatusCode != http.StatusOK {
		t.Fatalf("merge preview status = %d", mergePreview.StatusCode)
	}
	var previewResp entityMutationResponse
	decodeHTTPResponse(t, mergePreview, &previewResp)
	if !previewResp.Preview || previewResp.Applied || previewResp.Source.MergedIntoEntityID == nil || *previewResp.Source.MergedIntoEntityID != targetID {
		t.Fatalf("merge preview response = %#v", previewResp)
	}

	merged := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "req-recon-merge", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID})
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d", merged.StatusCode)
	}

	splitPreview := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID+"/split-provenance?observationId="+observationID, token, "req-recon-split-provenance")
	defer splitPreview.Body.Close()
	if splitPreview.StatusCode != http.StatusOK {
		t.Fatalf("split provenance status = %d", splitPreview.StatusCode)
	}
	var splitResp entityMutationResponse
	decodeHTTPResponse(t, splitPreview, &splitResp)
	if !splitResp.Preview || splitResp.Applied || splitResp.ObservationID != observationID || splitResp.Source.MergedIntoEntityID != nil {
		t.Fatalf("split provenance response = %#v", splitResp)
	}

	obsResp := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID+"/observations?limit=10", token, "req-recon-observations-canonical")
	defer obsResp.Body.Close()
	var obsPage entityObservationPageResponse
	decodeHTTPResponse(t, obsResp, &obsPage)
	if len(obsPage.Items) != 1 || obsPage.Items[0].ID != observationID || obsPage.Items[0].EntityID != sourceID {
		t.Fatalf("canonical observation page = %#v", obsPage)
	}

	idsResp := doEntityReadRequest(t, ts.Client(), ts.URL+"/api/v1/entities/"+sourceID+"/identifiers?limit=10", token, "req-recon-identifiers-canonical")
	defer idsResp.Body.Close()
	var idsPage entityIdentifierPageResponse
	decodeHTTPResponse(t, idsResp, &idsPage)
	got := make([]string, 0, len(idsPage.Items))
	for _, item := range idsPage.Items {
		got = append(got, item.Type+":"+item.Value)
	}
	want := []string{"fqdn:alpha.local", "fqdn:beta.local"}
	if len(got) != len(want) {
		t.Fatalf("canonical identifiers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("canonical identifiers = %v, want %v", got, want)
		}
	}
}

func TestReconReadQueryShapesRemainKeysetBounded(t *testing.T) {
	entitySource, err := os.ReadFile("entity_read.go")
	if err != nil {
		t.Fatalf("read entity source: %v", err)
	}
	reconSource, err := os.ReadFile("entity_recon.go")
	if err != nil {
		t.Fatalf("read recon source: %v", err)
	}
	for _, fragment := range []string{
		"AND ($2::timestamptz IS NULL OR (first_seen, id) > ($2, $3))",
		"ORDER BY first_seen ASC, id ASC",
		"LIMIT $4",
	} {
		if !containsString(string(entitySource), fragment) {
			t.Fatalf("entity source missing %q", fragment)
		}
	}
	if containsString(string(entitySource), "OFFSET") {
		t.Fatal("entity pagination unexpectedly uses OFFSET")
	}
	for _, fragment := range []string{
		"ORDER BY o.observed_at ASC, o.id ASC",
		"ORDER BY type ASC, value ASC",
		"ORDER BY first_seen ASC, id ASC",
	} {
		if !containsString(string(reconSource), fragment) {
			t.Fatalf("recon source missing %q", fragment)
		}
	}
	if containsString(string(reconSource), "OFFSET") {
		t.Fatal("recon pagination unexpectedly uses OFFSET")
	}
}

func seedObservationAt(t *testing.T, db *sql.DB, engagementID, actorID, entityID, fqdn string, observedAt time.Time) string {
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
	if err := db.QueryRowContext(ctx, `INSERT INTO observation (engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes, observed_at) VALUES ($1, $2, $3, $4, 'host', jsonb_build_array(jsonb_build_object('type', 'fqdn', 'value', $5)), '{"note":"seed"}'::jsonb, $6) RETURNING id`, engagementID, actionID, resultID, entityID, fqdn, observedAt).Scan(&observationID); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	return observationID
}

func containsString(s, fragment string) bool { return strings.Contains(s, fragment) }

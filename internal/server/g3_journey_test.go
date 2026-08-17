package server

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestG3AuthoritativeProvenanceJourney(t *testing.T) {
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
	aiActorID := "88888888-8888-4888-8888-888888888889"
	token := "g3-provenance-token"
	aiToken := "g3-provenance-ai-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Waypoint', 'gpt-4.1', '1.0', $4)`, aiActorID, engagementID, hashHex(aiToken), actorID)

	sourceID := "99999999-9999-4999-8999-999999999991"
	targetID := "99999999-9999-4999-8999-999999999992"
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'alpha.local', '{"role":"source"}'::jsonb)`, sourceID, engagementID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value, attributes) VALUES ($1, $2, 'host', 'fqdn', 'beta.local', '{"role":"target"}'::jsonb)`, targetID, engagementID)
	observationID := seedObservation(t, db, engagementID, actorID, sourceID, "alpha.local")

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	reconEnvelope := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "10.0.0.0/24")
	reconEnvelope["phase"] = "recon"
	recon := doCaptureRequest(t, ts.Config.Handler, token, "g3-recon", reconEnvelope, []byte("stdout"), []byte("stderr"))
	if recon.Code != http.StatusCreated {
		t.Fatalf("recon capture status = %d, want %d", recon.Code, http.StatusCreated)
	}
	var reconAck captureAckResponse
	decodeResponse(t, recon, &reconAck)
	if reconAck.ActionID == "" || reconAck.CaptureID == "" || reconAck.AuditEventCursor == "" {
		t.Fatalf("recon ack missing provenance: %#v", reconAck)
	}

	attackEnvelope := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "10.0.0.0/24")
	attackEnvelope["phase"] = "attacks"
	attack := doCaptureRequest(t, ts.Config.Handler, token, "g3-attack", attackEnvelope, []byte("stdout"), []byte("stderr"))
	if attack.Code != http.StatusCreated {
		t.Fatalf("attack capture status = %d, want %d", attack.Code, http.StatusCreated)
	}
	var attackAck captureAckResponse
	decodeResponse(t, attack, &attackAck)
	if attackAck.ActionID == "" || attackAck.CaptureID == "" || attackAck.AuditEventCursor == "" {
		t.Fatalf("attack ack missing provenance: %#v", attackAck)
	}

	assertAlertCounts(t, db, engagementID, 1, 1)

	freshEnvelope := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3", "10.0.1.0/24")
	freshEnvelope["phase"] = "attacks"
	fresh := doCaptureRequest(t, ts.Config.Handler, token, "g3-attack-fresh", freshEnvelope, []byte("stdout"), []byte("stderr"))
	if fresh.Code != http.StatusCreated {
		t.Fatalf("fresh attack capture status = %d, want %d", fresh.Code, http.StatusCreated)
	}
	var freshAck captureAckResponse
	decodeResponse(t, fresh, &freshAck)
	assertAlertCounts(t, db, engagementID, 1, 2)

	attemptedEnvelope := cloneMap(notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa4", "10.0.2.0/24"))
	attemptedEnvelope["phase"] = "attacks"
	attemptedEnvelope["initiatedBy"] = "ai"
	attemptedEnvelope["decisionContext"] = map[string]any{"rationale": "Validate the current share from the AI-assisted path before promotion.", "promptReference": "g3-ai-provenance"}
	attemptedEnvelope["command"] = "/opt/tools/mystery-scan.exe"
	attemptedEnvelope["argv"] = []any{"mystery-scan.exe", "--target", "fileserver.local"}
	attemptedEnvelope["target"] = map[string]any{"kind": "host", "value": "fileserver.local"}
	attemptedParsing := cloneMap(attemptedEnvelope["parsing"].(map[string]any))
	delete(attemptedParsing, "result")
	delete(attemptedParsing, "plugin")
	attemptedParsing["status"] = "needs-plugin"
	attemptedEnvelope["parsing"] = attemptedParsing
	attempted := doCaptureRequest(t, ts.Config.Handler, aiToken, "g3-ai-attempt", attemptedEnvelope, []byte("stdout"), []byte("stderr"))
	if attempted.Code != http.StatusCreated {
		t.Fatalf("ai attempted capture status = %d, want %d", attempted.Code, http.StatusCreated)
	}
	var attemptedAck captureAckResponse
	decodeResponse(t, attempted, &attemptedAck)
	if attemptedAck.ActionID == "" || attemptedAck.CaptureID == "" || attemptedAck.AuditEventCursor == "" {
		t.Fatalf("ai attempt ack missing provenance: %#v", attemptedAck)
	}
	assertActionSnapshot(t, ctx, db, attemptedAck.ActionID, aiActorID, "ai_agent", "ai", "needs-plugin", "", true)
	assertSingleResultAndObservation(t, ctx, db, attemptedAck.ActionID, 0, 0)
	var aiActorKind, aiActorHandle, aiAuthorizedBy string
	if err := db.QueryRowContext(ctx, `SELECT actor_kind, actor_handle, COALESCE(actor_authorized_by::text, '') FROM audit_event WHERE subject_type = 'action' AND subject_id = $1 ORDER BY id ASC LIMIT 1`, attemptedAck.ActionID).Scan(&aiActorKind, &aiActorHandle, &aiAuthorizedBy); err != nil {
		t.Fatalf("load ai attempt audit provenance: %v", err)
	}
	if aiActorKind != "ai_agent" || aiActorHandle != "field-agent-7" || aiAuthorizedBy != actorID {
		t.Fatalf("ai attempt audit provenance = kind:%q handle:%q authorizedBy:%q", aiActorKind, aiActorHandle, aiAuthorizedBy)
	}

	page := decodeAuditPage(t, doAuditRequest(t, ts.Client(), ts.URL+"/api/v1/audit-events?limit=10", token, "g3-page", "", ""))
	if len(page.Items) < 5 {
		t.Fatalf("audit page items = %d, want at least 5", len(page.Items))
	}
	wantTypes := map[string]bool{
		"capture.accepted": false,
		"alert.notable":    false,
	}
	for _, item := range page.Items {
		if item.EngagementID != engagementID {
			t.Fatalf("audit page leaked engagement %q, want only %q", item.EngagementID, engagementID)
		}
		if _, ok := wantTypes[item.Type]; ok {
			wantTypes[item.Type] = true
		}
	}
	for typ, seen := range wantTypes {
		if !seen {
			t.Fatalf("audit page missing %s event", typ)
		}
	}
	var sawReconPhase, sawAttacksPhase bool
	for _, item := range page.Items {
		if bytes.Contains(item.Data, []byte(`"phase":"recon"`)) {
			sawReconPhase = true
		}
		if bytes.Contains(item.Data, []byte(`"phase":"attacks"`)) {
			sawAttacksPhase = true
		}
	}
	if !sawReconPhase || !sawAttacksPhase {
		t.Fatalf("journey log data lost captured phases: recon=%v attacks=%v", sawReconPhase, sawAttacksPhase)
	}

	findingsBeforeResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings?status=open", token, "g3-findings-before", http.MethodGet, nil)
	defer findingsBeforeResp.Body.Close()
	if findingsBeforeResp.StatusCode != http.StatusOK {
		t.Fatalf("findings before promotion status = %d, want %d", findingsBeforeResp.StatusCode, http.StatusOK)
	}
	var findingsBefore findingListResponse
	decodeHTTPResponse(t, findingsBeforeResp, &findingsBefore)
	if len(findingsBefore.Items) != 0 {
		t.Fatalf("findings before promotion = %#v, want empty", findingsBefore)
	}

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events?after="+reconAck.AuditEventCursor, token, "g3-sse", "", reconAck.AuditEventCursor)
	frame := readSSEFrame(t, sseResp.Body)
	_ = sseResp.Body.Close()
	if frame["event"] != "alert.notable" {
		t.Fatalf("sse event = %q, want alert.notable", frame["event"])
	}
	if !strings.Contains(frame["data"], `"sourceCaptureId":"`+reconAck.CaptureID+`"`) || !strings.Contains(frame["data"], `"sourceActionId":"`+reconAck.ActionID+`"`) {
		t.Fatalf("alert SSE lost source provenance: %s", frame["data"])
	}

	var currentSourceRev, currentTargetRev int
	if err := db.QueryRowContext(ctx, `SELECT s.revision, t.revision FROM entity s, entity t WHERE s.id = $1 AND t.id = $2`, sourceID, targetID).Scan(&currentSourceRev, &currentTargetRev); err != nil {
		t.Fatalf("load revisions: %v", err)
	}

	mergePreview := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "g3-merge-preview", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID, Preview: true})
	defer mergePreview.Body.Close()
	if mergePreview.StatusCode != http.StatusOK {
		t.Fatalf("merge preview status = %d, want %d", mergePreview.StatusCode, http.StatusOK)
	}
	var mergePreviewResp entityMutationResponse
	decodeHTTPResponse(t, mergePreview, &mergePreviewResp)
	if !mergePreviewResp.Preview || mergePreviewResp.Applied || mergePreviewResp.Source.MergedIntoEntityID == nil || *mergePreviewResp.Source.MergedIntoEntityID != targetID {
		t.Fatalf("merge preview response = %#v", mergePreviewResp)
	}

	merged := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/merge", token, "g3-merge", entityMergeRequest{SourceEntityID: sourceID, TargetEntityID: targetID})
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d, want %d", merged.StatusCode, http.StatusOK)
	}
	var mergedResp entityMutationResponse
	decodeHTTPResponse(t, merged, &mergedResp)
	if !mergedResp.Applied || mergedResp.Source.MergedIntoEntityID == nil || *mergedResp.Source.MergedIntoEntityID != targetID {
		t.Fatalf("merge response = %#v", mergedResp)
	}

	var mergedInto sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT merged_into_entity_id::text FROM entity WHERE id = $1`, sourceID).Scan(&mergedInto); err != nil {
		t.Fatalf("load merge state: %v", err)
	}
	if !mergedInto.Valid || mergedInto.String != targetID {
		t.Fatalf("merged entity lineage = %#v, want %q", mergedInto, targetID)
	}

	splitPreview := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/split", token, "g3-split-preview", entitySplitRequest{EntityID: sourceID, ObservationID: observationID, Preview: true, ExpectedSourceRevision: intPtr(currentSourceRev), ExpectedTargetRevision: intPtr(currentTargetRev)})
	defer splitPreview.Body.Close()
	if splitPreview.StatusCode != http.StatusOK {
		t.Fatalf("split preview status = %d, want %d", splitPreview.StatusCode, http.StatusOK)
	}
	var splitPreviewResp entityMutationResponse
	decodeHTTPResponse(t, splitPreview, &splitPreviewResp)
	if !splitPreviewResp.Preview || splitPreviewResp.Applied || splitPreviewResp.Source.MergedIntoEntityID != nil {
		t.Fatalf("split preview response = %#v", splitPreviewResp)
	}

	split := doEntityMutationRequest(t, ts.Client(), ts.URL+"/api/v1/entities/split", token, "g3-split", entitySplitRequest{EntityID: sourceID, ObservationID: observationID, ExpectedSourceRevision: intPtr(currentSourceRev), ExpectedTargetRevision: intPtr(currentTargetRev)})
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
		t.Fatalf("split did not clear merge lineage: %#v", mergedInto)
	}

	promoteReq := map[string]any{"sourceActionId": attackAck.ActionID, "title": "Anonymous shares exposed", "severity": "high", "remediation": "Restrict share access and review group membership.", "status": "open"}
	promoteResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/promote", token, "g3-finding-promote", http.MethodPost, promoteReq)
	defer promoteResp.Body.Close()
	if promoteResp.StatusCode != http.StatusCreated {
		t.Fatalf("finding promote status = %d, want %d", promoteResp.StatusCode, http.StatusCreated)
	}
	var promoted findingItem
	decodeHTTPResponse(t, promoteResp, &promoted)
	if promoted.Revision != 1 || promoted.PromotedBy != actorID || promoted.Status != "open" || len(promoted.EvidenceActionIDs) != 1 || promoted.EvidenceActionIDs[0] != attackAck.ActionID {
		t.Fatalf("promoted finding = %#v", promoted)
	}

	findingsOpenResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings?status=open", token, "g3-findings-open", http.MethodGet, nil)
	defer findingsOpenResp.Body.Close()
	if findingsOpenResp.StatusCode != http.StatusOK {
		t.Fatalf("findings open status = %d, want %d", findingsOpenResp.StatusCode, http.StatusOK)
	}
	var findingsOpen findingListResponse
	decodeHTTPResponse(t, findingsOpenResp, &findingsOpen)
	if len(findingsOpen.Items) != 1 || findingsOpen.Items[0].ID != promoted.ID || findingsOpen.Items[0].Revision != 1 || findingsOpen.Items[0].Status != "open" {
		t.Fatalf("findings open list = %#v", findingsOpen)
	}

	updateFindingReq := map[string]any{"expectedRevision": promoted.Revision, "status": "triage", "remediation": "Restrict share access and review group membership immediately."}
	updateFindingResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID, token, "g3-finding-update", http.MethodPatch, updateFindingReq)
	defer updateFindingResp.Body.Close()
	if updateFindingResp.StatusCode != http.StatusOK {
		t.Fatalf("finding update status = %d, want %d", updateFindingResp.StatusCode, http.StatusOK)
	}
	var updatedFinding findingItem
	decodeHTTPResponse(t, updateFindingResp, &updatedFinding)
	if updatedFinding.Revision != 2 || updatedFinding.Status != "triage" {
		t.Fatalf("updated finding = %#v", updatedFinding)
	}

	revisionsResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings/"+promoted.ID+"/revisions", token, "g3-finding-revisions", http.MethodGet, nil)
	defer revisionsResp.Body.Close()
	if revisionsResp.StatusCode != http.StatusOK {
		t.Fatalf("finding revisions status = %d, want %d", revisionsResp.StatusCode, http.StatusOK)
	}
	var findingRevisions findingRevisionsResponse
	decodeHTTPResponse(t, revisionsResp, &findingRevisions)
	if len(findingRevisions.Items) != 2 || findingRevisions.Items[0].Subject.Revision != 1 || findingRevisions.Items[1].Subject.Revision != 2 {
		t.Fatalf("finding revisions = %#v", findingRevisions)
	}
	if findingRevisions.Items[0].Type != "finding.promoted" || findingRevisions.Items[1].Type != "finding.status-changed" {
		t.Fatalf("finding revision event types = %#v", findingRevisions.Items)
	}
	if findingRevisions.Items[0].Actor.Kind != "human" || findingRevisions.Items[1].Actor.Kind != "human" {
		t.Fatalf("finding revision actors = %#v", findingRevisions.Items)
	}

	findingsTriageResp := doFindingRequest(t, ts.Client(), ts.URL+"/api/v1/findings?status=triage", token, "g3-findings-triage", http.MethodGet, nil)
	defer findingsTriageResp.Body.Close()
	if findingsTriageResp.StatusCode != http.StatusOK {
		t.Fatalf("findings triage status = %d, want %d", findingsTriageResp.StatusCode, http.StatusOK)
	}
	var findingsTriage findingListResponse
	decodeHTTPResponse(t, findingsTriageResp, &findingsTriage)
	if len(findingsTriage.Items) != 1 || findingsTriage.Items[0].ID != promoted.ID || findingsTriage.Items[0].Revision != 2 || findingsTriage.Items[0].Status != "triage" {
		t.Fatalf("findings triage list = %#v", findingsTriage)
	}

	page2 := decodeAuditPage(t, doAuditRequest(t, ts.Client(), ts.URL+"/api/v1/audit-events?limit=20", token, "g3-page-2", "", ""))
	seen := map[string]bool{}
	var sawObservationLink bool
	for _, item := range page2.Items {
		seen[item.Type] = true
		if bytes.Contains(item.Data, []byte(`"observationId":"`+observationID+`"`)) {
			sawObservationLink = true
		}
	}
	for _, typ := range []string{"entity.merged", "entity.split"} {
		if !seen[typ] {
			t.Fatalf("audit page missing %s event", typ)
		}
	}
	if !sawObservationLink {
		t.Fatalf("split provenance did not retain observation linkage")
	}

	var observedEntityID string
	if err := db.QueryRowContext(ctx, `SELECT entity_id FROM observation WHERE id = $1`, observationID).Scan(&observedEntityID); err != nil {
		t.Fatalf("load restored observation: %v", err)
	}
	if observedEntityID != "99999999-9999-4999-8999-999999999991" {
		t.Fatalf("observation entity = %q, want restored source entity", observedEntityID)
	}
}

package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

// Traces PRD-AUD-002/003, PRD-ID-001/002, and EV-03.
func TestAttributionHostGateHumanAndAIActorsAcrossProvisionCaptureRotateRevokeAndReport(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	evidenceDir := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceDir)

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	bootstrapID := "22222222-2222-4222-8222-222222222222"
	bootstrapToken := "bootstrap-owner-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.owner', $3, 'owner')`, bootstrapID, engagementID, hashHex(bootstrapToken))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	provision := func(body map[string]any, authToken string) actorCredentialResponse {
		resp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors", authToken, http.MethodPost, body, "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("provision %s status = %d", body["handle"], resp.StatusCode)
		}
		var out actorCredentialResponse
		decodeResponse(t, responseRecorderFromHTTP(resp), &out)
		if out.Token == "" || out.ActorRecord.Actor.ID == "" {
			t.Fatalf("provision %s response = %#v", body["handle"], out)
		}
		var storedHash string
		if err := db.QueryRowContext(ctx, `SELECT token_hash FROM actor WHERE id = $1`, out.ActorRecord.Actor.ID).Scan(&storedHash); err != nil {
			t.Fatalf("load token hash for %s: %v", body["handle"], err)
		}
		if storedHash != hashHex(out.Token) {
			t.Fatalf("stored token hash for %s = %s, want %s", body["handle"], storedHash, hashHex(out.Token))
		}
		return out
	}

	humanOne := provision(map[string]any{"kind": "human", "handle": "beatrice.operator", "role": "operator"}, bootstrapToken)
	humanTwo := provision(map[string]any{"kind": "human", "handle": "carol.operator", "role": "operator"}, bootstrapToken)
	ai := provision(map[string]any{
		"kind":         "ai_agent",
		"handle":       "field-agent-7",
		"role":         "operator",
		"agentName":    "Synthetic Field Agent",
		"model":        "gpt-4.1",
		"version":      "2025.01",
		"authorizedBy": humanOne.ActorRecord.Actor.ID,
	}, humanOne.Token)

	if humanOne.Token == humanTwo.Token || humanOne.Token == ai.Token || humanTwo.Token == ai.Token {
		t.Fatal("expected distinct one-time tokens for every provisioned actor")
	}
	if ai.ActorRecord.Actor.Kind != "ai_agent" || ai.ActorRecord.Actor.AuthorizedBy != humanOne.ActorRecord.Actor.ID || ai.ActorRecord.Actor.Model != "gpt-4.1" || ai.ActorRecord.Actor.Version != "2025.01" {
		t.Fatalf("ai provisioned actor = %#v", ai)
	}

	stdout := []byte("abc")
	stderr := []byte{}
	humanOneEnv := attributionEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "operator_wrapper", "waypoint-wrapper", "linux", "amd64", "manual", nil, "recon", "/usr/bin/whoami", []string{"whoami"}, "/home/operator/engagement", "host", "alpha.local", "10.10.0.12", time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC))
	humanTwoEnv := attributionEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "operator_wrapper", "waypoint-wrapper", "linux", "amd64", "manual", nil, "recon", "/usr/bin/id", []string{"id"}, "/home/operator/engagement", "host", "beta.local", "10.10.0.13", time.Date(2025, 1, 15, 10, 1, 0, 0, time.UTC))
	aiEnv := attributionEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3", "remote_agent", "waypoint-agent", "macos", "arm64", "ai", map[string]any{"rationale": "Confirm the AI-authorized path before promotion.", "promptReference": "host-gate-01"}, "attacks", "/opt/tools/agent-scan", []string{"agent-scan", "--target", "demo.local"}, "/Users/agent/engagement", "host", "gamma.local", "10.10.0.14", time.Date(2025, 1, 15, 10, 2, 0, 0, time.UTC))

	assertCapture := func(token string, envelope map[string]any, wantActorID, wantKind, wantInitiatedBy, wantPluginID string, wantDecisionContext bool) string {
		resp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", token, envelope["captureId"].(string), envelope, stdout, stderr)
		if resp.status != http.StatusCreated {
			t.Fatalf("capture %s status = %d, body=%s", envelope["captureId"], resp.status, resp.body)
		}
		var ack captureAckResponse
		decodeBody(t, resp.body, &ack)
		if ack.Idempotency != "created" || ack.ActionID == "" {
			t.Fatalf("capture %s ack = %#v", envelope["captureId"], ack)
		}
		assertActionSnapshot(t, ctx, db, ack.ActionID, wantActorID, wantKind, wantInitiatedBy, "raw", wantPluginID, wantDecisionContext)
		assertEvidenceLinked(t, ctx, db, evidenceDir, ack.ActionID, string(stdout), string(stderr))
		return ack.ActionID
	}

	humanOneActionID := assertCapture(humanOne.Token, humanOneEnv, humanOne.ActorRecord.Actor.ID, "human", "manual", "", false)
	humanTwoActionID := assertCapture(humanTwo.Token, humanTwoEnv, humanTwo.ActorRecord.Actor.ID, "human", "manual", "", false)

	aiMCPArgs := map[string]any{
		"idempotencyKey": aiEnv["captureId"],
		"envelope":       aiEnv,
		"stdoutBase64":   base64.StdEncoding.EncodeToString(stdout),
		"stderrBase64":   base64.StdEncoding.EncodeToString(stderr),
	}
	aiResp := doLiveMCPRequest(t, ts.Client(), ts.URL+"/mcp", ai.Token, aiEnv["captureId"].(string), map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "waypoint_ingest_capture",
			"arguments": aiMCPArgs,
		},
	})
	if aiResp.status != http.StatusOK {
		t.Fatalf("ai capture status = %d, body=%s", aiResp.status, aiResp.body)
	}
	var aiBody struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Ack struct {
					ActionID    string `json:"actionId"`
					CaptureID   string `json:"captureId"`
					Idempotency string `json:"idempotency"`
				} `json:"ack"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeBody(t, aiResp.body, &aiBody)
	if aiBody.Result.IsError || aiBody.Result.StructuredContent.Ack.ActionID == "" || aiBody.Result.StructuredContent.Ack.Idempotency != "created" {
		t.Fatalf("ai capture response = %#v", aiBody.Result)
	}
	aiActionID := aiBody.Result.StructuredContent.Ack.ActionID
	assertActionSnapshot(t, ctx, db, aiActionID, ai.ActorRecord.Actor.ID, "ai_agent", "ai", "raw", "", true)
	assertEvidenceLinked(t, ctx, db, evidenceDir, aiActionID, string(stdout), string(stderr))

	if humanOneActionID == humanTwoActionID || humanOneActionID == aiActionID || humanTwoActionID == aiActionID {
		t.Fatal("expected distinct action ids for every capture")
	}

	var aiAuditKind, aiAuditHandle, aiAuditAuthorizedBy string
	if err := db.QueryRowContext(ctx, `SELECT actor_kind, actor_handle, COALESCE(actor_authorized_by::text, '') FROM audit_event WHERE subject_type = 'action' AND subject_id = $1 ORDER BY id ASC LIMIT 1`, aiActionID).Scan(&aiAuditKind, &aiAuditHandle, &aiAuditAuthorizedBy); err != nil {
		t.Fatalf("load ai audit provenance: %v", err)
	}
	if aiAuditKind != "ai_agent" || aiAuditHandle != "field-agent-7" || aiAuditAuthorizedBy != humanOne.ActorRecord.Actor.ID {
		t.Fatalf("ai audit provenance = kind:%q handle:%q authorizedBy:%q", aiAuditKind, aiAuditHandle, aiAuditAuthorizedBy)
	}

	restful := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", "definitely-not-a-token", humanOneEnv["captureId"].(string), humanOneEnv, stdout, stderr)
	if restful.status != http.StatusUnauthorized {
		t.Fatalf("anonymous capture status = %d, want %d", restful.status, http.StatusUnauthorized)
	}

	rotateResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors/"+humanTwo.ActorRecord.Actor.ID+"/rotate", bootstrapToken, http.MethodPost, nil, `"1"`)
	defer rotateResp.Body.Close()
	if rotateResp.StatusCode != http.StatusCreated {
		t.Fatalf("rotate status = %d", rotateResp.StatusCode)
	}
	var rotated actorCredentialResponse
	decodeResponse(t, responseRecorderFromHTTP(rotateResp), &rotated)
	if rotated.Token == "" || rotated.Token == humanTwo.Token || rotated.ActorRecord.CredentialVersion != 2 || rotated.ActorRecord.Revision != 2 {
		t.Fatalf("rotated actor = %#v", rotated)
	}

	badOld := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", humanTwo.Token, http.MethodGet, nil, "")
	defer badOld.Body.Close()
	if badOld.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want %d", badOld.StatusCode, http.StatusUnauthorized)
	}

	newTokenOk := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", rotated.Token, http.MethodGet, nil, "")
	defer newTokenOk.Body.Close()
	if newTokenOk.StatusCode != http.StatusOK {
		t.Fatalf("rotated token status = %d, want %d", newTokenOk.StatusCode, http.StatusOK)
	}

	revokeResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors/"+ai.ActorRecord.Actor.ID+"/revoke", humanOne.Token, http.MethodPost, nil, `"1"`)
	defer revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("ai revoke status = %d", revokeResp.StatusCode)
	}
	var revoked actorLifecycleRecord
	decodeResponse(t, responseRecorderFromHTTP(revokeResp), &revoked)
	if revoked.Status != actorStatusRevoked || revoked.RevokedAt == nil || revoked.RevokedBy != humanOne.ActorRecord.Actor.ID || revoked.Revision != 2 {
		t.Fatalf("revoked ai actor = %#v", revoked)
	}

	afterRevoke := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", ai.Token, http.MethodGet, nil, "")
	defer afterRevoke.Body.Close()
	if afterRevoke.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked ai token status = %d, want %d", afterRevoke.StatusCode, http.StatusUnauthorized)
	}

	reportResp, err := ts.Client().Do(mustReportRequest(t, ts.URL+"/api/v1/engagements/"+engagementID+"/summit/report.json", humanOne.Token))
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	defer reportResp.Body.Close()
	if reportResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(reportResp.Body)
		t.Fatalf("report status = %d, body=%s", reportResp.StatusCode, body)
	}
	var snapshot reportSnapshot
	if err := json.NewDecoder(reportResp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if snapshot.Engagement != "Demo" || snapshot.Version != "v1" {
		t.Fatalf("report snapshot = %#v", snapshot)
	}
	if len(snapshot.Evidence) != 3 {
		t.Fatalf("report evidence = %#v", snapshot.Evidence)
	}
	byActor := map[string]reportEvidence{}
	for _, item := range snapshot.Evidence {
		byActor[item.Actor] = item
		if item.Stdout.ByteLength != 3 || item.Stdout.SHA256 != hashHex("abc") || item.Stderr.ByteLength != 0 || item.Stderr.SHA256 != hashHex("") {
			t.Fatalf("evidence fidelity lost for %s: %#v", item.Actor, item)
		}
	}
	if got := byActor["beatrice.operator"]; got.InitiatedBy != "manual" || !strings.Contains(got.SourceAgent, "operator_wrapper") || got.Actor != "beatrice.operator" {
		t.Fatalf("human one projection = %#v", got)
	}
	if got := byActor["carol.operator"]; got.InitiatedBy != "manual" || !strings.Contains(got.SourceAgent, "operator_wrapper") || got.Actor != "carol.operator" {
		t.Fatalf("human two projection = %#v", got)
	}
	aiActorDisplay := "field-agent-7 · model gpt-4.1 · version 2025.01 · authorized by beatrice.operator"
	if got := byActor[aiActorDisplay]; got.InitiatedBy != "ai" || !strings.Contains(got.SourceAgent, "remote_agent") || got.Actor != aiActorDisplay {
		t.Fatalf("ai projection = %#v", got)
	}
	if len(snapshot.Attribution) < 3 {
		t.Fatalf("report attribution = %#v", snapshot.Attribution)
	}
	byTitle := map[string][]string{}
	for _, item := range snapshot.Attribution {
		byTitle[item.Title] = item.Items
	}
	if got := byTitle["Operator"]; len(got) != 2 || got[0] != "beatrice.operator" || got[1] != "carol.operator" {
		t.Fatalf("operator attribution = %#v", got)
	}
	if got := byTitle["AI actor"]; len(got) != 1 || !strings.Contains(got[0], "field-agent-7") || !strings.Contains(got[0], "authorized by beatrice.operator") {
		t.Fatalf("ai attribution = %#v", got)
	}
	if got := byTitle["Exec host IP"]; len(got) != 3 || got[0] != "10.10.0.12/32" || got[1] != "10.10.0.13/32" || got[2] != "10.10.0.14/32" {
		t.Fatalf("exec host attribution = %#v", got)
	}

	t.Run("PRD-ID-001 stored digest is not a bearer credential", func(t *testing.T) {
		digestBearer := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actions?limit=1", hashHex(humanOne.Token), http.MethodGet, nil, "")
		defer digestBearer.Body.Close()
		if digestBearer.StatusCode != http.StatusUnauthorized {
			t.Fatalf("stored digest bearer status = %d, want %d", digestBearer.StatusCode, http.StatusUnauthorized)
		}
	})
}

func attributionEnvelope(captureID, sourceKind, sourceName, sourceOS, sourceArch, initiatedBy string, decisionContext map[string]any, phase, command string, argv []string, cwd, targetKind, targetValue, execHostIP string, startedAt time.Time) map[string]any {
	envelope := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       captureID,
		"sourceAgent": map[string]any{
			"id":      captureID,
			"kind":    sourceKind,
			"name":    sourceName,
			"version": "1.0.0",
			"platform": map[string]any{
				"os":   sourceOS,
				"arch": sourceArch,
			},
		},
		"phase":       phase,
		"initiatedBy": initiatedBy,
		"command":     command,
		"argv":        argv,
		"cwd":         cwd,
		"target": map[string]any{
			"kind":  targetKind,
			"value": targetValue,
		},
		"timing": map[string]any{
			"startedAt":  startedAt.UTC().Format(time.RFC3339Nano),
			"endedAt":    startedAt.UTC().Add(time.Second).Format(time.RFC3339Nano),
			"durationMs": 1000,
		},
		"execution": map[string]any{
			"status":   "exited",
			"exitCode": 0,
		},
		"network": map[string]any{
			"execHost": map[string]any{
				"address":    execHostIP,
				"method":     "route_selection",
				"confidence": "confirmed",
			},
			"egress": map[string]any{
				"mode":   "off",
				"status": "disabled",
			},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{
				"mediaType":  "text/plain; charset=utf-8",
				"byteLength": 3,
				"sha256":     hashHex("abc"),
			},
			"stderr": map[string]any{
				"mediaType":  "text/plain; charset=utf-8",
				"byteLength": 0,
				"sha256":     hashHex(""),
			},
		},
		"parsing": map[string]any{
			"status": "raw",
		},
	}
	if decisionContext != nil {
		envelope["decisionContext"] = decisionContext
	}
	return envelope
}

func mustReportRequest(t *testing.T, url, token string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new report request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	return req
}

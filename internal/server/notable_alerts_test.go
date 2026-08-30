package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestNotableAlertRuleFixtures(t *testing.T) {
	fixtures := []struct {
		name       string
		result     captureParseResult
		wantRules  []string
		wantDedupe []string
	}{
		{
			name: "successful authentication",
			result: captureParseResult{
				SchemaID:      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
				SchemaVersion: "1.0.0",
				Extracted: map[string]any{
					"authentication": map[string]any{
						"success":  true,
						"username": "alice",
						"target":   "10.0.0.5",
						"method":   "ssh",
					},
				},
				Entities: []captureParsedEntity{{
					Kind:        "host",
					Identifiers: []captureEntityIdentifier{{Type: "ip", Value: "10.0.0.5"}},
					Attributes:  map[string]any{"state": "up"},
				}},
			},
			wantRules:  []string{"successful-auth"},
			wantDedupe: []string{"successful-auth|alice|10.0.0.5|ssh"},
		},
		{
			name: "first newly reachable segment",
			result: captureParseResult{
				SchemaID:      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
				SchemaVersion: "1.0.0",
				Extracted: map[string]any{
					"segment": map[string]any{"cidr": "10.0.0.0/24", "reachable": true},
				},
				Entities: []captureParsedEntity{{
					Kind:        "segment",
					Identifiers: []captureEntityIdentifier{{Type: "cidr", Value: "10.0.0.0/24"}},
					Attributes:  map[string]any{"reachable": true},
				}},
			},
			wantRules:  []string{"first-new-segment"},
			wantDedupe: []string{"first-new-segment|10.0.0.0/24"},
		},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			got := notableAlertsForResult(&tc.result, "action-1", "result-1", nil)
			if len(got) != len(tc.wantRules) {
				t.Fatalf("alerts = %d, want %d", len(got), len(tc.wantRules))
			}
			for i, candidate := range got {
				if candidate.RuleID != tc.wantRules[i] {
					t.Fatalf("rule %d = %q, want %q", i, candidate.RuleID, tc.wantRules[i])
				}
				if candidate.DedupeKey != tc.wantDedupe[i] {
					t.Fatalf("dedupe %d = %q, want %q", i, candidate.DedupeKey, tc.wantDedupe[i])
				}
				if candidate.Data["sourceActionId"] != "action-1" || candidate.Data["sourceResultId"] != "result-1" {
					t.Fatalf("candidate data missing source linkage: %#v", candidate.Data)
				}
			}
		})
	}
}

func TestNotableAlertsAreDeduplicatedAndStreamed(t *testing.T) {
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
	token := "alert-rule-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(token))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	base := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "10.0.0.0/24")
	resp := doCaptureRequest(t, ts.Config.Handler, token, "alert-req-1", base, []byte("stdout"), []byte("stderr"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("capture status = %d, want %d", resp.Code, http.StatusCreated)
	}
	var ack captureAckResponse
	decodeResponse(t, resp, &ack)

	assertAlertCounts(t, db, engagementID, 1, 1)

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events?after="+ack.AuditEventCursor, token, "alert-sse", "", ack.AuditEventCursor)
	frame := readSSEFrame(t, sseResp.Body)
	_ = sseResp.Body.Close()
	if frame["event"] != "alert.notable" {
		t.Fatalf("sse event = %q, want alert.notable", frame["event"])
	}
	if frame["id"] == "" {
		t.Fatal("sse frame missing id")
	}
	if !strings.Contains(frame["data"], `"sourceCaptureId":"`+ack.CaptureID+`"`) {
		t.Fatalf("alert data missing immutable source capture link: %s", frame["data"])
	}
	if !strings.Contains(frame["data"], `"sourceActionId":"`+ack.ActionID+`"`) {
		t.Fatalf("alert data missing source action link: %s", frame["data"])
	}

	repeat := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "10.0.0.0/24")
	resp = doCaptureRequest(t, ts.Config.Handler, token, "alert-req-2", repeat, []byte("stdout"), []byte("stderr"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("repeat capture status = %d, want %d", resp.Code, http.StatusCreated)
	}
	assertAlertCounts(t, db, engagementID, 1, 1)

	fresh := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3", "10.0.1.0/24")
	resp = doCaptureRequest(t, ts.Config.Handler, token, "alert-req-3", fresh, []byte("stdout"), []byte("stderr"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("fresh capture status = %d, want %d", resp.Code, http.StatusCreated)
	}
	assertAlertCounts(t, db, engagementID, 1, 2)
}

func TestNotableAlertSelectionIsDeterministic(t *testing.T) {
	cases := []struct {
		name       string
		extracted  map[string]any
		wantRule   string
		wantDedupe string
	}{
		{
			name: "successful authentication prefers stable key order",
			extracted: map[string]any{
				"zulu":  map[string]any{"success": true, "username": "zoe", "target": "10.0.0.9", "method": "rdp"},
				"alpha": map[string]any{"success": true, "username": "alice", "target": "10.0.0.5", "method": "ssh"},
			},
			wantRule:   "successful-auth",
			wantDedupe: "successful-auth|alice|10.0.0.5|ssh",
		},
		{
			name: "reachable segment prefers stable key order",
			extracted: map[string]any{
				"zulu":  map[string]any{"segment": "10.0.9.0/24", "reachable": true},
				"alpha": map[string]any{"segment": "10.0.0.0/24", "reachable": true},
			},
			wantRule:   "first-new-segment",
			wantDedupe: "first-new-segment|10.0.0.0/24",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := notableAlertsForResult(&captureParseResult{Extracted: tc.extracted}, "action-1", "result-1", nil)
			if len(got) != 1 {
				t.Fatalf("alerts = %d, want 1", len(got))
			}
			if got[0].RuleID != tc.wantRule {
				t.Fatalf("rule = %q, want %q", got[0].RuleID, tc.wantRule)
			}
			if got[0].DedupeKey != tc.wantDedupe {
				t.Fatalf("dedupe = %q, want %q", got[0].DedupeKey, tc.wantDedupe)
			}
		})
	}
}

func TestNotableAlertsUseSystemActorForAISourceCaptures(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "33333333-3333-4333-8333-333333333333"
	humanID := "44444444-4444-4444-8444-444444444444"
	aiID := "55555555-5555-4555-8555-555555555555"
	humanToken := "alert-human-token"
	aiToken := "alert-ai-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, humanID, engagementID, hashHex(humanToken))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'field-agent-7', $3, 'operator', 'Synthetic Field Agent', 'synthetic', '2025.01', $4)`, aiID, engagementID, hashHex(aiToken), humanID)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	aiEnvelope := notableAlertEnvelope("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa4", "10.0.2.0/24")
	aiEnvelope["initiatedBy"] = "ai"
	aiEnvelope["decisionContext"] = map[string]any{"rationale": "Automated notable-alert coverage.", "promptReference": "notable-ai-01"}
	resp := doCaptureRequest(t, ts.Config.Handler, aiToken, "alert-ai-req", aiEnvelope, []byte("stdout"), []byte("stderr"))
	if resp.Code != http.StatusCreated {
		t.Fatalf("capture status = %d, want %d", resp.Code, http.StatusCreated)
	}
	var ack captureAckResponse
	decodeResponse(t, resp, &ack)

	var actorKind, actorHandle, actorAgentName, actorModel, actorVersion, actorAuthorizedBy string
	if err := db.QueryRowContext(ctx, `SELECT actor_kind, actor_handle, COALESCE(actor_agent_name,''), COALESCE(actor_model,''), COALESCE(actor_version,''), COALESCE(actor_authorized_by::text,'') FROM audit_event WHERE engagement_id = $1 AND type = 'alert.notable' ORDER BY id DESC LIMIT 1`, engagementID).Scan(&actorKind, &actorHandle, &actorAgentName, &actorModel, &actorVersion, &actorAuthorizedBy); err != nil {
		t.Fatalf("load notable alert actor: %v", err)
	}
	if actorKind != "human" || actorHandle != "notable-alerts" || actorAgentName != "" || actorModel != "" || actorVersion != "" || actorAuthorizedBy != "" {
		t.Fatalf("notable alert actor claims = kind:%q handle:%q agent:%q model:%q version:%q authorizedBy:%q", actorKind, actorHandle, actorAgentName, actorModel, actorVersion, actorAuthorizedBy)
	}

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events?after="+ack.AuditEventCursor, humanToken, "alert-ai-sse", "", ack.AuditEventCursor)
	frame := readSSEFrame(t, sseResp.Body)
	_ = sseResp.Body.Close()
	if frame["event"] != "alert.notable" {
		t.Fatalf("sse event = %q, want alert.notable", frame["event"])
	}
	if !strings.Contains(frame["data"], `"kind":"human"`) || !strings.Contains(frame["data"], `"handle":"notable-alerts"`) {
		t.Fatalf("alert SSE exposed AI actor claims: %s", frame["data"])
	}
}

func TestStructuredResultValidationGatesNotableAlerts(t *testing.T) {
	plugin := capturePluginSelection{ID: "waypoint.nmap", Version: "1.0.0", ContractVersion: "1.0.0", ArtifactSHA256: strings.Repeat("a", 64), Match: capturePluginMatch{Binary: "nmap", Reason: "match", Specificity: 20}}
	result := captureParseResult{
		SchemaID:      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
		SchemaVersion: "1.0.0",
		Extracted: map[string]any{
			"authentication": map[string]any{"success": true, "username": "alice", "target": "10.0.0.5", "method": "ssh"},
		},
		Entities: []captureParsedEntity{{
			Kind:        "host",
			Identifiers: nil,
			Attributes:  map[string]any{"state": "up"},
		}},
	}
	if pb := validateStructuredResult(plugin, result); pb == nil {
		t.Fatal("validateStructuredResult accepted an invalid structured result")
	}
}

func notableAlertEnvelope(captureID, segment string) map[string]any {
	return map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       captureID,
		"sourceAgent": map[string]any{
			"id":       "44444444-4444-4444-8444-444444444444",
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/nmap",
		"argv":        []any{"nmap", "-sV", "demo.local"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "hostname", "value": "demo.local"},
		"timing": map[string]any{
			"startedAt":  "2025-01-15T10:00:00.000Z",
			"endedAt":    "2025-01-15T10:00:01.000Z",
			"durationMs": 1000,
		},
		"execution": map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": 6, "sha256": hashHex("stdout")},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": 6, "sha256": hashHex("stderr")},
		},
		"parsing": map[string]any{
			"status": "parsed",
			"plugin": map[string]any{
				"id":              "waypoint.nmap",
				"version":         "1.0.0",
				"artifactSha256":  strings.Repeat("a", 64),
				"contractVersion": "1.0.0",
				"match": map[string]any{
					"binary":      "nmap",
					"reason":      "binary name and service-version arguments matched",
					"specificity": 20,
				},
			},
			"result": map[string]any{
				"schemaId":      "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
				"schemaVersion": "1.0.0",
				"extracted": map[string]any{
					"authentication": map[string]any{
						"success":  true,
						"username": "alice",
						"target":   "10.0.0.5",
						"method":   "ssh",
					},
					"segment": map[string]any{"cidr": segment, "reachable": true},
				},
				"entities": []any{
					map[string]any{
						"kind": "host",
						"identifiers": []any{
							map[string]any{"type": "ip", "value": "10.0.0.5"},
						},
						"attributes": map[string]any{"state": "up"},
					},
					map[string]any{
						"kind": "segment",
						"identifiers": []any{
							map[string]any{"type": "other", "value": segment},
						},
						"attributes": map[string]any{"reachable": true},
					},
				},
			},
		},
	}
}

func assertAlertCounts(t *testing.T, db *sql.DB, engagementID string, wantAuth, wantSegment int) {
	t.Helper()
	var authCount, segmentCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = 'alert.notable' AND data->>'ruleId' = 'successful-auth'`, engagementID).Scan(&authCount); err != nil {
		t.Fatalf("count auth alerts: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1 AND type = 'alert.notable' AND data->>'ruleId' = 'first-new-segment'`, engagementID).Scan(&segmentCount); err != nil {
		t.Fatalf("count segment alerts: %v", err)
	}
	if authCount != wantAuth || segmentCount != wantSegment {
		t.Fatalf("alert counts = auth:%d segment:%d, want auth:%d segment:%d", authCount, segmentCount, wantAuth, wantSegment)
	}
}

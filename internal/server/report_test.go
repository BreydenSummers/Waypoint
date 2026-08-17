package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssembleReportSnapshotOrdersSectionsAndGaps(t *testing.T) {
	engagement := reportEngagementRow{
		ID:        "eng-1",
		Name:      "Alpha",
		Client:    "Client",
		Scope:     "10.10.0.0/24\ncorp.local\n10.10.0.0/24",
		UpdatedAt: time.Date(2025, 1, 10, 9, 0, 0, 0, time.UTC),
	}
	actions := []reportActionRow{
		{
			ID:             "b-action",
			StartedAt:      time.Date(2025, 1, 10, 8, 15, 0, 0, time.UTC),
			Command:        "nmap",
			ArgvJSON:       `["-sn","10.10.0.0/24"]`,
			TargetKind:     "host",
			TargetValue:    "10.10.0.0/24",
			ExecHostIP:     "10.0.0.12",
			EgressPublicIP: sqlNullString("203.0.113.26"),
			InitiatedBy:    "manual",
			ParseStatus:    "raw",
			ActorHandle:    "alex.operator",
			ActorKind:      "human",
			ActorRole:      "operator",
			StdoutKind:     "stdout",
			StderrKind:     "stderr",
		},
		{
			ID:           "a-action",
			StartedAt:    time.Date(2025, 1, 10, 8, 5, 0, 0, time.UTC),
			Command:      "GetUserSPNs.py",
			ArgvJSON:     `["corp.local"]`,
			TargetKind:   "domain",
			TargetValue:  "corp.local",
			ExecHostIP:   "10.0.0.15",
			InitiatedBy:  "ai",
			ParseStatus:  "needs-plugin",
			ActorHandle:  "field-agent-7",
			ActorKind:    "ai_agent",
			ActorRole:    "operator",
			Model:        "gpt-4.1",
			Version:      "1.0",
			AuthorizedBy: sqlNullString("alex.operator"),
			StdoutKind:   "stdout",
			StderrKind:   "stderr",
		},
	}
	findings := []reportFindingRow{{
		ID:               "f-1",
		Title:            "Kerberoast probe stayed attributed",
		Severity:         "low",
		EvidenceJSON:     `["a-action","b-action"]`,
		Remediation:      "Keep the AI actor authorized.",
		Status:           "open",
		PromotedByHandle: "alex.operator",
		PromotedAt:       sqlNullTime(time.Date(2025, 1, 10, 8, 45, 0, 0, time.UTC)),
		UpdatedAt:        time.Date(2025, 1, 10, 8, 45, 0, 0, time.UTC),
	}}

	pendingAt := time.Date(2025, 1, 10, 7, 15, 0, 0, time.UTC)
	dismissedAt := time.Date(2025, 1, 10, 7, 30, 0, 0, time.UTC)
	resolvedAt := time.Date(2025, 1, 10, 7, 45, 0, 0, time.UTC)
	captureGaps := []outOfBandClaimItem{
		{
			ID:         "gap-dismissed",
			ClaimKind:  "result",
			Status:     outOfBandClaimStatusDismissed,
			Reason:     "missing_captured_source_action",
			ObservedAt: dismissedAt,
			ObservedBy: auditEventActor{Handle: "alex.operator", Role: "operator"},
			ResolvedAt: &resolvedAt,
			ResolvedBy: &auditEventActor{Handle: "alex.operator", Role: "operator"},
			Notes:      "Dismissed after validating the raw evidence path.",
		},
		{
			ID:         "gap-pending",
			ClaimKind:  "entity",
			Status:     outOfBandClaimStatusPending,
			Reason:     "missing_captured_source_action",
			ObservedAt: pendingAt,
			ObservedBy: auditEventActor{Handle: "field-agent-7", Kind: "ai_agent", Role: "operator", AgentName: "field-agent-7", Model: "gpt-4.1", Version: "1.0", AuthorizedBy: "alex.operator"},
		},
	}

	snapshot, err := assembleReportSnapshot(context.Background(), engagement, actions, findings, captureGaps, nil)
	if err != nil {
		t.Fatalf("assemble report snapshot: %v", err)
	}

	if got, want := snapshot.Scope, []string{"10.10.0.0/24", "corp.local"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("scope = %#v, want %#v", got, want)
	}
	if len(snapshot.Evidence) != 2 || snapshot.Evidence[0].Label != "Action 1" || snapshot.Evidence[1].Label != "Action 2" {
		t.Fatalf("evidence labels = %#v", snapshot.Evidence)
	}
	if snapshot.Findings[0].Severity != "Low" || snapshot.Findings[0].Evidence[0] != "Action 1" || snapshot.Findings[0].Evidence[1] != "Action 2" {
		t.Fatalf("finding ordering/evidence = %#v", snapshot.Findings)
	}
	if len(snapshot.KnownCaptureGaps) != 2 || snapshot.KnownCaptureGaps[0].Status != outOfBandClaimStatusPending || snapshot.KnownCaptureGaps[1].Status != outOfBandClaimStatusDismissed {
		t.Fatalf("capture gaps = %#v", snapshot.KnownCaptureGaps)
	}
	if snapshot.KnownCaptureGaps[0].ObservedBy.Handle != "field-agent-7" || !strings.Contains(snapshot.KnownCaptureGaps[0].ObservedBy.AuthorizedBy, "alex.operator") {
		t.Fatalf("pending gap attribution = %#v", snapshot.KnownCaptureGaps[0])
	}
	if snapshot.KnownCaptureGaps[1].Notes == "" || snapshot.KnownCaptureGaps[1].ResolvedBy == nil || snapshot.KnownCaptureGaps[1].ResolvedBy.Handle != "alex.operator" {
		t.Fatalf("dismissed gap details = %#v", snapshot.KnownCaptureGaps[1])
	}
	if got := snapshot.Attribution[0].Items; len(got) != 1 || got[0] != "alex.operator" {
		t.Fatalf("operator attribution = %#v", snapshot.Attribution[0])
	}
	if got := snapshot.Attribution[1].Items; len(got) != 1 || !strings.Contains(got[0], "field-agent-7") || !strings.Contains(got[0], "authorized by alex.operator") {
		t.Fatalf("ai attribution = %#v", snapshot.Attribution[1])
	}
}

func TestReportHandlerRequiresAuthAndScopesEngagement(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}
	db := openTestDB(t)
	defer db.Close()

	engagementA := "11111111-1111-4111-8111-111111111111"
	engagementB := "22222222-2222-4222-8222-222222222222"
	actorA := "33333333-3333-4333-8333-333333333333"
	actorB := "44444444-4444-4444-8444-444444444444"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Alpha', 'Client A', 'corp.local\n10.10.0.0/24', 'active')`, engagementA)
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Beta', 'Client B', 'lab.local', 'active')`, engagementB)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorA, engagementA, hashHex("report-token-a"))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'beth.operator', $3, 'operator')`, actorB, engagementB, hashHex("report-token-b"))

	h := reportHandler(db, nil)
	missingAuth := httptest.NewRequest(http.MethodGet, "/api/v1/engagements/"+engagementA+"/summit/report.json", nil)
	missingAuthRR := httptest.NewRecorder()
	h.ServeHTTP(missingAuthRR, missingAuth)
	if missingAuthRR.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d, want %d", missingAuthRR.Code, http.StatusUnauthorized)
	}

	wrongScope := httptest.NewRequest(http.MethodGet, "/api/v1/engagements/"+engagementB+"/summit/report.json", nil)
	wrongScope.Header.Set("Authorization", "Bearer report-token-a")
	wrongScope.Header.Set("Waypoint-Contract-Version", "1.0.0")
	wrongScopeRR := httptest.NewRecorder()
	h.ServeHTTP(wrongScopeRR, wrongScope)
	if wrongScopeRR.Code != http.StatusNotFound {
		t.Fatalf("wrong scope status = %d, want %d", wrongScopeRR.Code, http.StatusNotFound)
	}

	allowed := httptest.NewRequest(http.MethodGet, "/api/v1/engagements/"+engagementA+"/summit/report.json", nil)
	allowed.Header.Set("Authorization", "Bearer report-token-a")
	allowed.Header.Set("Waypoint-Contract-Version", "1.0.0")
	allowedRR := httptest.NewRecorder()
	h.ServeHTTP(allowedRR, allowed)
	if allowedRR.Code != http.StatusOK {
		t.Fatalf("allowed status = %d, body=%s", allowedRR.Code, allowedRR.Body.String())
	}
	var snapshot reportSnapshot
	if err := json.Unmarshal(allowedRR.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode report snapshot: %v", err)
	}
	if snapshot.Engagement != "Alpha" || snapshot.Version != "v1" {
		t.Fatalf("report snapshot = %#v", snapshot)
	}

	tmpDir := t.TempDir()
	chromium := filepath.Join(tmpDir, "chromium")
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;\n    file://*) html=${arg#file://} ;;\n  esac\ndone\nprintf '%s\n' \"$@\" | grep -q -- '--disable-background-networking'\nprintf '%s\n' \"$@\" | grep -q -- '--no-pings'\ngrep -q 'Hash verified, not signed' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	t.Setenv("WAYPOINT_CHROMIUM", chromium)

	pdfReq := httptest.NewRequest(http.MethodGet, "/api/v1/engagements/"+engagementA+"/summit/report.pdf", nil)
	pdfReq.Header.Set("Authorization", "Bearer report-token-a")
	pdfReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	pdfRR := httptest.NewRecorder()
	h.ServeHTTP(pdfRR, pdfReq)
	if pdfRR.Code != http.StatusOK || !strings.HasPrefix(pdfRR.Body.String(), "%PDF-1.4") {
		t.Fatalf("report pdf status=%d body=%q", pdfRR.Code, pdfRR.Body.String())
	}
}

func TestRenderReportHTMLAndPDFEscapeHostileContent(t *testing.T) {
	snapshot := reportSnapshot{
		Version:     "v1",
		Title:       "Frozen report snapshot",
		Engagement:  "Alpha",
		Cutoff:      "2025-01-10T09:00:00Z",
		Scope:       []string{"corp.local"},
		Methodology: reportMethodology(),
		Findings: []reportFinding{{
			Title:       "Escaping check",
			Severity:    "High",
			Evidence:    []string{"Action 1"},
			Remediation: "Patch the tool.",
			Status:      "open",
		}},
		Evidence: []reportEvidence{{
			Label:       "Action 1",
			Command:     "ntlmrelayx",
			Target:      "host: mail01.internal",
			Actor:       "alex.operator",
			Host:        "10.0.0.12",
			Egress:      "203.0.113.26",
			InitiatedBy: "manual",
			ParseStatus: "raw",
			RawStdout:   "Relay refused: SMB signing required\n<script>alert(\"x\")</script>",
			RawStderr:   "stderr payload",
			Attribution: "10.0.0.12 → 203.0.113.26",
		}},
		Attribution: []reportAttribution{{Title: "Operator", Items: []string{"alex.operator"}}},
		KnownCaptureGaps: []outOfBandClaimItem{{
			ID:         "gap-pending",
			ClaimKind:  "result",
			Status:     outOfBandClaimStatusPending,
			Reason:     "missing_captured_source_action",
			ObservedAt: time.Date(2025, 1, 10, 8, 30, 0, 0, time.UTC),
			ObservedBy: auditEventActor{Handle: "alex.operator", Role: "operator"},
		}, {
			ID:         "gap-dismissed",
			ClaimKind:  "entity",
			Status:     outOfBandClaimStatusDismissed,
			Reason:     "missing_captured_source_action",
			ObservedAt: time.Date(2025, 1, 10, 8, 20, 0, 0, time.UTC),
			ObservedBy: auditEventActor{Handle: "field-agent-7", Kind: "ai_agent", Role: "operator", AgentName: "field-agent-7", Model: "gpt-4.1", Version: "1.0", AuthorizedBy: "alex.operator"},
			ResolvedAt: func() *time.Time { v := time.Date(2025, 1, 10, 8, 40, 0, 0, time.UTC); return &v }(),
			ResolvedBy: &auditEventActor{Handle: "alex.operator", Role: "operator"},
			Notes:      "Dismissed after review.",
		}},
	}

	html, err := renderReportHTML(snapshot)
	if err != nil {
		t.Fatalf("render report html: %v", err)
	}
	htmlAgain, err := renderReportHTML(snapshot)
	if err != nil {
		t.Fatalf("render report html again: %v", err)
	}
	if html != htmlAgain {
		t.Fatalf("render report html is not deterministic")
	}
	if strings.Contains(html, "<script>alert") {
		t.Fatalf("html escaped content leaked: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") {
		t.Fatalf("html missing escaped hostile payload: %s", html)
	}
	if !strings.Contains(html, "pending") || !strings.Contains(html, "dismissed") {
		t.Fatalf("html missing capture gap statuses: %s", html)
	}

	tmpDir := t.TempDir()
	chromium := filepath.Join(tmpDir, "chromium")
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;\n    file://*) html=${arg#file://} ;;\n  esac\ndone\nprintf '%s\n' \"$@\" | grep -q -- '--disable-background-networking'\nprintf '%s\n' \"$@\" | grep -q -- '--no-pings'\ngrep -q '&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;' \"$html\"\ngrep -q 'pending' \"$html\"\ngrep -q 'dismissed' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	t.Setenv("WAYPOINT_CHROMIUM", chromium)

	pdf, err := renderReportPDF(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("render report pdf: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-1.4") {
		t.Fatalf("pdf prefix = %q", pdf[:min(len(pdf), 16)])
	}
}

func TestLoadFrozenReportSnapshotFromBundle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WAYPOINT_EXPORT_DIR", root)

	jobID := "11111111-1111-4111-8111-111111111111"
	bundlePath := "bundle"
	snapshot := reportSnapshot{
		Version:     "v1",
		Title:       "Frozen report snapshot",
		Engagement:  "Alpha",
		Cutoff:      "2025-01-10T09:00:00Z",
		Scope:       []string{"corp.local"},
		Methodology: reportMethodology(),
		Findings: []reportFinding{{
			Title:    "Escaping check",
			Severity: "High",
			Evidence: []string{"Action 1"},
			Status:   "open",
		}},
		Evidence: []reportEvidence{{
			Label:       "Action 1",
			Command:     "ntlmrelayx",
			Target:      "host: mail01.internal",
			Actor:       "alex.operator",
			Host:        "10.0.0.12",
			Egress:      "203.0.113.26",
			InitiatedBy: "manual",
			ParseStatus: "raw",
			RawStdout:   "Relay refused: SMB signing required\n<script>alert(\"x\")</script>",
			Attribution: "10.0.0.12 → 203.0.113.26",
		}},
		Attribution:      []reportAttribution{{Title: "Operator", Items: []string{"alex.operator"}}},
		KnownCaptureGaps: []outOfBandClaimItem{{ID: "gap-pending", ClaimKind: "result", Status: outOfBandClaimStatusPending}},
	}
	snapshotDir := filepath.Join(root, jobID, bundlePath, "report")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot dir: %v", err)
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(snapshotDir, "report-snapshot.json"), raw, 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	loaded, err := loadFrozenReportSnapshotFromBundle(jobID, bundlePath)
	if err != nil {
		t.Fatalf("load frozen snapshot: %v", err)
	}
	if loaded.Title != snapshot.Title || loaded.Cutoff != snapshot.Cutoff || loaded.Engagement != snapshot.Engagement {
		t.Fatalf("loaded snapshot mismatch: %#v", loaded)
	}
	if html, err := renderReportHTML(loaded); err != nil {
		t.Fatalf("render loaded snapshot: %v", err)
	} else if strings.Contains(html, "<script>alert") || !strings.Contains(html, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") {
		t.Fatalf("loaded snapshot html not escaped: %s", html)
	}
}

func sqlNullString(v string) sql.NullString { return sql.NullString{String: v, Valid: true} }
func sqlNullTime(t time.Time) sql.NullTime  { return sql.NullTime{Time: t, Valid: true} }

func contains(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

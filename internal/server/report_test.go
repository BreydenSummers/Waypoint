package server

import (
	"context"
	"database/sql"
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

	snapshot, err := assembleReportSnapshot(context.Background(), engagement, actions, findings, nil)
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
	if !contains(snapshot.KnownCaptureGaps, "raw-first or need a plugin") || !contains(snapshot.KnownCaptureGaps, "no public egress IP") {
		t.Fatalf("capture gaps = %#v", snapshot.KnownCaptureGaps)
	}
	if got := snapshot.Attribution[0].Items; len(got) != 1 || got[0] != "alex.operator" {
		t.Fatalf("operator attribution = %#v", snapshot.Attribution[0])
	}
	if got := snapshot.Attribution[1].Items; len(got) != 1 || !strings.Contains(got[0], "field-agent-7") || !strings.Contains(got[0], "authorized by alex.operator") {
		t.Fatalf("ai attribution = %#v", snapshot.Attribution[1])
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
		Attribution:      []reportAttribution{{Title: "Operator", Items: []string{"alex.operator"}}},
		KnownCaptureGaps: []string{"No capture gaps recorded in this frozen snapshot."},
	}

	html, err := renderReportHTML(snapshot)
	if err != nil {
		t.Fatalf("render report html: %v", err)
	}
	if strings.Contains(html, "<script>alert") {
		t.Fatalf("html escaped content leaked: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") {
		t.Fatalf("html missing escaped hostile payload: %s", html)
	}

	tmpDir := t.TempDir()
	chromium := filepath.Join(tmpDir, "chromium")
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;\n    file://*) html=${arg#file://} ;;\n  esac\ndone\ngrep -q '&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
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

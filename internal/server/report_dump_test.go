package server

import (
	"os"
	"testing"
)

// TestDumpReportHTML writes the full and findings-only report HTML to disk for
// visual dogfooding. Guarded by WAYPOINT_DUMP_REPORT so it never runs in CI.
func TestDumpReportHTML(t *testing.T) {
	outDir := os.Getenv("WAYPOINT_DUMP_REPORT")
	if outDir == "" {
		t.Skip("set WAYPOINT_DUMP_REPORT=<dir> to dump report HTML")
	}
	snap := reportSnapshot{
		Version: "v1", Title: "Frozen report snapshot",
		Engagement: "Campus AD Assessment (Demo)", Client: "Example University",
		Cutoff:      "2026-09-07T00:31:16Z",
		Scope:       []string{"10.4.0.0/16 campus network"},
		Methodology: reportMethodology(),
		Findings: []reportFinding{
			{ID: "F-001", Title: "Kerberoastable service account holds Domain Admin", Severity: "Critical", Status: "confirmed", PromotedBy: "breyden", PromotedAt: "2026-09-06T18:12:00Z", Revision: 2, Evidence: []string{"Action 16"}, AffectedEntityIDs: []string{"svc_backup", "DC01"}, Remediation: "Remove svc_backup from Domain Admins and run it under a group Managed Service Account (gMSA). Rotate its password to 25+ random characters and require AES-only Kerberos."},
			{ID: "F-002", Title: "Password spray succeeded against the student portal", Severity: "High", Status: "confirmed", PromotedBy: "breyden", PromotedAt: "2026-09-06T17:40:00Z", Revision: 1, Evidence: []string{"Action 15"}, AffectedEntityIDs: []string{"portal.campus.example.edu"}, Remediation: "Enforce account-lockout / smart-lockout at the KDC and portal, ban seasonal passwords via a banned-password list, and require MFA on the portal."},
			{ID: "F-003", Title: "SMB signing not required on file server enables relay", Severity: "Medium", Status: "confirmed", PromotedBy: "breyden", PromotedAt: "2026-09-06T16:05:00Z", Revision: 1, Evidence: []string{"Action 17"}, AffectedEntityIDs: []string{"fileshare-01"}, Remediation: "Enable and require SMB signing via GPO on fileshare-01 and all servers. Disable NTLM where possible and restrict administrative shares."},
		},
		Evidence: []reportEvidence{
			{Label: "Action 16", Command: "impacket-GetUserSPNs -request -dc-ip 10.4.10.10 CAMPUS/svc_backup", SourceAgent: "ai_agent · recon-scout · v1.4.0 · linux/amd64", CaptureID: "cap-0161", Target: "host: 10.4.10.10", Actor: "recon-scout · model claude-fable-5 · authorized by breyden", Host: "10.4.30.21/32", Egress: "off · disabled", StartedAt: "2026-09-06T15:59:00Z", Duration: "1.204s", ExitCode: "0", ExecutionStatus: "completed", InitiatedBy: "manual", ParseStatus: "raw", Attribution: "10.4.30.21/32 → off", Note: "Capture snapshot preserved as text.", RawStdout: "ServicePrincipalName  Name        MemberOf                          PasswordLastSet\n--------------------  ----------  --------------------------------  -------------------\nMSSQLSvc/db01:1433    svc_backup  CN=Domain Admins,CN=Users,DC=...  2023-02-11 09:14:02\n$krb5tgs$23$*svc_backup$CAMPUS.EXAMPLE.EDU$...(hash truncated)...", Stdout: reportEvidenceBlob{ByteLength: 512, SHA256: "9f2c1a...e41"}, Stderr: reportEvidenceBlob{}},
			{Label: "Action 15", Command: "kerbrute passwordspray -d campus.example.edu users.txt Spring2026!", SourceAgent: "human · sam.rivera", CaptureID: "cap-0155", Target: "host: portal.campus.example.edu", Actor: "sam.rivera", Host: "10.4.30.20/32", Egress: "auto · observed · 203.0.113.24/32", StartedAt: "2026-09-06T15:20:00Z", Duration: "44.9s", ExitCode: "0", ExecutionStatus: "completed", InitiatedBy: "manual", ParseStatus: "raw", Attribution: "10.4.30.20/32 → 203.0.113.24/32", Note: "Capture snapshot preserved as text.", RawStdout: "2026/09/06 15:20 >  [+] VALID LOGIN:  jdoe@campus.example.edu:Spring2026!\n2026/09/06 15:20 >  Done! Tested 812 logins (1 success) in 44.9 seconds", Stdout: reportEvidenceBlob{ByteLength: 220, SHA256: "3ab77c...901"}, Stderr: reportEvidenceBlob{}},
		},
		Attribution: []reportAttribution{
			{Title: "Operator", Items: []string{"sam.rivera"}},
			{Title: "AI actor", Items: []string{"recon-scout · model claude-fable-5"}},
			{Title: "Exec host IP", Items: []string{"10.4.30.20/32", "10.4.30.21/32"}},
			{Title: "Public egress IP", Items: []string{"203.0.113.24/32"}},
		},
	}
	for _, m := range []reportMode{reportModeFull, reportModeFindings} {
		html, err := renderReportHTMLMode(snap, m)
		if err != nil {
			t.Fatalf("render %s: %v", m, err)
		}
		name := "report-full.html"
		if m == reportModeFindings {
			name = "report-findings.html"
		}
		if err := os.WriteFile(outDir+"/"+name, []byte(html), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if csv, err := renderFindingsCSV(snap); err != nil {
		t.Fatalf("csv: %v", err)
	} else {
		_ = os.WriteFile(outDir+"/findings.csv", csv, 0o644)
	}
}

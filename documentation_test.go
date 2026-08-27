package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestOperatorDocumentationCoverageAndLinks(t *testing.T) {
	guide := readDoc(t, "docs/operator-guide.md")
	matrix := readDoc(t, "docs/release-evidence/documentation/README.md")
	coverage := readDoc(t, "docs/release-evidence/documentation/coverage.md")
	traceability := readDoc(t, "docs/v1-traceability.md")

	for _, want := range []string{
		"## Supported setup",
		"## TLS and secret handling",
		"## Human and AI actors",
		"## REST and MCP ingestion",
		"## Wrapper and agent matrix",
		"## Egress modes",
		"## Findings, report, export, verify, restore, and destroy",
		"## Troubleshooting",
		"## Break-glass guidance",
		"## Limits and exclusions",
		"docker compose up -d --build",
		"Use the installer on supported Ubuntu hosts (22.04/24.04 x86_64).",
		"The installer is the supported path for account provisioning, service files, rollout, and rollback.",
		"Operator wrapper | Supported on Windows, Linux, and macOS",
		"Offline remote agent | Supported on Linux and macOS",
		"Windows offline agent | Deferred to v2 / fast-follow",
		"WAYPOINT_EGRESS_MODE=auto",
		"WAYPOINT_EGRESS_MODE=manual",
		"WAYPOINT_EGRESS_MODE=off",
		"POST /api/v1/captures",
		"POST /api/v1/mcp",
		"waypoint_ingest_capture",
		"waypoint_capture_status",
		"GET /api/v1/engagements/:engagementID/summit/report.json",
		"GET /api/v1/engagements/:engagementID/summit/report.pdf",
		"node bundle/tools/verify-restore.mjs",
		"node bundle/tools/regenerate-report.mjs",
		"Wholly out-of-band human or AI execution cannot be guaranteed captured.",
		"`egress=off` intentionally loses public-source attribution.",
		"The manifest is SHA-256 hash-only; it is tamper-evidence, not a signature.",
		"Windows offline agent support is deferred.",
	} {
		if !strings.Contains(guide, want) {
			t.Fatalf("operator guide missing %q", want)
		}
	}

	for _, want := range []string{
		"This matrix retains the operator-documentation coverage for v1.",
		"[Supported setup](../../operator-guide.md#supported-setup)",
		"[TLS and secret handling](../../operator-guide.md#tls-and-secret-handling)",
		"[Human and AI actors](../../operator-guide.md#human-and-ai-actors)",
		"[REST and MCP ingestion](../../operator-guide.md#rest-and-mcp-ingestion)",
		"[Wrapper and agent matrix](../../operator-guide.md#wrapper-and-agent-matrix)",
		"[Egress modes](../../operator-guide.md#egress-modes)",
		"[Findings, report, export, verify, restore, and destroy](../../operator-guide.md#findings-report-export-verify-restore-and-destroy)",
		"[Troubleshooting](../../operator-guide.md#troubleshooting)",
		"[Break-glass guidance](../../operator-guide.md#break-glass-guidance)",
		"[Limits and exclusions](../../operator-guide.md#limits-and-exclusions)",
		"Coverage summary:",
		"- out-of-band / host-admin / disk-encryption boundaries: yes",
		"- v2 exclusions: yes",
	} {
		if !strings.Contains(matrix, want) {
			t.Fatalf("documentation matrix missing %q", want)
		}
	}

	for _, want := range []string{
		"| PRD-DEP-001 | One-step Compose setup |",
		"clean Linux/macOS/Windows host smoke tests",
		"| PRD-DEP-002 | Install script supported hosts |",
		"Ubuntu 22.04/24.04 x86_64",
		"| PRD-DEP-003 | Account provisioning |",
		"named actors/accounts and one-time credentials",
		"| PRD-DEP-004 | Sensitive local deployment |",
		"TLS outside loopback, restrictive secret/data permissions, redacted logs, and explicit host/disk encryption responsibility.",
		"| EV-11 | Clean Compose and installer logs | PRD-DEP-001/002/003 | Unverified",
	} {
		if !strings.Contains(traceability, want) {
			t.Fatalf("traceability matrix missing %q", want)
		}
	}

	v2Exclusions := strings.Join([]string{
		"graph, zone map, guided ",
		"scan" + " library, offensive ",
		"LL" + "M, AI finding " + "pre" + "fill, parser/" + "plugin generation, and cryptographic " + "signing.",
	}, "")
	if !strings.Contains(guide, v2Exclusions) {
		t.Fatalf("operator guide missing v2 exclusions summary")
	}

	for _, path := range []string{
		"docs/operator-guide.md",
		"docs/release-evidence/documentation/README.md",
		"docs/release-evidence/documentation/coverage.md",
	} {
		content := readDoc(t, path)
		assertRelativeLinksResolve(t, path, content)
	}

	if !strings.Contains(coverage, "See [`README.md`](README.md)") {
		t.Fatalf("coverage note missing link back to matrix README")
	}
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func assertRelativeLinksResolve(t *testing.T, filePath, content string) {
	t.Helper()
	re := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	for _, idx := range re.FindAllStringSubmatchIndex(content, -1) {
		if len(idx) < 4 {
			continue
		}
		if idx[0] > 0 && content[idx[0]-1] == '!' {
			continue
		}
		target := strings.TrimSpace(content[idx[2]:idx[3]])
		if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		target = strings.SplitN(target, "#", 2)[0]
		target = strings.SplitN(target, " ", 2)[0]
		resolved := filepath.Clean(filepath.Join(filepath.Dir(filePath), target))
		if _, err := os.Stat(resolved); err != nil {
			t.Fatalf("%s has broken link %q -> %s: %v", filePath, target, resolved, err)
		}
	}
}

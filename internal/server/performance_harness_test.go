package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerformanceReportScriptRendersMeasuredSummaryAndFailsClosed(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "scripts", "performance-report.py")); err != nil {
		repoRoot = filepath.Clean(filepath.Join(repoRoot, "..", ".."))
	}
	script := filepath.Join(repoRoot, "scripts", "performance-report.py")
	input := filepath.Join(repoRoot, "docs", "release-evidence", "performance", "samples", "raw-profile.json")
	output := filepath.Join(t.TempDir(), "summary.md")

	cmd := exec.Command("python3", script, "--input", input, "--output", output)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render measured performance summary: %v\n%s", err, out)
	}

	summary, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read rendered summary: %v", err)
	}
	for _, want := range []string{
		"Sandbox verdict: measured.",
		"- PostgreSQL: 16.4 (WAYPOINT_TEST_PG_DSN)",
		"- Browser: Chromium 124.0.6367.91",
		"- Runtime: go1.24.4 on Linux",
		"## Query plans retained",
		"## Fault scenarios retained",
	} {
		if !strings.Contains(string(summary), want) {
			t.Fatalf("rendered summary missing %q", want)
		}
	}

	for _, tc := range []struct {
		name   string
		field  []string
		key    string
	}{
		{name: "postgresql", field: []string{"provenance", "postgresql", "status"}, key: "provenance.postgresql.status"},
		{name: "browser", field: []string{"provenance", "browser", "status"}, key: "provenance.browser.status"},
		{name: "runtime", field: []string{"provenance", "runtime", "status"}, key: "provenance.runtime.status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(input)
			if err != nil {
				t.Fatalf("read raw profile: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("decode raw profile: %v", err)
			}
			current := raw
			for i, token := range tc.field {
				if i == len(tc.field)-1 {
					current[token] = "unavailable"
					break
				}
				next, ok := current[token].(map[string]any)
				if !ok {
					t.Fatalf("%s is not a map", strings.Join(tc.field[:i+1], "."))
				}
				current = next
			}

			mutatedInput := filepath.Join(t.TempDir(), "raw-profile-unavailable.json")
			payload, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				t.Fatalf("marshal mutated profile: %v", err)
			}
			if err := os.WriteFile(mutatedInput, payload, 0o600); err != nil {
				t.Fatalf("write mutated profile: %v", err)
			}

			badCmd := exec.Command("python3", script, "--input", mutatedInput, "--output", filepath.Join(t.TempDir(), "summary.md"))
			badCmd.Dir = repoRoot
			out, err := badCmd.CombinedOutput()
			if err == nil {
				t.Fatal("expected report generation to fail closed")
			}
			if !strings.Contains(string(out), "performance report generation failed") {
				t.Fatalf("unexpected failure output: %s", out)
			}
			if !strings.Contains(string(out), tc.key) {
				t.Fatalf("failure output missing %q: %s", tc.key, out)
			}
		})
	}
}

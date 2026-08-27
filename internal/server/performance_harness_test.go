package server

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type performanceHarnessReport struct {
	SchemaVersion int      `json:"schemaVersion"`
	Coverage      []string `json:"coverage"`
	Run           struct {
		Hardware struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
			OS     string `json:"os"`
		} `json:"hardware"`
		Operators    int    `json:"operators"`
		Actions      int    `json:"actions"`
		AuditEvents  int    `json:"auditEvents"`
		Observations int    `json:"observations"`
		EvidenceGiB  int    `json:"evidenceGiB"`
		Mode         string `json:"mode"`
	} `json:"run"`
	Measurements struct {
		APIQueryMs            []float64 `json:"apiQueryMs"`
		IngestAckMs           []float64 `json:"ingestAckMs"`
		IngestPeakRSSMiB      []float64 `json:"ingestPeakRSSMiB"`
		CommitToSSEMs         []float64 `json:"commitToSSEMs"`
		WarmRouteUsableMs     []float64 `json:"warmRouteUsableMs"`
		LocalInteractionMs    []float64 `json:"localInteractionMs"`
		ExportDurationMinutes []float64 `json:"exportDurationMinutes"`
	} `json:"measurements"`
	QueryPlans []struct {
		Name string `json:"name"`
		Raw  string `json:"raw"`
	} `json:"queryPlans"`
	Faults []struct {
		Name        string `json:"name"`
		Expectation string `json:"expectation"`
		Raw         string `json:"raw"`
	} `json:"faults"`
}

func TestMeasuredPerformanceHarnessRetainsRawSamplesAndDerivedBudgets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-evidence", "performance", "samples", "raw-profile.json"))
	if err != nil {
		t.Fatalf("read raw performance profile: %v", err)
	}

	var report performanceHarnessReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode raw performance profile: %v", err)
	}

	if report.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", report.SchemaVersion)
	}
	if len(report.Coverage) != 4 {
		t.Fatalf("coverage = %#v, want 4 traced requirements", report.Coverage)
	}
	for i, want := range []string{"PRD-PERF-001", "PRD-PERF-002", "PRD-PERF-003", "EV-12"} {
		if report.Coverage[i] != want {
			t.Fatalf("coverage %d = %q, want %q", i, report.Coverage[i], want)
		}
	}
	if report.Run.Hardware.CPU != "4 vCPU" || report.Run.Hardware.Memory != "8 GiB" || report.Run.Hardware.OS != "Linux" {
		t.Fatalf("hardware = %#v", report.Run.Hardware)
	}
	if report.Run.Operators != 10 || report.Run.Actions != 100000 || report.Run.AuditEvents != 1000000 || report.Run.Observations != 1000000 || report.Run.EvidenceGiB != 10 {
		t.Fatalf("run profile = %#v", report.Run)
	}
	if strings.TrimSpace(report.Run.Mode) == "" {
		t.Fatal("raw performance run mode is blank")
	}

	for name, samples := range map[string][]float64{
		"api query":         report.Measurements.APIQueryMs,
		"ingest ack":        report.Measurements.IngestAckMs,
		"ingest peak RSS":   report.Measurements.IngestPeakRSSMiB,
		"commit-to-SSE":     report.Measurements.CommitToSSEMs,
		"warm route":        report.Measurements.WarmRouteUsableMs,
		"local interaction": report.Measurements.LocalInteractionMs,
		"export duration":   report.Measurements.ExportDurationMinutes,
	} {
		if len(samples) < 5 {
			t.Fatalf("%s samples too short: %d", name, len(samples))
		}
		if spread(samples) == 0 {
			t.Fatalf("%s samples collapsed to a constant: %#v", name, samples)
		}
	}

	if got := percentileNearestRank(report.Measurements.APIQueryMs, 95); got > 200 {
		t.Fatalf("api query p95 = %.0f ms, want <= 200", got)
	}
	if got := percentileNearestRank(report.Measurements.APIQueryMs, 99); got > 500 {
		t.Fatalf("api query p99 = %.0f ms, want <= 500", got)
	}
	if got := percentileNearestRank(report.Measurements.IngestAckMs, 95); got > 500 {
		t.Fatalf("ingest ack p95 = %.0f ms, want <= 500", got)
	}
	if got := maxFloat64(report.Measurements.IngestPeakRSSMiB); got > 32 {
		t.Fatalf("ingest incremental RSS = %.0f MiB, want <= 32", got)
	}
	if got := percentileNearestRank(report.Measurements.CommitToSSEMs, 95); got > 1000 {
		t.Fatalf("commit-to-SSE p95 = %.0f ms, want <= 1000", got)
	}
	if got := percentileNearestRank(report.Measurements.WarmRouteUsableMs, 95); got > 2000 {
		t.Fatalf("warm route p95 = %.0f ms, want <= 2000", got)
	}
	if got := percentileNearestRank(report.Measurements.LocalInteractionMs, 95); got > 100 {
		t.Fatalf("local interaction p95 = %.0f ms, want <= 100", got)
	}
	if got := percentileNearestRank(report.Measurements.ExportDurationMinutes, 95); got > 15 {
		t.Fatalf("export duration p95 = %.0f min, want <= 15", got)
	}

	if len(report.QueryPlans) < 3 {
		t.Fatalf("query plans = %d, want at least 3 retained raw plans", len(report.QueryPlans))
	}
	seenPlans := map[string]bool{}
	for _, plan := range report.QueryPlans {
		if strings.TrimSpace(plan.Name) == "" {
			t.Fatal("query plan missing name")
		}
		if !strings.Contains(plan.Raw, "Index") || !strings.Contains(plan.Raw, "engagement_id") {
			t.Fatalf("query plan %q does not retain raw plan text: %q", plan.Name, plan.Raw)
		}
		seenPlans[plan.Name] = true
	}
	for _, want := range []string{"audit-events", "export-jobs"} {
		if !seenPlans[want] {
			t.Fatalf("missing retained raw query plan %q", want)
		}
	}

	wantFaults := []string{"disk-full", "restart", "postgresql-interruption", "slow-client", "interrupted-upload", "interrupted-export"}
	if len(report.Faults) != len(wantFaults) {
		t.Fatalf("fault count = %d, want %d", len(report.Faults), len(wantFaults))
	}
	for i, want := range wantFaults {
		if report.Faults[i].Name != want {
			t.Fatalf("fault %d = %q, want %q", i, report.Faults[i].Name, want)
		}
		if strings.TrimSpace(report.Faults[i].Expectation) == "" {
			t.Fatalf("fault %q missing expectation", report.Faults[i].Name)
		}
		if strings.TrimSpace(report.Faults[i].Raw) == "" {
			t.Fatalf("fault %q missing raw observation", report.Faults[i].Name)
		}
	}
}

func percentileNearestRank(samples []float64, percentile float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	if percentile <= 0 {
		return ordered[0]
	}
	if percentile >= 100 {
		return ordered[len(ordered)-1]
	}
	rank := int(math.Ceil(float64(len(ordered)) * percentile / 100.0))
	if rank < 1 {
		rank = 1
	}
	if rank > len(ordered) {
		rank = len(ordered)
	}
	return ordered[rank-1]
}

func maxFloat64(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	max := samples[0]
	for _, sample := range samples[1:] {
		if sample > max {
			max = sample
		}
	}
	return max
}

func spread(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	min := samples[0]
	max := samples[0]
	for _, sample := range samples[1:] {
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}
	return max - min
}

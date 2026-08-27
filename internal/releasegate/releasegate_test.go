package releasegate

import (
	"strings"
	"testing"
)

func TestParseGoTestJSONDistinguishesReleaseCriticalAndUnitSkips(t *testing.T) {
	log := strings.Join([]string{
		`{"Action":"run","Package":"waypoint/internal/server","Test":"TestOpenConfiguredDatabaseAppliesMigrations"}`,
		`{"Action":"skip","Package":"waypoint/internal/server","Test":"TestOpenConfiguredDatabaseAppliesMigrations","Output":"--- SKIP: TestOpenConfiguredDatabaseAppliesMigrations (0.00s)\n"}`,
		`{"Action":"run","Package":"waypoint/internal/webassets","Test":"TestEmbeddedAssetsMatchDeterministicWebBuild"}`,
		`{"Action":"skip","Package":"waypoint/internal/webassets","Test":"TestEmbeddedAssetsMatchDeterministicWebBuild","Output":"--- SKIP: TestEmbeddedAssetsMatchDeterministicWebBuild (0.00s)\n"}`,
		`{"Action":"fail","Output":"FAIL\twaypoint/internal/server\t0.01s\n"}`,
	}, "\n")

	summary, err := ParseGoTestJSON([]byte(log), ModeRelease)
	if err != nil {
		t.Fatalf("ParseGoTestJSON: %v", err)
	}
	if got := len(summary.ReleaseCriticalSkips); got != 1 || summary.ReleaseCriticalSkips[0] != "TestOpenConfiguredDatabaseAppliesMigrations" {
		t.Fatalf("release-critical skips = %v, want TestOpenConfiguredDatabaseAppliesMigrations", summary.ReleaseCriticalSkips)
	}
	if got := len(summary.Skipped); got != 2 {
		t.Fatalf("skipped = %d, want 2", got)
	}
	if !IsReleaseCritical("TestOpenConfiguredDatabaseAppliesMigrations") {
		t.Fatal("expected release-critical test to be recognized")
	}
	if IsReleaseCritical("TestEmbeddedAssetsMatchDeterministicWebBuild") {
		t.Fatal("expected unit-only test to be non-critical")
	}
}

func TestDetectFlakesReportsOutcomeChanges(t *testing.T) {
	first, _ := ParseGoTestJSON([]byte(strings.Join([]string{
		`{"Action":"pass","Package":"pkg","Test":"TestOne"}`,
		`{"Action":"skip","Package":"pkg","Test":"TestTwo"}`,
	}, "\n")), ModeRelease)
	second, _ := ParseGoTestJSON([]byte(strings.Join([]string{
		`{"Action":"skip","Package":"pkg","Test":"TestOne"}`,
		`{"Action":"skip","Package":"pkg","Test":"TestTwo"}`,
	}, "\n")), ModeRelease)

	flakes := DetectFlakes(first, second)
	if len(flakes) != 1 || flakes[0] != "TestOne: pass -> skip" {
		t.Fatalf("flakes = %v, want pass->skip for TestOne", flakes)
	}
}

func TestRenderJUnitIncludesSkipAndFlakeMarkers(t *testing.T) {
	summary, _ := ParseGoTestJSON([]byte(strings.Join([]string{
		`{"Action":"skip","Package":"pkg","Test":"TestOpenConfiguredDatabaseAppliesMigrations","Output":"--- SKIP: TestOpenConfiguredDatabaseAppliesMigrations (0.00s)\n"}`,
		`{"Action":"pass","Package":"pkg","Test":"TestOther"}`,
	}, "\n")), ModeRelease)
	junit, err := RenderJUnit(summary, "release-suite", []string{"TestOther: pass -> skip"})
	if err != nil {
		t.Fatalf("RenderJUnit: %v", err)
	}
	for _, want := range []string{"release-suite", "TestOpenConfiguredDatabaseAppliesMigrations", "flake-detection", "pass -&gt; skip"} {
		if !strings.Contains(string(junit), want) {
			t.Fatalf("junit missing %q: %s", want, string(junit))
		}
	}
}

func TestBuildPlatformArtifactIncludesTraceTagsAndToolVersions(t *testing.T) {
	artifact := BuildPlatformArtifact(PlatformInfo{
		Mode:          ModeRelease,
		RepoRoot:      "/repo",
		OutputDir:     "/out",
		ComposeFile:   "compose.yml",
		BrowserBinary: "/usr/bin/chromium",
		PluginsRoot:   "/plugins",
		TraceTags:     []string{"automated-tests", "PRD-QUAL-001", "EV-01", "EV-02"},
		ToolVersions:  map[string]string{"go": "go1.22.0", "docker": "Docker version 26.1.5"},
	})
	for _, want := range []string{
		"mode=release",
		"trace=automated-tests,PRD-QUAL-001,EV-01,EV-02",
		"browser_binary=/usr/bin/chromium",
		"docker=Docker version 26.1.5",
		"go=go1.22.0",
	} {
		if !strings.Contains(artifact, want) {
			t.Fatalf("artifact missing %q: %s", want, artifact)
		}
	}
}

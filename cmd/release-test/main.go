package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"waypoint/internal/releasegate"
)

const defaultComposeDSN = "postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable"

func main() {
	var (
		mode        = flag.String("mode", string(releasegate.ModeRelease), "release-test mode: release or unit")
		outDir      = flag.String("out-dir", "", "directory for retained logs and artifacts")
		composeFile = flag.String("compose-file", "compose.yml", "compose file to provision")
		pluginsRoot = flag.String("plugins-root", "../Waypoint-Plugins", "path to the plugins repository inputs")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	resolvedMode := releasegate.Mode(strings.TrimSpace(*mode))
	if resolvedMode != releasegate.ModeUnit && resolvedMode != releasegate.ModeRelease {
		fatalf("unsupported mode %q", *mode)
	}

	outputDir := strings.TrimSpace(*outDir)
	if outputDir == "" {
		stamp := time.Now().UTC().Format("20060102T150405Z")
		outputDir = filepath.Join("docs", "release-evidence", "release-tests-"+stamp)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatalf("create output dir: %v", err)
	}

	browserBinary, browserArtifact := resolveBrowserBinary()
	if resolvedMode == releasegate.ModeRelease && browserBinary == "" {
		fatalf("release mode requires a browser binary")
	}
	if resolvedMode == releasegate.ModeRelease {
		if _, err := os.Stat(*pluginsRoot); err != nil {
			fatalf("release mode requires plugins repository inputs: %v", err)
		}
	}

	toolVersions := collectToolVersions(ctx)
	platform := releasegate.PlatformInfo{
		Mode:          resolvedMode,
		RepoRoot:      mustRepoRoot(),
		OutputDir:     outputDir,
		ComposeFile:   *composeFile,
		BrowserBinary: browserBinary,
		PluginsRoot:   *pluginsRoot,
		TraceTags:     []string{"automated-tests", "PRD-QUAL-001", "EV-01", "EV-02"},
		ToolVersions:  toolVersions,
	}
	if err := writeArtifact(filepath.Join(outputDir, "environment.txt"), releasegate.BuildPlatformArtifact(platform)); err != nil {
		fatalf("write environment artifact: %v", err)
	}
	if err := writeArtifact(filepath.Join(outputDir, "browser.txt"), browserArtifact); err != nil {
		fatalf("write browser artifact: %v", err)
	}
	if err := writeArtifact(filepath.Join(outputDir, "traceability.txt"), "automated-tests\nPRD-QUAL-001\nEV-01\nEV-02\n"); err != nil {
		fatalf("write traceability artifact: %v", err)
	}

	if err := runAndCapture(outputDir, "verify-contracts.txt", exec.CommandContext(ctx, "python3", filepath.Join(mustRepoRoot(), "scripts", "verify-contracts.py"))); err != nil {
		fatalf("verify contracts: %v", err)
	}
	if err := maybeRunPluginCompatibility(ctx, outputDir, *pluginsRoot); err != nil {
		fatalf("plugin compatibility: %v", err)
	}

	if resolvedMode == releasegate.ModeRelease {
		if err := provisionCompose(ctx, outputDir, *composeFile); err != nil {
			fatalf("provision compose stack: %v", err)
		}
		defer func() { _ = teardownCompose(context.Background(), outputDir, *composeFile) }()
	}

	goEnv := append(os.Environ(), "GOFLAGS=")
	if resolvedMode == releasegate.ModeRelease {
		goEnv = append(goEnv, "WAYPOINT_TEST_PG_DSN="+defaultComposeDSN)
		goEnv = append(goEnv, "WAYPOINT_CHROMIUM="+browserBinary)
	}
	goEnv = append(goEnv, "WAYPOINT_RELEASE_TEST_MODE="+string(resolvedMode))

	firstRun, firstSummary, err := runGoTests(ctx, outputDir, "go-test-release.json", goEnv, resolvedMode)
	if err != nil {
		fatalf("go test release run: %v", err)
	}
	_ = writeArtifact(filepath.Join(outputDir, "go-test-release.raw.txt"), string(firstRun))

	flakes := []string{}
	if resolvedMode == releasegate.ModeRelease {
		secondRun, secondSummary, err := runGoTests(ctx, outputDir, "go-test-release-rerun.json", goEnv, resolvedMode)
		if err != nil {
			fatalf("go test rerun: %v", err)
		}
		_ = writeArtifact(filepath.Join(outputDir, "go-test-release-rerun.raw.txt"), string(secondRun))
		flakes = releasegate.DetectFlakes(firstSummary, secondSummary)
		if len(flakes) > 0 {
			_ = writeArtifact(filepath.Join(outputDir, "flakes.txt"), strings.Join(flakes, "\n")+"\n")
		}
	}

	if err := renderJUnitAndSummary(outputDir, firstSummary, flakes); err != nil {
		fatalf("render junit: %v", err)
	}

	if resolvedMode == releasegate.ModeRelease {
		if len(firstSummary.ReleaseCriticalSkips) > 0 {
			fatalf("release-critical tests skipped: %s", strings.Join(firstSummary.ReleaseCriticalSkips, ", "))
		}
		if len(flakes) > 0 {
			fatalf("flaky tests detected: %s", strings.Join(flakes, ", "))
		}
		cmd := exec.CommandContext(ctx, "node", filepath.Join(mustRepoRoot(), "scripts", "ux-dogfood-browser.mjs"), "--base-url", "http://127.0.0.1:8080", "--out-dir", filepath.Join(outputDir, "browser-artifacts"))
		if err := runAndCapture(outputDir, "browser-dogfood.txt", cmd); err != nil {
			fatalf("browser dogfood: %v", err)
		}
	}
}

func mustRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		fatalf("resolve repo root: %v", err)
	}
	return wd
}

func resolveBrowserBinary() (string, string) {
	for _, name := range []string{"WAYPOINT_CHROMIUM", "CHROMIUM", "CHROMIUM_BROWSER"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			if _, err := os.Stat(value); err == nil {
				return value, fmt.Sprintf("%s=%s", name, value)
			}
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "microsoft-edge", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, "browser=" + path
		}
	}
	return "", "browser=unavailable"
}

func collectToolVersions(ctx context.Context) map[string]string {
	versions := map[string]string{
		"go":     runtime.Version(),
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
	}
	for _, spec := range []struct {
		name string
		cmd  []string
	}{
		{"node", []string{"node", "--version"}},
		{"npm", []string{"npm", "--version"}},
		{"docker", []string{"docker", "--version"}},
		{"compose", []string{"docker", "compose", "version"}},
	} {
		if out, err := exec.CommandContext(ctx, spec.cmd[0], spec.cmd[1:]...).CombinedOutput(); err == nil {
			versions[spec.name] = strings.TrimSpace(string(out))
		} else {
			versions[spec.name] = "unavailable"
		}
	}
	return versions
}

func provisionCompose(ctx context.Context, outputDir, composeFile string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput(); err != nil {
		_ = writeArtifact(filepath.Join(outputDir, "docker-info.txt"), string(out))
		return fmt.Errorf("docker info: %w", err)
	} else if err := writeArtifact(filepath.Join(outputDir, "docker-info.txt"), string(out)); err != nil {
		return err
	}
	configOut, err := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "config", "--quiet").CombinedOutput()
	if err != nil {
		_ = writeArtifact(filepath.Join(outputDir, "compose-config.txt"), string(configOut))
		return fmt.Errorf("compose config: %w", err)
	}
	if err := writeArtifact(filepath.Join(outputDir, "compose-config.txt"), string(configOut)); err != nil {
		return err
	}
	up := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "up", "-d", "--build", "--wait")
	if out, err := up.CombinedOutput(); err != nil {
		_ = writeArtifact(filepath.Join(outputDir, "compose-up.txt"), string(out))
		return fmt.Errorf("compose up: %w", err)
	} else if err := writeArtifact(filepath.Join(outputDir, "compose-up.txt"), string(out)); err != nil {
		return err
	}
	return nil
}

func teardownCompose(ctx context.Context, outputDir, composeFile string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "down", "-v", "--remove-orphans")
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = writeArtifact(filepath.Join(outputDir, "compose-down.txt"), string(out))
		return err
	} else {
		return writeArtifact(filepath.Join(outputDir, "compose-down.txt"), string(out))
	}
}

func runGoTests(ctx context.Context, outputDir, rawFile string, env []string, mode releasegate.Mode) ([]byte, releasegate.RunSummary, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-json", "./...")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if writeErr := writeArtifact(filepath.Join(outputDir, rawFile), string(out)); writeErr != nil && err == nil {
		return nil, releasegate.RunSummary{}, writeErr
	}
	summary, parseErr := releasegate.ParseGoTestJSON(out, mode)
	if parseErr != nil {
		return out, summary, parseErr
	}
	return out, summary, err
}

func renderJUnitAndSummary(outputDir string, summary releasegate.RunSummary, flakes []string) error {
	junit, err := releasegate.RenderJUnit(summary, "waypoint-release-tests", flakes)
	if err != nil {
		return err
	}
	if err := writeArtifact(filepath.Join(outputDir, "go-test-release.junit.xml"), string(junit)); err != nil {
		return err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "mode=%s\n", summary.Mode)
	fmt.Fprintf(&builder, "passed=%d\nfailed=%d\nskipped=%d\nrelease_critical_skips=%d\nflakes=%d\n", len(summary.Passed), len(summary.Failed), len(summary.Skipped), len(summary.ReleaseCriticalSkips), len(flakes))
	if len(summary.ReleaseCriticalSkips) > 0 {
		fmt.Fprintf(&builder, "release_critical_skip_names=%s\n", strings.Join(summary.ReleaseCriticalSkips, ","))
	}
	if len(flakes) > 0 {
		fmt.Fprintf(&builder, "flake_names=%s\n", strings.Join(flakes, ","))
	}
	return writeArtifact(filepath.Join(outputDir, "go-test-release-summary.txt"), builder.String())
}

func maybeRunPluginCompatibility(ctx context.Context, outputDir, pluginsRoot string) error {
	if strings.TrimSpace(pluginsRoot) == "" {
		return writeArtifact(filepath.Join(outputDir, "plugins-compatibility.txt"), "plugins_root=unset\n")
	}
	script := filepath.Join(pluginsRoot, "scripts", "verify-contracts.py")
	if _, err := os.Stat(script); err != nil {
		return writeArtifact(filepath.Join(outputDir, "plugins-compatibility.txt"), "plugins_verify_contracts=unavailable\n")
	}
	cmd := exec.CommandContext(ctx, "python3", script)
	cmd.Dir = pluginsRoot
	return runAndCapture(outputDir, "plugins-compatibility.txt", cmd)
}

func runAndCapture(outputDir, file string, cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if writeErr := writeArtifact(filepath.Join(outputDir, file), string(out)); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

func writeArtifact(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

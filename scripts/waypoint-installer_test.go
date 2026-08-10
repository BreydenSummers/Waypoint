package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerValidatesInstallsUpgradesAndRollsBack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "waypoint")
	stateRoot := filepath.Join(root, "var", "lib", "waypoint")
	logRoot := filepath.Join(root, "var", "log", "waypoint")
	workDir := filepath.Join(root, "work")
	mustMkdirAll(t, workDir)

	osRelease := filepath.Join(root, "os-release")
	mustWriteFile(t, osRelease, "ID=ubuntu\nVERSION_ID=\"24.04\"\n")

	packagePath := filepath.Join(root, "waypoint-bin")
	mustWriteFile(t, packagePath, "#!/bin/sh\necho waypoint\n")
	if err := os.Chmod(packagePath, 0o755); err != nil {
		t.Fatalf("chmod package: %v", err)
	}

	configPath := filepath.Join(root, "installer.env")
	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.0",
		"WAYPOINT_PUBLIC_URL=https://waypoint.example.test",
		"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
		"WAYPOINT_PACKAGE_PATH=" + packagePath,
		"WAYPOINT_INSTALL_ROOT=" + installRoot,
		"WAYPOINT_STATE_ROOT=" + stateRoot,
		"WAYPOINT_LOG_ROOT=" + logRoot,
		"",
	}, "\n"))

	provisionPath := filepath.Join(root, "provision.json")
	provision := map[string]any{
		"engagement": map[string]any{
			"id":     "00000000-0000-0000-0000-000000000001",
			"name":   "Demo",
			"client": "Client",
			"scope":  "Campus",
			"status": "active",
		},
		"actors": []any{
			map[string]any{
				"id":     "00000000-0000-0000-0000-000000000010",
				"kind":   "human",
				"handle": "alice",
				"role":   "owner",
			},
			map[string]any{
				"id":            "00000000-0000-0000-0000-000000000011",
				"kind":          "ai_agent",
				"handle":        "waypoint-bot",
				"role":          "operator",
				"agent_name":    "Waypoint",
				"model":         "gpt-4.1",
				"version":       "1.0",
				"authorized_by": "00000000-0000-0000-0000-000000000010",
			},
		},
	}
	mustWriteJSON(t, provisionPath, provision)

	env := baseInstallerEnv(osRelease)
	env = append(env,
		"WAYPOINT_INSTALLER_SKIP_DB=1",
		"WAYPOINT_INSTALLER_UNAME_MACHINE=x86_64",
	)

	scriptPath := installerScriptPath(t)

	validate := runInstaller(t, scriptPath, env, workDir, "validate", "--config", configPath, "--provision", provisionPath)
	if !strings.Contains(validate, "config-ok") {
		t.Fatalf("validate output = %q, want config-ok", validate)
	}

	runInstaller(t, scriptPath, env, workDir, "install", "--config", configPath, "--provision", provisionPath)

	current := filepath.Join(installRoot, "current")
	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("read current symlink: %v", err)
	}
	if !strings.HasSuffix(target, filepath.Join("releases", "1.0.0")) {
		t.Fatalf("current symlink = %q, want release 1.0.0", target)
	}

	servicePath := filepath.Join(target, "systemd", "waypoint.service")
	if _, err := os.Stat(servicePath); err != nil {
		t.Fatalf("service file missing: %v", err)
	}

	statePath := filepath.Join(stateRoot, "install.state")
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	humanToken := readToken(t, filepath.Join(stateRoot, "tokens", "00000000-0000-0000-0000-000000000010.token"))
	aiToken := readToken(t, filepath.Join(stateRoot, "tokens", "00000000-0000-0000-0000-000000000011.token"))
	if humanToken == aiToken || humanToken == "" || aiToken == "" {
		t.Fatalf("unexpected tokens: human=%q ai=%q", humanToken, aiToken)
	}

	runInstaller(t, scriptPath, env, workDir, "install", "--config", configPath, "--provision", provisionPath)
	stateAfter, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if string(stateBefore) != string(stateAfter) {
		t.Fatalf("idempotent install changed state:\nbefore=%s\nafter=%s", stateBefore, stateAfter)
	}
	if got := readToken(t, filepath.Join(stateRoot, "tokens", "00000000-0000-0000-0000-000000000010.token")); got != humanToken {
		t.Fatalf("human token rotated on repeated install: %q -> %q", humanToken, got)
	}

	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.1",
		"WAYPOINT_PUBLIC_URL=https://waypoint.example.test",
		"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
		"WAYPOINT_PACKAGE_PATH=" + packagePath,
		"WAYPOINT_INSTALL_ROOT=" + installRoot,
		"WAYPOINT_STATE_ROOT=" + stateRoot,
		"WAYPOINT_LOG_ROOT=" + logRoot,
		"",
	}, "\n"))
	runInstaller(t, scriptPath, env, workDir, "upgrade", "--config", configPath, "--provision", provisionPath)
	target, err = os.Readlink(current)
	if err != nil {
		t.Fatalf("read upgraded symlink: %v", err)
	}
	if !strings.HasSuffix(target, filepath.Join("releases", "1.0.1")) {
		t.Fatalf("current symlink after upgrade = %q, want release 1.0.1", target)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "rollback")); err != nil {
		t.Fatalf("rollback backup not created: %v", err)
	}

	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.2",
		"WAYPOINT_PUBLIC_URL=https://waypoint.example.test",
		"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
		"WAYPOINT_PACKAGE_PATH=" + packagePath,
		"WAYPOINT_INSTALL_ROOT=" + installRoot,
		"WAYPOINT_STATE_ROOT=" + stateRoot,
		"WAYPOINT_LOG_ROOT=" + logRoot,
		"",
	}, "\n"))
	failed := runInstallerExpectFailure(t, scriptPath, append(env, "WAYPOINT_INSTALLER_FAIL_AT=after_release"), workDir, "install", "--config", configPath, "--provision", provisionPath)
	if !strings.Contains(failed, "failure injected after release materialization") {
		t.Fatalf("unexpected failure output: %s", failed)
	}
	target, err = os.Readlink(current)
	if err != nil {
		t.Fatalf("read current after rollback: %v", err)
	}
	if !strings.HasSuffix(target, filepath.Join("releases", "1.0.1")) {
		t.Fatalf("rollback did not restore 1.0.1: %q", target)
	}
	failureLog := filepath.Join(logRoot, "installer", "last-failure.txt")
	data, err := os.ReadFile(failureLog)
	if err != nil {
		t.Fatalf("read failure log: %v", err)
	}
	if !strings.Contains(string(data), "after_release") {
		t.Fatalf("failure log missing injected fail point: %s", data)
	}
}

func installerScriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "waypoint-installer.sh")
}

func baseInstallerEnv(osRelease string) []string {
	return append(os.Environ(),
		"WAYPOINT_INSTALLER_OS_RELEASE_FILE="+osRelease,
	)
}

func runInstaller(t *testing.T, scriptPath string, env []string, workDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func runInstallerExpectFailure(t *testing.T, scriptPath string, env []string, workDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("installer %v unexpectedly succeeded: %s", args, out)
	}
	return string(out)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustWriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	mustWriteFile(t, path, string(b)+"\n")
}

func readToken(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

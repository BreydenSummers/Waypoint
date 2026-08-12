package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	stubDir := filepath.Join(root, "stubs")
	mustMkdirAll(t, stubDir)
	stubLog := filepath.Join(root, "installer-calls.log")
	mustWriteExecutable(t, filepath.Join(stubDir, "systemctl"), "#!/bin/sh\necho systemctl \"$*\" >> \"$INSTALLER_STUB_LOG\"\ncase \"$1\" in\n  is-active) echo active ;;\nesac\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "psql"), "#!/bin/sh\necho psql \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "pg_isready"), "#!/bin/sh\necho pg_isready \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")

	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer readyServer.Close()

	configPath := filepath.Join(root, "installer.env")
	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.0",
		"WAYPOINT_PUBLIC_URL=" + readyServer.URL,
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
		"WAYPOINT_INSTALLER_UNAME_MACHINE=x86_64",
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"INSTALLER_STUB_LOG="+stubLog,
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

	installedLog, err := os.ReadFile(stubLog)
	if err != nil {
		t.Fatalf("read installer log: %v", err)
	}
	for _, want := range []string{"systemctl enable --now postgresql", "systemctl enable waypoint", "systemctl restart waypoint", "psql postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable -X -v ON_ERROR_STOP=1 -f", "pg_isready -d"} {
		if !strings.Contains(string(installedLog), want) {
			t.Fatalf("installer log missing %q:\n%s", want, installedLog)
		}
	}

	diagnostics := runInstaller(t, scriptPath, env, workDir, "diagnostics", "--config", configPath, "--provision", provisionPath)
	for _, want := range []string{"waypoint_service=active", "database_ready=ready", "installed_version=1.0.0"} {
		if !strings.Contains(diagnostics, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics)
		}
	}

	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.1",
		"WAYPOINT_PUBLIC_URL=" + readyServer.URL,
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
		"WAYPOINT_PUBLIC_URL=" + readyServer.URL,
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

func mustWriteExecutable(t *testing.T, path, content string) {
	t.Helper()
	mustWriteFile(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func readToken(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

type destroyFixture struct {
	scriptPath  string
	workDir     string
	configPath  string
	bundlePath  string
	receiptPath string
	installRoot string
	stateRoot   string
	logRoot     string
	env         []string
}

func TestInstallerDestroyRequiresVerifiedReceiptAndSupportsBreakGlass(t *testing.T) {
	blocked := setupDestroyFixture(t)
	wrongBundle := filepath.Join(filepath.Dir(blocked.bundlePath), "wrong.tar")
	mustWriteFile(t, wrongBundle, "wrong bundle")

	failed := runInstallerExpectFailure(t, blocked.scriptPath, blocked.env, blocked.workDir, "destroy", "--config", blocked.configPath, "--bundle", blocked.bundlePath)
	if !strings.Contains(failed, "--receipt is required") {
		t.Fatalf("destroy without receipt failed with %q, want receipt guard", failed)
	}
	if _, err := os.Stat(blocked.stateRoot); err != nil {
		t.Fatalf("state root removed on guarded failure: %v", err)
	}

	failed = runInstallerExpectFailure(t, blocked.scriptPath, blocked.env, blocked.workDir, "destroy", "--config", blocked.configPath, "--bundle", wrongBundle, "--receipt", blocked.receiptPath)
	if !strings.Contains(failed, "does not match the requested bundle") {
		t.Fatalf("destroy with wrong bundle failed with %q, want bundle-path guard", failed)
	}
	if _, err := os.Stat(blocked.installRoot); err != nil {
		t.Fatalf("install root removed on failed destroy: %v", err)
	}

	runInstaller(t, blocked.scriptPath, blocked.env, blocked.workDir, "destroy", "--config", blocked.configPath, "--bundle", blocked.bundlePath, "--receipt", blocked.receiptPath)
	for _, path := range []string{blocked.installRoot, blocked.stateRoot, blocked.logRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got err=%v", path, err)
		}
	}
	installedLog, err := os.ReadFile(filepath.Join(filepath.Dir(blocked.configPath), "installer-calls.log"))
	if err != nil {
		t.Fatalf("read installer log: %v", err)
	}
	for _, want := range []string{"systemctl stop waypoint", "systemctl stop postgresql"} {
		if !strings.Contains(string(installedLog), want) {
			t.Fatalf("installer log missing %q:\n%s", want, installedLog)
		}
	}

	breakGlass := setupDestroyFixture(t)
	runInstaller(t, breakGlass.scriptPath, breakGlass.env, breakGlass.workDir, "destroy", "--config", breakGlass.configPath, "--bundle", breakGlass.bundlePath, "--force")
	for _, path := range []string{breakGlass.installRoot, breakGlass.stateRoot, breakGlass.logRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed by break-glass teardown, got err=%v", path, err)
		}
	}
}

func setupDestroyFixture(t *testing.T) destroyFixture {
	t.Helper()

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

	stubDir := filepath.Join(root, "stubs")
	mustMkdirAll(t, stubDir)
	stubLog := filepath.Join(root, "installer-calls.log")
	mustWriteExecutable(t, filepath.Join(stubDir, "systemctl"), "#!/bin/sh\necho systemctl \"$*\" >> \"$INSTALLER_STUB_LOG\"\ncase \"$1\" in\n  is-active) echo active ;;\nesac\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "psql"), "#!/bin/sh\necho psql \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "pg_isready"), "#!/bin/sh\necho pg_isready \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")

	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	t.Cleanup(readyServer.Close)

	configPath := filepath.Join(root, "installer.env")
	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.0",
		"WAYPOINT_PUBLIC_URL=" + readyServer.URL,
		"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
		"WAYPOINT_PACKAGE_PATH=" + packagePath,
		"WAYPOINT_INSTALL_ROOT=" + installRoot,
		"WAYPOINT_STATE_ROOT=" + stateRoot,
		"WAYPOINT_LOG_ROOT=" + logRoot,
		"",
	}, "\n"))

	bundlePath := filepath.Join(root, "bundle", "verified-export.tar")
	mustWriteFile(t, bundlePath, "bundle")
	receiptPath := filepath.Join(root, "receipt.json")
	mustWriteJSON(t, receiptPath, map[string]any{
		"status":       "verified",
		"receiptId":    "receipt-q3-2025-01-10",
		"verifiedAt":   "2025-01-10T09:02:14Z",
		"bundlePath":   bundlePath,
		"manifestHash": "8e0f1d2c3b4a59687766554433221100ffeeddccbbaa99887766554433221100",
	})

	env := baseInstallerEnv(osRelease)
	env = append(env,
		"WAYPOINT_INSTALLER_UNAME_MACHINE=x86_64",
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"INSTALLER_STUB_LOG="+stubLog,
	)

	scriptPath := installerScriptPath(t)
	runInstaller(t, scriptPath, env, workDir, "install", "--config", configPath)

	return destroyFixture{
		scriptPath:  scriptPath,
		workDir:     workDir,
		configPath:  configPath,
		bundlePath:  bundlePath,
		receiptPath: receiptPath,
		installRoot: installRoot,
		stateRoot:   stateRoot,
		logRoot:     logRoot,
		env:         env,
	}
}

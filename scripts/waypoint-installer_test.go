package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
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
	mustWriteExecutable(t, filepath.Join(stubDir, "sudo"), "#!/bin/sh\necho sudo \"$*\" >> \"$INSTALLER_STUB_LOG\"\nwhile [ $# -gt 0 ]; do\n  case \"$1\" in\n    -n|-H|-E) shift ;;\n    -u) shift 2 ;;\n    --) shift; break ;;\n    *) break ;;\n  esac\ndone\nexec \"$@\"\n")
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
		"WAYPOINT_TLS_CERT_FILE=/etc/waypoint/tls/server.crt",
		"WAYPOINT_TLS_KEY_FILE=/etc/waypoint/tls/server.key",
		"WAYPOINT_EGRESS_MODE=auto",
		"WAYPOINT_EGRESS_ENDPOINT=https://egress.waypoint.example/resolve",
		"WAYPOINT_EGRESS_ADDRESS=198.51.100.10",
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
	assertFileMode(t, filepath.Join(stateRoot, "config.env"), 0o600)
	assertFileMode(t, filepath.Join(target, "waypoint.env"), 0o600)
	configData, err := os.ReadFile(filepath.Join(stateRoot, "config.env"))
	if err != nil {
		t.Fatalf("read config.env: %v", err)
	}
	for _, want := range []string{"WAYPOINT_TLS_CERT_FILE=/etc/waypoint/tls/server.crt", "WAYPOINT_EGRESS_MODE=auto", "WAYPOINT_EGRESS_ADDRESS=198.51.100.10", "WAYPOINT_ADDR=127.0.0.1:8080"} {
		if !strings.Contains(string(configData), want) {
			t.Fatalf("config.env missing %q:\n%s", want, configData)
		}
	}
	waypointEnv, err := os.ReadFile(filepath.Join(target, "waypoint.env"))
	if err != nil {
		t.Fatalf("read waypoint.env: %v", err)
	}
	for _, want := range []string{"WAYPOINT_TLS_KEY_FILE=/etc/waypoint/tls/server.key", "WAYPOINT_EGRESS_ENDPOINT=https://egress.waypoint.example/resolve", "WAYPOINT_ADDR=127.0.0.1:8080"} {
		if !strings.Contains(string(waypointEnv), want) {
			t.Fatalf("waypoint.env missing %q:\n%s", want, waypointEnv)
		}
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
	bootstrapIdx := strings.Index(string(installedLog), "CREATE ROLE")
	databaseIdx := strings.Index(string(installedLog), "CREATE DATABASE")
	readyIdx := strings.Index(string(installedLog), "pg_isready -d postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable")
	if bootstrapIdx == -1 || databaseIdx == -1 || readyIdx == -1 || bootstrapIdx > readyIdx || databaseIdx > readyIdx {
		t.Fatalf("bootstrap did not precede configured readiness check:\n%s", installedLog)
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
		"WAYPOINT_TLS_CERT_FILE=/etc/waypoint/tls/server.crt",
		"WAYPOINT_TLS_KEY_FILE=/etc/waypoint/tls/server.key",
		"WAYPOINT_EGRESS_MODE=auto",
		"WAYPOINT_EGRESS_ENDPOINT=https://egress.waypoint.example/resolve",
		"WAYPOINT_EGRESS_ADDRESS=198.51.100.10",
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
		"WAYPOINT_TLS_CERT_FILE=/etc/waypoint/tls/server.crt",
		"WAYPOINT_TLS_KEY_FILE=/etc/waypoint/tls/server.key",
		"WAYPOINT_EGRESS_MODE=auto",
		"WAYPOINT_EGRESS_ENDPOINT=https://egress.waypoint.example/resolve",
		"WAYPOINT_EGRESS_ADDRESS=198.51.100.10",
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
	for _, forbidden := range []string{"postgres://", "server.crt", "server.key", "destroy-token"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("failure log leaked %q: %s", forbidden, data)
		}
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

func mustWriteVerifiedBundle(t *testing.T, root string) (string, string, string) {
	t.Helper()
	bundleDir := filepath.Join(root, "bundle-src")
	mustMkdirAll(t, filepath.Join(bundleDir, "database"))
	mustMkdirAll(t, filepath.Join(bundleDir, "metadata"))
	mustMkdirAll(t, filepath.Join(bundleDir, "report"))
	mustMkdirAll(t, filepath.Join(bundleDir, "tools"))

	mustWriteFile(t, filepath.Join(bundleDir, "database", "engagement.dump"), "dump\n")
	mustWriteFile(t, filepath.Join(bundleDir, "metadata", "export-metadata.json"), "{\"export\":\"metadata\"}\n")
	mustWriteFile(t, filepath.Join(bundleDir, "report", "report-snapshot.json"), "snapshot\n")
	mustWriteFile(t, filepath.Join(bundleDir, "report", "frozen-report.pdf"), "%PDF-1.4\n")
	mustWriteFile(t, filepath.Join(bundleDir, "tools", "verify-restore.mjs"), "console.log('verify')\n")

	payloads := []map[string]any{
		{"path": "bundle/database/engagement.dump", "byteLength": int64(len("dump\n")), "sha256": sha256HexString("dump\n"), "kind": "database"},
		{"path": "bundle/metadata/export-metadata.json", "byteLength": int64(len("{\"export\":\"metadata\"}\n")), "sha256": sha256HexString("{\"export\":\"metadata\"}\n"), "kind": "metadata"},
		{"path": "bundle/report/report-snapshot.json", "byteLength": int64(len("snapshot\n")), "sha256": sha256HexString("snapshot\n"), "kind": "report_snapshot"},
		{"path": "bundle/report/frozen-report.pdf", "byteLength": int64(len("%PDF-1.4\n")), "sha256": sha256HexString("%PDF-1.4\n"), "kind": "pdf"},
		{"path": "bundle/tools/verify-restore.mjs", "byteLength": int64(len("console.log('verify')\n")), "sha256": sha256HexString("console.log('verify')\n"), "kind": "tool"},
	}
	manifest := map[string]any{
		"formatVersion": "1.0.0",
		"exportJobId":   "77777777-7777-4777-8777-777777777777",
		"engagementId":  "11111111-1111-4111-8111-111111111111",
		"cutoff":        "2025-01-10T09:02:14Z",
		"payloads":      payloads,
		"signatures":    map[string]any{"version": "v1", "items": []string{}},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal bundle manifest: %v", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	mustWriteFile(t, filepath.Join(bundleDir, "metadata", "export-manifest.json"), string(manifestBytes))

	archivePath := filepath.Join(root, "bundle", "verified-export.tar.gz")
	mustWriteTarGz(t, archivePath, bundleDir)
	return archivePath, sha256HexFile(t, archivePath), sha256HexBytes(manifestBytes)
}

func mustWriteTamperedBundleWithExtraEntry(t *testing.T, sourceArchivePath, destinationArchivePath string) string {
	t.Helper()
	src, err := os.Open(sourceArchivePath)
	if err != nil {
		t.Fatalf("open source archive: %v", err)
	}
	defer src.Close()
	gz, err := gzip.NewReader(src)
	if err != nil {
		t.Fatalf("read source archive: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	if err := os.MkdirAll(filepath.Dir(destinationArchivePath), 0o755); err != nil {
		t.Fatalf("mkdir destination archive dir: %v", err)
	}
	dst, err := os.Create(destinationArchivePath)
	if err != nil {
		t.Fatalf("create destination archive: %v", err)
	}
	defer dst.Close()
	gw := gzip.NewWriter(dst)
	gw.Header.ModTime = zeroTime
	gw.Header.OS = 255
	zw := tar.NewWriter(gw)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read source tar entry: %v", err)
		}
		copyHdr := *hdr
		copyHdr.ModTime = zeroTime
		copyHdr.AccessTime = time.Time{}
		copyHdr.ChangeTime = time.Time{}
		if err := zw.WriteHeader(&copyHdr); err != nil {
			t.Fatalf("write source tar header %s: %v", hdr.Name, err)
		}
		if hdr.Size > 0 {
			if _, err := io.Copy(zw, tr); err != nil {
				t.Fatalf("copy source tar entry %s: %v", hdr.Name, err)
			}
		}
	}

	extra := []byte("teardown review note\n")
	if err := zw.WriteHeader(&tar.Header{Name: "bundle/notes/teardown-review.txt", Mode: 0o600, Size: int64(len(extra)), ModTime: zeroTime, Uid: 0, Gid: 0}); err != nil {
		t.Fatalf("write extra tar header: %v", err)
	}
	if _, err := zw.Write(extra); err != nil {
		t.Fatalf("write extra tar entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return sha256HexFile(t, destinationArchivePath)
}

func mustWriteTarGz(t *testing.T, archivePath, bundleDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gw := gzip.NewWriter(f)
	gw.Header.ModTime = zeroTime
	gw.Header.OS = 255
	zw := tar.NewWriter(gw)
	var files []string
	if err := filepath.Walk(bundleDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == bundleDir || info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		t.Fatalf("walk bundle dir: %v", err)
	}
	sort.Strings(files)
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		rel, err := filepath.Rel(bundleDir, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		hdr := &tar.Header{Name: filepath.ToSlash(filepath.Join("bundle", rel)), Mode: int64(info.Mode().Perm()), Size: info.Size(), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime, Uid: 0, Gid: 0}
		if err := zw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if _, err := zw.Write(data); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func sha256HexString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sha256HexBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func sha256HexFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return sha256HexBytes(data)
}

var zeroTime = time.Unix(0, 0).UTC()

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

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", path, got, want)
	}
}

type destroyFixture struct {
	scriptPath    string
	workDir       string
	configPath    string
	provisionPath string
	bundlePath    string
	receiptPath   string
	installRoot   string
	stateRoot     string
	logRoot       string
	env           []string
}

func TestInstallerDestroyRequiresVerifiedReceiptAndSupportsBreakGlass(t *testing.T) {
	var probe struct {
		mu                  sync.Mutex
		createReqBody       []byte
		consumeReqBody      []byte
		createSeen          bool
		consumeSeen         bool
		expectedArchiveSHA  string
		expectedManifestSHA string
		expectedBundlePath  string
	}
	teardownServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/readyz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case r.URL.Path == "/api/v1/teardown-authorizations" && r.Method == http.MethodPost:
			if got := r.Header.Get("Authorization"); got != "Bearer destroy-token" {
				t.Fatalf("create authorization auth header = %q", got)
			}
			if got := r.Header.Get("Waypoint-Contract-Version"); got != "1.0.0" {
				t.Fatalf("create authorization contract version = %q", got)
			}
			createReqBody, _ := io.ReadAll(r.Body)
			probe.mu.Lock()
			probe.createSeen = true
			probe.createReqBody = createReqBody
			expectedBundlePath := probe.expectedBundlePath
			expectedArchiveSHA := probe.expectedArchiveSHA
			expectedManifestSHA := probe.expectedManifestSHA
			probe.mu.Unlock()
			var req struct {
				ReceiptID      string `json:"receiptId"`
				BundlePath     string `json:"bundlePath"`
				ArchiveSHA256  string `json:"archiveSha256"`
				ManifestSHA256 string `json:"manifestSha256"`
				Confirmation   string `json:"confirmation"`
			}
			if err := json.Unmarshal(createReqBody, &req); err != nil {
				t.Fatalf("decode create authorization body: %v", err)
			}
			if req.BundlePath != expectedBundlePath || req.ArchiveSHA256 != expectedArchiveSHA || req.ManifestSHA256 != expectedManifestSHA || req.Confirmation != "destroy verified engagement data" {
				t.Fatalf("create authorization body = %#v expected bundle=%q archive=%q manifest=%q", req, expectedBundlePath, expectedArchiveSHA, expectedManifestSHA)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"contractVersion":"1.0.0","id":"grant-1","engagementId":"11111111-1111-4111-8111-111111111111","receiptId":"receipt-q3-2025-01-10","exportJobId":"77777777-7777-4777-8777-777777777777","bundlePath":"` + expectedBundlePath + `","archiveSha256":"` + expectedArchiveSHA + `","manifestSha256":"` + expectedManifestSHA + `","requestedBy":{"id":"00000000-0000-0000-0000-000000000010","kind":"human","handle":"alice","role":"owner"},"requestedAt":"2025-01-10T09:02:14Z","expiresAt":"2025-01-10T09:07:14Z","status":"authorized"}`))
		case r.URL.Path == "/api/v1/teardown-authorizations/grant-1/consume" && r.Method == http.MethodPost:
			if got := r.Header.Get("Authorization"); got != "Bearer destroy-token" {
				t.Fatalf("consume authorization auth header = %q", got)
			}
			consumeReqBody, _ := io.ReadAll(r.Body)
			probe.mu.Lock()
			probe.consumeSeen = true
			probe.consumeReqBody = consumeReqBody
			expectedBundlePath := probe.expectedBundlePath
			expectedArchiveSHA := probe.expectedArchiveSHA
			expectedManifestSHA := probe.expectedManifestSHA
			probe.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"contractVersion":"1.0.0","id":"grant-1","engagementId":"11111111-1111-4111-8111-111111111111","receiptId":"receipt-q3-2025-01-10","exportJobId":"77777777-7777-4777-8777-777777777777","bundlePath":"` + expectedBundlePath + `","archiveSha256":"` + expectedArchiveSHA + `","manifestSha256":"` + expectedManifestSHA + `","requestedBy":{"id":"00000000-0000-0000-0000-000000000010","kind":"human","handle":"alice","role":"owner"},"requestedAt":"2025-01-10T09:02:14Z","expiresAt":"2025-01-10T09:07:14Z","status":"consumed","consumedAt":"2025-01-10T09:02:15Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(teardownServer.Close)

	blocked := setupDestroyFixtureForHost(t, "24.04", "x86_64", teardownServer.URL)
	var receipt struct {
		BundlePath     string `json:"bundlePath"`
		ArchiveSHA256  string `json:"archiveSha256"`
		ManifestSHA256 string `json:"manifestSha256"`
	}
	receiptBytes, err := os.ReadFile(blocked.receiptPath)
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	probe.mu.Lock()
	probe.expectedBundlePath = receipt.BundlePath
	probe.expectedArchiveSHA = receipt.ArchiveSHA256
	probe.expectedManifestSHA = receipt.ManifestSHA256
	probe.mu.Unlock()
	wrongBundle := filepath.Join(filepath.Dir(blocked.bundlePath), "wrong.tar.gz")
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

	tamperedBundle := filepath.Join(filepath.Dir(blocked.bundlePath), "tampered.tar.gz")
	tamperedArchiveSHA := mustWriteTamperedBundleWithExtraEntry(t, blocked.bundlePath, tamperedBundle)
	mustWriteJSON(t, blocked.receiptPath, map[string]any{
		"status":         "verified",
		"receiptId":      "receipt-q3-2025-01-10",
		"verifiedAt":     "2025-01-10T09:02:14Z",
		"bundlePath":     tamperedBundle,
		"archiveSha256":  tamperedArchiveSHA,
		"manifestSha256": receipt.ManifestSHA256,
	})
	probe.mu.Lock()
	probe.expectedBundlePath = tamperedBundle
	probe.expectedArchiveSHA = tamperedArchiveSHA
	probe.mu.Unlock()

	failed = runInstallerExpectFailure(t, blocked.scriptPath, blocked.env, blocked.workDir, "destroy", "--config", blocked.configPath, "--bundle", tamperedBundle, "--receipt", blocked.receiptPath)
	if !strings.Contains(failed, "unexpected archive entry") {
		t.Fatalf("destroy with extra archive entry failed with %q, want archive inventory guard", failed)
	}
	if _, err := os.Stat(blocked.installRoot); err != nil {
		t.Fatalf("install root removed on archive guard failure: %v", err)
	}

	mustWriteJSON(t, blocked.receiptPath, map[string]any{
		"status":         "verified",
		"receiptId":      "receipt-q3-2025-01-10",
		"verifiedAt":     "2025-01-10T09:02:14Z",
		"bundlePath":     receipt.BundlePath,
		"archiveSha256":  receipt.ArchiveSHA256,
		"manifestSha256": receipt.ManifestSHA256,
	})
	probe.mu.Lock()
	probe.expectedBundlePath = receipt.BundlePath
	probe.expectedArchiveSHA = receipt.ArchiveSHA256
	probe.expectedManifestSHA = receipt.ManifestSHA256
	probe.mu.Unlock()

	destroyOutput := runInstaller(t, blocked.scriptPath, blocked.env, blocked.workDir, "destroy", "--config", blocked.configPath, "--bundle", blocked.bundlePath, "--receipt", blocked.receiptPath)
	if !strings.Contains(destroyOutput, "teardown authorization requested") || !strings.Contains(destroyOutput, "teardown authorization consumed") {
		t.Fatalf("destroy output missing authorization lifecycle:\n%s", destroyOutput)
	}
	for _, path := range []string{blocked.installRoot, blocked.stateRoot, blocked.logRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got err=%v", path, err)
		}
	}
	probe.mu.Lock()
	createSeen := probe.createSeen
	consumeSeen := probe.consumeSeen
	createReqBody := append([]byte(nil), probe.createReqBody...)
	consumeReqBody := append([]byte(nil), probe.consumeReqBody...)
	probe.mu.Unlock()
	if !createSeen || !consumeSeen {
		t.Fatalf("teardown authorization calls missing: create=%q consume=%q", createReqBody, consumeReqBody)
	}
	for _, want := range []string{`"receiptId":`, `"archiveSha256":`, `"manifestSha256":`} {
		if !strings.Contains(string(createReqBody), want) {
			t.Fatalf("create authorization body missing %q: %s", want, createReqBody)
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

func TestInstallerSupportsSupportedHostsAndFreshRestartAfterTeardown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		osVer   string
		machine string
	}{
		{name: "ubuntu-22.04-x86_64", osVer: "22.04", machine: "x86_64"},
		{name: "ubuntu-24.04-x86_64", osVer: "24.04", machine: "x86_64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/readyz" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ready"}`))
			}))
			t.Cleanup(readyServer.Close)
			fixture := setupDestroyFixtureForHost(t, tc.osVer, tc.machine, readyServer.URL)
			statePath := filepath.Join(fixture.stateRoot, "install.state")
			firstHumanToken := readToken(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000010.token"))
			firstAIToken := readToken(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000011.token"))
			if firstHumanToken == firstAIToken || firstHumanToken == "" || firstAIToken == "" {
				t.Fatalf("unexpected provision tokens: human=%q ai=%q", firstHumanToken, firstAIToken)
			}
			assertFileMode(t, filepath.Join(fixture.stateRoot, "config.env"), 0o600)
			assertFileMode(t, filepath.Join(fixture.stateRoot, "install.state"), 0o600)
			assertFileMode(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000010.token"), 0o600)
			assertFileMode(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000011.token"), 0o600)
			assertFileMode(t, filepath.Join(fixture.stateRoot, "last-provision.sql"), 0o600)

			diagnostics := runInstaller(t, fixture.scriptPath, fixture.env, fixture.workDir, "diagnostics", "--config", fixture.configPath, "--provision", fixture.provisionPath)
			for _, want := range []string{"waypoint_service=active", "database_ready=ready", "installed_version=1.0.0"} {
				if !strings.Contains(diagnostics, want) {
					t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics)
				}
			}

			runInstaller(t, fixture.scriptPath, fixture.env, fixture.workDir, "destroy", "--config", fixture.configPath, "--bundle", fixture.bundlePath, "--receipt", fixture.receiptPath, "--force")
			for _, path := range []string{fixture.installRoot, fixture.stateRoot, fixture.logRoot} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected %s to be removed after teardown, got err=%v", path, err)
				}
			}

			runInstaller(t, fixture.scriptPath, fixture.env, fixture.workDir, "install", "--config", fixture.configPath, "--provision", fixture.provisionPath)
			reinstalledState, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read reinstated state: %v", err)
			}
			if !strings.Contains(string(reinstalledState), "WAYPOINT_INSTALLED_VERSION=1.0.0") {
				t.Fatalf("reinstalled state missing version marker: %s", reinstalledState)
			}
			secondHumanToken := readToken(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000010.token"))
			secondAIToken := readToken(t, filepath.Join(fixture.stateRoot, "tokens", "00000000-0000-0000-0000-000000000011.token"))
			if secondHumanToken == firstHumanToken || secondAIToken == firstAIToken {
				t.Fatalf("fresh restart reused prior tokens: human=%q->%q ai=%q->%q", firstHumanToken, secondHumanToken, firstAIToken, secondAIToken)
			}
			if _, err := os.Stat(filepath.Join(fixture.stateRoot, "rollback")); !os.IsNotExist(err) {
				t.Fatalf("fresh restart should not leave rollback state behind: %v", err)
			}
		})
	}
}

func TestInstallerRollbackAcceptsTargetVersionAndPreservesState(t *testing.T) {
	t.Parallel()

	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	t.Cleanup(readyServer.Close)
	fixture := setupDestroyFixtureForHost(t, "24.04", "x86_64", readyServer.URL)
	before, err := os.ReadFile(filepath.Join(fixture.stateRoot, "install.state"))
	if err != nil {
		t.Fatalf("read state before rollback: %v", err)
	}

	configData, err := os.ReadFile(fixture.configPath)
	if err != nil {
		t.Fatalf("read config before upgrade: %v", err)
	}
	updatedConfig := strings.Replace(string(configData), "WAYPOINT_VERSION=1.0.0", "WAYPOINT_VERSION=1.0.1", 1)
	mustWriteFile(t, fixture.configPath, updatedConfig)
	runInstaller(t, fixture.scriptPath, fixture.env, fixture.workDir, "upgrade", "--config", fixture.configPath, "--provision", fixture.provisionPath)

	runInstaller(t, fixture.scriptPath, fixture.env, fixture.workDir, "rollback", "1.0.0", "--config", fixture.configPath, "--provision", fixture.provisionPath)

	target, err := os.Readlink(filepath.Join(fixture.installRoot, "current"))
	if err != nil {
		t.Fatalf("read current after rollback: %v", err)
	}
	if !strings.HasSuffix(target, filepath.Join("releases", "1.0.0")) {
		t.Fatalf("rollback symlink = %q, want release 1.0.0", target)
	}
	after, err := os.ReadFile(filepath.Join(fixture.stateRoot, "install.state"))
	if err != nil {
		t.Fatalf("read state after rollback: %v", err)
	}
	for _, want := range []string{"WAYPOINT_INSTALLED_VERSION=1.0.0", "WAYPOINT_PACKAGE_PATH=", "WAYPOINT_CONFIG_SHA256="} {
		if !strings.Contains(string(after), want) {
			t.Fatalf("rollback state missing %q:\n%s", want, after)
		}
	}
	if !strings.Contains(string(before), "WAYPOINT_PACKAGE_SHA256=") || !strings.Contains(string(after), "WAYPOINT_PACKAGE_SHA256=") {
		t.Fatalf("rollback should preserve package metadata:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestInstallerRejectsUnsupportedHostMatrix(t *testing.T) {
	for _, tc := range []struct {
		name        string
		osVersion   string
		machine     string
		wantErrPart string
	}{
		{name: "unsupported-os", osVersion: "20.04", machine: "x86_64", wantErrPart: "unsupported Ubuntu version"},
		{name: "unsupported-arch", osVersion: "24.04", machine: "arm64", wantErrPart: "unsupported architecture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			workDir := filepath.Join(root, "work")
			mustMkdirAll(t, workDir)

			osRelease := filepath.Join(root, "os-release")
			mustWriteFile(t, osRelease, "ID=ubuntu\nVERSION_ID=\""+tc.osVersion+"\"\n")

			packagePath := filepath.Join(root, "waypoint-bin")
			mustWriteFile(t, packagePath, "#!/bin/sh\necho waypoint\n")
			if err := os.Chmod(packagePath, 0o755); err != nil {
				t.Fatalf("chmod package: %v", err)
			}

			configPath := filepath.Join(root, "installer.env")
			mustWriteFile(t, configPath, strings.Join([]string{
				"WAYPOINT_VERSION=1.0.0",
				"WAYPOINT_PUBLIC_URL=http://127.0.0.1:8080",
				"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
				"WAYPOINT_PACKAGE_PATH=" + packagePath,
				"WAYPOINT_INSTALL_ROOT=" + filepath.Join(root, "opt", "waypoint"),
				"WAYPOINT_STATE_ROOT=" + filepath.Join(root, "var", "lib", "waypoint"),
				"WAYPOINT_LOG_ROOT=" + filepath.Join(root, "var", "log", "waypoint"),
				"",
			}, "\n"))

			scriptPath := installerScriptPath(t)
			env := baseInstallerEnv(osRelease)
			env = append(env, "WAYPOINT_INSTALLER_UNAME_MACHINE="+tc.machine)
			failed := runInstallerExpectFailure(t, scriptPath, env, workDir, "validate", "--config", configPath)
			if !strings.Contains(failed, tc.wantErrPart) {
				t.Fatalf("validate failed with %q, want %q", failed, tc.wantErrPart)
			}
		})
	}
}

func setupDestroyFixture(t *testing.T) destroyFixture {
	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	t.Cleanup(readyServer.Close)
	return setupDestroyFixtureForHost(t, "24.04", "x86_64", readyServer.URL)
}

func setupDestroyFixtureForHost(t *testing.T, osVersion, machine, publicURL string) destroyFixture {
	t.Helper()

	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "waypoint")
	stateRoot := filepath.Join(root, "var", "lib", "waypoint")
	logRoot := filepath.Join(root, "var", "log", "waypoint")
	workDir := filepath.Join(root, "work")
	mustMkdirAll(t, workDir)

	osRelease := filepath.Join(root, "os-release")
	mustWriteFile(t, osRelease, "ID=ubuntu\nVERSION_ID=\""+osVersion+"\"\n")

	packagePath := filepath.Join(root, "waypoint-bin")
	mustWriteFile(t, packagePath, "#!/bin/sh\necho waypoint\n")
	if err := os.Chmod(packagePath, 0o755); err != nil {
		t.Fatalf("chmod package: %v", err)
	}

	stubDir := filepath.Join(root, "stubs")
	mustMkdirAll(t, stubDir)
	stubLog := filepath.Join(root, "installer-calls.log")
	mustWriteExecutable(t, filepath.Join(stubDir, "systemctl"), "#!/bin/sh\necho systemctl \"$*\" >> \"$INSTALLER_STUB_LOG\"\ncase \"$1\" in\n  is-active) echo active ;;\nesac\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "sudo"), "#!/bin/sh\necho sudo \"$*\" >> \"$INSTALLER_STUB_LOG\"\nwhile [ $# -gt 0 ]; do\n  case \"$1\" in\n    -n|-H|-E) shift ;;\n    -u) shift 2 ;;\n    --) shift; break ;;\n    *) break ;;\n  esac\ndone\nexec \"$@\"\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "psql"), "#!/bin/sh\necho psql \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")
	mustWriteExecutable(t, filepath.Join(stubDir, "pg_isready"), "#!/bin/sh\necho pg_isready \"$*\" >> \"$INSTALLER_STUB_LOG\"\nexit 0\n")

	configPath := filepath.Join(root, "installer.env")
	mustWriteFile(t, configPath, strings.Join([]string{
		"WAYPOINT_VERSION=1.0.0",
		"WAYPOINT_PUBLIC_URL=" + publicURL,
		"WAYPOINT_DB_DSN=postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable",
		"WAYPOINT_PACKAGE_PATH=" + packagePath,
		"WAYPOINT_INSTALL_ROOT=" + installRoot,
		"WAYPOINT_STATE_ROOT=" + stateRoot,
		"WAYPOINT_LOG_ROOT=" + logRoot,
		"",
	}, "\n"))

	provisionPath := filepath.Join(root, "provision.json")
	mustWriteJSON(t, provisionPath, map[string]any{
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
	})

	bundlePath, archiveSHA, manifestSHA := mustWriteVerifiedBundle(t, root)
	receiptPath := filepath.Join(root, "receipt.json")
	mustWriteJSON(t, receiptPath, map[string]any{
		"status":         "verified",
		"receiptId":      "receipt-q3-2025-01-10",
		"verifiedAt":     "2025-01-10T09:02:14Z",
		"bundlePath":     bundlePath,
		"archiveSha256":  archiveSHA,
		"manifestSha256": manifestSHA,
	})

	env := baseInstallerEnv(osRelease)
	env = append(env,
		"WAYPOINT_INSTALLER_UNAME_MACHINE="+machine,
		"WAYPOINT_DESTROY_TOKEN=destroy-token",
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"INSTALLER_STUB_LOG="+stubLog,
	)

	scriptPath := installerScriptPath(t)
	runInstaller(t, scriptPath, env, workDir, "install", "--config", configPath, "--provision", provisionPath)

	return destroyFixture{
		scriptPath:    scriptPath,
		workDir:       workDir,
		configPath:    configPath,
		provisionPath: provisionPath,
		bundlePath:    bundlePath,
		receiptPath:   receiptPath,
		installRoot:   installRoot,
		stateRoot:     stateRoot,
		logRoot:       logRoot,
		env:           env,
	}
}

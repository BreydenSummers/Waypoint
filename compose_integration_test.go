package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

const (
	composeTestTimeout    = 15 * time.Minute
	composeStartupTimeout = 5 * time.Minute
	composeCleanupTimeout = 2 * time.Minute
)

func TestComposeStackStartsCleanlyTwice(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon unavailable: %v\n%s", err, out)
	}

	project := fmt.Sprintf("waypoint-compose-clean-%d-%d", os.Getpid(), time.Now().UnixNano())
	overridePath := filepath.Join(t.TempDir(), "compose.override.yml")
	override := []byte("services:\n  waypoint:\n    ports:\n      - target: 8080\n        published: 0\n        protocol: tcp\n")
	if err := os.WriteFile(overridePath, override, 0o600); err != nil {
		t.Fatalf("write compose override: %v", err)
	}

	prepareComposeDockerConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), composeTestTimeout)
	defer cancel()

	runCompose := func(args ...string) string {
		t.Helper()
		out, err := composeOutput(ctx, project, overridePath, args...)
		if err != nil {
			t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), composeCleanupTimeout)
		defer cleanupCancel()
		_, _ = composeOutput(cleanupCtx, project, overridePath, "down", "-v", "--remove-orphans")
	})

	for i := 0; i < 2; i++ {
		buildOut := mustComposeBuildNoCache(t, ctx, project, overridePath)
		t.Logf("docker compose build --no-cache (cycle %d):\n%s", i+1, strings.TrimSpace(buildOut))
		upOut := runCompose("up", "-d", "--wait")
		t.Logf("docker compose up --wait (cycle %d):\n%s", i+1, strings.TrimSpace(upOut))
		assertComposeMigrationState(t, runCompose)
		port := waitForComposePort(t, ctx, project, overridePath)
		baseURL := "http://127.0.0.1:" + port

		waitForHTTP(t, baseURL+"/readyz", http.StatusOK)
		assertHTTPBodyContains(t, baseURL+"/engagements/demo", http.StatusOK, `id="root"`)

		downOut := runCompose("down", "-v", "--remove-orphans")
		t.Logf("docker compose down -v --remove-orphans (cycle %d):\n%s", i+1, strings.TrimSpace(downOut))
		if got := strings.TrimSpace(runCompose("ps", "-q")); got != "" {
			t.Fatalf("cycle %d compose ps = %q, want empty", i+1, got)
		}
		if got := strings.Fields(runCompose("ps", "-q", "postgres")); len(got) != 0 {
			t.Fatalf("cycle %d postgres ps = %v, want empty", i+1, got)
		}
	}
}

func TestComposeStackPersistsDBAndEvidenceAcrossRestart(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon unavailable: %v\n%s", err, out)
	}

	project := fmt.Sprintf("waypoint-compose-%d-%d", os.Getpid(), time.Now().UnixNano())
	overridePath := filepath.Join(t.TempDir(), "compose.override.yml")
	override := []byte("services:\n  waypoint:\n    ports:\n      - target: 8080\n        published: 0\n        protocol: tcp\n")
	if err := os.WriteFile(overridePath, override, 0o600); err != nil {
		t.Fatalf("write compose override: %v", err)
	}

	prepareComposeDockerConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), composeTestTimeout)
	defer cancel()

	runCompose := func(args ...string) string {
		t.Helper()
		out, err := composeOutput(ctx, project, overridePath, args...)
		if err != nil {
			t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), composeCleanupTimeout)
		defer cleanupCancel()
		_, _ = composeOutput(cleanupCtx, project, overridePath, "down", "-v", "--remove-orphans")
	})

	buildOut := mustComposeBuildNoCache(t, ctx, project, overridePath)
	t.Logf("docker compose build --no-cache:\n%s", strings.TrimSpace(buildOut))
	upOut := runCompose("up", "-d", "--wait")
	t.Logf("docker compose up --wait:\n%s", strings.TrimSpace(upOut))
	assertComposeMigrationState(t, runCompose)
	port := waitForComposePort(t, ctx, project, overridePath)
	baseURL := "http://127.0.0.1:" + port

	waitForHTTP(t, baseURL+"/readyz", http.StatusOK)
	assertHTTPBodyContains(t, baseURL+"/engagements/demo", http.StatusOK, `id="root"`)

	if got := strings.Fields(runCompose("ps", "-q")); len(got) != 2 {
		t.Fatalf("compose ps container count = %d, want 2 (%v)", len(got), got)
	}
	if got := strings.Fields(runCompose("ps", "-q", "postgres")); len(got) != 1 {
		t.Fatalf("postgres container count = %d, want 1 (%v)", len(got), got)
	}
	assertComposeMigrationState(t, runCompose)
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", "SELECT COUNT(*) FROM engagement")); got != "0" {
		t.Fatalf("fresh engagement count = %s, want 0", got)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	token := "compose-stack-token"
	tokenHash := sha256Hex(token)
	mustComposeExec(t, ctx, project, overridePath,
		"exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-v", "ON_ERROR_STOP=1", "-c",
		fmt.Sprintf(`INSERT INTO engagement (id, name, client, scope, status) VALUES ('%s', 'Demo', 'Client', 'Scope', 'active'); INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ('%s', '%s', 'human', 'alex.operator', '%s', 'operator');`, engagementID, "22222222-2222-4222-8222-222222222222", engagementID, tokenHash),
	)

	stdout := []byte("operator\n")
	stderr := []byte("warning\n")
	captureURL := baseURL + "/api/v1/captures"
	captureBody := map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"sourceAgent": map[string]any{
			"id":       "44444444-4444-4444-8444-444444444444",
			"kind":     "operator_wrapper",
			"name":     "waypoint-wrapper",
			"version":  "1.0.0",
			"platform": map[string]any{"os": "linux", "arch": "amd64"},
		},
		"phase":       "recon",
		"initiatedBy": "manual",
		"command":     "/usr/bin/whoami",
		"argv":        []string{"whoami"},
		"cwd":         "/home/operator/engagement",
		"target":      map[string]any{"kind": "host", "value": "jumpbox"},
		"timing":      map[string]any{"startedAt": "2025-01-15T10:00:00.000Z", "endedAt": "2025-01-15T10:00:01.000Z", "durationMs": 1000},
		"execution":   map[string]any{"status": "exited", "exitCode": 0},
		"network": map[string]any{
			"execHost":   map[string]any{"address": "10.10.0.12", "method": "route_selection", "confidence": "confirmed"},
			"egress":     map[string]any{"mode": "off", "status": "disabled"},
			"pivotChain": []any{},
		},
		"evidence": map[string]any{
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": sha256Hex(string(stdout))},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": sha256Hex(string(stderr))},
		},
		"parsing": map[string]any{"status": "raw"},
	}
	captureResp := postMultipartCapture(t, captureURL, token, captureBody, stdout, stderr)
	if captureResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(captureResp.Body)
		captureResp.Body.Close()
		t.Fatalf("capture status = %d, want %d: %s", captureResp.StatusCode, http.StatusCreated, body)
	}
	captureResp.Body.Close()

	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM action WHERE engagement_id = '%s'", engagementID))); got != "1" {
		t.Fatalf("action count = %s, want 1", got)
	}
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM evidence WHERE engagement_id = '%s'", engagementID))); got != "2" {
		t.Fatalf("evidence count = %s, want 2", got)
	}

	runCompose("restart", "waypoint")
	waitForHTTP(t, baseURL+"/readyz", http.StatusOK)
	assertHTTPBodyContains(t, baseURL+"/engagements/demo", http.StatusOK, `id="root"`)

	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM action WHERE engagement_id = '%s'", engagementID))); got != "1" {
		t.Fatalf("post-waypoint-restart action count = %s, want 1", got)
	}
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM evidence WHERE engagement_id = '%s'", engagementID))); got != "2" {
		t.Fatalf("post-waypoint-restart evidence count = %s, want 2", got)
	}

	runCompose("stop", "postgres")
	waitForHTTP(t, baseURL+"/readyz", http.StatusServiceUnavailable)

	runCompose("start", "postgres")
	waitForHTTP(t, baseURL+"/readyz", http.StatusOK)
	assertHTTPBodyContains(t, baseURL+"/engagements/demo", http.StatusOK, `id="root"`)

	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM action WHERE engagement_id = '%s'", engagementID))); got != "1" {
		t.Fatalf("post-postgres-restart action count = %s, want 1", got)
	}
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf("SELECT COUNT(*) FROM evidence WHERE engagement_id = '%s'", engagementID))); got != "2" {
		t.Fatalf("post-postgres-restart evidence count = %s, want 2", got)
	}

	stdoutPath := filepath.Join("/var/lib/waypoint/evidence", "captures", sha256Hex(string(stdout)), "stdout")
	stderrPath := filepath.Join("/var/lib/waypoint/evidence", "captures", sha256Hex(string(stderr)), "stderr")
	if got := runCompose("exec", "-T", "waypoint", "sh", "-lc", fmt.Sprintf("cat %s", shellQuote(stdoutPath))); got != string(stdout) {
		t.Fatalf("stdout evidence = %q, want %q", got, stdout)
	}
	if got := runCompose("exec", "-T", "waypoint", "sh", "-lc", fmt.Sprintf("cat %s", shellQuote(stderrPath))); got != string(stderr) {
		t.Fatalf("stderr evidence = %q, want %q", got, stderr)
	}

	downOut := runCompose("down", "-v", "--remove-orphans")
	t.Logf("docker compose down -v --remove-orphans:\n%s", strings.TrimSpace(downOut))
	if got := strings.TrimSpace(runCompose("ps", "-q")); got != "" {
		t.Fatalf("post-teardown compose ps = %q, want empty", got)
	}
	if got := strings.Fields(runCompose("ps", "-q", "postgres")); len(got) != 0 {
		t.Fatalf("post-teardown postgres ps = %v, want empty", got)
	}
}

func assertComposeMigrationState(t *testing.T, runCompose func(args ...string) string) {
	t.Helper()

	if got := strings.Fields(runCompose("ps", "-q", "postgres")); len(got) != 1 {
		t.Fatalf("postgres container count = %d, want 1 (%v)", len(got), got)
	}

	wantVersions, err := dbm.EmbeddedMigrationVersions()
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	gotVersions := strings.Fields(strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", "SELECT version FROM schema_migrations ORDER BY version")))
	if !slices.Equal(gotVersions, wantVersions) {
		t.Fatalf("schema_migrations versions = %v, want %v", gotVersions, wantVersions)
	}

	assertComposeHasTables(t, runCompose, []string{"export_job", "export_receipt", "teardown_authorization"})
	assertComposeHasIndexes(t, runCompose, []string{
		"export_job_state_updated_at_idx",
		"export_job_engagement_updated_at_idx",
		"export_job_bundle_receipt_unique",
		"export_receipt_export_job_unique",
		"export_receipt_id_export_job_unique",
		"teardown_authorization_engagement_requested_at_idx",
		"teardown_authorization_receipt_idx",
		"teardown_authorization_status_expires_idx",
	})
}

func assertComposeHasTables(t *testing.T, runCompose func(args ...string) string, names []string) {
	t.Helper()
	for _, name := range names {
		got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = '%s'`, name)))
		if got != "1" {
			t.Fatalf("table %s count = %s, want 1", name, got)
		}
	}
}

func assertComposeHasIndexes(t *testing.T, runCompose func(args ...string) string, names []string) {
	t.Helper()
	for _, name := range names {
		got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", fmt.Sprintf(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = '%s'`, name)))
		if got != "1" {
			t.Fatalf("index %s count = %s, want 1", name, got)
		}
	}
}

func composeOutput(ctx context.Context, project, overridePath string, args ...string) (string, error) {
	full := append([]string{"compose", "-p", project, "-f", "compose.yml", "-f", overridePath}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustComposeExec(t *testing.T, ctx context.Context, project, overridePath string, args ...string) {
	t.Helper()
	out, err := composeOutput(ctx, project, overridePath, args...)
	if err != nil {
		t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func mustComposeBuildNoCache(t *testing.T, ctx context.Context, project, overridePath string) string {
	t.Helper()
	out, err := composeOutput(ctx, project, overridePath, "build", "--no-cache", "--progress", "plain")
	if err != nil {
		t.Fatalf("docker compose build --no-cache failed: %v\n%s", err, out)
	}
	return out
}

func waitForComposePort(t *testing.T, ctx context.Context, project, overridePath string) string {
	t.Helper()
	deadline := time.Now().Add(composeStartupTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		out, err := composeOutput(ctx, project, overridePath, "port", "waypoint", "8080")
		if err == nil {
			if _, port, err := net.SplitHostPort(strings.TrimSpace(out)); err == nil && port != "" {
				return port
			}
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for compose port: %v", lastErr)
	return ""
}

func waitForHTTP(t *testing.T, url string, wantStatus int) {
	t.Helper()
	deadline := time.Now().Add(composeStartupTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for %s to return %d", url, wantStatus)
}

func assertHTTPBodyContains(t *testing.T, url string, wantStatus int, want string) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, wantStatus, body)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("body %q missing %q", body, want)
	}
}

func postMultipartCapture(t *testing.T, url, token string, envelope map[string]any, stdout, stderr []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	enc, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := mw.WriteField("envelope", string(enc)); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := mw.WriteField("stdout", string(stdout)); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := mw.WriteField("stderr", string(stderr)); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", envelope["captureId"].(string))
	req.Header.Set("X-Request-ID", "req-compose-stack")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("post capture: %v", err)
	}
	return resp
}

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}

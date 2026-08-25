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
	"strings"
	"testing"
	"time"
)

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cleanupCancel()
		_, _ = composeOutput(cleanupCtx, project, overridePath, "down", "-v", "--remove-orphans")
	})

	runCompose("up", "-d", "--wait", "--build")
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
	if got := strings.TrimSpace(runCompose("exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-tAc", "SELECT COUNT(*) FROM schema_migrations")); got != "4" {
		t.Fatalf("schema_migrations count = %s, want 4", got)
	}
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

	runCompose("restart", "postgres")
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

	runCompose("down", "-v", "--remove-orphans")
	if got := strings.TrimSpace(runCompose("ps", "-q")); got != "" {
		t.Fatalf("post-teardown compose ps = %q, want empty", got)
	}
	if got := strings.TrimSpace(runCompose("ps", "-q", "postgres")); got != "" {
		t.Fatalf("post-teardown postgres ps = %q, want empty", got)
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

func waitForComposePort(t *testing.T, ctx context.Context, project, overridePath string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
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
	deadline := time.Now().Add(2 * time.Minute)
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

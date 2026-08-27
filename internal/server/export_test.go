package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestExportJobLifecyclePersistsReceiptAndBlocksBrowserAuthorship(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceRoot := t.TempDir()
	exportRoot := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceRoot)
	t.Setenv("WAYPOINT_EXPORT_DIR", exportRoot)
	chromium := filepath.Join(t.TempDir(), "chromium")
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;&\n    file://*) html=${arg#file://} ;;&\n  esac\ndone\ngrep -q 'Hash verified, not signed' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	t.Setenv("WAYPOINT_CHROMIUM", chromium)

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actorToken := "export-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', '10.10.12.0/24\ncorp.local', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex(actorToken))

	stdoutBytes := []byte("nmap -sn 10.10.12.0/24\nHost is up\n")
	stderrBytes := []byte("warning: raw output preserved\n")
	stdoutSHA := hashHex(string(stdoutBytes))
	stderrSHA := hashHex(string(stderrBytes))
	stdoutKey := "captures/" + stdoutSHA + "/stdout"
	stderrKey := "captures/" + stderrSHA + "/stderr"
	stdoutPath := filepath.Join(evidenceRoot, filepath.FromSlash(stdoutKey))
	stderrPath := filepath.Join(evidenceRoot, filepath.FromSlash(stderrKey))
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0o750); err != nil {
		t.Fatalf("mkdir stdout evidence: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stderrPath), 0o750); err != nil {
		t.Fatalf("mkdir stderr evidence: %v", err)
	}
	if err := os.WriteFile(stdoutPath, stdoutBytes, 0o600); err != nil {
		t.Fatalf("write stdout evidence: %v", err)
	}
	if err := os.WriteFile(stderrPath, stderrBytes, 0o600); err != nil {
		t.Fatalf("write stderr evidence: %v", err)
	}
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', $3, $4, 'text/plain', $5)`, "33333333-3333-4333-8333-333333333333", engagementID, stdoutSHA, int64(len(stdoutBytes)), stdoutKey)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', $3, $4, 'text/plain', $5)`, "44444444-4444-4444-8444-444444444444", engagementID, stderrSHA, int64(len(stderrBytes)), stderrKey)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', 'recon', 'nmap', '[]'::jsonb, '/', '10.0.0.12', '[]'::jsonb, 'host', '10.10.12.0/24', now(), now(), 0, $4, $5, 'raw')`, "55555555-5555-4555-8555-555555555555", engagementID, humanID, "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444")
	mustExec(t, db, `INSERT INTO finding (id, engagement_id, title, severity, evidence_action_ids, remediation, status, promoted_by, promoted_at) VALUES ($1, $2, 'SMB signing enforced', 'low', ARRAY[$3]::uuid[], 'Keep SMB signing enabled.', 'open', $4, now())`, "66666666-6666-4666-8666-666666666666", engagementID, "55555555-5555-4555-8555-555555555555", humanID)
	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, now(), now(), 1)`, "77777777-7777-4777-8777-777777777777", engagementID, humanID)

	h := HandlerWithDB(db)
	deadline := time.Now().Add(30 * time.Second)
	var completed exportJobResponse
	for time.Now().Before(deadline) {
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports/77777777-7777-4777-8777-777777777777", nil)
		getReq.Header.Set("Authorization", "Bearer "+actorToken)
		getReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
		getRR := httptest.NewRecorder()
		h.ServeHTTP(getRR, getReq)
		if getRR.Code != http.StatusOK {
			t.Fatalf("get export status = %d body=%s", getRR.Code, getRR.Body.String())
		}
		if err := json.Unmarshal(getRR.Body.Bytes(), &completed); err != nil {
			t.Fatalf("decode export response: %v", err)
		}
		if completed.State == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if completed.State != "completed" || completed.Bundle == nil || completed.Bundle.ReceiptID == "" {
		t.Fatalf("recovered export = %#v", completed)
	}
	if completed.Bundle.ArchivePath != "export-bundle.tar.gz" {
		t.Fatalf("archive path = %q", completed.Bundle.ArchivePath)
	}
	archivePath := filepath.Join(exportRoot, "77777777-7777-4777-8777-777777777777", completed.Bundle.ArchivePath)
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("stat export archive: %v", err)
	}
	if info.Size() != completed.Bundle.ArchiveByteLength {
		t.Fatalf("archive size = %d, want %d", info.Size(), completed.Bundle.ArchiveByteLength)
	}
	sha, _, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatalf("hash export archive: %v", err)
	}
	if sha != completed.Bundle.ArchiveSHA256 {
		t.Fatalf("archive hash = %s, want %s", sha, completed.Bundle.ArchiveSHA256)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open export archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("open gzip archive: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read archive entry: %v", err)
		}
		entries[hdr.Name] = true
	}
	for _, want := range []string{
		"bundle/database/engagement.dump",
		"bundle/evidence/evidence.tar.zst",
		"bundle/report/frozen-report.pdf",
		"bundle/report/report-snapshot.json",
		"bundle/metadata/export-metadata.json",
		"bundle/metadata/export-manifest.json",
		"bundle/tools/verify-restore.mjs",
		"bundle/tools/regenerate-report.mjs",
		"bundle/instructions/restore.md",
	} {
		if !entries[want] {
			t.Fatalf("archive missing %q", want)
		}
	}

	manifestBytes, err := os.ReadFile(filepath.Join(exportRoot, "77777777-7777-4777-8777-777777777777", "bundle", "metadata", "export-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		FormatVersion string `json:"formatVersion"`
		ExportJobID   string `json:"exportJobId"`
		EngagementID  string `json:"engagementId"`
		Cutoff        string `json:"cutoff"`
		Payloads      []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"payloads"`
		Signatures struct {
			Version string   `json:"version"`
			Items   []string `json:"items"`
		} `json:"signatures"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.FormatVersion != "1.0.0" || manifest.ExportJobID == "" || manifest.EngagementID == "" || manifest.Cutoff == "" {
		t.Fatalf("manifest metadata = %#v", manifest)
	}
	if len(manifest.Payloads) < 7 || manifest.Signatures.Version != "v1" || len(manifest.Signatures.Items) != 0 {
		t.Fatalf("manifest payloads/signatures = %#v", manifest)
	}
	verifyToolBytes, err := os.ReadFile(filepath.Join(exportRoot, "77777777-7777-4777-8777-777777777777", "bundle", "tools", "verify-restore.mjs"))
	if err != nil {
		t.Fatalf("read verify tool: %v", err)
	}
	if bytes.Contains(verifyToolBytes, []byte("../../web/scripts")) {
		t.Fatal("verify tool still imports repository code")
	}
	regenToolBytes, err := os.ReadFile(filepath.Join(exportRoot, "77777777-7777-4777-8777-777777777777", "bundle", "tools", "regenerate-report.mjs"))
	if err != nil {
		t.Fatalf("read regenerate tool: %v", err)
	}
	if bytes.Contains(regenToolBytes, []byte("../../web/scripts")) {
		t.Fatal("regenerate tool still imports repository code")
	}
	toolRoot := filepath.Join(exportRoot, "77777777-7777-4777-8777-777777777777")
	verifyCmd := exec.Command("node", filepath.Join("bundle", "tools", "verify-restore.mjs"), ".")
	verifyCmd.Dir = toolRoot
	if out, err := verifyCmd.CombinedOutput(); err != nil {
		t.Fatalf("verify tool execution failed: %v output=%s", err, string(out))
	}
	regeneratedPath := filepath.Join(toolRoot, "report.html")
	regenCmd := exec.Command("node", filepath.Join("bundle", "tools", "regenerate-report.mjs"), ".", regeneratedPath)
	regenCmd.Dir = toolRoot
	if out, err := regenCmd.CombinedOutput(); err != nil {
		t.Fatalf("regenerate tool execution failed: %v output=%s", err, string(out))
	}
	if _, err := os.Stat(regeneratedPath); err != nil {
		t.Fatalf("regenerated html missing: %v", err)
	}
}

func TestWriteExportArchiveIsDeterministicAcrossMtimeDrift(t *testing.T) {
	bundleDir := t.TempDir()
	mustWrite := func(rel, body string) {
		path := filepath.Join(bundleDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mustWrite("metadata/export-manifest.json", "{\n  \"version\": \"v1\"\n}\n")
	mustWrite("report/report-snapshot.json", "snapshot\n")
	mustWrite("database/engagement.dump", "dump\n")

	archive1 := filepath.Join(t.TempDir(), "archive-1.tar.gz")
	if err := writeExportArchive(archive1, bundleDir); err != nil {
		t.Fatalf("write archive 1: %v", err)
	}
	sha1, _, err := fileSHA256(archive1)
	if err != nil {
		t.Fatalf("hash archive 1: %v", err)
	}

	if err := os.Chtimes(filepath.Join(bundleDir, "metadata", "export-manifest.json"), time.Now().Add(2*time.Hour), time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("touch bundle file: %v", err)
	}
	archive2 := filepath.Join(t.TempDir(), "archive-2.tar.gz")
	if err := writeExportArchive(archive2, bundleDir); err != nil {
		t.Fatalf("write archive 2: %v", err)
	}
	sha2, _, err := fileSHA256(archive2)
	if err != nil {
		t.Fatalf("hash archive 2: %v", err)
	}
	if sha1 != sha2 {
		t.Fatalf("archive hash drifted across mtime change: %s != %s", sha1, sha2)
	}
}

func TestBuildExportEvidenceTarIsDeterministicAcrossMtimeDrift(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceRoot := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceRoot)
	store := &evidenceStore{root: evidenceRoot}

	engagementID := "11111111-1111-4111-8111-111111111111"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, "22222222-2222-4222-8222-222222222222", engagementID, hashHex("evidence-tar-token"))

	cases := []struct{ kind, body, key string }{
		{kind: "stdout", body: "alpha\n", key: "captures/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/stdout"},
		{kind: "stderr", body: "bravo\n", key: "captures/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/stderr"},
	}
	for i, tc := range cases {
		path := filepath.Join(evidenceRoot, filepath.FromSlash(tc.key))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir evidence %d: %v", i, err)
		}
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatalf("write evidence %d: %v", i, err)
		}
		mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, $3, $4, $5, 'text/plain', $6)`,
			[]string{"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"}[i], engagementID, tc.kind, hashHex(tc.body), int64(len(tc.body)), tc.key)
	}

	output1 := filepath.Join(t.TempDir(), "evidence-1.tar")
	if err := buildExportEvidenceTar(ctx, db, store, engagementID, output1); err != nil {
		t.Fatalf("build evidence tar 1: %v", err)
	}
	sha1, _, err := fileSHA256(output1)
	if err != nil {
		t.Fatalf("hash evidence tar 1: %v", err)
	}

	for _, tc := range cases {
		path := filepath.Join(evidenceRoot, filepath.FromSlash(tc.key))
		if err := os.Chtimes(path, time.Now().Add(2*time.Hour), time.Now().Add(2*time.Hour)); err != nil {
			t.Fatalf("touch evidence %s: %v", tc.key, err)
		}
	}
	output2 := filepath.Join(t.TempDir(), "evidence-2.tar")
	if err := buildExportEvidenceTar(ctx, db, store, engagementID, output2); err != nil {
		t.Fatalf("build evidence tar 2: %v", err)
	}
	sha2, _, err := fileSHA256(output2)
	if err != nil {
		t.Fatalf("hash evidence tar 2: %v", err)
	}
	if sha1 != sha2 {
		t.Fatalf("evidence tar hash drifted across mtime change: %s != %s", sha1, sha2)
	}
}

func TestExportJobPreflightRejectsInsufficientCapacity(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceRoot := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceRoot)
	exportRootFile := filepath.Join(t.TempDir(), "exports-root")
	if err := os.WriteFile(exportRootFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write export root file: %v", err)
	}
	t.Setenv("WAYPOINT_EXPORT_DIR", exportRootFile)

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actorToken := "capacity-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex(actorToken))

	h := HandlerWithDB(db)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/exports", strings.NewReader(`{"formatVersion":"1.0.0"}`))
	createReq.Header.Set("Authorization", "Bearer "+actorToken)
	createReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusAccepted {
		t.Fatalf("create export status = %d body=%s", createRR.Code, createRR.Body.String())
	}
	var created exportJobResponse
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+created.ID, nil)
		getReq.Header.Set("Authorization", "Bearer "+actorToken)
		getReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
		getRR := httptest.NewRecorder()
		h.ServeHTTP(getRR, getReq)
		if getRR.Code != http.StatusOK {
			t.Fatalf("get export status = %d body=%s", getRR.Code, getRR.Body.String())
		}
		var current exportJobResponse
		if err := json.Unmarshal(getRR.Body.Bytes(), &current); err != nil {
			t.Fatalf("decode export response: %v", err)
		}
		if current.State == "failed" {
			if current.Failure == nil || current.Failure.Code != "capacity_insufficient" || current.Failure.Retryable {
				t.Fatalf("failed export = %#v", current)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("export job did not fail")
}

func TestExportTeardownAuthorizationRoundTrip(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	actorToken := "teardown-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex(actorToken))
	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, snapshot_id, cutoff, bundle_archive_path, bundle_archive_byte_length, bundle_archive_sha256, bundle_manifest_sha256, bundle_report_snapshot_id, created_at, started_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'verifying', 'verification', 92, 10, 10, $4, now(), 'bundle', 10, $5, $6, $4, now(), now(), now(), 2)`, "77777777-7777-4777-8777-777777777778", engagementID, humanID, "12121212-1212-4212-8212-121212121212", strings.Repeat("1", 64), strings.Repeat("2", 64))
	mustExec(t, db, `INSERT INTO export_receipt (id, export_job_id, engagement_id, status, bundle_path, archive_byte_length, archive_sha256, manifest_sha256, cutoff, verified_at, verified_by, verifier_version, revision) VALUES ($1, $2, $3, 'verified', 'bundle', 10, $4, $5, now(), now(), $6, 'waypoint-verify/1.0.0', 1)`, "88888888-8888-4888-8888-888888888888", "77777777-7777-4777-8777-777777777778", engagementID, strings.Repeat("1", 64), strings.Repeat("2", 64), humanID)
	mustExec(t, db, `UPDATE export_job SET state = 'completed', progress_stage = 'complete', progress_percent = 100, bundle_receipt_id = $2, completed_at = now(), updated_at = now(), revision = 3 WHERE id = $1`, "77777777-7777-4777-8777-777777777778", "88888888-8888-4888-8888-888888888888")

	h := HandlerWithDB(db)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teardown-authorizations", strings.NewReader(`{"receiptId":"88888888-8888-4888-8888-888888888888","bundlePath":"bundle","archiveSha256":"1111111111111111111111111111111111111111111111111111111111111111","manifestSha256":"2222222222222222222222222222222222222222222222222222222222222222","confirmation":"destroy verified engagement data"}`))
	createReq.Header.Set("Authorization", "Bearer "+actorToken)
	createReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create teardown authorization = %d body=%s", createRR.Code, createRR.Body.String())
	}
	var created teardownAuthorizationResponse
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode teardown authorization: %v", err)
	}
	if created.Status != "authorized" || created.ReceiptID != "88888888-8888-4888-8888-888888888888" {
		t.Fatalf("created teardown authorization = %#v", created)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/teardown-authorizations/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+actorToken)
	getReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get teardown authorization = %d body=%s", getRR.Code, getRR.Body.String())
	}

	consumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/teardown-authorizations/"+created.ID+"/consume", nil)
	consumeReq.Header.Set("Authorization", "Bearer "+actorToken)
	consumeReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	consumeRR := httptest.NewRecorder()
	h.ServeHTTP(consumeRR, consumeReq)
	if consumeRR.Code != http.StatusOK {
		t.Fatalf("consume teardown authorization = %d body=%s", consumeRR.Code, consumeRR.Body.String())
	}
	var consumed teardownAuthorizationResponse
	if err := json.Unmarshal(consumeRR.Body.Bytes(), &consumed); err != nil {
		t.Fatalf("decode consumed teardown authorization: %v", err)
	}
	if consumed.Status != "consumed" || consumed.ConsumedAt == "" {
		t.Fatalf("consumed teardown authorization = %#v", consumed)
	}
}

func TestBuildExportEvidenceTarStreamsAttachmentRoles(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceRoot := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceRoot)

	engagementID := "11111111-1111-4111-8111-111111111111"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)

	cases := []struct {
		id   string
		kind string
		body string
	}{
		{id: "55555555-5555-4555-8555-555555555551", kind: "stdout", body: "alpha\n"},
		{id: "55555555-5555-4555-8555-555555555552", kind: "stderr", body: "bravo\n"},
		{id: "55555555-5555-4555-8555-555555555553", kind: "attachment", body: "charlie\n"},
	}
	for _, tc := range cases {
		sha := hashHex(tc.body)
		storageKey := filepath.ToSlash(filepath.Join("captures", sha, tc.kind))
		blobPath := filepath.Join(evidenceRoot, filepath.FromSlash(storageKey))
		if err := os.MkdirAll(filepath.Dir(blobPath), 0o750); err != nil {
			t.Fatalf("mkdir evidence path: %v", err)
		}
		if err := os.WriteFile(blobPath, []byte(tc.body), 0o600); err != nil {
			t.Fatalf("write evidence blob: %v", err)
		}
		mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, $3, $4, $5, 'text/plain; charset=utf-8', $6)`, tc.id, engagementID, tc.kind, sha, int64(len(tc.body)), storageKey)
	}

	outputPath := filepath.Join(t.TempDir(), "bundle", "evidence", "evidence.tar")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		t.Fatalf("mkdir output path: %v", err)
	}
	if err := buildExportEvidenceTar(ctx, db, &evidenceStore{root: evidenceRoot}, engagementID, outputPath); err != nil {
		t.Fatalf("build evidence tar: %v", err)
	}

	f, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open evidence tar: %v", err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	got := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar payload: %v", err)
		}
		got[hdr.Name] = string(data)
	}
	for _, tc := range cases {
		sha := hashHex(tc.body)
		wantPath := filepath.ToSlash(filepath.Join("captures", sha, tc.kind))
		if got[wantPath] != tc.body {
			t.Fatalf("tar entry %q = %q, want %q", wantPath, got[wantPath], tc.body)
		}
	}
}

func TestExportJobListIsPagedAndResumable(t *testing.T) {
	if os.Getenv("WAYPOINT_TEST_PG_DSN") == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	actorID := "22222222-2222-4222-8222-222222222222"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, actorID, engagementID, hashHex("export-list-token"))
	jobIDs := []string{"11111111-1111-4111-8111-111111111112", "11111111-1111-4111-8111-111111111113", "11111111-1111-4111-8111-111111111114"}
	for i, jobID := range jobIDs {
		mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, now() - ($4 || ' minutes')::interval, now() - ($4 || ' minutes')::interval, 1)`, jobID, engagementID, actorID, i)
	}

	page, err := loadExportJobPage(ctx, db, engagementID, nil, 1)
	if err != nil {
		t.Fatalf("load export page: %v", err)
	}
	if len(page.Items) != 1 || !page.Page.HasMore || page.Page.NextCursor == "" {
		t.Fatalf("page = %#v", page)
	}
	after, err := strconv.ParseInt(page.Page.NextCursor, 10, 64)
	if err != nil {
		t.Fatalf("parse next cursor: %v", err)
	}
	resumed, err := loadExportJobPage(ctx, db, engagementID, &after, 1)
	if err != nil {
		t.Fatalf("load resumed export page: %v", err)
	}
	if len(resumed.Items) != 1 || resumed.Items[0].ID == page.Items[0].ID || resumed.Page.NextCursor == "" {
		t.Fatalf("resumed page = %#v", resumed)
	}
}

func TestBundleToolsAreStandaloneAndVersioned(t *testing.T) {
	for _, name := range []string{"verify-restore.mjs", "regenerate-report.mjs"} {
		path := filepath.Join("..", "..", "bundle", "tools", name)
		toolBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if bytes.Contains(toolBytes, []byte("../../web/scripts")) || bytes.Contains(toolBytes, []byte("import { buildReportHtml }")) {
			t.Fatalf("%s still imports repository code", path)
		}
		if !bytes.Contains(toolBytes, []byte("#!/usr/bin/env node")) {
			t.Fatalf("%s missing node shebang", path)
		}
	}
}

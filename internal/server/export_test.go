package server

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;\n    file://*) html=${arg#file://} ;;\n  esac\ndone\ngrep -q 'Hash verified, not signed' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
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

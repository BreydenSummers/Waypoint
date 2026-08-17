package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\nout=\"\"\nhtml=\"\"\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --print-to-pdf=*) out=${arg#*=} ;;\n    file://*) html=${arg#file://} ;;\n  esac\ndone\ngrep -q 'Hash verified, not signed' \"$html\"\nprintf '%s' '%PDF-1.4\\n%%fake\\n%%EOF' > \"$out\"\n"), 0o755); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	t.Setenv("WAYPOINT_CHROMIUM", chromium)

	engagementID := "11111111-1111-4111-8111-111111111111"
	humanID := "22222222-2222-4222-8222-222222222222"
	stdoutID := "33333333-3333-4333-8333-333333333333"
	stderrID := "44444444-4444-4444-8444-444444444444"
	actionID := "55555555-5555-4555-8555-555555555555"
	findingID := "66666666-6666-4666-8666-666666666666"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', '10.10.12.0/24\ncorp.local', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex("export-token"))

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
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', $3, $4, 'text/plain', $5)`, stdoutID, engagementID, stdoutSHA, int64(len(stdoutBytes)), stdoutKey)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', $3, $4, 'text/plain', $5)`, stderrID, engagementID, stderrSHA, int64(len(stderrBytes)), stderrKey)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', 'recon', 'nmap', '[]'::jsonb, '/', '10.0.0.12', '[]'::jsonb, 'host', '10.10.12.0/24', now(), now(), 0, $4, $5, 'raw')`, actionID, engagementID, humanID, stdoutID, stderrID)
	mustExec(t, db, `INSERT INTO finding (id, engagement_id, title, severity, evidence_action_ids, remediation, status, promoted_by, promoted_at) VALUES ($1, $2, 'SMB signing enforced', 'low', ARRAY[$3]::uuid[], 'Keep SMB signing enabled.', 'open', $4, now())`, findingID, engagementID, actionID, humanID)

	h := HandlerWithDB(db)
	createReqBody := strings.NewReader(`{"formatVersion":"1.0.0"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/exports", createReqBody)
	createReq.Header.Set("Authorization", "Bearer export-token")
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
	if created.State != "queued" {
		t.Fatalf("created state = %s", created.State)
	}

	var completed exportJobResponse
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+created.ID, nil)
		getReq.Header.Set("Authorization", "Bearer export-token")
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
	if completed.State != "completed" {
		t.Fatalf("export never completed: %#v", completed)
	}
	if completed.Bundle == nil || completed.Bundle.ArchiveSHA256 == "" || completed.Bundle.ReceiptID == "" {
		t.Fatalf("completed bundle = %#v", completed.Bundle)
	}
	if completed.Bundle.ReportSnapshotID == "" || completed.SnapshotID == "" {
		t.Fatalf("completed snapshot ids missing: %#v", completed)
	}

	receiptReq := httptest.NewRequest(http.MethodGet, "/api/v1/export-receipts/"+completed.Bundle.ReceiptID, nil)
	receiptReq.Header.Set("Authorization", "Bearer export-token")
	receiptReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	receiptRR := httptest.NewRecorder()
	h.ServeHTTP(receiptRR, receiptReq)
	if receiptRR.Code != http.StatusOK {
		t.Fatalf("receipt status = %d body=%s", receiptRR.Code, receiptRR.Body.String())
	}
	var receipt exportReceiptResponse
	if err := json.Unmarshal(receiptRR.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.Status != "verified" || receipt.ExportJobID != completed.ID || receipt.BundlePath != "bundle" {
		t.Fatalf("receipt = %#v", receipt)
	}

	postReceiptReq := httptest.NewRequest(http.MethodPost, "/api/v1/export-receipts/"+completed.Bundle.ReceiptID, nil)
	postReceiptReq.Header.Set("Authorization", "Bearer export-token")
	postReceiptReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	postReceiptRR := httptest.NewRecorder()
	h.ServeHTTP(postReceiptRR, postReceiptReq)
	if postReceiptRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("receipt POST status = %d, want %d", postReceiptRR.Code, http.StatusMethodNotAllowed)
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
	humanID := "22222222-2222-4222-8222-222222222222"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', '10.10.12.0/24\ncorp.local', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex("export-token"))

	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, $4, $4, 1)`, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", engagementID, humanID, time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, $4, $4, 1)`, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", engagementID, humanID, time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC))
	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, $4, $4, 1)`, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3", engagementID, humanID, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))

	h := HandlerWithDB(db)
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports?limit=2", nil)
	listReq.Header.Set("Authorization", "Bearer export-token")
	listReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list export status = %d body=%s", listRR.Code, listRR.Body.String())
	}
	var page exportJobPageResponse
	if err := json.Unmarshal(listRR.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !page.Page.HasMore || page.Page.NextCursor == "" {
		t.Fatalf("page metadata = %#v", page.Page)
	}
	if got := []string{page.Items[0].ID, page.Items[1].ID}; got[0] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3" || got[1] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2" {
		t.Fatalf("first page order = %#v", got)
	}

	resumeReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports?limit=2&after="+page.Page.NextCursor, nil)
	resumeReq.Header.Set("Authorization", "Bearer export-token")
	resumeReq.Header.Set("Waypoint-Contract-Version", "1.0.0")
	resumeRR := httptest.NewRecorder()
	h.ServeHTTP(resumeRR, resumeReq)
	if resumeRR.Code != http.StatusOK {
		t.Fatalf("resume list status = %d body=%s", resumeRR.Code, resumeRR.Body.String())
	}
	if err := json.Unmarshal(resumeRR.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode resume response: %v", err)
	}
	if page.Page.HasMore || page.Page.NextCursor != "" || len(page.Items) != 1 || page.Items[0].ID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1" {
		t.Fatalf("resume page = %#v", page)
	}
}

func TestExportJobRecoversAfterRestart(t *testing.T) {
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
	stdoutID := "33333333-3333-4333-8333-333333333333"
	stderrID := "44444444-4444-4444-8444-444444444444"
	actionID := "55555555-5555-4555-8555-555555555555"
	findingID := "66666666-6666-4666-8666-666666666666"
	jobID := "77777777-7777-4777-8777-777777777777"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Q3 launch', 'Client', '10.10.12.0/24\ncorp.local', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'owner')`, humanID, engagementID, hashHex("export-token"))

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
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', $3, $4, 'text/plain', $5)`, stdoutID, engagementID, stdoutSHA, int64(len(stdoutBytes)), stdoutKey)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', $3, $4, 'text/plain', $5)`, stderrID, engagementID, stderrSHA, int64(len(stderrBytes)), stderrKey)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', 'recon', 'nmap', '[]'::jsonb, '/', '10.0.0.12', '[]'::jsonb, 'host', '10.10.12.0/24', now(), now(), 0, $4, $5, 'raw')`, actionID, engagementID, humanID, stdoutID, stderrID)
	mustExec(t, db, `INSERT INTO finding (id, engagement_id, title, severity, evidence_action_ids, remediation, status, promoted_by, promoted_at) VALUES ($1, $2, 'SMB signing enforced', 'low', ARRAY[$3]::uuid[], 'Keep SMB signing enabled.', 'open', $4, now())`, findingID, engagementID, actionID, humanID)
	mustExec(t, db, `INSERT INTO export_job (id, engagement_id, requested_by, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes, created_at, updated_at, revision) VALUES ($1, $2, $3, '1.0.0', 'queued', 'queued', 0, 0, 0, now(), now(), 1)`, jobID, engagementID, humanID)

	h := HandlerWithDB(db)
	deadline := time.Now().Add(30 * time.Second)
	var completed exportJobResponse
	for time.Now().Before(deadline) {
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+jobID, nil)
		getReq.Header.Set("Authorization", "Bearer export-token")
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

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestEvidenceMetadataAndContentReadsStayEngagementScoped(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	evidenceDir := t.TempDir()
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceDir)

	engagementA := "11111111-1111-4111-8111-111111111111"
	engagementB := "22222222-2222-4222-8222-222222222222"
	actorA := "33333333-3333-4333-8333-333333333333"
	actorB := "44444444-4444-4444-8444-444444444444"
	tokenA := "evidence-token-a"
	tokenB := "evidence-token-b"
	stdoutID := "55555555-5555-4555-8555-555555555551"
	stderrID := "55555555-5555-4555-8555-555555555552"
	foreignStdoutID := "66666666-6666-4666-8666-666666666661"
	foreignStderrID := "66666666-6666-4666-8666-666666666662"
	actionID := "77777777-7777-4777-8777-777777777771"
	foreignActionID := "77777777-7777-4777-8777-777777777772"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo A', 'Client A', 'Scope A', 'active')`, engagementA)
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo B', 'Client B', 'Scope B', 'active')`, engagementB)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorA, engagementA, hashHex(tokenA))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'beth.operator', $3, 'operator')`, actorB, engagementB, hashHex(tokenB))

	stdout := []byte("abc\n")
	stderr := []byte("def\n")
	sha := hashHex(string(stdout))
	blobPath := filepath.Join(evidenceDir, "captures", sha, "stdout")
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o750); err != nil {
		t.Fatalf("mkdir evidence path: %v", err)
	}
	if err := os.WriteFile(blobPath, stdout, 0o600); err != nil {
		t.Fatalf("write evidence blob: %v", err)
	}
	stderrPath := filepath.Join(evidenceDir, "captures", hashHex(string(stderr)), "stderr")
	if err := os.MkdirAll(filepath.Dir(stderrPath), 0o750); err != nil {
		t.Fatalf("mkdir stderr evidence path: %v", err)
	}
	if err := os.WriteFile(stderrPath, stderr, 0o600); err != nil {
		t.Fatalf("write stderr blob: %v", err)
	}

	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', $3, $4, 'text/plain; charset=utf-8', $5)`, stdoutID, engagementA, sha, int64(len(stdout)), "captures/"+sha+"/stdout")
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', $3, $4, 'bad' || chr(13) || chr(10) || 'value', $5)`, stderrID, engagementA, hashHex(string(stderr)), int64(len(stderr)), "captures/"+hashHex(string(stderr))+"/stderr")
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', 'recon', 'whoami', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'demo.local', now(), now(), 0, $4, $5, 'raw')`, actionID, engagementA, actorA, stdoutID, stderrID)

	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', $3, $4, 'text/plain; charset=utf-8', $5)`, foreignStdoutID, engagementB, sha, int64(len(stdout)), "captures/"+sha+"/stdout")
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', $3, $4, 'text/plain; charset=utf-8', $5)`, foreignStderrID, engagementB, hashHex(string(stderr)), int64(len(stderr)), "captures/"+hashHex(string(stderr))+"/stderr")
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $3, 'manual', 'recon', 'whoami', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'demo.local', now(), now(), 0, $4, $5, 'raw')`, foreignActionID, engagementB, actorB, foreignStdoutID, foreignStderrID)

	h := HandlerWithDB(db)

	metaRR := doEvidenceRequest(t, h, http.MethodGet, "/api/v1/evidence/"+stdoutID, tokenA, "req-evidence-meta", nil, nil)
	if metaRR.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, body=%s", metaRR.Code, metaRR.Body.String())
	}
	var meta evidenceResponse
	decodeBody(t, metaRR.Body.Bytes(), &meta)
	if meta.ContractVersion != evidenceContractVersion || meta.ID != stdoutID || meta.ActionID != actionID || meta.ContentPath != "/api/v1/evidence/"+stdoutID+"/content" {
		t.Fatalf("metadata response = %#v", meta)
	}
	if meta.Role != "stdout" || meta.MediaType != "text/plain; charset=utf-8" || meta.ByteLength != int64(len(stdout)) || meta.SHA256 != sha {
		t.Fatalf("metadata fields = %#v", meta)
	}

	contentRR := doEvidenceRequest(t, h, http.MethodGet, meta.ContentPath, tokenA, "req-evidence-content", nil, map[string]string{"Range": "bytes=1-3"})
	if contentRR.Code != http.StatusPartialContent {
		t.Fatalf("content status = %d, body=%s", contentRR.Code, contentRR.Body.String())
	}
	if got := contentRR.Header().Get("Content-Disposition"); got != "attachment; filename=\""+stdoutID+"\"" {
		t.Fatalf("content disposition = %q", got)
	}
	if got := contentRR.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("content nosniff = %q", got)
	}
	if got := contentRR.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := contentRR.Header().Get("Content-Range"); got != "bytes 1-3/4" {
		t.Fatalf("content range = %q", got)
	}
	if got := contentRR.Body.String(); got != "bc\n" {
		t.Fatalf("content body = %q", got)
	}

	stderrContentRR := doEvidenceRequest(t, h, http.MethodGet, "/api/v1/evidence/"+stderrID+"/content", tokenA, "req-evidence-stderr-content", nil, nil)
	if stderrContentRR.Code != http.StatusOK {
		t.Fatalf("stderr content status = %d, body=%s", stderrContentRR.Code, stderrContentRR.Body.String())
	}
	if got := stderrContentRR.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("stderr content type = %q", got)
	}

	missingRR := doEvidenceRequest(t, h, http.MethodGet, "/api/v1/evidence/99999999-9999-4999-8999-999999999999", tokenA, "req-evidence-missing", nil, nil)
	foreignRR := doEvidenceRequest(t, h, http.MethodGet, "/api/v1/evidence/"+foreignStdoutID, tokenA, "req-evidence-foreign", nil, nil)
	for _, rr := range []*httptest.ResponseRecorder{missingRR, foreignRR} {
		if rr.Code != http.StatusNotFound {
			t.Fatalf("not-found status = %d, body=%s", rr.Code, rr.Body.String())
		}
	}
	var missingProb, foreignProb struct {
		Code      string `json:"code"`
		Status    int    `json:"status"`
		Detail    string `json:"detail"`
		Retryable bool   `json:"retryable"`
	}
	decodeBody(t, missingRR.Body.Bytes(), &missingProb)
	decodeBody(t, foreignRR.Body.Bytes(), &foreignProb)
	if missingProb != foreignProb || missingProb.Code != "resource_not_found" || missingProb.Detail != "The resource does not exist in this engagement or is not available to this actor." {
		t.Fatalf("not-found problems differ: missing=%#v foreign=%#v", missingProb, foreignProb)
	}
}

func doEvidenceRequest(t *testing.T, h http.Handler, method, path, token, requestID string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(string(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

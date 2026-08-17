package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

type performanceProfileFixture struct {
	SchemaVersion int `json:"schemaVersion"`
	Baseline      struct {
		Hardware struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
			OS     string `json:"os"`
		} `json:"hardware"`
		Operators    int `json:"operators"`
		Actions      int `json:"actions"`
		AuditEvents  int `json:"auditEvents"`
		Observations int `json:"observations"`
		EvidenceGiB  int `json:"evidenceGiB"`
	} `json:"baseline"`
	Budgets struct {
		QueryP95Ms            int `json:"queryP95Ms"`
		QueryP99Ms            int `json:"queryP99Ms"`
		IngestAckP95Ms        int `json:"ingestAckP95Ms"`
		IngestPeakRSSMiB      int `json:"ingestPeakRSSMiB"`
		SSEVisibleP95Ms       int `json:"sseVisibleP95Ms"`
		WarmRouteUsableMs     int `json:"warmRouteUsableMs"`
		LocalInteractionMs    int `json:"localInteractionMs"`
		ExportCompleteMinutes int `json:"exportCompleteMinutes"`
	} `json:"budgets"`
	Faults []struct {
		Name        string `json:"name"`
		Expectation string `json:"expectation"`
	} `json:"faults"`
}

func TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "v1", "fixtures", "performance-profile.json"))
	if err != nil {
		t.Fatalf("read performance profile fixture: %v", err)
	}

	var profile performanceProfileFixture
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("decode performance profile fixture: %v", err)
	}

	if profile.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", profile.SchemaVersion)
	}
	if profile.Baseline.Hardware.CPU != "4 vCPU" || profile.Baseline.Hardware.Memory != "8 GiB" || profile.Baseline.Hardware.OS != "Linux" {
		t.Fatalf("baseline hardware = %#v", profile.Baseline.Hardware)
	}
	if profile.Baseline.Operators != 10 || profile.Baseline.Actions != 100000 || profile.Baseline.AuditEvents != 1000000 || profile.Baseline.Observations != 1000000 || profile.Baseline.EvidenceGiB != 10 {
		t.Fatalf("baseline workload = %#v", profile.Baseline)
	}
	if profile.Budgets.QueryP95Ms != 200 || profile.Budgets.QueryP99Ms != 500 || profile.Budgets.IngestAckP95Ms != 500 || profile.Budgets.IngestPeakRSSMiB != 32 || profile.Budgets.SSEVisibleP95Ms != 1000 || profile.Budgets.WarmRouteUsableMs != 2000 || profile.Budgets.LocalInteractionMs != 100 || profile.Budgets.ExportCompleteMinutes != 15 {
		t.Fatalf("performance budgets = %#v", profile.Budgets)
	}

	wantFaults := []string{"disk-full", "restart", "postgresql-interruption", "slow-client", "interrupted-upload", "interrupted-export"}
	if len(profile.Faults) != len(wantFaults) {
		t.Fatalf("fault scenario count = %d, want %d", len(profile.Faults), len(wantFaults))
	}
	for i, want := range wantFaults {
		if profile.Faults[i].Name != want {
			t.Fatalf("fault %d = %q, want %q", i, profile.Faults[i].Name, want)
		}
		if strings.TrimSpace(profile.Faults[i].Expectation) == "" {
			t.Fatalf("fault %q missing expectation", profile.Faults[i].Name)
		}
	}
}

func TestAuditQueryShapeRemainsKeysetBounded(t *testing.T) {
	data, err := os.ReadFile("audit.go")
	if err != nil {
		t.Fatalf("read audit source: %v", err)
	}
	source := string(data)
	for _, fragment := range []string{
		"WHERE engagement_id = $1 AND id > $2",
		"ORDER BY id ASC",
		"LIMIT $3",
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("audit query source missing %q", fragment)
		}
	}
	if strings.Contains(source, "OFFSET") {
		t.Fatal("audit pagination source unexpectedly uses OFFSET")
	}

	if limit, pb := parseAuditLimit(""); pb != nil || limit != 100 {
		t.Fatalf("parseAuditLimit(\"\") = %d, %v; want 100, nil", limit, pb)
	}
	if limit, pb := parseAuditLimit("500"); pb != nil || limit != 500 {
		t.Fatalf("parseAuditLimit(\"500\") = %d, %v; want 500, nil", limit, pb)
	}
	if _, pb := parseAuditLimit("0"); pb == nil || len(pb.FieldErrors) == 0 || pb.FieldErrors[0].Code != "invalid_range" {
		t.Fatalf("parseAuditLimit(\"0\") = %#v, want invalid_range field error", pb)
	}
	if _, pb := parseAuditLimit("501"); pb == nil || len(pb.FieldErrors) == 0 || pb.FieldErrors[0].Code != "invalid_range" {
		t.Fatalf("parseAuditLimit(\"501\") = %#v, want invalid_range field error", pb)
	}
}

func TestHandlersReturnServiceUnavailableWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   io.Reader
	}{
		{name: "audit history", method: http.MethodGet, path: "/api/v1/audit-events", body: nil},
		{name: "capture ingest", method: http.MethodPost, path: "/api/v1/captures", body: bytes.NewReader(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, tc.body)
			req.Header.Set("Authorization", "Bearer demo-token")
			req.Header.Set("Waypoint-Contract-Version", "1.0.0")
			req.Header.Set("X-Request-ID", "req-demo")

			rr := httptest.NewRecorder()
			Handler().ServeHTTP(rr, req)

			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
			}
			var prob captureProblem
			if err := json.NewDecoder(rr.Body).Decode(&prob); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if prob.Code != "service_unavailable" || !prob.Retryable {
				t.Fatalf("problem = %#v, want retryable service_unavailable", prob)
			}
		})
	}
}

func TestReadCaptureRequestRejectsInterruptedMultipartUpload(t *testing.T) {
	t.Setenv("WAYPOINT_EVIDENCE_DIR", t.TempDir())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("envelope", `{"captureId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","evidence":{"stdout":{"mediaType":"text/plain","byteLength":2,"sha256":"2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df"},"stderr":{"mediaType":"text/plain","byteLength":0,"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}}`); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := mw.WriteField("stdout", "ok"); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	// The body is intentionally truncated: no stderr part and no closing boundary.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/captures", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if _, err := readCaptureRequest(req, newEvidenceStore()); err == nil {
		t.Fatal("expected interrupted multipart upload to be rejected")
	}
}

func TestCaptureEnvelopeLimitMatchesSchemaCeiling(t *testing.T) {
	if maxCaptureEnvelopeBytes != 1<<20 {
		t.Fatalf("maxCaptureEnvelopeBytes = %d, want 1048576", maxCaptureEnvelopeBytes)
	}
	if maxCaptureEvidenceBytes != 10<<30 {
		t.Fatalf("maxCaptureEvidenceBytes = %d, want 10737418240", maxCaptureEvidenceBytes)
	}
}

func TestReadCaptureRequestRejectsOversizedEnvelope(t *testing.T) {
	t.Setenv("WAYPOINT_EVIDENCE_DIR", t.TempDir())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	payload := `{"captureId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1","evidence":{"stdout":{"mediaType":"text/plain","byteLength":0,"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},"stderr":{"mediaType":"text/plain","byteLength":0,"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}}` + strings.Repeat(" ", int(maxCaptureEnvelopeBytes))
	if err := mw.WriteField("envelope", payload); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := mw.WriteField("stdout", ""); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := mw.WriteField("stderr", ""); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/captures", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if _, err := readCaptureRequest(req, newEvidenceStore()); err == nil {
		t.Fatal("expected oversized envelope upload to be rejected")
	}
}

func TestCopyEvidenceStreamRejectsOverLimit(t *testing.T) {
	var dst bytes.Buffer
	hasher := sha256.New()
	written, err := copyEvidenceStream(context.Background(), &dst, hasher, io.LimitReader(zeroReader{}, 2), 1, "/evidence/stdout")
	if err == nil {
		t.Fatal("expected oversized evidence stream to be rejected")
	}
	if written != 0 {
		t.Fatalf("written = %d, want 0", written)
	}
}

func TestUpsertEvidenceRejectsImmutableMetadataChanges(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "99999999-9999-4999-8999-999999999998"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)

	sha := hashHex("same bytes")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := upsertEvidence(ctx, tx, engagementID, "stdout", "text/plain", sha, int64(len("same bytes"))); err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed evidence: %v", err)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin conflict tx: %v", err)
	}
	defer tx.Rollback()
	if _, err := upsertEvidence(ctx, tx, engagementID, "stdout", "application/octet-stream", sha, int64(len("same bytes"))); err == nil {
		t.Fatal("expected immutable evidence metadata conflict")
	} else {
		var pb captureRequestProblem
		if !errors.As(err, &pb) || pb.problem.Code != "evidence_metadata_conflict" {
			t.Fatalf("unexpected error: %#v", err)
		}
	}
}

func TestCaptureIngestRejectsDiskPressureBeforeCommitting(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "77777777-7777-4777-8777-777777777777"
	actorID := "88888888-8888-4888-8888-888888888888"
	token := "disk-pressure-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	evidenceRoot := filepath.Join(t.TempDir(), "evidence-root")
	if err := os.WriteFile(evidenceRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("seed evidence root file: %v", err)
	}
	t.Setenv("WAYPOINT_EVIDENCE_DIR", evidenceRoot)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	envelope := rawCaptureEnvelope("dddddddd-dddd-4ddd-8ddd-ddddddddddd1", []byte("operator\n"), []byte("warning\n"))
	resp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", token, "req-disk-pressure", envelope, []byte("operator\n"), []byte("warning\n"))
	if resp.status != http.StatusServiceUnavailable {
		t.Fatalf("disk-pressure status = %d, want %d: %s", resp.status, http.StatusServiceUnavailable, resp.body)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action WHERE capture_id = $1`, envelope["captureId"]).Scan(&count); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	if count != 0 {
		t.Fatalf("action count = %d, want 0 after disk-pressure failure", count)
	}
}

func TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	t.Setenv("WAYPOINT_EVIDENCE_DIR", t.TempDir())
	engagementID := "99999999-9999-4999-8999-999999999999"
	actorID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	token := "restart-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	envelope := rawCaptureEnvelope("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeee1", []byte("operator\n"), []byte("warning\n"))
	var partial bytes.Buffer
	mw := multipart.NewWriter(&partial)
	enc, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := mw.WriteField("envelope", string(enc)); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	if err := mw.WriteField("stdout", "operator\n"); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	truncated := httptest.NewRequest(http.MethodPost, "/api/v1/captures", bytes.NewReader(partial.Bytes()))
	truncated.Header.Set("Content-Type", mw.FormDataContentType())
	truncated.Header.Set("Authorization", "Bearer "+token)
	truncated.Header.Set("Waypoint-Contract-Version", "1.0.0")
	truncated.Header.Set("Idempotency-Key", envelope["captureId"].(string))
	truncated.Header.Set("X-Request-ID", "req-restart-truncated")
	truncatedRR := httptest.NewRecorder()
	HandlerWithDB(db).ServeHTTP(truncatedRR, truncated)
	if truncatedRR.Code == http.StatusCreated {
		t.Fatal("expected interrupted upload to fail, got created")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action WHERE capture_id = $1`, envelope["captureId"]).Scan(&count); err != nil {
		t.Fatalf("count actions after interrupted upload: %v", err)
	}
	if count != 0 {
		t.Fatalf("action count after interrupted upload = %d, want 0", count)
	}

	resp := doLiveCaptureRequest(t, ts.Client(), ts.URL+"/api/v1/captures", token, "req-restart-complete", envelope, []byte("operator\n"), []byte("warning\n"))
	if resp.status != http.StatusCreated {
		t.Fatalf("restart retry status = %d, want %d: %s", resp.status, http.StatusCreated, resp.body)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action WHERE capture_id = $1`, envelope["captureId"]).Scan(&count); err != nil {
		t.Fatalf("count actions after retry: %v", err)
	}
	if count != 1 {
		t.Fatalf("action count after retry = %d, want 1", count)
	}
}

func rawCaptureEnvelope(captureID string, stdout, stderr []byte) map[string]any {
	return map[string]any{
		"contractVersion": "1.0.0",
		"captureId":       captureID,
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
			"stdout": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stdout), "sha256": hashHex(string(stdout))},
			"stderr": map[string]any{"mediaType": "text/plain; charset=utf-8", "byteLength": len(stderr), "sha256": hashHex(string(stderr))},
		},
		"parsing": map[string]any{"status": "raw"},
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

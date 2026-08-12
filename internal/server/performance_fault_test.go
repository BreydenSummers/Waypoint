package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestReadCaptureRequestRejectsOversizedEvidenceBudget(t *testing.T) {
	t.Setenv("WAYPOINT_EVIDENCE_DIR", t.TempDir())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("envelope", `{"captureId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1","evidence":{"stdout":{"mediaType":"text/plain","byteLength":33554432,"sha256":"<ignored>"},"stderr":{"mediaType":"text/plain","byteLength":0,"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}}`); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	stdout, err := mw.CreateFormField("stdout")
	if err != nil {
		t.Fatalf("create stdout field: %v", err)
	}
	if _, err := io.CopyN(stdout, zeroReader{}, 33<<20); err != nil {
		t.Fatalf("seed oversized stdout: %v", err)
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
		t.Fatal("expected oversized evidence upload to be rejected")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

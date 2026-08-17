package server

import "testing"

func TestEvidenceHelpers(t *testing.T) {
	if got := evidenceRequestPath("/api/v1/evidence/11111111-1111-4111-8111-111111111111/content"); got != "11111111-1111-4111-8111-111111111111/content" {
		t.Fatalf("evidenceRequestPath() = %q", got)
	}
	if got := evidenceRequestPath("/api/v1/evidence"); got != "" {
		t.Fatalf("evidenceRequestPath() root = %q", got)
	}
	if got := safeMediaType("text/plain; charset=utf-8"); got != "text/plain; charset=utf-8" {
		t.Fatalf("safeMediaType() = %q", got)
	}
	if got := safeMediaType("bad\r\nvalue"); got != "application/octet-stream" {
		t.Fatalf("safeMediaType() fallback = %q", got)
	}

	store := &evidenceStore{root: "/tmp/waypoint-evidence"}
	if _, err := store.resolveStoragePath("captures/abc/stdout"); err != nil {
		t.Fatalf("resolveStoragePath() = %v", err)
	}
	if _, err := store.resolveStoragePath("../escape"); err == nil {
		t.Fatal("resolveStoragePath() accepted traversal")
	}
}

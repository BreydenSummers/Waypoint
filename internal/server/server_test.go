package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesHealthAndSPA(t *testing.T) {
	ts := httptest.NewServer(Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("healthz body = %q, want ok", body)
	}

	resp, err = http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("index request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Waypoint") {
		t.Fatalf("index body missing app title: %q", body)
	}

	noRedirectClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err = noRedirectClient.Get(ts.URL + "/engagements/demo")
	if err != nil {
		t.Fatalf("SPA fallback request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SPA fallback status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Fatalf("SPA fallback unexpectedly redirected to %q", loc)
	}
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="root"`) || !strings.Contains(string(body), "/assets/waypoint.js") {
		t.Fatalf("SPA fallback body missing app shell: %q", body)
	}

	resp, err = http.Get(ts.URL + "/assets/waypoint.css")
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyzRequiresDatabase(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":          &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.js":  &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.css": &fstest.MapFile{Data: []byte("ok")},
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	readyz(assets, nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyzChecksDatabaseHealth(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":          &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.js":  &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.css": &fstest.MapFile{Data: []byte("ok")},
	}

	healthy := &fakeReadyDB{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	readyz(assets, healthy).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !healthy.called {
		t.Fatal("expected readiness check to ping the database")
	}

	unhealthy := &fakeReadyDB{err: errors.New("db down")}
	rr = httptest.NewRecorder()
	readyz(assets, unhealthy).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestRequiredAssetsPresentDetectsMissingAsset(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.js":      &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.css":     &fstest.MapFile{Data: []byte("ok")},
		"assets/extra/ignored.js": &fstest.MapFile{Data: []byte("ok")},
	}
	if err := requiredAssetsPresent(assets); err != nil {
		t.Fatalf("required assets unexpectedly missing: %v", err)
	}

	missingCSS := fstest.MapFS{
		"index.html":         &fstest.MapFile{Data: []byte("ok")},
		"assets/waypoint.js": &fstest.MapFile{Data: []byte("ok")},
	}
	if err := requiredAssetsPresent(missingCSS); err == nil {
		t.Fatal("requiredAssetsPresent() = nil, want error when css is missing")
	}
}

type fakeReadyDB struct {
	called bool
	err    error
}

func (f *fakeReadyDB) PingContext(context.Context) error {
	f.called = true
	return f.err
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestBootstrapWizardHappyPathAndGate(t *testing.T) {
	rawDB := openTestDB(t)
	defer rawDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resetPublicSchema(t, rawDB)
	if err := dbm.ApplyMigrations(ctx, rawDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const code = "ABCD-EFGH-JKMN-PQRS"
	runtime := RuntimeState{SetupCodeHash: SetupCodeHash(code)}
	ts := httptest.NewServer(HandlerWithDBAndRuntime(rawDB, runtime))
	defer ts.Close()
	client := ts.Client()

	// Runtime endpoint advertises that setup is required with a code gate.
	setup := getSetup(t, client, ts.URL)
	if !setup.Required || !setup.CodeRequired {
		t.Fatalf("expected setup required+codeRequired, got %#v", setup)
	}

	// Missing code -> 401 setup_code_required.
	if got := postBootstrap(t, client, ts.URL, map[string]any{
		"engagement": map[string]any{"name": "Autumn", "client": "Acme U", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "alex.operator"},
	}); got.status != http.StatusUnauthorized || got.code != "setup_code_required" {
		t.Fatalf("missing code: status=%d code=%s", got.status, got.code)
	}

	// Wrong code -> 401 invalid_setup_code.
	if got := postBootstrap(t, client, ts.URL, map[string]any{
		"setupCode":  "ZZZZ-ZZZZ-ZZZZ-ZZZZ",
		"engagement": map[string]any{"name": "Autumn", "client": "Acme U", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "alex.operator"},
	}); got.status != http.StatusUnauthorized || got.code != "invalid_setup_code" {
		t.Fatalf("wrong code: status=%d code=%s", got.status, got.code)
	}

	// Correct code (lower-cased + spaced to prove normalization) -> 201.
	ok := postBootstrap(t, client, ts.URL, map[string]any{
		"setupCode":  "abcd efgh jkmn pqrs",
		"engagement": map[string]any{"name": "Autumn", "client": "Acme U", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "alex.operator"},
	})
	if ok.status != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", ok.status, ok.raw)
	}
	var resp bootstrapResponse
	if err := json.Unmarshal(ok.raw, &resp); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if resp.Token == "" || resp.EngagementID == "" || resp.ActorRecord.Actor.Role != "owner" || resp.ActorRecord.Actor.Handle != "alex.operator" {
		t.Fatalf("unexpected bootstrap response: %#v", resp)
	}
	if bytes.Contains([]byte(mustJSON(resp.ActorRecord)), []byte(resp.Token)) {
		t.Fatalf("actor record leaked token")
	}

	// The minted token authenticates as the owner.
	authActor, err := lookupActor(ctx, rawDB, resp.Token)
	if err != nil {
		t.Fatalf("owner token does not authenticate: %v", err)
	}
	if authActor.Role != "owner" || authActor.Kind != "human" || authActor.EngagementID != resp.EngagementID {
		t.Fatalf("owner actor = %#v", authActor)
	}

	// A bootstrap audit event was written with bootstrap origin.
	var originKind, subjectType string
	if err := rawDB.QueryRowContext(ctx, `SELECT origin_kind, subject_type FROM audit_event WHERE type = 'engagement.provisioned' LIMIT 1`).Scan(&originKind, &subjectType); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if originKind != "bootstrap" || subjectType != "engagement" {
		t.Fatalf("audit origin=%s subject=%s", originKind, subjectType)
	}

	// Runtime no longer reports setup required.
	if setup := getSetup(t, client, ts.URL); setup.Required {
		t.Fatalf("setup should be complete, got %#v", setup)
	}

	// Second attempt is a conflict.
	if got := postBootstrap(t, client, ts.URL, map[string]any{
		"setupCode":  code,
		"engagement": map[string]any{"name": "Second", "client": "Acme U", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "mallory"},
	}); got.status != http.StatusConflict || got.code != "already_provisioned" {
		t.Fatalf("second bootstrap: status=%d code=%s", got.status, got.code)
	}
}

func TestBootstrapDisabledWithoutCode(t *testing.T) {
	rawDB := openTestDB(t)
	defer rawDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resetPublicSchema(t, rawDB)
	if err := dbm.ApplyMigrations(ctx, rawDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ts := httptest.NewServer(HandlerWithDBAndRuntime(rawDB, RuntimeState{}))
	defer ts.Close()

	if setup := getSetup(t, ts.Client(), ts.URL); !setup.Required || setup.CodeRequired {
		t.Fatalf("expected required without code gate, got %#v", setup)
	}
	if got := postBootstrap(t, ts.Client(), ts.URL, map[string]any{
		"engagement": map[string]any{"name": "Autumn", "client": "Acme U", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "alex.operator"},
	}); got.status != http.StatusForbidden || got.code != "setup_disabled" {
		t.Fatalf("expected setup_disabled, got status=%d code=%s", got.status, got.code)
	}
}

func TestAutoBootstrapIdempotent(t *testing.T) {
	rawDB := openTestDB(t)
	defer rawDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resetPublicSchema(t, rawDB)
	if err := dbm.ApplyMigrations(ctx, rawDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	token, provisioned, err := AutoBootstrap(ctx, rawDB, BootstrapParams{EngagementName: "Autumn", Client: "Acme U", Scope: "campus /16", OwnerHandle: "alex.operator"})
	if err != nil || !provisioned || token == "" {
		t.Fatalf("first AutoBootstrap: token=%q provisioned=%v err=%v", token, provisioned, err)
	}
	if _, err := lookupActor(ctx, rawDB, token); err != nil {
		t.Fatalf("generated owner token does not authenticate: %v", err)
	}
	// A restart re-runs AutoBootstrap; it must not create a second engagement.
	token2, provisioned2, err := AutoBootstrap(ctx, rawDB, BootstrapParams{EngagementName: "Autumn", Client: "Acme U", Scope: "campus /16", OwnerHandle: "alex.operator"})
	if err != nil || provisioned2 || token2 != "" {
		t.Fatalf("second AutoBootstrap should be a no-op: token=%q provisioned=%v err=%v", token2, provisioned2, err)
	}
	var count int
	if err := rawDB.QueryRowContext(ctx, `SELECT count(*) FROM engagement`).Scan(&count); err != nil {
		t.Fatalf("count engagements: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 engagement, got %d", count)
	}
}

func TestSetupCodeNormalization(t *testing.T) {
	if SetupCodeHash("abcd-efgh") != SetupCodeHash("ABCD EFGH") || SetupCodeHash("abcd-efgh") != SetupCodeHash(" AbCdEfGh ") {
		t.Fatalf("setup code normalization is inconsistent")
	}
	if SetupCodeHash("abcd") == SetupCodeHash("abce") {
		t.Fatalf("distinct codes hashed equal")
	}
}

// --- test helpers ---

type bootstrapResult struct {
	status int
	code   string
	raw    []byte
}

func postBootstrap(t *testing.T, client *http.Client, baseURL string, body map[string]any) bootstrapResult {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/bootstrap", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Waypoint-Contract-Version", actorContractVersion)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw := readAll(t, resp)
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &problem)
	return bootstrapResult{status: resp.StatusCode, code: problem.Code, raw: raw}
}

func getSetup(t *testing.T, client *http.Client, baseURL string) SetupState {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/v1/runtime")
	if err != nil {
		t.Fatalf("get runtime: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Setup SetupState `json:"setup"`
	}
	if err := json.Unmarshal(readAll(t, resp), &out); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	return out.Setup
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActorLifecycleProvisionRotateRevokeAndAuthorization(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	engagementID := "11111111-1111-4111-8111-111111111111"
	authID := "22222222-2222-4222-8222-222222222222"
	authToken := "operator-lifecycle-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, authID, engagementID, hashHex(authToken))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	provisionedResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors", authToken, http.MethodPost, map[string]any{"kind": "human", "handle": "beatrice.operator", "role": "operator"}, "")
	defer provisionedResp.Body.Close()
	if provisionedResp.StatusCode != http.StatusCreated {
		t.Fatalf("provision status = %d", provisionedResp.StatusCode)
	}
	var provisioned actorCredentialResponse
	decodeResponse(t, responseRecorderFromHTTP(provisionedResp), &provisioned)
	if provisioned.Token == "" || provisioned.ActorRecord.Actor.Handle != "beatrice.operator" || provisioned.ActorRecord.CredentialVersion != 1 {
		t.Fatalf("provisioned actor = %#v", provisioned)
	}
	actorRecordJSON, err := json.Marshal(provisioned.ActorRecord)
	if err != nil {
		t.Fatalf("marshal actor record: %v", err)
	}
	if bytes.Contains(actorRecordJSON, []byte(provisioned.Token)) {
		t.Fatalf("actor record leaked token")
	}

	listResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=10", authToken, http.MethodGet, nil, "")
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listResp.StatusCode)
	}
	var page actorPageResponse
	decodeResponse(t, responseRecorderFromHTTP(listResp), &page)
	if len(page.Items) == 0 || page.Items[0].Actor.Handle != "beatrice.operator" {
		t.Fatalf("actor list = %#v", page)
	}
	pageJSON, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	if bytes.Contains(pageJSON, []byte(provisioned.Token)) {
		t.Fatalf("actor list leaked token")
	}

	getResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors/"+provisioned.ActorRecord.Actor.ID, authToken, http.MethodGet, nil, "")
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", getResp.StatusCode)
	}
	var current actorLifecycleRecord
	decodeResponse(t, responseRecorderFromHTTP(getResp), &current)
	if current.Actor.Handle != "beatrice.operator" || current.Status != actorStatusActive || current.CredentialVersion != 1 {
		t.Fatalf("actor record = %#v", current)
	}

	rotateResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors/"+provisioned.ActorRecord.Actor.ID+"/rotate", authToken, http.MethodPost, nil, `"1"`)
	defer rotateResp.Body.Close()
	if rotateResp.StatusCode != http.StatusCreated {
		t.Fatalf("rotate status = %d", rotateResp.StatusCode)
	}
	var rotated actorCredentialResponse
	decodeResponse(t, responseRecorderFromHTTP(rotateResp), &rotated)
	if rotated.Token == "" || rotated.Token == provisioned.Token || rotated.ActorRecord.CredentialVersion != 2 || rotated.ActorRecord.Revision != 2 {
		t.Fatalf("rotated actor = %#v", rotated)
	}

	badOld := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", provisioned.Token, http.MethodGet, nil, "")
	defer badOld.Body.Close()
	if badOld.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token status = %d", badOld.StatusCode)
	}

	newTokenOk := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", rotated.Token, http.MethodGet, nil, "")
	defer newTokenOk.Body.Close()
	if newTokenOk.StatusCode != http.StatusOK {
		t.Fatalf("new token status = %d", newTokenOk.StatusCode)
	}

	revokeResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors/"+provisioned.ActorRecord.Actor.ID+"/revoke", authToken, http.MethodPost, nil, `"2"`)
	defer revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d", revokeResp.StatusCode)
	}
	var revoked actorLifecycleRecord
	decodeResponse(t, responseRecorderFromHTTP(revokeResp), &revoked)
	if revoked.Status != actorStatusRevoked || revoked.RevokedAt == nil || revoked.RevokedBy != authID || revoked.Revision != 3 {
		t.Fatalf("revoked actor = %#v", revoked)
	}

	afterRevoke := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors?limit=1", rotated.Token, http.MethodGet, nil, "")
	defer afterRevoke.Body.Close()
	if afterRevoke.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d", afterRevoke.StatusCode)
	}

	aiProvisionedResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors", authToken, http.MethodPost, map[string]any{
		"kind":         "ai_agent",
		"handle":       "field-agent-7",
		"role":         "operator",
		"agentName":    "Synthetic Field Agent",
		"model":        "gpt-4.1",
		"version":      "2025.01",
		"authorizedBy": authID,
	}, "")
	defer aiProvisionedResp.Body.Close()
	if aiProvisionedResp.StatusCode != http.StatusCreated {
		t.Fatalf("ai provision status = %d", aiProvisionedResp.StatusCode)
	}
	var aiProvisioned actorCredentialResponse
	decodeResponse(t, responseRecorderFromHTTP(aiProvisionedResp), &aiProvisioned)
	if aiProvisioned.ActorRecord.Actor.AuthorizedBy != authID || aiProvisioned.ActorRecord.Actor.Kind != "ai_agent" {
		t.Fatalf("ai provisioned actor = %#v", aiProvisioned)
	}

	rejectedAIResp := doActorJSON(t, ts.Client(), ts.URL+"/api/v1/actors", authToken, http.MethodPost, map[string]any{
		"kind":         "ai_agent",
		"handle":       "field-agent-8",
		"role":         "operator",
		"agentName":    "Second Agent",
		"model":        "gpt-4.1",
		"version":      "2025.01",
		"authorizedBy": aiProvisioned.ActorRecord.Actor.ID,
	}, "")
	defer rejectedAIResp.Body.Close()
	if rejectedAIResp.StatusCode != http.StatusConflict {
		t.Fatalf("ai authorizer status = %d", rejectedAIResp.StatusCode)
	}
}

func doActorJSON(t *testing.T, client *http.Client, url, token, method string, body any, ifMatch string) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		if raw, ok := body.(string); ok {
			payload = []byte(raw)
		} else {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", actorContractVersion)
	req.Header.Set("X-Request-ID", "req-actor-test")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func responseRecorderFromHTTP(resp *http.Response) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	for k, values := range resp.Header {
		for _, v := range values {
			rr.Header().Add(k, v)
		}
	}
	rr.WriteHeader(resp.StatusCode)
	_, _ = rr.Body.ReadFrom(resp.Body)
	return rr
}

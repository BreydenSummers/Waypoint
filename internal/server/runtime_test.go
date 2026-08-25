package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	egresspolicy "waypoint/internal/egresspolicy"
)

func TestRuntimeEndpointReturnsStartupEgressState(t *testing.T) {
	h := HandlerWithDBAndRuntime(nil, RuntimeState{Egress: egresspolicy.State{Mode: egresspolicy.ModeManual, Status: egresspolicy.StatusDeclared, Address: "198.51.100.24"}})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got RuntimeState
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	if got.Egress.Mode != egresspolicy.ModeManual || got.Egress.Status != egresspolicy.StatusDeclared || got.Egress.Address != "198.51.100.24" {
		t.Fatalf("runtime = %#v", got)
	}
}

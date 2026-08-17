package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestActionListQueryIsKeysetBoundedAndFilterable(t *testing.T) {
	startedAfter := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	startedBefore := startedAfter.Add(30 * time.Minute)
	endedAfter := startedAfter.Add(5 * time.Minute)
	endedBefore := startedAfter.Add(10 * time.Minute)
	filters, pb := parseActionListFilters(url.Values{
		"technique":     []string{"waypoint.nmap"},
		"target":        []string{"demo.local"},
		"host":          []string{"10.10.0.12"},
		"actor":         []string{"alex.operator"},
		"result":        []string{"failure"},
		"initiatedBy":   []string{"manual"},
		"provenance":    []string{"manual"},
		"parseStatus":   []string{"needs-plugin"},
		"startedAfter":  []string{startedAfter.Format(time.RFC3339Nano)},
		"startedBefore": []string{startedBefore.Format(time.RFC3339Nano)},
		"endedAfter":    []string{endedAfter.Format(time.RFC3339Nano)},
		"endedBefore":   []string{endedBefore.Format(time.RFC3339Nano)},
	})
	if pb != nil {
		t.Fatalf("parseActionListFilters() error = %#v", pb)
	}

	after := int64(99)
	query, args := buildActionListQuery("eng-1", &after, 50, filters)
	for _, want := range []string{
		"WHERE a.engagement_id = $1",
		"AND ae.id < $2",
		"AND a.plugin_id = $3",
		"AND (a.target_kind = $4 OR a.target_value = $4)",
		"AND a.exec_host_ip::text = $5",
		"AND (ar.id::text = $6 OR ar.handle = $6)",
		"AND (a.execution_status <> 'exited' OR COALESCE(a.exit_code, 0) <> 0)",
		"AND a.initiated_by::text = $7",
		"AND (a.initiated_by::text = $8 OR a.parse_status::text = $8",
		"AND a.parse_status::text = $9",
		"AND a.started_at >= $10",
		"AND a.started_at <= $11",
		"AND a.ended_at >= $12",
		"AND a.ended_at <= $13",
		"ORDER BY ae.id DESC LIMIT $14",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q\n%s", want, query)
		}
	}
	if strings.Contains(strings.ToUpper(query), " OFFSET ") {
		t.Fatalf("query unexpectedly uses OFFSET: %s", query)
	}
	if len(args) != 14 {
		t.Fatalf("arg count = %d, want 14", len(args))
	}
	if args[0] != "eng-1" || args[1] != after || args[2] != "waypoint.nmap" {
		t.Fatalf("leading args = %#v", args[:3])
	}
}

func TestActionListTimeParsingRejectsBadTimestamp(t *testing.T) {
	if _, pb := parseActionTimeParam("not-a-time", "/startedAfter"); pb == nil || len(pb.FieldErrors) == 0 || pb.FieldErrors[0].Code != "invalid_timestamp" {
		t.Fatalf("parseActionTimeParam() = %#v, want invalid_timestamp", pb)
	}
	if got, pb := parseActionTimeParam("2025-01-15T10:00:00Z", "/startedAfter"); pb != nil || got == nil || got.IsZero() {
		t.Fatalf("parseActionTimeParam() = %v, %#v", got, pb)
	}
}

func TestActionHandlerWithoutDatabaseIsUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/actions", nil)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Authorization", "Bearer demo-token")
	rr := httptest.NewRecorder()
	actionsHandler(nil, false).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestActionHandlerRejectsMissingAuthorizationBeforeQuerying(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/actions", nil)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	rr := httptest.NewRecorder()
	actionsHandler(noopActionQueryer{}, false).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

type noopActionQueryer struct{}

func (noopActionQueryer) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (noopActionQueryer) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("unexpected query")
}

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

// TestDemoSeedPopulatesEngagement drives the real first-run bootstrap with the
// demo flag set and asserts the engagement comes out coherently populated:
// actions across every phase, discovered entities, promoted findings, notable
// alerts, and a report snapshot that assembles without error.
func TestDemoSeedPopulatesEngagement(t *testing.T) {
	rawDB := openTestDB(t)
	defer rawDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	res := postBootstrap(t, client, ts.URL, map[string]any{
		"setupCode":  code,
		"engagement": map[string]any{"name": "Autumn Campus Assessment", "client": "Acme University", "scope": "campus /16"},
		"owner":      map[string]any{"handle": "alex.operator"},
		"demo":       true,
	})
	if res.status != 201 {
		t.Fatalf("bootstrap status = %d (code %q): %s", res.status, res.code, res.raw)
	}
	var resp struct {
		EngagementID string `json:"engagementId"`
	}
	if err := json.Unmarshal(res.raw, &resp); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	engagementID := resp.EngagementID
	if engagementID == "" {
		t.Fatal("bootstrap returned no engagement id")
	}

	// Actions land across all three phases.
	phaseCounts := map[string]int{}
	rows, err := rawDB.QueryContext(ctx, `SELECT phase::text, count(*) FROM action WHERE engagement_id = $1 GROUP BY phase`, engagementID)
	if err != nil {
		t.Fatalf("query actions: %v", err)
	}
	for rows.Next() {
		var phase string
		var n int
		if err := rows.Scan(&phase, &n); err != nil {
			t.Fatalf("scan action count: %v", err)
		}
		phaseCounts[phase] = n
	}
	rows.Close()
	for _, phase := range []string{"recon", "attacks", "findings"} {
		if phaseCounts[phase] == 0 {
			t.Errorf("expected at least one %s action, got none", phase)
		}
	}

	// the story creates 7 named entities and seedEstate fills out the wider estate;
	// assert a lower bound so estate tweaks don't make this brittle.
	var entityCount int
	if err := rawDB.QueryRowContext(ctx, "SELECT count(*) FROM entity WHERE engagement_id = $1", engagementID).Scan(&entityCount); err != nil {
		t.Fatalf("count entities: %v", err)
	}
	if entityCount < 70 {
		t.Errorf("expected a populated estate (>=70 entities), got %d", entityCount)
	}
	assertCount(t, ctx, rawDB, "SELECT count(*) FROM finding WHERE engagement_id = $1", engagementID, 3)
	assertCount(t, ctx, rawDB, "SELECT count(*) FROM audit_event WHERE engagement_id = $1 AND type = 'alert.notable'", engagementID, 2)
	assertCount(t, ctx, rawDB, "SELECT count(*) FROM result WHERE engagement_id = $1", engagementID, 9)
	assertCount(t, ctx, rawDB, "SELECT count(*) FROM observation WHERE engagement_id = $1", engagementID, 11)

	// AI-initiated actions must carry a decision context; manual ones must not.
	var aiWithoutContext int
	if err := rawDB.QueryRowContext(ctx, `SELECT count(*) FROM action WHERE engagement_id = $1 AND initiated_by = 'ai' AND decision_context IS NULL`, engagementID).Scan(&aiWithoutContext); err != nil {
		t.Fatalf("query ai actions: %v", err)
	}
	if aiWithoutContext != 0 {
		t.Errorf("found %d AI actions with no decision context", aiWithoutContext)
	}

	// A critical finding should be present and rank first in the report.
	store := newEvidenceStore()
	snapshot, err := buildReportSnapshot(ctx, rawDB, store, engagementID)
	if err != nil {
		t.Fatalf("build report snapshot: %v", err)
	}
	if len(snapshot.Findings) != 3 {
		t.Fatalf("expected 3 findings in report, got %d", len(snapshot.Findings))
	}
	if !strings.EqualFold(snapshot.Findings[0].Severity, "critical") {
		t.Errorf("expected critical finding ranked first, got %q", snapshot.Findings[0].Severity)
	}
}

func assertCount(t *testing.T, ctx context.Context, db *sql.DB, query, engagementID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(ctx, query, engagementID).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Errorf("count query %q = %d, want %d", query, got, want)
	}
}

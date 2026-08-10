package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	dbm "waypoint/internal/db"
)

func TestAuditHistoryPaginationSSEReconnectAndRevocation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "11111111-1111-4111-8111-111111111111"
	actorID := "22222222-2222-4222-8222-222222222222"
	token := "audit-stream-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	eventIDs := appendAuditEvents(t, db, engagementID, actorID, []string{"journey.note.added", "journey.note.updated", "journey.note.closed"})

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	page := decodeAuditPage(t, doAuditRequest(t, ts.Client(), ts.URL+"/api/v1/audit-events?limit=2", token, "req-page", "", ""))
	if len(page.Items) != 2 {
		t.Fatalf("page items = %d, want 2", len(page.Items))
	}
	if page.Page.NextCursor != eventIDs[1] {
		t.Fatalf("nextCursor = %q, want %q", page.Page.NextCursor, eventIDs[1])
	}
	if page.Page.HighWaterCursor == nil || *page.Page.HighWaterCursor != eventIDs[2] {
		t.Fatalf("highWaterCursor = %#v, want %q", page.Page.HighWaterCursor, eventIDs[2])
	}

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events?after="+url.QueryEscape(eventIDs[1]), token, "req-sse", eventIDs[1], "")
	frame := readSSEFrame(t, sseResp.Body)
	_ = sseResp.Body.Close()
	if frame["id"] != eventIDs[2] {
		t.Fatalf("sse id = %q, want %q", frame["id"], eventIDs[2])
	}
	if frame["event"] != "journey.note.closed" {
		t.Fatalf("sse event = %q, want journey.note.closed", frame["event"])
	}
	if !strings.Contains(frame["data"], eventIDs[2]) {
		t.Fatalf("sse data = %q, want event payload", frame["data"])
	}

	mustExec(t, db, `UPDATE actor SET revoked_at = now() WHERE id = $1`, actorID)
	revoked := doAuditRequest(t, ts.Client(), ts.URL+"/events", token, "req-revoked", "", "")
	defer revoked.Body.Close()
	if revoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked SSE status = %d, want %d", revoked.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuditEventsCursorExpiredReturnsResyncLink(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "33333333-3333-4333-8333-333333333333"
	actorID := "44444444-4444-4444-8444-444444444444"
	token := "cursor-expired-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	appendAuditEvents(t, db, engagementID, actorID, 105)

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	resp := doAuditRequest(t, ts.Client(), ts.URL+"/events?after=40", token, "req-expired", "40", "")
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expired SSE status = %d, want %d", resp.StatusCode, http.StatusGone)
	}
	defer resp.Body.Close()
	var prob captureProblem
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode expired problem: %v", err)
	}
	if prob.Code != "cursor_expired" || prob.MinimumAvailableCursor == nil || *prob.MinimumAvailableCursor != "41" {
		t.Fatalf("problem = %#v", prob)
	}
	if prob.Resync != "/api/v1/audit-events?after=40" {
		t.Fatalf("resync = %q, want /api/v1/audit-events?after=40", prob.Resync)
	}
}

func TestTailAuditEventsStopsWhenQueueIsFull(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "55555555-5555-4555-8555-555555555555"
	actorID := "66666666-6666-4666-8666-666666666666"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', repeat('b', 64), 'operator')`, actorID, engagementID)
	appendAuditEvents(t, db, engagementID, actorID, []string{"queue.test.one", "queue.test.two", "queue.test.three"})

	out := make(chan auditStreamMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- tailAuditEvents(ctx, db, actorID, engagementID, nil, out)
	}()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "slow client") {
			t.Fatalf("tail error = %v, want slow client", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tailAuditEvents did not stop for a full queue")
	}
}

func appendAuditEvents(t *testing.T, db *sql.DB, engagementID, actorID string, types any) []string {
	t.Helper()
	var eventTypes []string
	switch v := types.(type) {
	case int:
		eventTypes = make([]string, 0, v)
		for i := 0; i < v; i++ {
			eventTypes = append(eventTypes, fmt.Sprintf("queue.test.%d", i+1))
		}
	case []string:
		eventTypes = append(eventTypes, v...)
	default:
		t.Fatalf("unsupported event type input %T", types)
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	ids := make([]string, 0, len(eventTypes))
	for i, typ := range eventTypes {
		id, err := dbm.AppendAuditEvent(context.Background(), tx, dbm.AuditEventInput{
			EngagementID: engagementID,
			Type:         typ,
			Actor: dbm.AuditActorSnapshot{
				ID:     actorID,
				Kind:   "human",
				Handle: "alex.operator",
				Role:   "operator",
			},
			Origin:        dbm.AuditOrigin{Kind: "rest"},
			Subject:       dbm.AuditSubject{Type: "action", ID: fmt.Sprintf("77777777-7777-4%03d-8777-777777777777", i), Revision: 1},
			RequestID:     fmt.Sprintf("req-%d", i+1),
			CorrelationID: fmt.Sprintf("corr-%d", i+1),
			Data:          map[string]any{"ordinal": i + 1},
		})
		if err != nil {
			t.Fatalf("append audit event: %v", err)
		}
		ids = append(ids, fmt.Sprint(id))
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	return ids
}

func doAuditRequest(t *testing.T, client *http.Client, rawURL, token, requestID, after, lastEventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("X-Request-ID", requestID)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeAuditPage(t *testing.T, resp *http.Response) auditPageResponse {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("page status = %d, body=%s", resp.StatusCode, body)
	}
	var page auditPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return page
}

func readSSEFrame(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	frame := map[string]string{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		frame[key] = value
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan sse frame: %v", err)
	}
	return frame
}

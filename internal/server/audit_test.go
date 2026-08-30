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

func TestAuditHistoryPaginationSSEReconnectFilteringAndRevocation(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementA := "11111111-1111-4111-8111-111111111111"
	engagementB := "22222222-2222-4222-8222-222222222222"
	actorA := "33333333-3333-4333-8333-333333333333"
	actorB := "44444444-4444-4444-8444-444444444444"
	tokenA := "audit-stream-token-a"
	tokenB := "audit-stream-token-b"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo A', 'Client A', 'Scope A', 'active')`, engagementA)
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo B', 'Client B', 'Scope B', 'active')`, engagementB)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorA, engagementA, hashHex(tokenA))
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'beth.operator', $3, 'operator')`, actorB, engagementB, hashHex(tokenB))

	a1 := appendAuditEvents(t, db, engagementA, actorA, []string{"journey.note.added"})[0]
	b1 := appendAuditEvents(t, db, engagementB, actorB, []string{"journey.note.added"})[0]
	a2 := appendAuditEvents(t, db, engagementA, actorA, []string{"journey.note.updated"})[0]
	a3 := appendAuditEvents(t, db, engagementA, actorA, []string{"journey.note.closed"})[0]

	if !(mustCursor(a1) < mustCursor(b1) && mustCursor(b1) < mustCursor(a2) && mustCursor(a2) < mustCursor(a3)) {
		t.Fatalf("event ids are not monotonic: %q %q %q %q", a1, b1, a2, a3)
	}

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	page := decodeAuditPage(t, doAuditRequest(t, ts.Client(), ts.URL+"/api/v1/audit-events?limit=2", tokenA, "req-page", "", ""))
	if len(page.Items) != 2 {
		t.Fatalf("page items = %d, want 2", len(page.Items))
	}
	if page.Items[0].ID != a1 || page.Items[1].ID != a2 {
		t.Fatalf("page ids = %q, want %q then %q", []string{page.Items[0].ID, page.Items[1].ID}, a1, a2)
	}
	if page.Items[0].EngagementID != engagementA || page.Items[1].EngagementID != engagementA {
		t.Fatalf("page engagement ids = %q, want only %q", []string{page.Items[0].EngagementID, page.Items[1].EngagementID}, engagementA)
	}
	if page.Page.NextCursor != a2 {
		t.Fatalf("nextCursor = %q, want %q", page.Page.NextCursor, a2)
	}
	if page.Page.HighWaterCursor == nil || *page.Page.HighWaterCursor != a3 {
		t.Fatalf("highWaterCursor = %#v, want %q", page.Page.HighWaterCursor, a3)
	}
	if !page.Page.HasMore {
		t.Fatalf("page hasMore = false, want true")
	}

	page2 := decodeAuditPage(t, doAuditRequest(t, ts.Client(), ts.URL+"/api/v1/audit-events?after="+url.QueryEscape(a2)+"&limit=2", tokenA, "req-page-2", "", ""))
	if len(page2.Items) != 1 || page2.Items[0].ID != a3 {
		t.Fatalf("page2 items = %#v, want only %q", page2.Items, a3)
	}
	if page2.Page.HasMore {
		t.Fatalf("page2 hasMore = true, want false")
	}

	sseResp := doAuditRequest(t, ts.Client(), ts.URL+"/events", tokenA, "req-sse", "", a1)
	frame := readSSEFrame(t, sseResp.Body)
	_ = sseResp.Body.Close()
	if frame["id"] != a2 {
		t.Fatalf("sse id = %q, want %q", frame["id"], a2)
	}
	if frame["event"] != "journey.note.updated" {
		t.Fatalf("sse event = %q, want journey.note.updated", frame["event"])
	}
	if !strings.Contains(frame["data"], `"engagementId":"`+engagementA+`"`) {
		t.Fatalf("sse data = %q, want engagement %q", frame["data"], engagementA)
	}

	mustExec(t, db, `UPDATE actor SET revoked_at = now() WHERE id = $1`, actorA)
	revoked := doAuditRequest(t, ts.Client(), ts.URL+"/events", tokenA, "req-revoked", "", a2)
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

func TestAuditSSEHeartbeatAndCommittedCaptureVisibility(t *testing.T) {
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
	token := "heartbeat-token"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alex.operator', $3, 'operator')`, actorID, engagementID, hashHex(token))

	ts := httptest.NewServer(HandlerWithDB(db))
	defer ts.Close()

	resp := doAuditRequest(t, ts.Client(), ts.URL+"/events", token, "req-heartbeat", "", "")

	heartbeatCh := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		line, err := readSSELineValue(resp.Body)
		heartbeatCh <- struct {
			line string
			err  error
		}{line: line, err: err}
	}()

	select {
	case result := <-heartbeatCh:
		if result.err != nil {
			t.Fatalf("read heartbeat line: %v", result.err)
		}
		if result.line != ": heartbeat" {
			t.Fatalf("heartbeat line = %q, want %q", result.line, ": heartbeat")
		}
	case <-time.After(12 * time.Second):
		t.Fatal("timed out waiting for SSE heartbeat")
	}
	_ = resp.Body.Close()

	auditResp := doAuditRequest(t, ts.Client(), ts.URL+"/events", token, "req-visibility", "", "")
	defer auditResp.Body.Close()

	visibleCh := make(chan struct {
		frame map[string]string
		err   error
	}, 1)
	go func() {
		frame, err := readSSEFrameValue(auditResp.Body)
		visibleCh <- struct {
			frame map[string]string
			err   error
		}{frame: frame, err: err}
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	eventID, err := dbm.AppendAuditEvent(ctx, tx, dbm.AuditEventInput{
		EngagementID: engagementID,
		Type:         "capture.accepted",
		Actor: dbm.AuditActorSnapshot{
			ID:     actorID,
			Kind:   "human",
			Handle: "alex.operator",
			Role:   "operator",
		},
		Origin:        dbm.AuditOrigin{Kind: "rest"},
		Subject:       dbm.AuditSubject{Type: "action", ID: "99999999-9999-4999-8999-999999999999", Revision: 1},
		RequestID:     "req-visibility",
		CorrelationID: "corr-visibility",
		Data:          map[string]any{"captureId": "99999999-9999-4999-8999-999999999999"},
	})
	if err != nil {
		t.Fatalf("append capture audit event: %v", err)
	}

	var beforeVisible int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE id = $1`, eventID).Scan(&beforeVisible); err != nil {
		t.Fatalf("count uncommitted audit event: %v", err)
	}
	if beforeVisible != 0 {
		t.Fatalf("uncommitted audit event count = %d, want 0", beforeVisible)
	}

	select {
	case result := <-visibleCh:
		t.Fatalf("sse frame became visible before commit: %#v %v", result.frame, result.err)
	case <-time.After(250 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	select {
	case result := <-visibleCh:
		if result.err != nil {
			t.Fatalf("read committed sse frame: %v", result.err)
		}
		if result.frame["id"] != fmt.Sprint(eventID) {
			t.Fatalf("committed sse id = %q, want %q", result.frame["id"], fmt.Sprint(eventID))
		}
		if result.frame["event"] != "capture.accepted" {
			t.Fatalf("committed sse event = %q, want capture.accepted", result.frame["event"])
		}
		if !strings.Contains(result.frame["data"], `"captureId":"99999999-9999-4999-8999-999999999999"`) {
			t.Fatalf("committed sse data = %q, want capture payload", result.frame["data"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for committed capture event on SSE")
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

	var actorKind, actorHandle, actorRole string
	if err := db.QueryRowContext(context.Background(), `SELECT kind, handle, role FROM actor WHERE id = $1`, actorID).Scan(&actorKind, &actorHandle, &actorRole); err != nil {
		t.Fatalf("load actor snapshot: %v", err)
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
				Kind:   actorKind,
				Handle: actorHandle,
				Role:   actorRole,
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
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if after != "" {
		q := parsed.Query()
		q.Set("after", after)
		parsed.RawQuery = q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
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
	frame, err := readSSEFrameValue(r)
	if err != nil {
		t.Fatalf("scan sse frame: %v", err)
	}
	return frame
}

func readSSEFrameValue(r io.Reader) (map[string]string, error) {
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
		return nil, err
	}
	return frame, nil
}

func readSSELineValue(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

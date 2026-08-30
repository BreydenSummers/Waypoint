package db

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRedactAuditData(t *testing.T) {
	redacted := RedactAuditData(map[string]any{
		"password": "secret",
		"nested": map[string]any{
			"token": "abc",
			"keep":  "ok",
		},
		"items": []any{map[string]any{"authorization": "bearer 123"}},
	})
	if got := redacted["password"]; got != "[redacted]" {
		t.Fatalf("password = %v, want redacted", got)
	}
	nested := redacted["nested"].(map[string]any)
	if got := nested["token"]; got != "[redacted]" {
		t.Fatalf("nested token = %v, want redacted", got)
	}
	items := redacted["items"].([]any)
	if got := items[0].(map[string]any)["authorization"]; got != "[redacted]" {
		t.Fatalf("authorization = %v, want redacted", got)
	}
}

func TestAppendAuditEventCapturesOutOfBandReviewLifecycle(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "00000000-0000-0000-0000-000000000051"
	humanID := "00000000-0000-0000-0000-000000000052"
	claimID := "00000000-0000-0000-0000-000000000053"
	actionID := "00000000-0000-0000-0000-000000000054"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, humanID, engagementID)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	flaggedID, err := AppendAuditEvent(ctx, tx, AuditEventInput{
		EngagementID:  engagementID,
		Type:          "out-of-band.flagged",
		Actor:         AuditActorSnapshot{ID: humanID, Kind: "human", Handle: "alice", Role: "owner"},
		Origin:        AuditOrigin{Kind: "service", Service: "claim-detector"},
		Subject:       AuditSubject{Type: "out_of_band_claim", ID: claimID, Revision: 1},
		RequestID:     "req-oob-flagged",
		CorrelationID: "corr-oob-flagged",
		Data: map[string]any{
			"claimId":           claimID,
			"claimKind":         "entity",
			"sourceActionId":    nil,
			"detectionBoundary": "best_effort",
			"reason":            "missing_captured_source_action",
			"observedAt":        time.Date(2025, 1, 15, 10, 25, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("append flagged event: %v", err)
	}
	resolvedID, err := AppendAuditEvent(ctx, tx, AuditEventInput{
		EngagementID:  engagementID,
		Type:          "out-of-band.resolved",
		Actor:         AuditActorSnapshot{ID: humanID, Kind: "human", Handle: "alice", Role: "owner"},
		Origin:        AuditOrigin{Kind: "rest"},
		Subject:       AuditSubject{Type: "out_of_band_claim", ID: claimID, Revision: 1},
		RequestID:     "req-oob-resolved",
		CorrelationID: "corr-oob-flagged",
		Data: map[string]any{
			"claimId":        claimID,
			"claimKind":      "entity",
			"sourceActionId": actionID,
			"resolution":     "linked",
			"resolvedAt":     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			"notes":          "Linked after operator review of the imported host record.",
		},
	})
	if err != nil {
		t.Fatalf("append resolved event: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, tc := range []struct {
		id       int64
		typeName string
		wantKey  string
		wantVal  string
	}{
		{id: flaggedID, typeName: "out-of-band.flagged", wantKey: "detectionBoundary", wantVal: "best_effort"},
		{id: resolvedID, typeName: "out-of-band.resolved", wantKey: "resolution", wantVal: "linked"},
	} {
		var typ, subjectType, raw string
		if err := db.QueryRowContext(ctx, `SELECT type, subject_type, data::text FROM audit_event WHERE id = $1`, tc.id).Scan(&typ, &subjectType, &raw); err != nil {
			t.Fatalf("load %s event: %v", tc.typeName, err)
		}
		if typ != tc.typeName || subjectType != "out_of_band_claim" {
			t.Fatalf("event = (%s, %s), want (%s, out_of_band_claim)", typ, subjectType, tc.typeName)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Fatalf("decode %s event data: %v", tc.typeName, err)
		}
		if parsed[tc.wantKey] != tc.wantVal {
			t.Fatalf("event data = %s, want %s=%s", raw, tc.wantKey, tc.wantVal)
		}
	}
}

func TestValidateAuditEventInputRejectsIncompleteAIActor(t *testing.T) {
	err := validateAuditEventInput(AuditEventInput{
		EngagementID:  "00000000-0000-0000-0000-000000000001",
		Actor:         AuditActorSnapshot{ID: "00000000-0000-0000-0000-000000000002", Kind: "ai_agent", Handle: "bot", Role: "operator"},
		Origin:        AuditOrigin{Kind: "rest"},
		Subject:       AuditSubject{Type: "action", ID: "00000000-0000-0000-0000-000000000003", Revision: 1},
		RequestID:     "req-1",
		CorrelationID: "corr-1",
	})
	if err == nil {
		t.Fatal("expected incomplete AI actor snapshot to fail validation")
	}
}

func TestAppendAuditEventRedactsSensitiveMetadata(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "00000000-0000-0000-0000-000000000101"
	actorID := "00000000-0000-0000-0000-000000000102"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, actorID, engagementID)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	eventID, err := AppendAuditEvent(ctx, tx, AuditEventInput{
		EngagementID: engagementID,
		Actor: AuditActorSnapshot{
			ID:     actorID,
			Kind:   "human",
			Handle: "alice",
			Role:   "owner",
		},
		Origin:        AuditOrigin{Kind: "rest"},
		Subject:       AuditSubject{Type: "action", ID: "00000000-0000-0000-0000-000000000103", Revision: 1},
		RequestID:     "req-1",
		CorrelationID: "corr-1",
		Data: map[string]any{
			"username": "alice",
			"password": "s3cr3t",
			"token":    "bearer-abc",
			"nested": map[string]any{
				"authorization": "Bearer abc",
				"note":          "ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("append audit event: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var raw string
	if err := db.QueryRowContext(ctx, `SELECT data::text FROM audit_event WHERE id = $1`, eventID).Scan(&raw); err != nil {
		t.Fatalf("load audit data: %v", err)
	}
	if strings.Contains(raw, "s3cr3t") || strings.Contains(raw, "bearer-abc") || strings.Contains(raw, "Bearer abc") {
		t.Fatalf("audit data leaked sensitive content: %s", raw)
	}
	if !strings.Contains(raw, "[redacted]") {
		t.Fatalf("audit data was not redacted: %s", raw)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal redacted payload: %v", err)
	}
	if got := payload["password"]; got != "[redacted]" {
		t.Fatalf("password = %v, want redacted", got)
	}
	if got := payload["token"]; got != "[redacted]" {
		t.Fatalf("token = %v, want redacted", got)
	}
}

func TestAppendAuditEventCommitsConcurrentlyAndRollsBackCleanly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	db.SetMaxOpenConns(8)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "00000000-0000-0000-0000-000000000201"
	actorID := "00000000-0000-0000-0000-000000000202"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, actorID, engagementID)

	const workers = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				errCh <- err
				return
			}
			defer tx.Rollback()

			_, err = AppendAuditEvent(ctx, tx, AuditEventInput{
				EngagementID:  engagementID,
				Actor:         AuditActorSnapshot{ID: actorID, Kind: "human", Handle: "alice", Role: "owner"},
				Origin:        AuditOrigin{Kind: "rest"},
				Subject:       AuditSubject{Type: "action", ID: "00000000-0000-0000-0000-000000000300", Revision: 1},
				RequestID:     "req-concurrent",
				CorrelationID: "corr-concurrent",
				Data:          map[string]any{"worker": i},
			})
			if err != nil {
				errCh <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errCh <- err
				return
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent append failed: %v", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1`, engagementID).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != workers {
		t.Fatalf("audit_event count = %d, want %d", count, workers)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rollback tx: %v", err)
	}
	if _, err := AppendAuditEvent(ctx, tx, AuditEventInput{
		EngagementID:  engagementID,
		Actor:         AuditActorSnapshot{ID: actorID, Kind: "human", Handle: "alice", Role: "owner"},
		Origin:        AuditOrigin{Kind: "rest"},
		Subject:       AuditSubject{Type: "action", ID: "00000000-0000-0000-0000-000000000301", Revision: 1},
		RequestID:     "req-rollback",
		CorrelationID: "corr-rollback",
		Data:          map[string]any{"worker": "rollback"},
	}); err != nil {
		t.Fatalf("append rollback event: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_event WHERE engagement_id = $1`, engagementID).Scan(&count); err != nil {
		t.Fatalf("recount events: %v", err)
	}
	if count != workers {
		t.Fatalf("rollback changed committed audit rows: got %d, want %d", count, workers)
	}
}

func TestAuditEventViewAndTableRemainAppendOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "00000000-0000-0000-0000-000000000401"
	actorID := "00000000-0000-0000-0000-000000000402"
	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, actorID, engagementID)
	mustExec(t, db, `INSERT INTO audit_event (engagement_id, actor_id, actor_kind, actor_handle, actor_role, origin_kind, subject_type, subject_id, request_id, correlation_id) VALUES ($1, $2, 'human', 'alice', 'owner', 'rest', 'action', $3, 'req-1', 'corr-1')`, engagementID, actorID, "00000000-0000-0000-0000-000000000403")

	mustReject(t, db, `UPDATE audit_event SET request_id = 'changed' WHERE engagement_id = $1`, engagementID)
	mustReject(t, db, `DELETE FROM audit_event WHERE engagement_id = $1`, engagementID)
	mustReject(t, db, `UPDATE audit SET request_id = 'changed' WHERE engagement_id = $1`, engagementID)
	mustReject(t, db, `DELETE FROM audit WHERE engagement_id = $1`, engagementID)
}

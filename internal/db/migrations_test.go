package db

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("WAYPOINT_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func TestApplyMigrationsOnRealPostgreSQL(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	mustHaveTables(t, db, []string{"engagement", "actor", "evidence", "action", "entity", "result", "observation", "finding", "audit_event", "audit", "schema_migrations"})
	mustHaveIndexes(t, db, []string{
		"action_engagement_started_at_idx",
		"action_engagement_actor_started_at_idx",
		"actor_engagement_handle_idx",
		"audit_event_engagement_occurred_at_idx",
		"audit_event_engagement_subject_idx",
		"audit_event_request_id_idx",
		"audit_event_correlation_id_idx",
		"action_capture_scope_unique_idx",
		"entity_identity_unique",
		"finding_affected_entity_ids_idx",
		"observation_entity_observed_at_idx",
		"result_action_unique",
		"evidence_engagement_created_at_idx",
	})
}

func TestApplyMigrationsSerializesConcurrentStarters(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- ApplyMigrations(ctx, db)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent apply migrations: %v", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("schema migration count = %d, want 2", count)
	}
}

func TestDatabaseProtectionsRejectMutations(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	engagementID := "00000000-0000-0000-0000-000000000001"
	humanID := "00000000-0000-0000-0000-000000000002"
	aiID := "00000000-0000-0000-0000-000000000003"
	stdoutID := "00000000-0000-0000-0000-000000000004"
	stderrID := "00000000-0000-0000-0000-000000000005"
	orphanEvidenceID := "00000000-0000-0000-0000-000000000006"
	actionID := "00000000-0000-0000-0000-000000000007"
	resultID := "00000000-0000-0000-0000-000000000008"
	entityID := "00000000-0000-0000-0000-000000000009"
	deleteActionStdoutID := "00000000-0000-0000-0000-000000000010"
	deleteActionStderrID := "00000000-0000-0000-0000-000000000011"
	deleteActionID := "00000000-0000-0000-0000-000000000012"
	deleteResultActionStdoutID := "00000000-0000-0000-0000-000000000013"
	deleteResultActionStderrID := "00000000-0000-0000-0000-000000000014"
	deleteResultActionID := "00000000-0000-0000-0000-000000000015"
	deleteResultID := "00000000-0000-0000-0000-000000000016"

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, humanID, engagementID)
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by) VALUES ($1, $2, 'ai_agent', 'bot', repeat('b', 64), 'operator', 'Waypoint', 'gpt-4.1', '1.0', $3)`, aiID, engagementID, humanID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', repeat('c', 64), 0, 'text/plain', 'stdout/1')`, stdoutID, engagementID)
	mustReject(t, db, `INSERT INTO evidence (engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, 'stdout', repeat('c', 64), 0, 'text/plain', 'stdout/2')`, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', repeat('d', 64), 0, 'text/plain', 'stderr/1')`, stderrID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'screenshot', repeat('e', 64), 0, 'image/png', 'screenshots/1')`, orphanEvidenceID, engagementID)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $4, 'ai', 'recon', 'nmap', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'demo.local', now(), now(), 0, $5, $6, 'raw')`, actionID, engagementID, aiID, aiID, stdoutID, stderrID)
	mustExec(t, db, `INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted) VALUES ($1, $2, $3, 'plugin.demo', 'https://schemas.waypoint.security/demo', '1.0.0', '{}'::jsonb)`, resultID, engagementID, actionID)
	mustExec(t, db, `INSERT INTO entity (id, engagement_id, kind, key_type, key_value) VALUES ($1, $2, 'host', 'fqdn', 'demo.local')`, entityID, engagementID)
	mustReject(t, db, `INSERT INTO entity (engagement_id, kind, key_type, key_value) VALUES ($1, 'host', 'fqdn', 'demo.local')`, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', repeat('f', 64), 0, 'text/plain', 'stdout/2')`, deleteActionStdoutID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', repeat('1', 64), 0, 'text/plain', 'stderr/2')`, deleteActionStderrID, engagementID)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $4, 'manual', 'recon', 'whoami', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'demo.local', now(), now(), 0, $5, $6, 'raw')`, deleteActionID, engagementID, humanID, humanID, deleteActionStdoutID, deleteActionStderrID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stdout', repeat('2', 64), 0, 'text/plain', 'stdout/3')`, deleteResultActionStdoutID, engagementID)
	mustExec(t, db, `INSERT INTO evidence (id, engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, 'stderr', repeat('3', 64), 0, 'text/plain', 'stderr/3')`, deleteResultActionStderrID, engagementID)
	mustExec(t, db, `INSERT INTO action (id, engagement_id, actor_id, source_agent_id, initiated_by, phase, command, argv, cwd, exec_host_ip, pivot_chain, target_kind, target_value, started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, parse_status) VALUES ($1, $2, $3, $4, 'manual', 'recon', 'whoami', '[]'::jsonb, '/', '127.0.0.1', '[]'::jsonb, 'host', 'demo.local', now(), now(), 0, $5, $6, 'raw')`, deleteResultActionID, engagementID, humanID, humanID, deleteResultActionStdoutID, deleteResultActionStderrID)
	mustExec(t, db, `INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted) VALUES ($1, $2, $3, 'plugin.demo', 'https://schemas.waypoint.security/demo', '1.0.0', '{}'::jsonb)`, deleteResultID, engagementID, deleteResultActionID)
	mustExec(t, db, `INSERT INTO observation (id, engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes) VALUES (gen_random_uuid(), $1, $2, $3, $4, 'host', '[]'::jsonb, '{}'::jsonb)`, engagementID, actionID, resultID, entityID)
	findingID := "00000000-0000-0000-0000-000000000017"
	mustExec(t, db, `INSERT INTO finding (id, engagement_id, title, severity, status) VALUES ($1, $2, 'Weak SSH', 'low', 'open')`, findingID, engagementID)
	mustReject(t, db, `UPDATE finding SET evidence_action_ids = ARRAY[$1]::uuid[] WHERE id = $2`, actionID, findingID)
	mustExec(t, db, `INSERT INTO audit_event (engagement_id, actor_id, actor_kind, actor_handle, actor_role, actor_agent_name, actor_model, actor_version, actor_authorized_by, origin_kind, subject_type, subject_id, request_id, correlation_id, data) VALUES ($1, $2, 'ai_agent', 'bot', 'operator', 'Waypoint', 'gpt-4.1', '1.0', $3, 'rest', 'action', $4, 'req-1', 'corr-1', '{}'::jsonb)`, engagementID, aiID, humanID, actionID)

	mustReject(t, db, `UPDATE action SET command = 'changed' WHERE id = $1`, actionID)
	mustReject(t, db, `DELETE FROM action WHERE id = $1`, deleteActionID)
	mustReject(t, db, `UPDATE evidence SET storage_key = 'other' WHERE id = $1`, stdoutID)
	mustReject(t, db, `DELETE FROM evidence WHERE id = $1`, orphanEvidenceID)
	mustReject(t, db, `UPDATE result SET plugin_id = 'other' WHERE id = $1`, resultID)
	mustReject(t, db, `DELETE FROM result WHERE id = $1`, deleteResultID)
	mustReject(t, db, `UPDATE observation SET kind = 'changed' WHERE engagement_id = $1`, engagementID)
	mustReject(t, db, `UPDATE audit_event SET request_id = 'changed' WHERE engagement_id = $1`, engagementID)
	mustReject(t, db, `DELETE FROM audit_event WHERE engagement_id = $1`, engagementID)
}

func TestActorAuthorizationConstraint(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resetPublicSchema(t, db)

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	mustExec(t, db, `INSERT INTO engagement (id, name, client, scope, status) VALUES ($1, 'Demo', 'Client', 'Scope', 'active')`, "00000000-0000-0000-0000-000000000011")
	mustExec(t, db, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role) VALUES ($1, $2, 'human', 'alice', repeat('a', 64), 'owner')`, "00000000-0000-0000-0000-000000000012", "00000000-0000-0000-0000-000000000011")

	if _, err := db.ExecContext(ctx, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, agent_name, model, version) VALUES ($1, $2, 'ai_agent', 'bot', repeat('b', 64), 'operator', 'Waypoint', 'gpt-4.1', '1.0')`, "00000000-0000-0000-0000-000000000013", "00000000-0000-0000-0000-000000000011"); err == nil {
		t.Fatal("expected ai_agent insert without authorized_by to fail")
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role, authorized_by) VALUES ($1, $2, 'human', 'bob', repeat('c', 64), 'viewer', $3)`, "00000000-0000-0000-0000-000000000014", "00000000-0000-0000-0000-000000000011", "00000000-0000-0000-0000-000000000012"); err == nil {
		t.Fatal("expected human insert with authorized_by to fail")
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func mustReject(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, query, args...); err == nil {
		t.Fatalf("query unexpectedly succeeded: %s", query)
	}
}

func mustHaveTables(t *testing.T, db *sql.DB, names []string) {
	t.Helper()
	for _, name := range names {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = current_schema()
              AND table_name = $1
        )`, name).Scan(&exists); err != nil {
			t.Fatalf("lookup table %s: %v", name, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", name)
		}
	}
}

func mustHaveIndexes(t *testing.T, db *sql.DB, names []string) {
	t.Helper()
	for _, name := range names {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (
            SELECT 1 FROM pg_indexes
            WHERE schemaname = current_schema()
              AND indexname = $1
        )`, name).Scan(&exists); err != nil {
			t.Fatalf("lookup index %s: %v", name, err)
		}
		if !exists {
			t.Fatalf("expected index %s to exist", name)
		}
	}
}

func resetPublicSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stmts := []string{
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA public`,
		`GRANT ALL ON SCHEMA public TO public`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("reset schema with %q: %v", stmt, err)
		}
	}
}

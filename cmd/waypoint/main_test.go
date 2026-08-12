package main

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestOpenConfiguredDatabaseAppliesMigrations(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_PG_DSN")
	if dsn == "" {
		t.Fatal("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer seedDB.Close()

	if _, err := seedDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop public schema: %v", err)
	}
	if _, err := seedDB.ExecContext(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("recreate public schema: %v", err)
	}
	if _, err := seedDB.ExecContext(ctx, `GRANT ALL ON SCHEMA public TO public`); err != nil {
		t.Fatalf("grant public schema: %v", err)
	}

	db, err := openConfiguredDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("open configured database: %v", err)
	}
	defer db.Close()

	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name = 'engagement'
	)`).Scan(&exists); err != nil {
		t.Fatalf("check engagement table: %v", err)
	}
	if !exists {
		t.Fatal("expected engagement table to exist after startup migrations")
	}
}

func TestOpenConfiguredDatabaseRequiresDSN(t *testing.T) {
	if _, err := openConfiguredDatabase(context.Background(), " "); err == nil {
		t.Fatal("expected missing DSN to fail")
	}
}

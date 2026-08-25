package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	dbm "waypoint/internal/db"
	"waypoint/internal/egresspolicy"
	"waypoint/internal/server"
)

func main() {
	addr := os.Getenv("WAYPOINT_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openConfiguredDatabase(startupCtx, os.Getenv("WAYPOINT_DB_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	egressState, err := egresspolicy.ResolveFromEnv(startupCtx, os.Getenv, nil)
	if err != nil {
		log.Fatal(err)
	}
	if egressState.Status == egresspolicy.StatusResolutionFailed {
		log.Printf("egress auto-resolution failed: %v", egressState.Notes)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: server.HandlerWithDBAndRuntime(db, server.RuntimeState{Egress: egressState}),
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("waypoint listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func openConfiguredDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("WAYPOINT_DB_DSN is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if err := dbm.ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return db, nil
}

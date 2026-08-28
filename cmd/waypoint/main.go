package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
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
	startup, err := resolveStartupTransportConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
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
		Handler: server.HandlerWithDBAndRuntime(db, server.RuntimeState{Egress: egressState}),
	}

	ln, err := net.Listen("tcp", startup.addr)
	if err != nil {
		log.Fatal(err)
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

	scheme := "http"
	serve := srv.Serve
	if startup.tlsCertFile != "" {
		scheme = "https"
		serve = func(listener net.Listener) error {
			return srv.ServeTLS(listener, startup.tlsCertFile, startup.tlsKeyFile)
		}
	}

	log.Printf("waypoint listening on %s://%s", scheme, ln.Addr())
	if err := serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func resolveStartupTransportConfig(env func(string) string) (startupTransportConfig, error) {
	addr := strings.TrimSpace(env("WAYPOINT_ADDR"))
	if addr == "" {
		addr = defaultWaypointAddr
	}

	certFile := strings.TrimSpace(env("WAYPOINT_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(env("WAYPOINT_TLS_KEY_FILE"))
	if certFile == "" || keyFile == "" {
		if certFile != keyFile {
			return startupTransportConfig{}, fmt.Errorf("WAYPOINT_TLS_CERT_FILE and WAYPOINT_TLS_KEY_FILE must both be set for TLS listeners")
		}
		if !isLoopbackBindAddress(addr) {
			return startupTransportConfig{}, fmt.Errorf("WAYPOINT_ADDR must bind loopback unless WAYPOINT_TLS_CERT_FILE and WAYPOINT_TLS_KEY_FILE are set")
		}
		return startupTransportConfig{addr: addr}, nil
	}

	return startupTransportConfig{addr: addr, tlsCertFile: certFile, tlsKeyFile: keyFile}, nil
}

func isLoopbackBindAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return net.ParseIP(strings.Trim(host, "[]")).IsLoopback()
}

const defaultWaypointAddr = "127.0.0.1:8080"

type startupTransportConfig struct {
	addr        string
	tlsCertFile string
	tlsKeyFile  string
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

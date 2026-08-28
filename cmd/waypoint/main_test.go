package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"waypoint/internal/server"
)

func TestOpenConfiguredDatabaseAppliesMigrations(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests")
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

func TestResolveStartupTransportConfigDefaultsToLoopback(t *testing.T) {
	cfg, err := resolveStartupTransportConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("resolve startup config: %v", err)
	}
	if cfg.addr != defaultWaypointAddr {
		t.Fatalf("addr = %q, want %q", cfg.addr, defaultWaypointAddr)
	}
	if cfg.tlsCertFile != "" || cfg.tlsKeyFile != "" {
		t.Fatalf("unexpected TLS config: %#v", cfg)
	}
}

func TestResolveStartupTransportConfigRejectsNonLoopbackWithoutTLS(t *testing.T) {
	_, err := resolveStartupTransportConfig(func(key string) string {
		switch key {
		case "WAYPOINT_ADDR":
			return "0.0.0.0:8080"
		default:
			return ""
		}
	})
	if err == nil {
		t.Fatal("expected non-loopback bind without TLS to fail")
	}
}

func TestResolveStartupTransportConfigAllowsTLSOnNonLoopback(t *testing.T) {
	cfg, err := resolveStartupTransportConfig(func(key string) string {
		switch key {
		case "WAYPOINT_ADDR":
			return "0.0.0.0:8443"
		case "WAYPOINT_TLS_CERT_FILE":
			return "/tmp/server.crt"
		case "WAYPOINT_TLS_KEY_FILE":
			return "/tmp/server.key"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("resolve TLS startup config: %v", err)
	}
	if cfg.addr != "0.0.0.0:8443" || cfg.tlsCertFile != "/tmp/server.crt" || cfg.tlsKeyFile != "/tmp/server.key" {
		t.Fatalf("unexpected TLS config: %#v", cfg)
	}
}

func TestTLSReadyzServesOverLoopback(t *testing.T) {
	certFile, keyFile := mustWriteSelfSignedCertPair(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: server.Handler()}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ServeTLS(ln, certFile, keyFile)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		if err := <-serveErr; err != nil && err != http.ErrServerClosed {
			t.Fatalf("serve TLS: %v", err)
		}
	}()

	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = client.Get("https://" + ln.Addr().String() + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /healthz over TLS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func mustWriteSelfSignedCertPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "waypoint-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	certOut, err := os.OpenFile(certFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		_ = certOut.Close()
		t.Fatalf("write cert: %v", err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatalf("close cert: %v", err)
	}
	keyOut, err := os.OpenFile(keyFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open key file: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		_ = keyOut.Close()
		t.Fatalf("write key: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		t.Fatalf("close key: %v", err)
	}
	return certFile, keyFile
}

package egresspolicy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseConfigFromEnv(t *testing.T) {
	t.Run("auto requires endpoint", func(t *testing.T) {
		_, err := ParseConfigFromEnv(func(key string) string {
			switch key {
			case "WAYPOINT_EGRESS_MODE":
				return "auto"
			default:
				return ""
			}
		})
		if err == nil {
			t.Fatal("expected missing auto endpoint to fail")
		}
	})

	t.Run("manual requires an explicit address", func(t *testing.T) {
		_, err := ParseConfigFromEnv(func(key string) string {
			switch key {
			case "WAYPOINT_EGRESS_MODE":
				return "manual"
			default:
				return ""
			}
		})
		if err == nil {
			t.Fatal("expected missing manual address to fail")
		}
	})

	t.Run("off is the default", func(t *testing.T) {
		cfg, err := ParseConfigFromEnv(func(string) string { return "" })
		if err != nil {
			t.Fatalf("parse default config: %v", err)
		}
		if cfg.Mode != ModeOff {
			t.Fatalf("mode = %s, want off", cfg.Mode)
		}
	})
}

func TestResolveAutoManualAndOff(t *testing.T) {
	t.Run("auto resolves through a configured endpoint", func(t *testing.T) {
		resolver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"address":"198.51.100.25"}`))
		}))
		defer resolver.Close()

		state, err := Resolve(context.Background(), Config{Mode: ModeAuto, Endpoint: resolver.URL}, resolver.Client())
		if err != nil {
			t.Fatalf("resolve auto: %v", err)
		}
		if state.Mode != ModeAuto || state.Status != StatusObserved || state.Address != "198.51.100.25" || state.ObservedAt == nil || state.ResolverEndpoint != resolver.URL {
			t.Fatalf("state = %#v", state)
		}
	})

	t.Run("manual sends no discovery traffic", func(t *testing.T) {
		trap, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Skipf("packet trap unavailable: %v", err)
		}
		defer trap.Close()

		received := make(chan []byte, 1)
		go func() {
			buf := make([]byte, 256)
			_ = trap.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			n, _, err := trap.ReadFrom(buf)
			if err == nil && n > 0 {
				pkt := make([]byte, n)
				copy(pkt, buf[:n])
				received <- pkt
			}
		}()

		state, err := Resolve(context.Background(), Config{Mode: ModeManual, Address: "198.51.100.24", Endpoint: "http://" + trap.LocalAddr().String()}, nil)
		if err != nil {
			t.Fatalf("resolve manual: %v", err)
		}
		if state.Mode != ModeManual || state.Status != StatusDeclared || state.Address != "198.51.100.24" || state.ObservedAt == nil {
			t.Fatalf("state = %#v", state)
		}
		select {
		case pkt := <-received:
			t.Fatalf("manual mode sent discovery traffic: %q", pkt)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("off sends no discovery traffic", func(t *testing.T) {
		trap, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Skipf("packet trap unavailable: %v", err)
		}
		defer trap.Close()

		received := make(chan []byte, 1)
		go func() {
			buf := make([]byte, 256)
			_ = trap.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			n, _, err := trap.ReadFrom(buf)
			if err == nil && n > 0 {
				pkt := make([]byte, n)
				copy(pkt, buf[:n])
				received <- pkt
			}
		}()

		state, err := Resolve(context.Background(), Config{Mode: ModeOff, Endpoint: "http://" + trap.LocalAddr().String(), Address: "198.51.100.24"}, nil)
		if err != nil {
			t.Fatalf("resolve off: %v", err)
		}
		if state.Mode != ModeOff || state.Status != StatusDisabled || state.Address != "" {
			t.Fatalf("state = %#v", state)
		}
		select {
		case pkt := <-received:
			t.Fatalf("off mode sent discovery traffic: %q", pkt)
		case <-time.After(200 * time.Millisecond):
		}
	})
}

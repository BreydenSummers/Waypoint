package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"

	"waypoint/internal/server"
)

// firstRunPlan is the outcome of resolving how a pristine instance should be
// provisioned: interactively (a setup code + web wizard) or automatically (from
// bootstrap environment variables).
type firstRunPlan struct {
	// setupCode, when non-empty, arms the web wizard gate and is printed in the
	// startup banner for the operator to paste in.
	setupCode string
	// autoOwnerToken, when non-empty, is a freshly generated owner token from an
	// automated bootstrap that did not supply its own token; it is printed once.
	autoOwnerToken string
	// autoOwnerHandle labels the automated owner token in the banner.
	autoOwnerHandle string
}

// setupCodeAlphabet excludes visually ambiguous characters (0/O, 1/I/L) so a
// code read off a terminal is easy to type back correctly.
const setupCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// configureFirstRun decides the first-run provisioning path for a pristine
// instance and performs any automated bootstrap. It returns the plan so main
// can arm the wizard gate and print the banner after the listener is up.
func configureFirstRun(ctx context.Context, db *sql.DB, env func(string) string) (firstRunPlan, error) {
	if !server.SetupRequiredNow(ctx, db) {
		return firstRunPlan{}, nil
	}

	bp, ok := resolveBootstrapEnv(env)
	if ok {
		token, provisioned, err := server.AutoBootstrap(ctx, db, bp)
		if err != nil {
			return firstRunPlan{}, fmt.Errorf("automated bootstrap: %w", err)
		}
		if provisioned && token != "" && strings.TrimSpace(bp.OwnerToken) == "" {
			// Only surface a token we generated; a caller-supplied token is
			// already known to the operator and must not be echoed.
			return firstRunPlan{autoOwnerToken: token, autoOwnerHandle: bp.OwnerHandle}, nil
		}
		return firstRunPlan{}, nil
	}

	if truthyEnv(env("WAYPOINT_DISABLE_SETUP_WIZARD")) {
		return firstRunPlan{}, nil
	}

	code, err := generateSetupCode()
	if err != nil {
		return firstRunPlan{}, fmt.Errorf("generate setup code: %w", err)
	}
	return firstRunPlan{setupCode: code}, nil
}

func resolveBootstrapEnv(env func(string) string) (server.BootstrapParams, bool) {
	bp := server.BootstrapParams{
		EngagementName: strings.TrimSpace(env("WAYPOINT_BOOTSTRAP_ENGAGEMENT_NAME")),
		Client:         strings.TrimSpace(env("WAYPOINT_BOOTSTRAP_ENGAGEMENT_CLIENT")),
		Scope:          strings.TrimSpace(env("WAYPOINT_BOOTSTRAP_ENGAGEMENT_SCOPE")),
		OwnerHandle:    strings.TrimSpace(env("WAYPOINT_BOOTSTRAP_OWNER_HANDLE")),
		OwnerToken:     strings.TrimSpace(env("WAYPOINT_BOOTSTRAP_OWNER_TOKEN")),
	}
	// The automated path activates only when the full first engagement + owner
	// is described; a partial set falls through to the interactive wizard.
	if bp.EngagementName == "" || bp.Client == "" || bp.Scope == "" || bp.OwnerHandle == "" {
		return server.BootstrapParams{}, false
	}
	return bp, true
}

func truthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// generateSetupCode returns a 16-character code grouped as XXXX-XXXX-XXXX-XXXX.
func generateSetupCode() (string, error) {
	const n = 16
	chars := make([]byte, 0, n)
	// Rejection sampling keeps the distribution uniform across the alphabet.
	limit := byte(256 - (256 % len(setupCodeAlphabet)))
	buf := make([]byte, 1)
	for len(chars) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		if buf[0] >= limit {
			continue
		}
		chars = append(chars, setupCodeAlphabet[int(buf[0])%len(setupCodeAlphabet)])
	}
	groups := []string{string(chars[0:4]), string(chars[4:8]), string(chars[8:12]), string(chars[12:16])}
	return strings.Join(groups, "-"), nil
}

// humanURLHost turns a listener address into one an operator can paste into a
// browser: wildcard binds become localhost, explicit hosts are kept.
func humanURLHost(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

// printSetupBanner writes the first-run instructions and setup code to stderr in
// a bright, bordered block that stands out in the startup logs.
func printSetupBanner(url, code string) {
	const (
		reset  = "\x1b[0m"
		border = "\x1b[1;33m" // bold yellow
		codehl = "\x1b[1;30;103m"
		label  = "\x1b[1;36m" // bold cyan
	)
	line := strings.Repeat("=", 68)
	b := &strings.Builder{}
	fmt.Fprint(b, "\n")
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprintf(b, "%s  WAYPOINT — FIRST-TIME SETUP%s\n", border, reset)
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprintf(b, "  Open %s%s%s and paste this setup code to create\n", label, url, reset)
	fmt.Fprintf(b, "  your engagement and owner account:\n\n")
	fmt.Fprintf(b, "      %s  %s  %s\n\n", codehl, code, reset)
	fmt.Fprintf(b, "  The code is shown once, lives only in memory, and stops working\n")
	fmt.Fprintf(b, "  the moment setup completes.\n")
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprint(os.Stderr, b.String())
}

// printOwnerTokenBanner announces an owner token minted by the automated
// bootstrap path (only when the operator did not supply one).
func printOwnerTokenBanner(handle, token string) {
	const (
		reset  = "\x1b[0m"
		border = "\x1b[1;33m"
		codehl = "\x1b[1;30;102m"
		label  = "\x1b[1;36m"
	)
	line := strings.Repeat("=", 68)
	b := &strings.Builder{}
	fmt.Fprint(b, "\n")
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprintf(b, "%s  WAYPOINT — OWNER CREDENTIAL PROVISIONED%s\n", border, reset)
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprintf(b, "  Owner %s%s%s — sign in with this token (shown once):\n\n", label, handle, reset)
	fmt.Fprintf(b, "      %s  %s  %s\n\n", codehl, token, reset)
	fmt.Fprintf(b, "  Only its SHA-256 digest is stored. Save it now.\n")
	fmt.Fprintf(b, "%s%s%s\n", border, line, reset)
	fmt.Fprint(os.Stderr, b.String())
}

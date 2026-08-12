package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestComposeDeploymentFilesCoverOneStepDeployment(t *testing.T) {
	compose := mustReadFile(t, "compose.yml")
	for _, want := range []string{
		"postgres:",
		"waypoint:",
		"waypoint-postgres:",
		"waypoint-evidence:",
		"WAYPOINT_DB_DSN: postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable",
		"WAYPOINT_EVIDENCE_DIR: /var/lib/waypoint/evidence",
		"condition: service_healthy",
		"healthcheck:",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose.yml missing %q", want)
		}
	}

	dockerfile := mustReadFile(t, "Dockerfile")
	for _, want := range []string{
		"FROM node:22-bookworm AS web",
		"FROM golang:1.22-bookworm AS build",
		"COPY --from=web /src/internal/webassets/dist ./internal/webassets/dist",
		"HEALTHCHECK --interval=10s --timeout=2s --start-period=15s --retries=6 CMD curl -fsS http://127.0.0.1:8080/readyz >/dev/null || exit 1",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}
}

func TestComposeConfigValidatesWhenDockerComposeIsAvailable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	cmd := exec.Command("docker", "compose", "-f", "compose.yml", "config")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, output)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeDeploymentFilesCoverOneStepDeployment(t *testing.T) {
	compose := mustReadFile(t, "compose.yml")
	for _, want := range []string{
		"postgres:",
		"waypoint:",
		"image: waypoint:compose",
		"restart: unless-stopped",
		"waypoint-postgres:",
		"waypoint-evidence:",
		"WAYPOINT_DB_DSN: postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable",
		"WAYPOINT_EVIDENCE_DIR: /var/lib/waypoint/evidence",
		"condition: service_healthy",
		"healthcheck:",
		"interval: 3s",
		"retries: 30",
		"start_period: 20s",
		"interval: 5s",
		"retries: 36",
		"start_period: 45s",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose.yml missing %q", want)
		}
	}
	for _, forbidden := range []string{"type: bind", "./", "../"} {
		if strings.Contains(compose, forbidden) {
			t.Fatalf("compose.yml unexpectedly contains %q; use named volumes for host portability", forbidden)
		}
	}

	dockerfile := mustReadFile(t, "Dockerfile")
	for _, want := range []string{
		"FROM node:22-bookworm AS web",
		"COPY contracts ./contracts",
		"FROM golang:1.22-bookworm AS build",
		"COPY --from=web /src/internal/webassets/dist ./internal/webassets/dist",
		"HEALTHCHECK --interval=5s --timeout=5s --start-period=45s --retries=36 CMD curl -fsS http://127.0.0.1:8080/readyz >/dev/null || exit 1",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}

	makefile := mustReadFile(t, "Makefile")
	for _, want := range []string{
		"seq 1 120",
		"curl -fsS http://127.0.0.1:18080/readyz",
		"grep -q '\"status\":\"ready\"' /tmp/waypoint-ready.json",
	} {
		if !strings.Contains(makefile, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}

func TestComposeConfigValidatesWhenDockerComposeIsAvailable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	mustComposeConfigOutput := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("docker", append([]string{"compose", "-f", "compose.yml"}, args...)...)
		cmd.Env = os.Environ()
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("docker compose %s failed: %v\n%s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}

	_ = mustComposeConfigOutput("config")

	services := strings.Fields(mustComposeConfigOutput("config", "--services"))
	if len(services) != 2 {
		t.Fatalf("docker compose config --services count = %d, want 2 (%v)", len(services), services)
	}
	serviceSet := map[string]bool{}
	for _, service := range services {
		serviceSet[service] = true
	}
	for _, want := range []string{"postgres", "waypoint"} {
		if !serviceSet[want] {
			t.Fatalf("docker compose config --services missing %q in %v", want, services)
		}
	}

	volumes := strings.Fields(mustComposeConfigOutput("config", "--volumes"))
	if len(volumes) != 2 {
		t.Fatalf("docker compose config --volumes count = %d, want 2 (%v)", len(volumes), volumes)
	}
	volumeSet := map[string]bool{}
	for _, volume := range volumes {
		volumeSet[volume] = true
	}
	for _, want := range []string{"waypoint-postgres", "waypoint-evidence"} {
		if !volumeSet[want] {
			t.Fatalf("docker compose config --volumes missing %q in %v", want, volumes)
		}
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

func prepareComposeDockerConfig(t *testing.T) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "docker-config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir docker config: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
}

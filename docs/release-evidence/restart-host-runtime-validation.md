# restart-002 host/runtime validation

Run: `2026-08-12T00:00:00Z`  
Status: **Blocked / Unverified**

## Environment

- `go version`: `go1.24.4 linux/amd64`
- `docker version`: `Client=26.1.5+dfsg1 Server=`
- `docker compose version`: `Docker Compose version 2.26.1-4`
- `psql --version`: not installed (`command not found`)
- Docker daemon: unavailable (`Cannot connect to the Docker daemon at unix:///var/run/docker.sock`)

## Commands run

### Validation / baseline

```sh
go test ./...
```

Result:

```text
ok   waypoint                    0.497s
ok   waypoint/scripts            4.296s
FAIL waypoint/cmd/waypoint      0.006s
FAIL waypoint/internal/db       0.009s
FAIL waypoint/internal/server   0.260s
```

Representative failure:

```text
WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests
```

### Compose file render

```sh
docker compose -f compose.yml config
```

Result: **PASS**

### Required runtime checks (attempted)

```sh
docker compose -f compose.yml build --no-cache
docker compose -f compose.yml up -d --wait
docker compose -f compose.yml down -v --remove-orphans
docker info --format '{{.ServerVersion}}'
```

Result for each: **blocked by unavailable Docker daemon**

Representative output:

```text
Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
```

### Targeted compose tests

```sh
go test -run TestComposeDeploymentFilesCoverOneStepDeployment -v ./...
go test -run TestComposeStackPersistsDBAndEvidenceAcrossRestart -v ./...
```

Results:

- deployment-file shape test: **PASS**
- restart/integration test: **SKIP** because Docker daemon unavailable

## Requirement-by-requirement status

| Requirement | Status | Notes |
|---|---|---|
| Clean `docker compose build --no-cache` | Unverified | Docker daemon unavailable |
| One-step `up --wait` | Unverified | Docker daemon unavailable |
| PostgreSQL migrations | Unverified | No running DB available |
| Real-DB Go tests | Unverified | `WAYPOINT_TEST_PG_DSN` unavailable; suite fails closed |
| Readiness / health checks | Unverified | No running stack |
| Authenticated human / AI capture round trips | Unverified | No running stack |
| Raw fallback / evidence persistence across restart | Unverified | No running stack |
| SSE / finding / report endpoints | Unverified | No running stack |
| `down -v` cleanup | Unverified | No running stack |

## Blocking notes

This host cannot complete the requested runtime validation because:

1. Docker daemon access is unavailable.
2. PostgreSQL client/server tooling is unavailable.
3. The real-DB Go gate requires `WAYPOINT_TEST_PG_DSN`, which is not present here.

No code changes were made.

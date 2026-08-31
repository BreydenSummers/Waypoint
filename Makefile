GO ?= go
NODE ?= npm
WEB_DIR := web
BIN_DIR := bin
BIN := $(BIN_DIR)/waypoint

.PHONY: lint test build smoke clean release-test dogfood

# Systematically drives every view/control of a RUNNING app with a headless
# browser and reports UI bugs. Needs a live, seeded instance:
#   DOGFOOD_BASE=http://127.0.0.1:8080 DOGFOOD_TOKEN=<owner> DOGFOOD_ENGAGEMENT=<id> make dogfood
dogfood:
	$(NODE) --prefix $(WEB_DIR) run dogfood

lint:
	$(GO) vet ./...
	$(NODE) --prefix $(WEB_DIR) run lint

test:
	# -p 1 serialises package test binaries: the DB-backed packages share one
	# PostgreSQL instance and each resets the public schema, so running them
	# concurrently makes them clobber each other's data.
	$(GO) test -p 1 ./...
	$(NODE) --prefix $(WEB_DIR) run test

build:
	$(NODE) --prefix $(WEB_DIR) run build
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) ./cmd/waypoint

smoke:
	@trap 'docker compose down -v --remove-orphans >/tmp/waypoint-compose-down.log 2>&1 || true' EXIT; \
	docker compose up -d --build >/tmp/waypoint-compose-up.log 2>&1; \
	for i in $$(seq 1 120); do \
		if curl -fsS http://127.0.0.1:8080/readyz >/tmp/waypoint-ready.json; then break; fi; \
		sleep 1; \
	done; \
	curl -fsS http://127.0.0.1:8080/ >/tmp/waypoint-index.html; \
	grep -q "Waypoint" /tmp/waypoint-index.html; \
	grep -q '"status":"ready"' /tmp/waypoint-ready.json

release-test:
	$(GO) run ./cmd/release-test --mode release --out-dir $(or $(RELEASE_TEST_OUT_DIR),docs/release-evidence/release-tests)

clean:
	rm -rf $(BIN_DIR)
	rm -f waypoint server.test

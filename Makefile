GO ?= go
NODE ?= npm
WEB_DIR := web
BIN_DIR := bin
BIN := $(BIN_DIR)/waypoint

.PHONY: lint test build smoke clean release-test

lint:
	$(GO) vet ./...
	$(NODE) --prefix $(WEB_DIR) run lint

test:
	$(GO) test ./...
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

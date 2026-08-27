GO ?= go
NODE ?= npm
WEB_DIR := web
BIN_DIR := bin
BIN := $(BIN_DIR)/waypoint

.PHONY: lint test build smoke clean

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

smoke: build
	@WAYPOINT_ADDR=127.0.0.1:18080 $(BIN) > /tmp/waypoint.log 2>&1 & \
	pid=$$!; \
	trap 'kill $$pid >/dev/null 2>&1 || true' EXIT; \
	for i in $$(seq 1 120); do \
		if curl -fsS http://127.0.0.1:18080/readyz >/tmp/waypoint-ready.json; then break; fi; \
		sleep 1; \
	done; \
	curl -fsS http://127.0.0.1:18080/ >/tmp/waypoint-index.html; \
	grep -q "Waypoint" /tmp/waypoint-index.html; \
	grep -q '"status":"ready"' /tmp/waypoint-ready.json

clean:
	rm -rf $(BIN_DIR)
	rm -f waypoint server.test

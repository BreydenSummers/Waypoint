FROM node:22-bookworm AS web
WORKDIR /src
COPY web/package*.json ./web/
RUN npm ci --prefix web
COPY web ./web
COPY internal ./internal
COPY contracts ./contracts
RUN cd web && npm run build

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY third_party ./third_party
COPY cmd ./cmd
COPY internal ./internal
COPY contracts ./contracts
COPY --from=web /src/internal/webassets/dist ./internal/webassets/dist
RUN CGO_ENABLED=0 go build -o /out/waypoint ./cmd/waypoint

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl ca-certificates chromium \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/waypoint /usr/local/bin/waypoint
ENV WAYPOINT_ADDR=:8080 \
    WAYPOINT_CHROMIUM=/usr/bin/chromium
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=5s --start-period=45s --retries=36 CMD /bin/sh -ec 'if [ -n "${WAYPOINT_TLS_CERT_FILE:-}" ] && [ -n "${WAYPOINT_TLS_KEY_FILE:-}" ]; then if [ -n "${WAYPOINT_TLS_CA_FILE:-}" ]; then curl -fsS --cacert "$WAYPOINT_TLS_CA_FILE" https://127.0.0.1:8080/readyz >/dev/null; else curl -fsSk https://127.0.0.1:8080/readyz >/dev/null; fi; else curl -fsS http://127.0.0.1:8080/readyz >/dev/null; fi'
ENTRYPOINT ["/usr/local/bin/waypoint"]

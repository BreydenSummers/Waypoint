# Documentation coverage matrix

This matrix retains the operator-documentation coverage for v1. It is the companion to [`../../operator-guide.md`](../../operator-guide.md).

| Area | Guide section | Source / command reference | Coverage note |
|---|---|---|---|
| Setup | [Supported setup](../../operator-guide.md#supported-setup) | [`compose.yml`](../../../compose.yml), [`cmd/waypoint/main.go`](../../../cmd/waypoint/main.go), [`scripts/waypoint-installer.sh`](../../../scripts/waypoint-installer.sh) | Compose disposable path and supported-host install/provision flows are documented. |
| TLS and secrets | [TLS and secret handling](../../operator-guide.md#tls-and-secret-handling) | [`scripts/waypoint-installer.sh`](../../../scripts/waypoint-installer.sh), [`cmd/waypoint/main.go`](../../../cmd/waypoint/main.go) | TLS paths, restrictive file modes, and HTTP-only runtime boundary are explicit. |
| Human / AI actors | [Human and AI actors](../../operator-guide.md#human-and-ai-actors) | [`internal/server/actor.go`](../../../internal/server/actor.go), [`scripts/waypoint-installer.sh`](../../../scripts/waypoint-installer.sh) | Named tokens, no shared token, and AI `authorized_by` requirements are covered. |
| REST / MCP ingestion | [REST and MCP ingestion](../../operator-guide.md#rest-and-mcp-ingestion) | [`internal/server/server.go`](../../../internal/server/server.go), [`adr/0008-standard-mcp-transport.md`](../../adr/0008-standard-mcp-transport.md) | REST, SSE, MCP, and out-of-band review endpoints are named. |
| Wrapper / agent matrix | [Wrapper and agent matrix](../../operator-guide.md#wrapper-and-agent-matrix) | [`v1-execution-plan.md`](../../v1-execution-plan.md), [`v1-traceability.md`](../../v1-traceability.md) | Windows wrapper support and Windows-agent deferral are retained. |
| Egress modes | [Egress modes](../../operator-guide.md#egress-modes) | [`internal/egresspolicy/egresspolicy.go`](../../../internal/egresspolicy/egresspolicy.go) | `auto`, `manual`, and `off` semantics are explicit, including no-discovery behavior. |
| Findings / report / export / verify / restore / destroy | [Findings, report, export, verify, restore, and destroy](../../operator-guide.md#findings-report-export-verify-restore-and-destroy) | [`internal/server/report.go`](../../../internal/server/report.go), [`internal/server/export.go`](../../../internal/server/export.go), [`bundle/tools/verify-restore.mjs`](../../../bundle/tools/verify-restore.mjs), [`bundle/tools/regenerate-report.mjs`](../../../bundle/tools/regenerate-report.mjs), [`scripts/waypoint-installer.sh`](../../../scripts/waypoint-installer.sh) | Frozen report, export jobs, bundle verification, restoration, and guarded teardown are covered. |
| Troubleshooting / break-glass | [Troubleshooting](../../operator-guide.md#troubleshooting), [Break-glass guidance](../../operator-guide.md#break-glass-guidance) | [`scripts/waypoint-installer.sh`](../../../scripts/waypoint-installer.sh) | Receipt mismatch, missing psql, and `--force` teardown are documented. |
| Limits / exclusions | [Limits and exclusions](../../operator-guide.md#limits-and-exclusions) | [`PRD.md`](../../../PRD.md), [`docs/adr/0006-v1-vocabulary-and-deferrals.md`](../../../docs/adr/0006-v1-vocabulary-and-deferrals.md) | Out-of-band limits, host-admin responsibility, egress-off attribution loss, hash-not-signature semantics, Windows-agent deferral, and v2 exclusions are explicit. |

Coverage summary:

- supported setup: yes
- TLS and secret handling: yes
- human/AI lifecycle: yes
- REST/MCP ingestion: yes
- wrapper/agent matrix: yes
- egress modes: yes
- findings/report/export/verify/restore/destroy: yes
- troubleshooting/break-glass: yes
- out-of-band / host-admin / disk-encryption boundaries: yes
- v2 exclusions: yes

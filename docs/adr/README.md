# Waypoint architecture decision records

These accepted records refine the v1 PRD without replacing it. The [PRD](../../PRD.md) remains the
product source of truth; the [execution plan](../v1-execution-plan.md) and
[traceability matrix](../v1-traceability.md) turn the decisions into gates and evidence.

A later decision that changes an accepted record must add a new ADR and mark the old record
`Superseded`; do not silently rewrite the decision. A change to a locked PRD decision still requires
product-owner approval.

| ADR | Decision | Primary PRD trace |
|---|---|---|
| [ADR-0001](0001-system-shape.md) | System shape and repository boundaries | PRD-DATA-001, PRD-CAP-007/008, PRD-RT-001 |
| [ADR-0002](0002-append-only-audit.md) | Append-only audit and correction semantics | PRD-CORE-002, PRD-AUD-004 |
| [ADR-0003](0003-content-addressed-evidence.md) | Content-addressed evidence storage | PRD-DATA-003 |
| [ADR-0004](0004-parser-isolation.md) | Parser isolation and raw-first failure behavior | PRD-CAP-002/003 |
| [ADR-0005](0005-export-semantics.md) | Snapshot, bundle, manifest, and teardown semantics | PRD-REP-002–005, PRD-LIFE-001 |
| [ADR-0006](0006-v1-vocabulary-and-deferrals.md) | v1 product vocabulary and deferred capability claims | PRD-UX-002–006, PRD-DEF-001–007 |
| [ADR-0007](0007-attribution-lifecycle-and-read-model.md) | Lossless action attribution, actor lifecycle, authoritative reads, and claim review | PRD-AUD-001, PRD-CAP-009, PRD-ID-001/002 |
| [ADR-0008](0008-standard-mcp-transport.md) | Standard MCP Streamable HTTP and REST service parity | PRD-CAP-008 |
| [ADR-0009](0009-export-jobs-receipts-and-teardown.md) | Persisted export jobs, verified receipts, and teardown authorization | PRD-REP-001–005, PRD-LIFE-001 |

## Verification

From the repository root, run:

```sh
python3 scripts/lint-architecture.py
```

The check validates the ADR inventory and local links, the common ADR sections, the machine-readable
scope policy, scope-linter regression cases, and product claim surfaces. The policy is
[`docs/v1-scope.json`](../v1-scope.json); ADR-0006 explains how to apply it.

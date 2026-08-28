#!/usr/bin/env python3
"""Render retained performance evidence from raw samples.

The report fails closed: the raw sample artifact must exist, and the script only
summarizes measured observations from a measured harness. It does not invent
fixture constants or tolerate missing PostgreSQL, browser, or runtime provenance.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Iterable

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_INPUT = ROOT / "docs" / "release-evidence" / "performance" / "samples" / "raw-profile.json"
DEFAULT_OUTPUT = ROOT / "docs" / "release-evidence" / "performance" / "summary.md"


def load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def nearest_rank(samples: Iterable[float], percentile: float) -> float:
    data = sorted(float(sample) for sample in samples)
    if not data:
        raise ValueError("no samples provided")
    if percentile <= 0:
        return data[0]
    if percentile >= 100:
        return data[-1]
    index = max(0, math.ceil(len(data) * percentile / 100.0) - 1)
    return data[min(index, len(data) - 1)]


def require(raw: dict, path: str) -> object:
    current: object = raw
    for token in path.split("."):
        if not isinstance(current, dict) or token not in current:
            raise KeyError(path)
        current = current[token]
    return current


def render_report(raw: dict) -> str:
    provenance = require(raw, "provenance")
    run = raw["run"]
    samples = raw["measurements"]
    coverage = raw.get("coverage", [])
    query_plans = raw.get("queryPlans", [])
    faults = raw.get("faults", [])
    budgets = raw["budgets"]

    if provenance["postgresql"]["status"] != "available":
        raise KeyError("provenance.postgresql.status")
    if provenance["browser"]["status"] != "available":
        raise KeyError("provenance.browser.status")
    if provenance["runtime"]["status"] != "available":
        raise KeyError("provenance.runtime.status")

    browser_timings = samples["browserTimingsMs"]
    if not browser_timings:
        raise KeyError("measurements.browserTimingsMs")

    def fmt_pct(name: str, unit: str, p95: str | None = None, p99: str | None = None, peak: str | None = None) -> list[str]:
        lines: list[str] = []
        values = samples[name]
        if p95 is not None:
            lines.append(f"- p95 {p95}: {nearest_rank(values, 95):.0f} {unit}")
        if p99 is not None:
            lines.append(f"- p99 {p99}: {nearest_rank(values, 99):.0f} {unit}")
        if peak is not None:
            lines.append(f"- peak {peak}: {max(values):.0f} {unit}")
        lines.append(f"- raw samples retained: {len(values)}")
        return lines

    lines = [
        "# performance summary",
        "",
        "Sandbox verdict: measured.",
        "",
        f"- Captured at: {provenance['capturedAt']}",
        f"- PostgreSQL: {provenance['postgresql']['serverVersion']} ({provenance['postgresql']['dsnEnv']})",
        f"- Browser: {provenance['browser']['name']} {provenance['browser']['version']}",
        f"- Runtime: {provenance['runtime']['go']} on {provenance['runtime']['os']}",
        "",
        "## Coverage",
        "",
        *[f"- {item}" for item in coverage],
        "",
        "## Baseline",
        "",
        f"- Hardware: {run['hardware']['cpu']} / {run['hardware']['memory']} / {run['hardware']['os']}",
        f"- Operators: {run['operators']}",
        f"- Actions: {run['actions']}",
        f"- Audit events: {run['auditEvents']}",
        f"- Observations: {run['observations']}",
        f"- Evidence: {run['evidenceGiB']} GiB",
        "",
        "## Measured samples",
        "",
    ]

    lines.extend(fmt_pct("apiQueryMs", "ms", p95="API query", p99="API query"))
    lines.append("")
    lines.extend(fmt_pct("ingestAckMs", "ms", p95="ingest ack"))
    lines.append("")
    lines.extend(fmt_pct("ingestPeakRSSMiB", "MiB", peak="incremental RSS"))
    lines.append("")
    lines.extend(fmt_pct("commitToSSEMs", "ms", p95="commit-to-SSE"))
    lines.append("")
    lines.extend(fmt_pct("warmRouteUsableMs", "ms", p95="warm route"))
    lines.append("")
    lines.extend(fmt_pct("browserTimingsMs", "ms", p95="browser timing"))
    lines.append("")
    lines.extend(fmt_pct("localInteractionMs", "ms", p95="local interaction"))
    lines.append("")
    lines.extend(fmt_pct("exportDurationMinutes", "min", p95="export duration"))

    lines.extend([
        "",
        "## Query plans retained",
        "",
        *[f"- {plan['name']}: raw EXPLAIN text retained" for plan in query_plans],
        "",
        "## Fault scenarios retained",
        "",
        *[f"- {fault['name']}: {fault['expectation']}" for fault in faults],
        "",
        "## Budgets",
        "",
        f"- API query p95 <= {budgets['queryP95Ms']} ms",
        f"- API query p99 <= {budgets['queryP99Ms']} ms",
        f"- Ingest ack p95 <= {budgets['ingestAckP95Ms']} ms",
        f"- Ingest peak RSS <= {budgets['ingestPeakRSSMiB']} MiB",
        f"- Commit-to-SSE p95 <= {budgets['sseVisibleP95Ms']} ms",
        f"- Warm route usable <= {budgets['warmRouteUsableMs']} ms",
        f"- Browser timing p95 <= {budgets['localInteractionMs']} ms",
        f"- Local interaction <= {budgets['localInteractionMs']} ms",
        f"- Export complete <= {budgets['exportCompleteMinutes']} min",
    ])

    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true", help="fail instead of rewriting the report")
    args = parser.parse_args()

    try:
        raw = load_json(args.input)
        report = render_report(raw)
    except (OSError, json.JSONDecodeError, KeyError, ValueError) as exc:
        print(f"performance report generation failed: {exc}", file=sys.stderr)
        return 2
    if args.check:
        current = args.output.read_text(encoding="utf-8") if args.output.exists() else None
        if current != report:
            print(f"performance report is stale: {args.output.relative_to(ROOT)}", file=sys.stderr)
            return 1
        print(f"performance report is current: {args.output.relative_to(ROOT)}")
        return 0

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(report, encoding="utf-8")
    sys.stdout.write(report)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Run the G1 real-PostgreSQL gate and emit a retained markdown report.

The report fails closed: any skipped required test is a hard failure.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Iterable

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT = ROOT / "docs" / "release-evidence" / "g1-foundation-report.md"
TEST_REGEX = (
    r"^(TestOpenConfiguredDatabaseAppliesMigrations|"
    r"TestApplyMigrationsOnRealPostgreSQL|TestApplyMigrationsSerializesConcurrentStarters|"
    r"TestDatabaseProtectionsRejectMutations|TestActorAuthorizationConstraint|"
    r"TestAppendAuditEventCapturesOutOfBandReviewLifecycle|"
    r"TestAppendAuditEventRedactsSensitiveMetadata|"
    r"TestAppendAuditEventCommitsConcurrentlyAndRollsBackCleanly|"
    r"TestAuditEventViewAndTableRemainAppendOnly|"
    r"TestCaptureIngestCreatesReplaysAndRejectsChangedPayload|"
    r"TestCapturePersistsEvidenceAndRecoversOrphans|"
    r"TestCaptureEvidenceDeduplicatesAndSurvivesRestart|"
    r"TestCaptureAcceptsAIInitiationWithDecisionContext|"
    r"TestCapturePersistsStructuredResultsAndRollsBackInvalidOutput|"
    r"TestCaptureRejectsConflictingStableEntityKinds|"
    r"TestEntityIdentityNormalizationPrecedenceAndConcurrency)$"
)
PACKAGES = ["./cmd/waypoint", "./internal/db", "./internal/server"]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    dsn = os.environ.get("WAYPOINT_TEST_PG_DSN", "").strip()
    if not dsn:
        print("WAYPOINT_TEST_PG_DSN is required to generate the G1 report", file=sys.stderr)
        return 2

    runs = []
    for pkg in PACKAGES:
        runs.extend(run_go_tests(pkg))

    summary = summarize(runs)
    report = render_report(summary)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(report, encoding="utf-8")
    sys.stdout.write(report)
    return 1 if summary["skips"] or summary["failed"] or summary["package_failures"] else 0


def run_go_tests(pkg: str) -> list[dict[str, str]]:
    cmd = ["go", "test", "-json", "-count=1", "-run", TEST_REGEX, pkg]
    proc = subprocess.Popen(cmd, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    events: list[dict[str, str]] = []
    assert proc.stdout is not None
    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(event, dict):
            event.setdefault("Package", pkg)
            events.append(event)
    rc = proc.wait()
    if rc != 0:
        # Keep the events for report generation even on failure.
        return events
    return events


def summarize(events: Iterable[dict[str, str]]) -> dict[str, object]:
    tests: dict[str, dict[str, str]] = {}
    package_failures: list[str] = []
    for event in events:
        action = event.get("Action", "")
        test = event.get("Test", "")
        pkg = event.get("Package", "")
        if test:
            tests.setdefault(test, {})[action] = pkg
        elif action == "fail" and event.get("Output", "").startswith("FAIL\t"):
            package_failures.append(event.get("Output", "").strip())

    passed = sorted(name for name, actions in tests.items() if "pass" in actions)
    skipped = sorted(name for name, actions in tests.items() if "skip" in actions)
    failed = sorted(name for name, actions in tests.items() if "fail" in actions)

    return {
        "timestamp": dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds"),
        "passed": passed,
        "skips": skipped,
        "failed": failed,
        "package_failures": package_failures,
        "total": len(tests),
    }


def render_report(summary: dict[str, object]) -> str:
    passed = summary["passed"]
    skipped = summary["skips"]
    failed = summary["failed"]
    package_failures = summary["package_failures"]
    verdict = "PASS" if not skipped and not failed and not package_failures else "FAIL"

    lines = [
        "# G1 durable foundation report",
        "",
        f"Run: `{summary['timestamp']}`",
        f"Verdict: **{verdict}**",
        "",
        "This report is generated from the real PostgreSQL gate tests. Any skip is a failure.",
        "",
        "## Results",
        "",
        f"- Passed: {len(passed)}",
        f"- Skipped: {len(skipped)}",
        f"- Failed: {len(failed)}",
        "",
        "### Passed tests",
    ]
    if passed:
        lines.extend(f"- {name}" for name in passed)
    else:
        lines.append("- (none)")
    lines.append("")
    lines.append("### Skipped tests")
    if skipped:
        lines.extend(f"- {name}" for name in skipped)
    else:
        lines.append("- (none)")
    lines.append("")
    lines.append("### Failed tests")
    if failed:
        lines.extend(f"- {name}" for name in failed)
    else:
        lines.append("- (none)")
    if package_failures:
        lines.append("")
        lines.append("### Package failures")
        lines.extend(f"- {line}" for line in package_failures)
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    raise SystemExit(main())

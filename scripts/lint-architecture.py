#!/usr/bin/env python3
"""Validate Waypoint ADR links/inventory and v1 product-scope claims."""

from __future__ import annotations

import fnmatch
import json
import re
import sys
from pathlib import Path
from typing import Iterable

ROOT = Path(__file__).resolve().parents[1]
POLICY_PATH = ROOT / "docs/v1-scope.json"
ADR_DIR = ROOT / "docs/adr"
ADR_INDEX = ADR_DIR / "README.md"
REQUIRED_HEADINGS = ("## Context", "## Decision", "## Consequences", "## Verification", "## Traceability")
LINK_RE = re.compile(r"(?<!!)\[[^]]+\]\(([^)]+)\)")
NOT_ONLY_RE = re.compile(r"\bnot only\b", re.IGNORECASE)


class LintFailure(Exception):
    """Raised for an invalid lint policy that prevents useful checking."""


def load_policy() -> dict:
    try:
        policy = json.loads(POLICY_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise LintFailure(f"cannot load {POLICY_PATH.relative_to(ROOT)}: {exc}") from exc

    required_keys = {
        "schemaVersion",
        "requiredAdrs",
        "textExtensions",
        "excludedPaths",
        "allowedContextPattern",
        "rules",
    }
    missing = sorted(required_keys - policy.keys())
    if missing:
        raise LintFailure(f"scope policy is missing keys: {', '.join(missing)}")
    if policy["schemaVersion"] != 1:
        raise LintFailure(f"unsupported scope policy schemaVersion {policy['schemaVersion']!r}")
    if not policy["rules"]:
        raise LintFailure("scope policy has no rules")

    ids = [rule.get("id") for rule in policy["rules"]]
    if any(not isinstance(rule_id, str) or not rule_id for rule_id in ids):
        raise LintFailure("every scope rule needs a non-empty string id")
    if len(ids) != len(set(ids)):
        raise LintFailure("scope rule ids must be unique")

    try:
        re.compile(policy["allowedContextPattern"], re.IGNORECASE)
        for rule in policy["rules"]:
            re.compile(rule["pattern"], re.IGNORECASE)
    except (KeyError, re.error) as exc:
        raise LintFailure(f"invalid scope rule regex: {exc}") from exc
    return policy


def lint_adrs(policy: dict) -> list[str]:
    errors: list[str] = []
    required = {Path(path) for path in policy["requiredAdrs"]}
    actual = {
        path.relative_to(ROOT)
        for path in ADR_DIR.glob("[0-9][0-9][0-9][0-9]-*.md")
        if path.is_file()
    }

    for path in sorted(required - actual):
        errors.append(f"missing required ADR: {path.as_posix()}")
    for path in sorted(actual - required):
        errors.append(f"ADR is not registered in scope policy: {path.as_posix()}")

    index_text = ADR_INDEX.read_text(encoding="utf-8") if ADR_INDEX.is_file() else ""
    if not index_text:
        errors.append("missing or empty docs/adr/README.md")

    for relative in sorted(required & actual):
        path = ROOT / relative
        text = path.read_text(encoding="utf-8")
        adr_number = relative.name[:4]
        if not text.startswith(f"# ADR-{adr_number}:"):
            errors.append(f"{relative}: title must start '# ADR-{adr_number}:'")
        if "- Status: Accepted" not in text:
            errors.append(f"{relative}: missing accepted status")
        for heading in REQUIRED_HEADINGS:
            if heading not in text:
                errors.append(f"{relative}: missing heading {heading!r}")
        if relative.name not in index_text:
            errors.append(f"{relative}: not linked from docs/adr/README.md")

    vocabulary = ROOT / "docs/adr/0006-v1-vocabulary-and-deferrals.md"
    vocabulary_text = vocabulary.read_text(encoding="utf-8") if vocabulary.is_file() else ""
    for rule in policy["rules"]:
        if f"`{rule['id']}`" not in vocabulary_text:
            errors.append(f"ADR-0006 does not document scope rule {rule['id']!r}")

    for markdown in sorted(ADR_DIR.glob("*.md")):
        text = markdown.read_text(encoding="utf-8")
        for raw_destination in LINK_RE.findall(text):
            destination = raw_destination.strip().split(maxsplit=1)[0].strip("<>")
            if not destination or destination.startswith(("#", "http://", "https://", "mailto:")):
                continue
            destination = destination.split("#", 1)[0]
            target = (markdown.parent / destination).resolve()
            try:
                target.relative_to(ROOT.resolve())
            except ValueError:
                errors.append(f"{markdown.relative_to(ROOT)}: link escapes repository: {destination}")
                continue
            if not target.exists():
                errors.append(f"{markdown.relative_to(ROOT)}: broken local link: {destination}")
    return errors


def compile_scope_rules(policy: dict) -> tuple[re.Pattern[str], list[tuple[str, re.Pattern[str]]]]:
    allowed = re.compile(policy["allowedContextPattern"], re.IGNORECASE)
    rules = [
        (rule["id"], re.compile(rule["pattern"], re.IGNORECASE))
        for rule in policy["rules"]
    ]
    return allowed, rules


def violations_for_line(
    line: str,
    allowed: re.Pattern[str],
    rules: Iterable[tuple[str, re.Pattern[str]]],
) -> list[str]:
    matched = [rule_id for rule_id, pattern in rules if pattern.search(line)]
    if not matched:
        return []
    # “Not only” is a positive construction, not deferral context.
    if allowed.search(line) and not NOT_ONLY_RE.search(line):
        return []
    return matched


def run_scope_regressions(
    allowed: re.Pattern[str], rules: list[tuple[str, re.Pattern[str]]]
) -> list[str]:
    errors: list[str] = []
    should_fail = (
        "Explore attack paths in our node graph.",
        "Open the firewall map to inspect reachability.",
        "Launch commands from the guided scan library.",
        "Ask the offensive LLM for the next move.",
        "Create AI-prefilled findings from this result.",
        "Generate a parser at engagement time.",
        "AI plugin generation is included.",
        "The Windows offline agent is supported.",
        "The report is digitally signed.",
        "The bundle includes cryptographic signing.",
        "Complete Recon to unlock Findings.",
        "Not only is the export digitally signed, it is encrypted.",
    )
    should_pass = (
        "The analytical graph is deferred to v2.",
        "Waypoint does not ship an offensive LLM in v1.",
        "The export is not cryptographically signed.",
        "Waypoints are not locked.",
        "AI actors are attributed in v1.",
        "SMB signing is explained in this technique note.",
        "Capture an operator's network scan.",
        "The Windows operator wrapper is supported in v1.",
    )
    for line in should_fail:
        if not violations_for_line(line, allowed, rules):
            errors.append(f"scope regression should fail but passed: {line!r}")
    for line in should_pass:
        if violations_for_line(line, allowed, rules):
            errors.append(f"scope regression should pass but failed: {line!r}")
    return errors


def is_excluded(relative: str, patterns: Iterable[str]) -> bool:
    return any(fnmatch.fnmatchcase(relative, pattern) for pattern in patterns)


def lint_claim_surfaces(
    policy: dict,
    allowed: re.Pattern[str],
    rules: list[tuple[str, re.Pattern[str]]],
) -> tuple[list[str], int]:
    errors: list[str] = []
    scanned = 0
    extensions = set(policy["textExtensions"])

    for path in sorted(ROOT.rglob("*")):
        if not path.is_file() or path.suffix.lower() not in extensions:
            continue
        relative = path.relative_to(ROOT).as_posix()
        if is_excluded(relative, policy["excludedPaths"]):
            continue
        scanned += 1
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except UnicodeDecodeError as exc:
            errors.append(f"{relative}: expected UTF-8 text: {exc}")
            continue
        for line_number, line in enumerate(lines, start=1):
            for rule_id in violations_for_line(line, allowed, rules):
                excerpt = line.strip()
                if len(excerpt) > 160:
                    excerpt = excerpt[:157] + "..."
                errors.append(f"{relative}:{line_number}: {rule_id}: {excerpt}")
    return errors, scanned


def main() -> int:
    try:
        policy = load_policy()
    except LintFailure as exc:
        print(f"architecture lint configuration error: {exc}", file=sys.stderr)
        return 2

    allowed, rules = compile_scope_rules(policy)
    errors = lint_adrs(policy)
    errors.extend(run_scope_regressions(allowed, rules))
    scope_errors, scanned = lint_claim_surfaces(policy, allowed, rules)
    errors.extend(scope_errors)

    if errors:
        print("architecture lint failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print(
        "architecture lint passed "
        f"({len(policy['requiredAdrs'])} ADRs, {len(rules)} scope rules, "
        f"{scanned} claim-surface files)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

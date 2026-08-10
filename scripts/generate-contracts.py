#!/usr/bin/env python3
"""Generate deterministic aggregate artifacts for the versioned contract schemas."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CONTRACT = ROOT / "contracts" / "v1"
GENERATED = CONTRACT / "generated"
OPENAPI = CONTRACT / "openapi.json"


def load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def expected_outputs() -> dict[Path, dict]:
    openapi = load_json(OPENAPI)
    components = openapi["components"]["schemas"]
    catalog_entries: list[dict] = []
    wire_refs: list[dict] = []
    wire_components = {
        "CaptureEnvelope",
        "CaptureAck",
        "AuditEvent",
        "AuditPage",
        "Problem",
    }

    for component, reference in components.items():
        source = reference.get("$ref")
        if not isinstance(source, str) or not source.startswith("schemas/"):
            raise ValueError(f"OpenAPI component {component} must reference a source schema")
        schema_path = CONTRACT / source
        schema = load_json(schema_path)
        schema_id = schema.get("$id")
        if not isinstance(schema_id, str):
            raise ValueError(f"{schema_path.relative_to(ROOT)} has no $id")
        relative_from_generated = f"../{source}"
        catalog_entries.append(
            {"component": component, "id": schema_id, "source": relative_from_generated}
        )
        if component in wire_components:
            wire_refs.append({"$ref": schema_id})

    aggregate = {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://schemas.waypoint.security/contracts/v1/generated/contract.schema.json",
        "title": "Generated Waypoint v1 wire object union",
        "description": "Generated aggregate for compatibility tooling; canonical schemas remain in ../schemas.",
        "oneOf": wire_refs,
    }
    catalog = {
        "contractVersion": openapi["info"]["version"],
        "draft": "https://json-schema.org/draft/2020-12/schema",
        "generated": True,
        "schemas": catalog_entries,
    }
    return {
        GENERATED / "contract.schema.json": aggregate,
        GENERATED / "schema-catalog.json": catalog,
    }


def render(value: dict) -> str:
    return json.dumps(value, indent=2, ensure_ascii=False) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--check", action="store_true", help="fail rather than rewrite when generated content differs"
    )
    args = parser.parse_args()

    try:
        outputs = expected_outputs()
    except (OSError, KeyError, json.JSONDecodeError, ValueError) as exc:
        print(f"contract generation failed: {exc}", file=sys.stderr)
        return 2

    stale: list[str] = []
    for path, expected in outputs.items():
        expected_text = render(expected)
        current = path.read_text(encoding="utf-8") if path.is_file() else None
        if current == expected_text:
            continue
        if args.check:
            stale.append(path.relative_to(ROOT).as_posix())
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(expected_text, encoding="utf-8")
            print(f"generated {path.relative_to(ROOT)}")

    if stale:
        print("generated contracts are stale; run python3 scripts/generate-contracts.py:", file=sys.stderr)
        for path in stale:
            print(f"- {path}", file=sys.stderr)
        return 1
    if args.check:
        print(f"generated contracts are current ({len(outputs)} artifacts)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

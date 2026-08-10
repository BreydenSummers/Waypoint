#!/usr/bin/env python3
"""Verify Waypoint v1 schemas, OpenAPI references, and compatibility fixtures."""

from __future__ import annotations

import base64
import copy
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

try:
    from jsonschema import Draft202012Validator, FormatChecker, RefResolver
except ImportError:
    print(
        "contract verification requires jsonschema; install contracts/requirements.txt",
        file=sys.stderr,
    )
    raise SystemExit(2)

ROOT = Path(__file__).resolve().parents[1]
CONTRACT = ROOT / "contracts" / "v1"
SCHEMAS = CONTRACT / "schemas"
FIXTURES = CONTRACT / "fixtures"
MAX_CURSOR = 9223372036854775807
CURSOR_RE = re.compile(r"^[1-9][0-9]{0,18}$")
FORMAT_CHECKER = FormatChecker()


class VerificationFailure(Exception):
    pass


def load_json(path: Path) -> dict:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise VerificationFailure(f"cannot load {path.relative_to(ROOT)}: {exc}") from exc
    if not isinstance(value, dict):
        raise VerificationFailure(f"{path.relative_to(ROOT)} must contain a JSON object")
    return value


def decode_pointer_token(token: str) -> str:
    return token.replace("~1", "/").replace("~0", "~")


def pointer_parent(document: Any, pointer: str) -> tuple[Any, str]:
    if not pointer.startswith("/"):
        raise VerificationFailure(f"fixture mutation is not a JSON Pointer: {pointer!r}")
    tokens = [decode_pointer_token(token) for token in pointer[1:].split("/")]
    current = document
    for token in tokens[:-1]:
        if isinstance(current, list):
            current = current[int(token)]
        elif isinstance(current, dict) and token in current:
            current = current[token]
        else:
            raise VerificationFailure(f"fixture mutation path does not exist: {pointer}")
    return current, tokens[-1]


def apply_mutations(document: dict, mutations: list[dict]) -> dict:
    result = copy.deepcopy(document)
    for mutation in mutations:
        operation = mutation.get("op")
        pointer = mutation.get("path")
        if operation not in {"add", "remove", "replace"} or not isinstance(pointer, str):
            raise VerificationFailure(f"unsupported fixture mutation: {mutation!r}")
        parent, key = pointer_parent(result, pointer)
        if isinstance(parent, list):
            index = int(key)
            if operation == "add":
                parent.insert(index, copy.deepcopy(mutation.get("value")))
            elif operation == "remove":
                parent.pop(index)
            else:
                parent[index] = copy.deepcopy(mutation.get("value"))
        elif isinstance(parent, dict):
            if operation in {"remove", "replace"} and key not in parent:
                raise VerificationFailure(f"fixture mutation path does not exist: {pointer}")
            if operation == "remove":
                del parent[key]
            else:
                parent[key] = copy.deepcopy(mutation.get("value"))
        else:
            raise VerificationFailure(f"fixture mutation parent is not a container: {pointer}")
    return result


def materialize_case(path: Path) -> dict:
    case = load_json(path)
    if "base" not in case:
        return case
    base_path = (path.parent / case["base"]).resolve()
    try:
        base_path.relative_to(FIXTURES.resolve())
    except ValueError as exc:
        raise VerificationFailure(f"fixture base escapes fixture tree: {case['base']}") from exc
    base = load_json(base_path)
    materialized = apply_mutations(base, case.get("mutations", []))
    materialized["name"] = case.get("name", materialized.get("name"))
    materialized["description"] = case.get("description", materialized.get("description"))
    materialized["expected"] = copy.deepcopy(case.get("expected", {}))
    return materialized


def schema_registry() -> tuple[dict[str, dict], dict[str, dict]]:
    by_name: dict[str, dict] = {}
    store: dict[str, dict] = {}
    ids: set[str] = set()
    for path in sorted(SCHEMAS.glob("*.schema.json")):
        schema = load_json(path)
        try:
            Draft202012Validator.check_schema(schema)
        except Exception as exc:
            raise VerificationFailure(f"invalid JSON Schema {path.relative_to(ROOT)}: {exc}") from exc
        schema_id = schema.get("$id")
        if not isinstance(schema_id, str) or schema_id in ids:
            raise VerificationFailure(f"missing or duplicate $id in {path.relative_to(ROOT)}")
        ids.add(schema_id)
        by_name[path.name] = schema
        store[schema_id] = schema
    return by_name, store


def validator(schema: dict, store: dict[str, dict]) -> Draft202012Validator:
    resolver = RefResolver.from_schema(schema, store=store)
    return Draft202012Validator(
        schema, resolver=resolver, format_checker=FORMAT_CHECKER
    )


def validation_errors(instance: Any, schema: dict, store: dict[str, dict]) -> list[str]:
    errors: list[str] = []
    for error in validator(schema, store).iter_errors(instance):
        pointer = "/" + "/".join(str(part).replace("~", "~0").replace("/", "~1") for part in error.absolute_path)
        errors.append(f"{pointer.rstrip('/') or '/'}: {error.message}")
    return sorted(errors)


def semantic_capture_errors(case: dict) -> list[str]:
    errors: list[str] = []
    actor = case.get("actor", {})
    request = case.get("request", {})
    headers = case.get("requestHeaders", {})
    parsing = request.get("parsing", {})
    initiated = request.get("initiatedBy")

    if headers.get("Waypoint-Contract-Version") != request.get("contractVersion"):
        errors.append("/contractVersion: transport and envelope versions differ")
    if headers.get("Idempotency-Key") != request.get("captureId"):
        errors.append("/captureId: idempotency_key_mismatch")
    if initiated == "scan-library":  # reserved compatibility value, unavailable in v1
        errors.append("/initiatedBy: reserved_value")
    if actor.get("kind") == "human" and initiated != "manual":
        errors.append("/initiatedBy: actor_initiator_mismatch")
    if actor.get("kind") == "ai_agent":
        if initiated != "ai":
            errors.append("/initiatedBy: actor_initiator_mismatch")
        if not actor.get("authorizedBy"):
            errors.append("/actor/authorizedBy: human authorizer is required")
        context = request.get("decisionContext", {})
        if not context.get("rationale") and not context.get("promptReference"):
            errors.append("/decisionContext: AI decision context is required")

    tool_class = case.get("toolClass")
    status = parsing.get("status")
    if tool_class == "known" and status not in {"parsed", "parse-failed", "raw"}:
        errors.append("/parsing/status: tool_class_parse_mismatch")
    if tool_class == "unknown" and status != "needs-plugin":
        errors.append("/parsing/status: tool_class_parse_mismatch")
    if status == "parsed" and "result" not in parsing:
        errors.append("/parsing/result: parsed result is required")

    raw_parts = case.get("rawParts", {})
    for role in ("stdout", "stderr"):
        encoded = raw_parts.get(f"{role}Base64")
        descriptor = request.get("evidence", {}).get(role, {})
        if not isinstance(encoded, str):
            errors.append(f"/rawParts/{role}Base64: fixture bytes are required")
            continue
        try:
            content = base64.b64decode(encoded, validate=True)
        except ValueError:
            errors.append(f"/rawParts/{role}Base64: invalid base64")
            continue
        if descriptor.get("byteLength") != len(content):
            errors.append(f"/evidence/{role}/byteLength: raw part length mismatch")
        if descriptor.get("sha256") != hashlib.sha256(content).hexdigest():
            errors.append(f"/evidence/{role}/sha256: raw part digest mismatch")
    return sorted(set(errors))


def event_consistency_errors(case: dict) -> list[str]:
    event = case.get("expectedEvent", {})
    request = case.get("request", {})
    errors: list[str] = []
    comparisons = (
        (event.get("actor", {}).get("id"), case.get("actor", {}).get("id"), "/actor/id"),
        (event.get("data", {}).get("captureId"), request.get("captureId"), "/data/captureId"),
        (event.get("data", {}).get("sourceAgentId"), request.get("sourceAgent", {}).get("id"), "/data/sourceAgentId"),
        (event.get("data", {}).get("phase"), request.get("phase"), "/data/phase"),
        (event.get("data", {}).get("initiatedBy"), request.get("initiatedBy"), "/data/initiatedBy"),
        (event.get("data", {}).get("command"), request.get("command"), "/data/command"),
        (event.get("data", {}).get("target"), {key: request.get("target", {}).get(key) for key in ("kind", "value")}, "/data/target"),
        (event.get("data", {}).get("execution", {}).get("status"), request.get("execution", {}).get("status"), "/data/execution/status"),
        (event.get("data", {}).get("execution", {}).get("exitCode"), request.get("execution", {}).get("exitCode"), "/data/execution/exitCode"),
        (event.get("data", {}).get("parseStatus"), request.get("parsing", {}).get("status"), "/data/parseStatus"),
        (event.get("data", {}).get("egressStatus"), request.get("network", {}).get("egress", {}).get("status"), "/data/egressStatus"),
        (event.get("data", {}).get("actionId"), event.get("subject", {}).get("id"), "/data/actionId"),
    )
    for actual, expected, pointer in comparisons:
        if actual != expected:
            errors.append(f"{pointer}: event_capture_mismatch")
    if event.get("actor") != case.get("actor"):
        errors.append("/actor: event actor snapshot mismatch")
    if case.get("actor", {}).get("kind") == "ai_agent" and not event.get("actor", {}).get("authorizedBy"):
        errors.append("/actor/authorizedBy: AI event authorizer is required")
    return sorted(set(errors))


def verify_capture_fixtures(schemas: dict[str, dict], store: dict[str, dict]) -> int:
    index_path = FIXTURES / "captures" / "index.json"
    index = load_json(index_path)
    listed = {entry["path"] for entry in index.get("cases", [])}
    actual = {
        path.relative_to(index_path.parent).as_posix()
        for path in index_path.parent.glob("**/*.json")
        if path != index_path
    }
    if listed != actual:
        raise VerificationFailure(
            f"capture fixture index mismatch: missing={sorted(actual - listed)}, stale={sorted(listed - actual)}"
        )

    valid_count = 0
    categories: set[tuple[str, str]] = set()
    for entry in index["cases"]:
        path = index_path.parent / entry["path"]
        case = materialize_case(path)
        if case.get("actor", {}).get("kind") != entry["actorKind"]:
            raise VerificationFailure(f"{entry['path']}: actorKind index mismatch")
        if case.get("toolClass") != entry["toolClass"]:
            raise VerificationFailure(f"{entry['path']}: toolClass index mismatch")
        if case.get("expected", {}).get("valid") != entry["valid"]:
            raise VerificationFailure(f"{entry['path']}: validity index mismatch")

        errors = validation_errors(case.get("actor"), schemas["actor.schema.json"], store)
        errors += validation_errors(case.get("request"), schemas["capture-envelope.schema.json"], store)
        errors += semantic_capture_errors(case)
        errors = sorted(set(errors))
        if entry["valid"]:
            if errors:
                raise VerificationFailure(f"{entry['path']} should be valid: {'; '.join(errors)}")
            event_errors = validation_errors(
                case.get("expectedEvent"), schemas["audit-event.schema.json"], store
            ) + event_consistency_errors(case)
            if event_errors:
                raise VerificationFailure(
                    f"{entry['path']} expected event is invalid: {'; '.join(sorted(set(event_errors)))}"
                )
            categories.add((entry["actorKind"], entry["toolClass"]))
            valid_count += 1
        elif not errors:
            raise VerificationFailure(f"{entry['path']} should be rejected but passed")
        else:
            expected_error = case.get("expected", {}).get("errorCode")
            allowed_errors = schemas["problem.schema.json"]["properties"]["code"]["enum"]
            if expected_error not in allowed_errors:
                raise VerificationFailure(f"{entry['path']} uses unknown API error code {expected_error!r}")
            pointer = case.get("expected", {}).get("pointer")
            if pointer and not any(error.startswith(pointer) for error in errors):
                raise VerificationFailure(
                    f"{entry['path']} did not fail at expected pointer {pointer}: {'; '.join(errors)}"
                )

    required = {("human", "known"), ("human", "unknown"), ("ai_agent", "known"), ("ai_agent", "unknown")}
    if categories != required:
        raise VerificationFailure(f"valid fixture matrix is incomplete: {sorted(required - categories)}")
    return len(index["cases"])


def verify_event_fixtures(schemas: dict[str, dict], store: dict[str, dict]) -> int:
    index_path = FIXTURES / "events" / "index.json"
    index = load_json(index_path)
    listed = {entry["path"] for entry in index["cases"]}
    actual = {
        path.relative_to(index_path.parent).as_posix()
        for path in index_path.parent.glob("**/*.json")
        if path != index_path
    }
    if listed != actual:
        raise VerificationFailure("event fixture index does not match files")
    for entry in index["cases"]:
        case = materialize_case(index_path.parent / entry["path"])
        errors = validation_errors(
            case.get("expectedEvent"), schemas["audit-event.schema.json"], store
        )
        if "request" in case:
            errors += event_consistency_errors(case)
        if entry["valid"] and errors:
            raise VerificationFailure(f"{entry['path']} should be a valid event: {errors}")
        if not entry["valid"] and not errors:
            raise VerificationFailure(f"{entry['path']} should be rejected but passed")
        pointer = case.get("expected", {}).get("pointer")
        if pointer and not any(error.startswith(pointer) for error in errors):
            raise VerificationFailure(f"{entry['path']} did not fail at {pointer}: {errors}")
    return len(index["cases"])


def canonical_request(request: dict) -> str:
    return json.dumps(request, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def verify_idempotency() -> int:
    fixture_path = FIXTURES / "idempotency.json"
    fixture = load_json(fixture_path)
    base = load_json(fixture_path.parent / fixture["base"])
    original_scope = (
        "11111111-1111-4111-8111-111111111111",
        base["actor"]["id"],
        base["request"]["sourceAgent"]["id"],
        base["request"]["captureId"],
    )
    original_fingerprint = hashlib.sha256(canonical_request(base["request"]).encode()).hexdigest()
    for case in fixture["cases"]:
        candidate = apply_mutations(base, case.get("mutations", []))
        scope = (
            case["scope"]["engagementId"],
            case["scope"]["actorId"],
            candidate["request"]["sourceAgent"]["id"],
            candidate["request"]["captureId"],
        )
        fingerprint = hashlib.sha256(canonical_request(candidate["request"]).encode()).hexdigest()
        disposition = "created" if scope != original_scope else ("replayed" if fingerprint == original_fingerprint else "conflict")
        if disposition != case["expected"]["disposition"]:
            raise VerificationFailure(f"idempotency case {case['name']} expected {case['expected']['disposition']}, got {disposition}")
        expected_status = {"created": 201, "replayed": 200, "conflict": 409}[disposition]
        if case["expected"].get("httpStatus") != expected_status:
            raise VerificationFailure(f"idempotency case {case['name']} has the wrong HTTP status")
        if case["expected"].get("newAuditEvent") != (disposition != "replayed"):
            raise VerificationFailure(f"idempotency case {case['name']} has the wrong event expectation")
    return len(fixture["cases"])


def valid_cursor(value: Any) -> bool:
    return isinstance(value, str) and bool(CURSOR_RE.fullmatch(value)) and int(value) <= MAX_CURSOR


def verify_cursors() -> int:
    fixture = load_json(FIXTURES / "cursors.json")
    for case in fixture["cases"]:
        after = case["after"]
        last = case["lastEventId"]
        error: str | None = None
        for value in (after, last):
            if value is not None and not valid_cursor(value):
                error = "cursor_invalid"
        if error is None and after is not None and last is not None and after != last:
            error = "cursor_mismatch"
        effective = after if after is not None else last
        if error is None and effective is not None and int(effective) < int(case["minimumAvailable"]):
            error = "cursor_expired"
        expected = case["expected"]
        if error:
            if expected.get("valid") is not False or expected.get("errorCode") != error:
                raise VerificationFailure(f"cursor case {case['name']} expected {expected}, got {error}")
            continue
        mode = "oldest-available" if case["endpoint"] == "audit-events" and effective is None else "live-only" if effective is None else "resume"
        resolved = case["connectionHighWater"] if mode == "live-only" else effective
        if not expected.get("valid") or expected.get("mode") != mode or expected.get("effectiveCursor") != resolved:
            raise VerificationFailure(f"cursor case {case['name']} did not resolve as expected")
    return len(fixture["cases"])


def verify_problems(schemas: dict[str, dict], store: dict[str, dict]) -> int:
    fixture = load_json(FIXTURES / "problems.json")
    for case in fixture["cases"]:
        errors = validation_errors(case["problem"], schemas["problem.schema.json"], store)
        if case["valid"] == bool(errors):
            raise VerificationFailure(f"problem case {case['name']} validity mismatch: {errors}")
    return len(fixture["cases"])


def resolve_pointer(document: Any, fragment: str) -> Any:
    if fragment in {"", "#"}:
        return document
    if not fragment.startswith("#/"):
        raise VerificationFailure(f"unsupported JSON reference fragment {fragment!r}")
    current = document
    for token in fragment[2:].split("/"):
        token = decode_pointer_token(token)
        if not isinstance(current, dict) or token not in current:
            raise VerificationFailure(f"unresolved JSON reference fragment {fragment}")
        current = current[token]
    return current


def verify_openapi() -> int:
    path = CONTRACT / "openapi.json"
    openapi = load_json(path)
    if openapi.get("openapi") != "3.1.0" or openapi.get("info", {}).get("version") != "1.0.0":
        raise VerificationFailure("OpenAPI and contract versions must both be 1.0.0")
    operation_ids: set[str] = set()
    ref_count = 0

    def walk(value: Any, source_path: Path, source_document: dict) -> None:
        nonlocal ref_count
        if isinstance(value, dict):
            reference = value.get("$ref")
            if isinstance(reference, str):
                ref_count += 1
                destination, separator, fragment_text = reference.partition("#")
                if destination:
                    target_path = (source_path.parent / destination).resolve()
                    try:
                        target_path.relative_to(CONTRACT.resolve())
                    except ValueError as exc:
                        raise VerificationFailure(f"OpenAPI reference escapes contract tree: {reference}") from exc
                    target_document = load_json(target_path)
                else:
                    target_path = source_path
                    target_document = source_document
                fragment = f"#{fragment_text}" if separator else ""
                resolve_pointer(target_document, fragment)
            for child in value.values():
                walk(child, source_path, source_document)
        elif isinstance(value, list):
            for child in value:
                walk(child, source_path, source_document)

    walk(openapi, path, openapi)
    for route, path_item in openapi.get("paths", {}).items():
        if any(word in route.lower() for word in ("exec", "shell", "command")):
            raise VerificationFailure(f"core must not expose command execution route: {route}")
        for method, operation in path_item.items():
            if method not in {"get", "post", "put", "patch", "delete"}:
                continue
            operation_id = operation.get("operationId")
            if not operation_id or operation_id in operation_ids:
                raise VerificationFailure(f"missing or duplicate operationId at {method.upper()} {route}")
            operation_ids.add(operation_id)
    if set(openapi["paths"]) != {"/captures", "/audit-events", "/events"}:
        raise VerificationFailure("v1 contract route inventory changed without compatibility review")
    return ref_count


def verify_generated(schemas: dict[str, dict], store: dict[str, dict]) -> int:
    process = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "generate-contracts.py"), "--check"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=False,
    )
    if process.returncode:
        raise VerificationFailure((process.stderr or process.stdout).strip())
    aggregate = load_json(CONTRACT / "generated" / "contract.schema.json")
    Draft202012Validator.check_schema(aggregate)
    generated_store = dict(store)
    generated_store[aggregate["$id"]] = aggregate
    valid_fixture = load_json(FIXTURES / "captures" / "valid" / "human-known-tool.json")
    errors = validation_errors(valid_fixture["request"], aggregate, generated_store)
    if errors:
        raise VerificationFailure(f"generated aggregate rejects valid capture: {errors}")
    return 2


def main() -> int:
    try:
        schemas, store = schema_registry()
        generated = verify_generated(schemas, store)
        refs = verify_openapi()
        captures = verify_capture_fixtures(schemas, store)
        events = verify_event_fixtures(schemas, store)
        idempotency = verify_idempotency()
        cursors = verify_cursors()
        problems = verify_problems(schemas, store)
    except (VerificationFailure, KeyError, TypeError, ValueError) as exc:
        print(f"contract verification failed: {exc}", file=sys.stderr)
        return 1

    print(
        "contract verification passed "
        f"({len(schemas)} schemas, {generated} generated artifacts, {refs} OpenAPI refs, "
        f"{captures} capture cases, {events} event cases, {idempotency} idempotency cases, "
        f"{cursors} cursor cases, {problems} problem cases)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

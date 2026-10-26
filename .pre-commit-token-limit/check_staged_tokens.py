#!/usr/bin/env python3
"""Reject oversized staged Git blobs without reading worktree file contents."""

from __future__ import annotations

import argparse
import fnmatch
import json
import math
import os
import subprocess
import sys
from pathlib import Path
from typing import Any

SCHEMA = "pre-commit-token-limit-result/v1"
CONFIG_SCHEMA = "pre-commit-token-limit-config/v1"
CONFIG_PATH = ".pre-commit-token-limit/config.json"
MAX_BYTES = 20_000
BYTES_PER_TOKEN = 4
ERROR_CODE = "STAGED_FILE_TOKEN_LIMIT"

DEFAULT_IGNORE_PATTERNS = (
    "package-lock.json",
    "**/package-lock.json",
    "npm-shrinkwrap.json",
    "**/npm-shrinkwrap.json",
    "pnpm-lock.yaml",
    "**/pnpm-lock.yaml",
    "yarn.lock",
    "**/yarn.lock",
    "bun.lock",
    "**/bun.lock",
    "bun.lockb",
    "**/bun.lockb",
    "uv.lock",
    "**/uv.lock",
    "poetry.lock",
    "**/poetry.lock",
    "Pipfile.lock",
    "**/Pipfile.lock",
    "Cargo.lock",
    "**/Cargo.lock",
    "composer.lock",
    "**/composer.lock",
    "Gemfile.lock",
    "**/Gemfile.lock",
    "go.sum",
    "**/go.sum",
)


class GateError(RuntimeError):
    pass


def default_config() -> dict[str, Any]:
    return {
        "schema": CONFIG_SCHEMA,
        "max_bytes": MAX_BYTES,
        "bytes_per_token": BYTES_PER_TOKEN,
        "ignore_patterns": [
            {"pattern": pattern, "reason": "dependency lock file"}
            for pattern in DEFAULT_IGNORE_PATTERNS
        ],
    }


def _run_git(
    repo: Path,
    *args: str,
    input_bytes: bytes | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    completed = subprocess.run(
        ["git", "-C", str(repo), *args],
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if check and completed.returncode != 0:
        detail = completed.stderr.decode("utf-8", errors="replace").strip()
        raise GateError(detail or f"git {' '.join(args)} failed")
    return completed


def resolve_root(repo: Path) -> Path:
    result = _run_git(repo, "rev-parse", "--show-toplevel")
    return Path(os.fsdecode(result.stdout.rstrip(b"\n"))).resolve()


def _validate_config(value: object) -> dict[str, Any]:
    if not isinstance(value, dict) or value.get("schema") != CONFIG_SCHEMA:
        raise GateError(f"config must use schema {CONFIG_SCHEMA}")
    if value.get("max_bytes") != MAX_BYTES:
        raise GateError(f"config max_bytes must remain {MAX_BYTES}")
    if value.get("bytes_per_token") != BYTES_PER_TOKEN:
        raise GateError(f"config bytes_per_token must remain {BYTES_PER_TOKEN}")
    patterns = value.get("ignore_patterns")
    if not isinstance(patterns, list):
        raise GateError("config ignore_patterns must be a list")
    for index, rule in enumerate(patterns):
        if not isinstance(rule, dict):
            raise GateError(f"ignore_patterns[{index}] must be an object")
        if not isinstance(rule.get("pattern"), str) or not rule["pattern"]:
            raise GateError(f"ignore_patterns[{index}].pattern must be non-empty")
        if not isinstance(rule.get("reason"), str) or not rule["reason"]:
            raise GateError(f"ignore_patterns[{index}].reason must be non-empty")
    return value


def load_config(root: Path) -> dict[str, Any]:
    path = root / CONFIG_PATH
    if not path.exists():
        return default_config()
    if not path.is_file():
        raise GateError(f"{CONFIG_PATH} is not a regular file")
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise GateError(f"cannot read {CONFIG_PATH}: {exc}") from exc
    return _validate_config(payload)


def _parse_raw_diff(data: bytes) -> list[dict[str, str]]:
    tokens = data.split(b"\0")
    if tokens and tokens[-1] == b"":
        tokens.pop()
    entries: list[dict[str, str]] = []
    index = 0
    while index < len(tokens):
        header = tokens[index]
        index += 1
        if not header.startswith(b":"):
            raise GateError("unexpected staged diff record")
        fields = header[1:].split()
        if len(fields) != 5 or index >= len(tokens):
            raise GateError("malformed staged diff record")
        status = fields[4].decode("ascii", errors="strict")
        path = tokens[index]
        index += 1
        if status[:1] in {"R", "C"}:
            if index >= len(tokens):
                raise GateError("rename or copy record is missing its destination")
            path = tokens[index]
            index += 1
        entries.append(
            {
                "status": status[:1],
                "mode": fields[1].decode("ascii", errors="strict"),
                "oid": fields[3].decode("ascii", errors="strict"),
                "path": os.fsdecode(path),
            }
        )
    return entries


def staged_entries(root: Path) -> list[dict[str, str]]:
    result = _run_git(
        root,
        "diff",
        "--cached",
        "--raw",
        "-z",
        "--no-abbrev",
        "--diff-filter=ACMRT",
        "--",
    )
    return _parse_raw_diff(result.stdout)


def _generated_paths(root: Path, paths: list[str]) -> set[str]:
    if not paths:
        return set()
    encoded = b"\0".join(os.fsencode(path) for path in paths) + b"\0"
    result = _run_git(
        root,
        "check-attr",
        "--cached",
        "-z",
        "--stdin",
        "linguist-generated",
        input_bytes=encoded,
    )
    fields = result.stdout.split(b"\0")
    if fields and fields[-1] == b"":
        fields.pop()
    if len(fields) % 3:
        raise GateError("git check-attr returned malformed output")
    generated: set[str] = set()
    for index in range(0, len(fields), 3):
        path, attribute, value = fields[index : index + 3]
        if attribute != b"linguist-generated":
            raise GateError("git check-attr returned an unexpected attribute")
        if value.lower() in {b"set", b"true", b"yes", b"1"}:
            generated.add(os.fsdecode(path))
    return generated


def _matches(path: str, pattern: str) -> bool:
    if fnmatch.fnmatchcase(path, pattern):
        return True
    return pattern.startswith("**/") and fnmatch.fnmatchcase(path, pattern[3:])


def _ignore_reason(
    path: str, config: dict[str, Any], generated: set[str]
) -> str | None:
    if path in generated:
        return "staged .gitattributes marks linguist-generated"
    for rule in config["ignore_patterns"]:
        if _matches(path, rule["pattern"]):
            return f"ignore pattern {rule['pattern']!r}: {rule['reason']}"
    return None


def _blob_size(root: Path, oid: str) -> int:
    if not oid or set(oid) == {"0"}:
        raise GateError("staged blob has no materialized object id")
    result = _run_git(root, "cat-file", "-s", oid)
    try:
        return int(result.stdout.strip())
    except ValueError as exc:
        raise GateError(f"git cat-file returned an invalid size for {oid}") from exc


def evaluate(repo: Path) -> dict[str, Any]:
    root = resolve_root(repo)
    config = load_config(root)
    entries = staged_entries(root)
    generated = _generated_paths(root, [entry["path"] for entry in entries])
    checked: list[dict[str, Any]] = []
    ignored: list[dict[str, str]] = []
    skipped: list[dict[str, str]] = []
    violations: list[dict[str, Any]] = []

    for entry in entries:
        path = entry["path"]
        reason = _ignore_reason(path, config, generated)
        if reason:
            ignored.append({"path": path, "status": entry["status"], "reason": reason})
            continue
        if entry["mode"] == "160000":
            skipped.append(
                {"path": path, "status": entry["status"], "reason": "gitlink is not a blob"}
            )
            continue
        if not (entry["mode"].startswith("100") or entry["mode"] == "120000"):
            skipped.append(
                {
                    "path": path,
                    "status": entry["status"],
                    "reason": f"unsupported index mode {entry['mode']}",
                }
            )
            continue
        size = _blob_size(root, entry["oid"])
        item = {
            "path": path,
            "status": entry["status"],
            "staged_bytes": size,
            "estimated_tokens": size / BYTES_PER_TOKEN,
        }
        checked.append(item)
        if size > MAX_BYTES:
            violations.append(
                {
                    **item,
                    "code": ERROR_CODE,
                    "limit_bytes": MAX_BYTES,
                    "limit_tokens": MAX_BYTES // BYTES_PER_TOKEN,
                    "over_bytes": size - MAX_BYTES,
                    "minimum_parts": math.ceil(size / MAX_BYTES),
                }
            )

    return {
        "schema": SCHEMA,
        "root": str(root),
        "gate": "fail" if violations else "pass",
        "limit": {"bytes": MAX_BYTES, "tokens": MAX_BYTES // BYTES_PER_TOKEN},
        "violations": violations,
        "checked": checked,
        "ignored": ignored,
        "skipped": skipped,
    }


def _format_tokens(value: float) -> str:
    return str(int(value)) if value.is_integer() else f"{value:.2f}"


def render_human(result: dict[str, Any], *, verbose: bool) -> str:
    lines: list[str] = []
    for item in result["violations"]:
        lines.extend(
            [
                f"ERROR [{ERROR_CODE}] {item['path']}",
                f"  staged size: {item['staged_bytes']} bytes (~{_format_tokens(item['estimated_tokens'])} tokens)",
                f"  limit: {item['limit_bytes']} bytes ({item['limit_tokens']} tokens); over by {item['over_bytes']} bytes",
                f"  split target: at least {item['minimum_parts']} cohesive files, each <= {MAX_BYTES} staged bytes",
            ]
        )
    if result["violations"]:
        lines.extend(
            [
                "",
                "Split by responsibility, domain concept, lifecycle, adapter boundary, or independently testable behavior.",
                "Do not split by line number or move unrelated code into generic utils/common modules.",
                "Invoke $pre-commit-token-limit to inspect imports, callers, tests, and repository conventions before editing.",
                "If this is generated, vendored, or lock content, add a reviewed precise ignore rule instead of bypassing the hook.",
            ]
        )
    if verbose:
        for item in result["ignored"]:
            lines.append(f"IGNORED {item['path']}: {item['reason']}")
        for item in result["skipped"]:
            lines.append(f"SKIPPED {item['path']}: {item['reason']}")
        lines.append(
            f"Checked {len(result['checked'])}; ignored {len(result['ignored'])}; skipped {len(result['skipped'])}."
        )
    return "\n".join(lines)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Reject staged Git blobs larger than 20,000 bytes."
    )
    parser.add_argument("--repo", type=Path, default=Path.cwd())
    parser.add_argument("--format", choices=("human", "json"), default="human")
    parser.add_argument("--verbose", action="store_true")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        result = evaluate(args.repo)
    except GateError as exc:
        if args.format == "json":
            print(json.dumps({"schema": SCHEMA, "gate": "blocked", "error": str(exc)}))
        else:
            print(f"BLOCKED [STAGED_FILE_TOKEN_LIMIT_ERROR] {exc}", file=sys.stderr)
        return 2
    if args.format == "json":
        print(json.dumps(result, ensure_ascii=True, sort_keys=True))
    else:
        output = render_human(result, verbose=args.verbose)
        if output:
            print(output, file=sys.stderr if result["violations"] else sys.stdout)
    return 1 if result["violations"] else 0


if __name__ == "__main__":
    raise SystemExit(main())

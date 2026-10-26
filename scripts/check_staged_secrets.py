#!/usr/bin/env python3
"""Reject private files and likely credentials in the actual Git index.

Diagnostics contain paths and rule names only. Run Gitleaks as the broader
release scan as well; this lightweight hook does not replace it.
"""
import argparse
import json
import math
import re
import subprocess
from collections import Counter
from pathlib import Path


PATTERNS = {
    "private-key": re.compile(rb"-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----"),
    "github-token": re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{70,})\b"),
    "cloud-access-key": re.compile(rb"\b(?:AKIA[0-9A-Z]{16}|AKLT[A-Za-z0-9]{16,})\b"),
    "credential-url": re.compile(rb"https?://[^/\s:@]+:[^/\s@]+@"),
    "machine-home-path": re.compile(rb"/Users/[A-Za-z0-9_.-]+/(?:workspace|orca|\.trae|\.zshrc)"),
}
ASSIGNMENT = re.compile(
    rb"(?i)\b(?:secret[_-]?(?:key|token)?|access[_-]?key|password|api[_-]?key|auth[_-]?token)"
    rb"\s*[:=]\s*[\"']([^\s\"']{20,})[\"']"
)


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args])


def evaluate(repo):
    findings = []
    names = git(repo, "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z").split(b"\0")
    for raw in names:
        if not raw:
            continue
        name = raw.decode("utf-8", "surrogateescape")
        p = Path(name)
        if (p.name == ".env" or p.name.startswith(".env.") or
                p.name in {".envrc", ".zshrc", "credentials.json"} or
                p.suffix.lower() in {".pem", ".key", ".p12", ".pfx"} or
                any(part in {".openmpa", ".trae", "node_modules"} for part in p.parts) or
                name.startswith(("desktop/dist/", "desktop/build/", "desktop/cmd/verify-"))):
            findings.append({"path": name, "rule": "private-or-environment-file"})
            continue
        data = git(repo, "show", ":" + name)
        for rule, pattern in PATTERNS.items():
            if pattern.search(data):
                findings.append({"path": name, "rule": rule})
        for match in ASSIGNMENT.finditer(data):
            value = match.group(1)
            if any(word in value.lower() for word in (b"fixture", b"example", b"placeholder", b"redacted")):
                continue
            counts = Counter(value)
            entropy = -sum((n / len(value)) * math.log2(n / len(value)) for n in counts.values())
            if entropy >= 3.6:
                findings.append({"path": name, "rule": "high-entropy-credential-assignment"})
                break
    return findings


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, default=Path.cwd())
    args = parser.parse_args()
    findings = evaluate(args.repo)
    print(json.dumps({"state": "fail" if findings else "pass", "findings": findings}))
    return bool(findings)


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Differential reference driver for the Go gitignore port (GO-07.S2).

Runs the REAL Python modules in lib/_internal/ via subprocess — the same
argv/cwd the sync spine uses — and emits canonical JSON so gitignore_test.go
can compare the Go port byte-for-byte against the frozen Python authority.

Modes:
  render <toml_path> <output_path>
      python3 lib/_internal/gitignore-render.py <toml_path> <output_path>
      -> {stdout, stderr, rc, file_bytes_sha256, file_bytes_b64}
  root-refresh <project_root> <template_path>
      python3 lib/_internal/gitignore-root-refresh.py <project_root> <template_path>
      -> {stdout, stderr, rc, action,
          resulting_gitignore_sha256, resulting_gitignore_b64}

`stderr` is included beyond the minimal field set because the missing-template
case asserts the exact `ERROR: template not found: ...` line the Python module
prints. `action` is derived from the module's own status line.
"""
from __future__ import annotations

import base64
import hashlib
import json
import subprocess
import sys
from pathlib import Path

# testdata/ -> internal/sync/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]
INTERNAL = REPO_ROOT / "lib" / "_internal"


def sha256_b64(path: Path):
    """Return (sha256_hex, base64) for a regular file, or (None, None)."""
    if not path.is_file():
        return None, None
    raw = path.read_bytes()
    return hashlib.sha256(raw).hexdigest(), base64.b64encode(raw).decode("ascii")


def run(module: str, *args: str):
    """Run one real Python module and return (stdout, stderr, rc)."""
    proc = subprocess.run(
        ["python3", str(INTERNAL / module), *args],
        capture_output=True,
    )
    return (
        proc.stdout.decode("utf-8"),
        proc.stderr.decode("utf-8"),
        proc.returncode,
    )


def main() -> int:
    mode = sys.argv[1]
    if mode == "render":
        toml_path, output_path = sys.argv[2], sys.argv[3]
        stdout, stderr, rc = run("gitignore-render.py", toml_path, output_path)
        sha, b64 = sha256_b64(Path(output_path))
        payload = {
            "stdout": stdout,
            "stderr": stderr,
            "rc": rc,
            "file_bytes_sha256": sha,
            "file_bytes_b64": b64,
        }
    elif mode == "root-refresh":
        root, template_path = sys.argv[2], sys.argv[3]
        stdout, stderr, rc = run(
            "gitignore-root-refresh.py", root, template_path)
        if "refreshed root .gitignore" in stdout:
            action = "refreshed"
        elif "appended root .gitignore" in stdout:
            action = "appended"
        else:
            action = ""
        sha, b64 = sha256_b64(Path(root) / ".gitignore")
        payload = {
            "stdout": stdout,
            "stderr": stderr,
            "rc": rc,
            "action": action,
            "resulting_gitignore_sha256": sha,
            "resulting_gitignore_b64": b64,
        }
    else:
        print(f"unknown mode {mode!r}", file=sys.stderr)
        return 2
    sys.stdout.write(json.dumps(payload, sort_keys=True, ensure_ascii=False))
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())

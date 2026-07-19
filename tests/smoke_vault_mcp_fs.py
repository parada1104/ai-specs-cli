#!/usr/bin/env python3
"""Smoke: vault-fs-mcp.sh launches real filesystem MCP with the right root.

No sync, no release, no LLM. Proves standalone CANONICAL_VAULT_PATH (OBSIDIAN
unset) is what the server scopes to.

Usage (from repo / worktree root):
  python3 tests/smoke_vault_mcp_fs.py
  python3 tests/smoke_vault_mcp_fs.py --path '/abs/path/with spaces/scope'
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WRAPPER = (
    ROOT
    / "catalog"
    / "recipes"
    / "vault-canonical-store"
    / "templates"
    / "vault-fs-mcp.sh"
)

ALLOWED_RE = re.compile(r"Allowed directories:\s*\[\s*'([^']+)'\s*\]", re.DOTALL)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--path", help="Absolute vault scope (default: temp dir)")
    parser.add_argument("--timeout", type=float, default=60.0)
    args = parser.parse_args()

    if not WRAPPER.is_file():
        print(f"FAIL: missing wrapper {WRAPPER}", file=sys.stderr)
        return 2

    cleanup: Path | None = None
    if args.path:
        vault = Path(args.path).expanduser().resolve()
        if not vault.is_dir():
            print(f"FAIL: --path not a directory: {vault}", file=sys.stderr)
            return 2
    else:
        cleanup = Path(tempfile.mkdtemp(prefix="vault-mcp-smoke-"))
        # include spaces in default path to match iCloud-style roots
        vault = cleanup / "Mobile Documents" / "scope"
        vault.mkdir(parents=True)
        (vault / "hello.md").write_text("# vault-mcp-smoke\n", encoding="utf-8")

    env = {k: v for k, v in os.environ.items() if k != "OBSIDIAN_VAULT_PATH"}
    env["CANONICAL_VAULT_PATH"] = str(vault)

    print(f"wrapper: {WRAPPER}")
    print(f"CANONICAL_VAULT_PATH={vault}")
    print("OBSIDIAN_VAULT_PATH unset")

    proc = subprocess.Popen(
        ["bash", str(WRAPPER)],
        stdin=subprocess.PIPE,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
    )
    deadline = time.time() + args.timeout
    buf = ""
    try:
        assert proc.stderr is not None
        while time.time() < deadline:
            line = proc.stderr.readline()
            if not line and proc.poll() is not None:
                break
            buf += line
            print(line.rstrip())
            match = ALLOWED_RE.search(buf)
            if match:
                allowed = Path(match.group(1)).resolve()
                expected = vault.resolve()
                # macOS /var vs /private/var
                if allowed == expected or allowed.resolve() == expected.resolve():
                    print(f"SMOKE_OK allowed={allowed}")
                    return 0
                # Compare samefile when both exist
                try:
                    if allowed.samefile(expected):
                        print(f"SMOKE_OK allowed={allowed} (samefile)")
                        return 0
                except OSError:
                    pass
                print(
                    f"SMOKE_FAIL: allowed {allowed} != expected {expected}",
                    file=sys.stderr,
                )
                return 1
        print("SMOKE_FAIL: timeout waiting for 'Allowed directories' on stderr", file=sys.stderr)
        print(buf[-2000:], file=sys.stderr)
        return 1
    finally:
        proc.kill()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.terminate()
        if cleanup is not None:
            for p in sorted(cleanup.rglob("*"), reverse=True):
                if p.is_file():
                    p.unlink(missing_ok=True)
                elif p.is_dir():
                    p.rmdir()
            cleanup.rmdir()


if __name__ == "__main__":
    raise SystemExit(main())

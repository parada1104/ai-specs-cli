#!/usr/bin/env python3
"""Differential reference driver for the Go mcp-render port (GO-07.S4).

Reads ONE JSON object from stdin describing a sandbox:

    {"manifest_rel": "ai-specs/ai-specs.toml",
     "manifest_b64": <b64 | null>,      # null -> do not create (missing manifest)
     "recipe_rel": "recipe-mcp.json",   # or null/"" -> no --recipe-mcp flag
     "recipe_b64": <b64 | null>,
     "target_rel": ".mcp.json",
     "target_b64": <b64 | null>,
     "agent": "claude",
     "mcp_key": "mcpServers",
     "dry_run": false}

It imports the REAL ``lib/_internal/mcp-render.py`` and calls its ``main()``
with a patched ``sys.argv`` inside a freshly created temp sandbox, so the
reference is exactly the script the Bash spine shells out to.

The sandbox root is replaced with ``<TEMP>`` in stdout/stderr (both legs run in
their own throwaway roots), and the written target file (if any) is returned
base64-encoded.

Emits ``{"stdout": ..., "stderr": ..., "rc": ..., "target_b64": ...}``.
"""
from __future__ import annotations

import base64
import contextlib
import importlib.util
import io
import json
import sys
import tempfile
from pathlib import Path

# testdata/ -> internal/sync/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]
MODULE_PATH = REPO_ROOT / "lib" / "_internal" / "mcp-render.py"

_MODULE = None


def load_module():
    """Import the real mcp-render.py exactly once per process."""
    global _MODULE
    if _MODULE is not None:
        return _MODULE
    spec = importlib.util.spec_from_file_location("mcp_render_ref", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load mcp-render.py at {MODULE_PATH}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    _MODULE = mod
    return mod


def _write(root: Path, rel: str, b64: object) -> Path:
    path = root / rel
    if b64 is not None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(base64.b64decode(b64))
    return path


def main() -> int:
    case = json.loads(sys.stdin.read())
    root = Path(tempfile.mkdtemp(prefix="mcpdiff-"))

    manifest_path = _write(root, case["manifest_rel"], case.get("manifest_b64"))
    recipe_path = None
    if case.get("recipe_rel"):
        recipe_path = _write(root, case["recipe_rel"], case.get("recipe_b64"))
    target_path = _write(root, case["target_rel"], case.get("target_b64"))

    argv = [str(manifest_path), case["agent"], str(target_path), case["mcp_key"]]
    if recipe_path is not None:
        argv += ["--recipe-mcp", str(recipe_path)]
    if case.get("dry_run"):
        argv.append("--dry-run")

    mod = load_module()
    out, err = io.StringIO(), io.StringIO()
    saved_argv = sys.argv
    sys.argv = ["mcp-render.py", *argv]
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            rc = mod.main()
    finally:
        sys.argv = saved_argv

    prefix = str(root)
    result = {
        "stdout": out.getvalue().replace(prefix, "<TEMP>"),
        "stderr": err.getvalue().replace(prefix, "<TEMP>"),
        "rc": rc,
        "target_b64": None,
    }
    if target_path.is_file():
        result["target_b64"] = base64.b64encode(target_path.read_bytes()).decode("ascii")

    sys.stdout.write(json.dumps(result))
    return 0


if __name__ == "__main__":
    sys.exit(main())

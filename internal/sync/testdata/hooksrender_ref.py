#!/usr/bin/env python3
"""Differential reference driver for the Go hooks-render port (GO-07.S5).

Reads ONE JSON object from stdin describing a sandbox:

    {"resolved_rel": "resolved-hooks.json",
     "resolved_b64": <b64 | null>,     # null -> do not create the resolved blob
     "resolved_dir": false,            # create the resolved path as a directory
     "unreadable_rel": "path or null", # chmod 0200 (read-denied) before running
     "agent": "claude",
     "files": {"<rel>": "<b64>", ...}} # pre-seeded project files

It imports the REAL ``lib/_internal/hooks-render.py`` and calls its ``render()``
with a freshly created temp sandbox as the project root, so the reference is
exactly the module the Bash spine shells out to.

Unlike mcp-render, hooks-render writes MANY files (settings.json, cursor
wrappers + hooks.json, TS adapters). The driver therefore snapshots the whole
sandbox after the run: ``tree`` maps every regular file to base64 bytes and
``modes`` maps it to the file mode bits (the cursor wrapper is chmod 0755).

Emits ``{"stdout": ..., "stderr": ..., "rc": ..., "tree": {...}, "modes": {...}}``
with the sandbox root collapsed to ``<TEMP>``.
"""
from __future__ import annotations

import base64
import contextlib
import importlib.util
import io
import json
import os
import sys
import tempfile
import traceback
from pathlib import Path

# testdata/ -> internal/sync/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]
MODULE_PATH = REPO_ROOT / "lib" / "_internal" / "hooks-render.py"

_MODULE = None


def load_module():
    """Import the real hooks-render.py exactly once per process."""
    global _MODULE
    if _MODULE is not None:
        return _MODULE
    spec = importlib.util.spec_from_file_location("hooks_render_ref", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load hooks-render.py at {MODULE_PATH}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    _MODULE = mod
    return mod


def _write_bytes(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def snapshot(root: Path) -> tuple[dict[str, str], dict[str, int]]:
    """Every regular file under root, as base64 bytes plus its mode bits.

    Symlinks and directories are skipped: the renderers only ever write
    regular files, and both legs start from the same pre-seeded layout.
    """
    tree: dict[str, str] = {}
    modes: dict[str, int] = {}
    for p in sorted(root.rglob("*")):
        if p.is_symlink() or not p.is_file():
            continue
        rel = p.relative_to(root).as_posix()
        tree[rel] = base64.b64encode(p.read_bytes()).decode("ascii")
        modes[rel] = p.stat().st_mode & 0o7777
    return tree, modes


def main() -> int:
    case = json.loads(sys.stdin.read())
    root = Path(tempfile.mkdtemp(prefix="hookdiff-"))

    for rel, b64 in (case.get("files") or {}).items():
        _write_bytes(root / rel, base64.b64decode(b64))

    resolved_path = root / case["resolved_rel"]
    if case.get("resolved_dir"):
        resolved_path.mkdir(parents=True, exist_ok=True)
    elif case.get("resolved_b64") is not None:
        _write_bytes(resolved_path, base64.b64decode(case["resolved_b64"]))

    if case.get("unreadable_rel"):
        # 0200: read denied. hooks-render's _load_json_file swallows the OSError
        # and then write_text() overwrites the read-only file.
        os.chmod(root / case["unreadable_rel"], 0o200)

    mod = load_module()
    out, err = io.StringIO(), io.StringIO()
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            mod.render(resolved_path, case["agent"], root)
        rc = 0
    except BaseException:
        # Invalid UTF-8 / a malformed hook field surfaces in the real module as
        # an uncaught traceback (rc 1). Catch it so the differential pins rc,
        # stdout, the written tree and the exception class instead of comparing
        # an unreproducible stack trace byte for byte.
        traceback.print_exc(file=err)
        rc = 1

    if case.get("unreadable_rel"):
        try:
            os.chmod(root / case["unreadable_rel"], 0o644)
        except OSError:
            pass

    tree, modes = snapshot(root)
    prefix = str(root)
    sys.stdout.write(json.dumps({
        "stdout": out.getvalue().replace(prefix, "<TEMP>"),
        "stderr": err.getvalue().replace(prefix, "<TEMP>"),
        "rc": rc,
        "tree": tree,
        "modes": modes,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())

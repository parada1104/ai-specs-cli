#!/usr/bin/env python3
"""Differential reference driver for the Go agents-render core port (GO-07.S3a).

Reads ONE JSON object from stdin:

    {"manifest_toml_b64": <b64 of a TOML manifest>,
     "resolved_json_b64": <b64 of the resolved-config JSON>}

The resolved JSON may carry "project_root" (as render() would setdefault it);
when it does not, the driver falls back to os.getcwd(), mirroring
``render()``.

It imports the REAL ``lib/_internal/agents-render.py`` and reproduces the exact
bytes ``render()`` would write when the governance decision is "write": the
resolved-config coercion render() performs, then
``"\\n".join(_render_lines(manifest, resolved)).encode()``. The governance
state machine (classify/adopt/preserve) is deliberately NOT exercised here --
that is slice S3b.

Emits ``{"bytes_b64": ..., "sha256": ...}`` or, when the Python authority
raises, ``{"error": {"type": ..., "message": ...}}``.
"""
from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import os
import sys
import tomllib
from pathlib import Path

# testdata/ -> internal/sync/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]
MODULE_PATH = REPO_ROOT / "lib" / "_internal" / "agents-render.py"

_MODULE = None


def load_module():
    """Import the real agents-render.py exactly once per process."""
    global _MODULE
    if _MODULE is not None:
        return _MODULE
    spec = importlib.util.spec_from_file_location("agents_render_ref", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load agents-render.py at {MODULE_PATH}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    _MODULE = mod
    return mod


def coerce_resolved(data):
    """Mirror render()'s inner-field coercion so wrong-typed values degrade."""
    if not isinstance(data, dict):
        data = {}
    if not isinstance(data.get("bindings"), dict):
        data["bindings"] = {}
    if not isinstance(data.get("recipes"), dict):
        data["recipes"] = {}
    if not isinstance(data.get("enabled"), list):
        data["enabled"] = []
    return data


def main() -> int:
    case = json.loads(sys.stdin.read())
    manifest = tomllib.loads(
        base64.b64decode(case["manifest_toml_b64"]).decode("utf-8")
    )
    resolved = coerce_resolved(
        json.loads(base64.b64decode(case["resolved_json_b64"]).decode("utf-8"))
    )
    resolved.setdefault("project_root", os.getcwd())

    mod = load_module()
    try:
        content = "\n".join(mod._render_lines(manifest, resolved)).encode("utf-8")
    except Exception as exc:  # noqa: BLE001 - the error IS the differential payload
        sys.stdout.write(
            json.dumps(
                {"error": {"type": type(exc).__name__, "message": str(exc)}},
                ensure_ascii=False,
            )
        )
        sys.stdout.write("\n")
        return 0

    sys.stdout.write(
        json.dumps(
            {
                "bytes_b64": base64.b64encode(content).decode("ascii"),
                "sha256": hashlib.sha256(content).hexdigest(),
            },
            ensure_ascii=False,
        )
    )
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())

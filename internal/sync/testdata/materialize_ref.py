#!/usr/bin/env python3
"""Differential reference driver for the native materializer spine (GO-07 S14a WU1).

Imports the REAL ``materialize_recipes`` from ``lib/_internal/recipe-materialize.py``
in a subprocess and drives the real orchestration — never a reimplementation.
``internal/sync/materialize_test.go`` compares the emitted stdout/stderr lines and
the serialized recipe-MCP / resolved-config / resolved-hooks documents byte for
byte against ``sync.MaterializeRecipes``.

Usage:
  <authority> <project_root> <ai_specs_home> <recipe_mcp_out|-> <resolved_config_out|-> <resolved_hooks_out|->

Emits compact JSON with the captured streams and each requested document:
  {"rc": int, "stdout": str, "stderr": str,
   "recipe_mcp": str, "resolved_config": str, "resolved_hooks": str}

A document is "" when its output argument was "-" (the caller reads the path the
leg reported on stdout instead). ``WORKTREE_GATE_BIN`` is inherited so the caller
can pin the bridge binary; the driver never substitutes a Python reimplementation.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[1]
spec = importlib.util.spec_from_file_location("materialize_spine_oracle", AUTHORITY)
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def _read(path: str) -> str:
    if path == "-":
        return ""
    try:
        return Path(path).read_text(encoding="utf-8")
    except OSError:
        return ""


def main() -> int:
    project_root = Path(sys.argv[2])
    ai_specs_home = Path(sys.argv[3])
    recipe_mcp_out = sys.argv[4]
    resolved_config_out = sys.argv[5]
    resolved_hooks_out = sys.argv[6]

    out, err = io.StringIO(), io.StringIO()
    rc = 0
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            rc = mod.materialize_recipes(
                project_root,
                ai_specs_home,
                None if recipe_mcp_out == "-" else Path(recipe_mcp_out),
                None if resolved_config_out == "-" else Path(resolved_config_out),
                None if resolved_hooks_out == "-" else Path(resolved_hooks_out),
            )
    except SystemExit as exc:
        code = exc.code
        rc = code if isinstance(code, int) else 1
    except BaseException as exc:  # noqa: BLE001 - mirror main()'s fail()
        mod.fail(str(exc))
        rc = 1

    payload = {
        "rc": rc,
        "stdout": out.getvalue(),
        "stderr": err.getvalue(),
        "recipe_mcp": _read(recipe_mcp_out),
        "resolved_config": _read(resolved_config_out),
        "resolved_hooks": _read(resolved_hooks_out),
    }
    sys.stdout.write(json.dumps(payload, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())

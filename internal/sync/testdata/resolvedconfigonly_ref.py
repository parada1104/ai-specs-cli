#!/usr/bin/env python3
"""Differential reference driver for the native resolved-config-only mode (GO-07 S14c).

Imports the REAL ``build_resolved_config_only`` from ``lib/_internal/recipe-materialize.py``
in a subprocess and drives it — never a reimplementation.
``internal/sync/resolvedconfigonly_test.go`` compares the captured streams, the exit
code and the written document byte for byte against ``sync.BuildResolvedConfigOnly``.

Usage:
  <authority> <project_root> <ai_specs_home> <resolved_config_out>

Emits compact JSON:
  {"rc": int, "stdout": str, "stderr": str, "document": str}

``document`` is "" when the mode wrote nothing. ``WORKTREE_GATE_BIN`` is inherited so
the caller can pin the bridge binary; the driver never substitutes a Python
reimplementation.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[1]
spec = importlib.util.spec_from_file_location("resolved_config_only_oracle", AUTHORITY)
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def main() -> int:
    project_root = Path(sys.argv[2])
    ai_specs_home = Path(sys.argv[3])
    out_path = Path(sys.argv[4])

    out, err = io.StringIO(), io.StringIO()
    rc = 0
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            rc = mod.build_resolved_config_only(project_root, out_path, ai_specs_home)
    except BaseException as exc:  # defensive: the mode catches its own exceptions
        err.write(f"  ✗ {exc}\n")
        rc = 1

    try:
        document = out_path.read_text(encoding="utf-8")
    except OSError:
        document = ""

    print(json.dumps({
        "rc": rc,
        "stdout": out.getvalue(),
        "stderr": err.getvalue(),
        "document": document,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())

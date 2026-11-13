#!/usr/bin/env python3
"""Differential reference driver for the native resolved-config authority (GO-07 S13 PR3).

Imports the REAL fail-open body ``_python_build_resolved_config`` in
``lib/_internal/recipe-materialize.py`` — never a re-implementation — and emits
canonical output so ``internal/sync/resolvedconfig_test.go`` can require equality
against the Go root authority (``internal/sync.BuildResolvedConfig``).

Modes:
  project <authority> <project_root>
      ``_python_build_resolved_config(Path(project_root))`` (:3763), the
      temporary Python projection the bridge falls back to. Emits compact JSON
      ``{"ok": true, "result": <envelope>}`` on success or
      ``{"ok": false, "exc_type": <name>, "message": <str>}`` when the manifest
      acquisition or the projection raises (missing/unparsable manifest), the
      exact transport failure the Go authority reports as a Go error.
"""
from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("materialize_resolvedconfig_oracle", AUTHORITY)
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def main() -> int:
    mode = sys.argv[1]
    project_root = Path(sys.argv[3])
    if mode == "project":
        try:
            result = mod._python_build_resolved_config(project_root)
        except Exception as exc:  # noqa: BLE001 - the oracle reports the message
            out = {"ok": False, "exc_type": type(exc).__name__, "message": str(exc)}
        else:
            out = {"ok": True, "result": result}
        sys.stdout.write(json.dumps(out, separators=(",", ":"), ensure_ascii=False))
        return 0
    print(f"unknown mode {mode!r}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())

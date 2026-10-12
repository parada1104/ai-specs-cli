#!/usr/bin/env python3
"""Differential reference driver for the native recipe hooks runner (GO-07 S13 PR4a).

Imports the REAL ``execute_hooks`` from ``lib/_internal/recipe-materialize.py``
in a subprocess and emits canonical output so ``internal/sync/hooksrunner_test.go``
can require equality against the Go native runner (``RunRecipeHooks``). It never
re-implements the Python body: it reads the fixture catalog recipe through the
real ``read_recipe`` and calls the real ``execute_hooks``.

Usage:
  run <authority> <catalog> <recipe_id> <project_root> <cli_home> <merged_json>
      ``read_recipe`` + ``execute_hooks`` under redirected stdout/stderr.
      -> compact JSON ``{"error": <raised str>, "stdout": <captured>, "stderr": <captured>}``
         (``error`` is empty when nothing was raised).
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("materialize_hooks_oracle", AUTHORITY)
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def main() -> int:
    mode = sys.argv[1]
    if mode != "run":
        print(f"unknown mode {mode!r}", file=sys.stderr)
        return 2

    catalog = Path(sys.argv[3])
    recipe_id = sys.argv[4]
    project_root = Path(sys.argv[5])
    cli_home = Path(sys.argv[6])
    merged = json.loads(sys.argv[7])

    recipe = mod.read_recipe(catalog, recipe_id)
    out, err = io.StringIO(), io.StringIO()
    message = ""
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            mod.execute_hooks(recipe, merged, project_root, cli_home=cli_home)
    except BaseException as exc:  # noqa: BLE001 - the oracle reports the message
        message = str(exc)

    sys.stdout.write(json.dumps({
        "error": message,
        "stdout": out.getvalue(),
        "stderr": err.getvalue(),
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""Differential reference driver for the native recipe-MCP build (GO-07 S13 PR4b-2).

Imports the REAL ``build_recipe_mcp`` and ``_resolve_provider_markers`` from
``lib/_internal/recipe-materialize.py`` in a subprocess and emits canonical
output so ``internal/sync/buildrecipemcp_test.go`` can require equality against
the Go native authority (``BuildRecipeMCP`` / ``resolveProviderMarkers``). It
never re-implements the Python body: it calls the real functions.

Usage:
  <mode> <authority> <catalog> <recipe_ids_json> <payload_json> <home_or_dash>

  mode "build":   payload is the manifest mcp mapping.
                  calls build_recipe_mcp(catalog, recipe_ids, payload, ai_specs_home=...).
  mode "resolve": payload is the merged mcp mapping; ``home`` must be a path.
                  calls _resolve_provider_markers(payload, catalog, recipe_ids, home)
                  and serializes the mutated mapping.

Emits compact JSON
``{"error": <raised str>, "stdout": <captured>, "stderr": <captured>, "document": <json.dumps(indent=2)+"\n">}``
(``error`` is empty when nothing was raised; ``document`` is empty on error).
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("materialize_mcp_oracle", AUTHORITY)
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def main() -> int:
    mode = sys.argv[1]
    if mode not in ("build", "resolve"):
        print(f"unknown mode {mode!r}", file=sys.stderr)
        return 2

    catalog = Path(sys.argv[3])
    try:
        recipe_ids = json.loads(sys.argv[4])
        payload = json.loads(sys.argv[5])
    except json.JSONDecodeError as exc:
        print(f"invalid JSON argument: {exc}", file=sys.stderr)
        return 2
    home_arg = sys.argv[6]
    ai_specs_home = None if home_arg == "-" else Path(home_arg)

    out, err = io.StringIO(), io.StringIO()
    message = ""
    document = ""
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            if mode == "build":
                result = mod.build_recipe_mcp(
                    catalog, recipe_ids, payload, ai_specs_home=ai_specs_home
                )
            else:
                mod._resolve_provider_markers(
                    payload, catalog, recipe_ids, ai_specs_home
                )
                result = payload
            document = json.dumps(result, indent=2) + "\n"
    except BaseException as exc:  # noqa: BLE001 - the oracle reports the message
        message = str(exc)

    sys.stdout.write(json.dumps({
        "error": message,
        "stdout": out.getvalue(),
        "stderr": err.getvalue(),
        "document": document,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())

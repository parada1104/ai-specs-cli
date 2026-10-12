#!/usr/bin/env python3
"""Differential reference driver for the native merge-config authority (GO-07 S13 PR2b).

Imports the REAL bodies in ``lib/_internal/recipe-materialize.py`` — never a
re-implementation — and emits canonical output so ``internal/sync/mergeconfig_test.go``
can require equality against the Go root authority
(``internal/sync.PlanMergeConfig``).

Modes:
  plan <authority> <catalog_dir> <recipe_id> <project_root>
      Loads the recipe through the real recipe_schema loader and the manifest
      config through the real toml-read reader, then runs ``_python_merge_config``
      (the fail-open Python authority, :3006). Emits compact JSON
      ``{"config": <object|null>, "warnings": [..], "error": <str|null>}``.
      ``error`` is the RuntimeError message a semantic failure raises; the config
      is null in that case, exactly as ``merge_config``'s caller projects it.
  envelope <authority> <catalog_dir> <recipe_id> <project_root>
      Builds the request with ``_merge_config_request_envelope`` (:2842) and
      serializes it with ``json.dumps(allow_nan=False)``. Emits
      ``{"ok": true, "request": <json string>}`` on success or
      ``{"ok": false, "exc_type": <name>, "message": <str>}`` when the envelope
      cannot be serialized (TOML datetime/date/time, non-finite float): the
      transport failure that makes the bridge fall back once.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("materialize_mergeconfig_oracle", AUTHORITY)
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def warn_lines(stderr: str) -> list[str]:
    """The ``  ! <msg>`` warning lines Python's warn() wrote to stderr."""
    return [line[4:] for line in stderr.splitlines() if line.startswith("  ! ")]


def load_inputs(catalog: Path, recipe_id: str, project_root: Path):
    recipe = mod.read_recipe(catalog, recipe_id)
    recipes = mod.load_recipes_from_manifest(project_root)
    manifest_config = recipes.get(recipe_id, {}).get("config", {})
    return recipe, manifest_config


def main() -> int:
    mode = sys.argv[1]
    catalog = Path(sys.argv[3])
    recipe_id = sys.argv[4]
    project_root = Path(sys.argv[5])

    if mode == "plan":
        recipe, manifest_config = load_inputs(catalog, recipe_id, project_root)
        captured = io.StringIO()
        try:
            with contextlib.redirect_stderr(captured):
                config = mod._python_merge_config(recipe, manifest_config)
        except RuntimeError as exc:
            out = {"config": None, "warnings": warn_lines(captured.getvalue()), "error": str(exc)}
        else:
            out = {"config": config, "warnings": warn_lines(captured.getvalue()), "error": None}
        sys.stdout.write(json.dumps(out, separators=(",", ":"), ensure_ascii=False))
        return 0

    if mode == "envelope":
        recipe, manifest_config = load_inputs(catalog, recipe_id, project_root)
        envelope = mod._merge_config_request_envelope(recipe, manifest_config)
        try:
            request = json.dumps(envelope, allow_nan=False)
        except (TypeError, ValueError) as exc:
            out = {"ok": False, "exc_type": type(exc).__name__, "message": str(exc)}
        else:
            out = {"ok": True, "request": request}
        sys.stdout.write(json.dumps(out, separators=(",", ":"), ensure_ascii=False))
        return 0

    print(f"unknown mode {mode!r}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())

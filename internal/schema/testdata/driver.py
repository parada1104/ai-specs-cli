#!/usr/bin/env python3
"""Differential driver for recipe.toml schema validation.

Usage (run from anywhere; paths are resolved relative to this file):
  python3 driver.py validate <recipe.toml> <recipedir-or-->
  python3 driver.py load    <recipe.toml>

- validate: loads the TOML and calls validate_recipe_toml(data, dir).
  "-" means recipe_dir=None (no-FS-resolution mode).
- load: calls load_recipe_toml(Path(path)) end to end.

Prints exactly one JSON line:
  {"ok": true, "recipe": <canonical projection>}
  {"ok": false, "error": "<exact Python message>"}

The projection is canonicalized (floats as repr strings) and emitted with
sort_keys/compact separators so the Go side can compare byte-for-byte.
"""

import importlib.util
import json
import os
import sys
from pathlib import Path

_DRIVER_DIR = os.path.dirname(os.path.abspath(__file__))
_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(_DRIVER_DIR)))
_SCHEMA_PATH = os.path.join(_REPO_ROOT, "lib", "_internal", "recipe_schema.py")

spec = importlib.util.spec_from_file_location("recipe_schema", _SCHEMA_PATH)
rs = importlib.util.module_from_spec(spec)
sys.modules["recipe_schema"] = rs
spec.loader.exec_module(rs)


def canon(v):
    """Canonicalize a tomllib value for byte-for-byte comparison."""
    if v is None or isinstance(v, (bool, int, str)):
        return v
    if isinstance(v, float):
        return repr(v)
    if isinstance(v, dict):
        return {k: canon(x) for k, x in v.items()}
    if isinstance(v, list):
        return [canon(x) for x in v]
    return str(v)


def proj(r):
    """Canonical projection of a Recipe dataclass."""
    brief = None
    if r.brief_fragments is not None:
        brief = {
            name: [{"text": canon(f.text), "key": canon(f.key)} for f in frags]
            for name, frags in vars(r.brief_fragments).items()
            if frags is not None
        }
    return {
        "id": r.id,
        "name": r.name,
        "description": r.description,
        "version": r.version,
        "author": r.author,
        "license": r.license,
        "tags": r.tags,
        "conflicts_with": r.conflicts_with,
        "skills": [
            {"id": s.id, "source": s.source, "url": s.url, "path": s.path}
            for s in r.skills
        ],
        "commands": [{"id": c.id, "path": c.path} for c in r.commands],
        "mcp": [{"id": m.id, "config": canon(m.config)} for m in r.mcp],
        "templates": [
            {
                "source": t.source,
                "target": t.target,
                "condition": t.condition,
                "update_policy": t.update_policy,
            }
            for t in r.templates
        ],
        "docs": [{"source": d.source, "target": d.target} for d in r.docs],
        "capabilities": [c.id for c in r.capabilities],
        "hooks": [{"event": h.event, "action": h.action} for h in r.hooks],
        "runtime_hooks": [
            {
                "id": h.id,
                "event": h.event,
                "script": h.script,
                "matcher": h.matcher,
                "blocking": h.blocking,
                "description": h.description,
            }
            for h in r.runtime_hooks
        ],
        "config": {
            "fields": {
                k: {
                    "type": f.type,
                    "default": canon(f.default),
                    "enum": f.enum,
                    "help_text": f.help_text,
                    "validation": canon(f.validation),
                }
                for k, f in r.config_schema.fields.items()
            },
            "extra": {k: canon(v) for k, v in r.config_schema.extra.items()},
            "tables": {
                k: {"shape": canon(t.shape), "values": canon(t.values)}
                for k, t in r.config_schema.tables.items()
            },
        },
        "cli_deps": [
            {
                "binary": d.binary,
                "purpose": d.purpose,
                "required": d.required,
                "install_url": d.install_url,
                "version_check": d.version_check,
                "min_version": d.min_version,
                "installer": d.installer,
                "repository": d.repository,
                "release_policy": d.release_policy,
            }
            for d in r.cli_deps
        ],
        "init": None
        if r.init is None
        else {
            "prompt": r.init.prompt,
            "description": r.init.description,
            "needs_manifest": r.init.needs_manifest,
            "needs_mcp": r.init.needs_mcp,
        },
        "brief": brief,
    }


def emit(payload):
    sys.stdout.write(
        json.dumps(payload, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
        + "\n"
    )


def main():
    mode = sys.argv[1]
    try:
        if mode == "load":
            path = sys.argv[2]
            r = rs.load_recipe_toml(Path(path))
            emit({"ok": True, "recipe": proj(r)})
            return
        if mode == "validate":
            path, dir_arg = sys.argv[2], sys.argv[3]
            with open(path, "rb") as fh:
                data = __import__("tomllib").load(fh)
            recipe_dir = None if dir_arg == "-" else Path(dir_arg)
            r = rs.validate_recipe_toml(data, recipe_dir)
            emit({"ok": True, "recipe": proj(r)})
            return
        raise SystemExit(f"unknown mode: {mode}")
    except rs.RecipeValidationError as e:
        emit({"ok": False, "error": str(e)})


if __name__ == "__main__":
    main()

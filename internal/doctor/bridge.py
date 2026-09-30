#!/usr/bin/env python3
"""Go->Python strangle bridge for the ai-specs doctor port (card [Go 08]).

Read-only. Imports the still-unported Python analysis modules out of
$AI_SPECS_HOME/lib/_internal and returns their raw results as one JSON object.
The Go side owns every check, severity, message, ordering and exit code; each
op here is independently guarded so a failing dependency degrades exactly the
way the legacy in-process call did.

Invoked as: python3 -c "<this file>" <ai-specs-home> <request-json>
"""
from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

HOME = Path(sys.argv[1]).resolve()
REQUEST = json.loads(sys.argv[2])
ROOT = Path(REQUEST["root"]).resolve()


def _load(name: str, filename: str):
    """Import one legacy module by file path, as doctor.py does."""
    path = HOME / "lib" / "_internal" / filename
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {filename}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def op_toml_error() -> dict:
    """doctor._check_manifest: the guidance renders tomllib's own diagnostic.

    That text belongs to the interpreter, and Python versions differ in it, so
    the port asks the same authority the legacy check did instead of inventing
    a lookalike.
    """
    import tomllib

    try:
        with (ROOT / "ai-specs" / "ai-specs.toml").open("rb") as fh:
            tomllib.load(fh)
    except Exception as exc:
        return {"error": f"{type(exc).__name__}: {exc}"}
    return {"error": None}


def _resolved_for_brief(materialize) -> dict:
    """Rebuild the resolved recipe data sync would pass to agents-render.

    Mirrors doctor._check_brief_provenance's reconstruction, including the
    auto-binding step, and is read-only: no project root and no write opt-in
    are passed to the bindings bridge.
    """
    resolved = materialize.build_resolved_config(ROOT)
    materialize.merge_catalog_defaults_into_resolved(resolved, HOME)
    materialize.attach_brief_fragments_to_resolved(resolved, HOME)
    enabled_ids = list(resolved.get("enabled") or [])
    if enabled_ids:
        manifest_bindings = materialize.load_bindings_from_manifest(ROOT)
        auto_bindings = materialize.resolve_bindings(
            HOME / "catalog" / "recipes", enabled_ids, manifest_bindings
        )
        if auto_bindings:
            resolved["bindings"] = auto_bindings
    resolved.setdefault("project_root", str(ROOT))
    return resolved


def op_brief_state() -> dict:
    """doctor._check_brief_provenance state; "undetermined" on any failure."""
    import tomllib

    manifest_path = ROOT / "ai-specs" / "ai-specs.toml"
    try:
        with manifest_path.open("rb") as fh:
            manifest = tomllib.load(fh)
        renderer = _load("agents_render_doctor", "agents-render.py")
        materialize = _load("recipe_materialize_doctor_brief", "recipe-materialize.py")
        resolved = _resolved_for_brief(materialize)
        would_write = "\n".join(renderer._render_lines(manifest, resolved)).encode()
        state = renderer.brief_effective_state(
            manifest_path, ROOT / "AGENTS.md", would_write
        )
    except Exception:
        state = "undetermined"
    return {"state": str(state)}


def op_brief_dead_fragments() -> dict:
    """doctor._check_brief_render_policy tail: no check when it fails."""
    try:
        policy = _load("brief_render_policy_bridge", "brief-render-policy.py")
        materialize = _load("recipe_materialize_doctor", "recipe-materialize.py")
        resolved = materialize.build_resolved_config(ROOT)
        materialize.attach_brief_fragments_to_resolved(resolved, HOME)
        dead = bool(policy.has_dead_recipe_fragments(resolved))
    except Exception:
        return {"dead": None}
    return {"dead": dead}


OPS = {
    "toml_error": op_toml_error,
    "brief_state": op_brief_state,
    "brief_dead_fragments": op_brief_dead_fragments,
}


def main() -> int:
    results = {}
    for name in REQUEST.get("ops") or []:
        fn = OPS.get(name)
        if fn is None:
            results[name] = {"error": f"unknown bridge op: {name}"}
            continue
        try:
            results[name] = fn()
        except Exception as exc:
            results[name] = {"error": f"{type(exc).__name__}: {exc}"}
    json.dump({"ops": results}, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())

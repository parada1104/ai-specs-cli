#!/usr/bin/env python3
"""Differential oracle for the doctor's brief probes (GO-07 S18).

Replicates the chain the retired internal/doctor/bridge.py used to run for its
two brief ops, through the REAL legacy modules, so the native `sync` brief view can be
compared against it state by state:

  state / would_write  -> op_brief_state     (_resolved_for_brief + _render_lines
                                              + brief_effective_state)
  dead                 -> op_brief_dead_fragments (build + fragments only, then
                                              has_dead_recipe_fragments)

Usage: briefref_ref.py <project_root> <ai_specs_home> <lib_internal_dir>
Prints one JSON object: {"error", "state", "would_write_sha", "dead"}.
"""
from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import json
import sys
import tomllib
from pathlib import Path


def load(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def main() -> int:
    root = Path(sys.argv[1]).resolve()
    home = Path(sys.argv[2]).resolve()
    lib = Path(sys.argv[3]).resolve()

    out: dict[str, object] = {
        "error": None,
        "state": None,
        "would_write_sha": None,
        "render_stdout": None,
        "dead": None,
    }
    try:
        materialize = load("recipe_materialize_ref", lib / "recipe-materialize.py")
        renderer = load("agents_render_ref", lib / "agents-render.py")
        policy = load("brief_render_policy_ref", lib / "brief-render-policy.py")

        manifest_path = root / "ai-specs" / "ai-specs.toml"
        with manifest_path.open("rb") as fh:
            manifest = tomllib.load(fh)

        # op_brief_state: _resolved_for_brief, then the render and the state.
        resolved = materialize.build_resolved_config(root)
        materialize.merge_catalog_defaults_into_resolved(resolved, home)
        materialize.attach_brief_fragments_to_resolved(resolved, home)
        enabled_ids = list(resolved.get("enabled") or [])
        if enabled_ids:
            manifest_bindings = materialize.load_bindings_from_manifest(root)
            auto_bindings = materialize.resolve_bindings(
                home / "catalog" / "recipes", enabled_ids, manifest_bindings
            )
            if auto_bindings:
                resolved["bindings"] = auto_bindings
        resolved.setdefault("project_root", str(root))
        # The renderer prints the unknown-VCS warning to stdout, which is the
        # same stream this oracle's JSON uses; capture it instead of emitting it
        # so the report stays readable (recorded in the JSON for evidence).
        render_stdout = io.StringIO()
        with contextlib.redirect_stdout(render_stdout):
            would_write = "\n".join(renderer._render_lines(manifest, resolved)).encode()
        out["render_stdout"] = render_stdout.getvalue()
        out["would_write_sha"] = hashlib.sha256(would_write).hexdigest()
        out["state"] = renderer.brief_effective_state(
            manifest_path, root / "AGENTS.md", would_write
        )

        # op_brief_dead_fragments: its own shorter chain.
        fragments_resolved = materialize.build_resolved_config(root)
        materialize.attach_brief_fragments_to_resolved(fragments_resolved, home)
        out["dead"] = bool(policy.has_dead_recipe_fragments(fragments_resolved))
    except Exception as exc:  # the bridge turns any exception into a degraded op
        out["error"] = f"{type(exc).__name__}: {exc}"

    print(json.dumps(out))
    return 0


if __name__ == "__main__":
    sys.exit(main())

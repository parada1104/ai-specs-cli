#!/usr/bin/env python3
"""Generate ai-specs/.envrc.example from enabled recipes' [[provides.mcp]] env refs.

Never writes .envrc (user-owned, gitignored). Existing .envrc.example is backed
up to .envrc.example.bak before overwrite.
"""
from __future__ import annotations

import argparse
import importlib.util
import os
import sys
from pathlib import Path


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_toml_read = _load_sibling("toml-read")
_recipe_read = _load_sibling("recipe-read")
_recipe_init = _load_sibling("recipe-init")
ENV_REFERENCE_RE = _recipe_init.ENV_REFERENCE_RE

_HEADER = """\
# ai-specs/.envrc.example — committed template (safe to regenerate).
# Copy to a project .envrc (gitignored) and fill in real values.
# Generated from enabled recipes' [[provides.mcp]] env references.
"""


def _catalog_dir() -> Path:
    home = os.environ.get("AI_SPECS_HOME")
    root = Path(home) if home else Path(__file__).resolve().parents[2]
    return root / "catalog" / "recipes"


def collect_env_vars(project_root: Path) -> dict[str, str]:
    """Collect $VAR references from enabled recipes' MCP env tables.

    Returns {VAR_NAME: purpose}. First declaration wins for purpose text.
    """
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest.is_file():
        return {}
    try:
        data = _toml_read.load_toml(manifest)
        recipes = _toml_read.read_recipes(data)
    except Exception:
        return {}

    catalog = _catalog_dir()
    collected: dict[str, str] = {}
    for recipe_id, entry in recipes.items():
        if not entry.get("enabled"):
            continue
        try:
            recipe = _recipe_read.read_recipe(catalog, recipe_id)
        except Exception:
            continue
        for preset in recipe.mcp:
            env = preset.config.get("env")
            if not isinstance(env, dict):
                continue
            for _key, value in env.items():
                if not isinstance(value, str):
                    continue
                match = ENV_REFERENCE_RE.match(value.strip())
                if not match:
                    continue
                var = match.group(1)
                purpose = f"required by {preset.id} ({recipe_id})"
                if var not in collected:
                    collected[var] = purpose
                else:
                    # Append additional source without clobbering first purpose.
                    if recipe_id not in collected[var]:
                        collected[var] = f"{collected[var]}; also {preset.id} ({recipe_id})"
    return collected


def generate_envrc_example(project_root: Path) -> Path:
    """Write ai-specs/.envrc.example. Never writes .envrc."""
    ai_specs = project_root / "ai-specs"
    ai_specs.mkdir(parents=True, exist_ok=True)
    target = ai_specs / ".envrc.example"
    if target.is_file():
        backup = ai_specs / ".envrc.example.bak"
        backup.write_text(target.read_text(encoding="utf-8"), encoding="utf-8")

    vars_map = collect_env_vars(project_root)
    lines = [_HEADER.rstrip(), ""]
    if vars_map:
        for var in sorted(vars_map):
            lines.append(f'export {var}=""  # {vars_map[var]}')
    else:
        lines.append("# (no env vars required by enabled recipes)")
    lines.append("")
    target.write_text("\n".join(lines), encoding="utf-8")
    return target


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Generate ai-specs/.envrc.example")
    parser.add_argument("path", nargs="?", default=".", help="Project root")
    args = parser.parse_args(argv)
    root = Path(args.path).resolve()
    if not (root / "ai-specs" / "ai-specs.toml").is_file():
        print(f"Proyecto no inicializado: missing ai-specs/ai-specs.toml under {root}", file=sys.stderr)
        return 1
    path = generate_envrc_example(root)
    print(f"Wrote {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

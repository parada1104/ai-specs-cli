#!/usr/bin/env python3
"""Parse and validate recipe.toml files from the catalog.

Usage:
  recipe-read.py <catalog_dir> <recipe_id>

Output: JSON representation of the recipe to stdout.
Exit 1 on missing recipe or validation error.
"""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


def _load_recipe_schema():
    module_path = Path(__file__).with_name("recipe_schema.py")
    spec = importlib.util.spec_from_file_location("recipe_schema", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load recipe_schema.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_recipe_schema = _load_recipe_schema()
Recipe = _recipe_schema.Recipe
RecipeValidationError = _recipe_schema.RecipeValidationError
load_recipe_toml = _recipe_schema.load_recipe_toml
CEREMONY_LEVELS = frozenset({"trivial", "local_fix", "behavior_change", "domain_change"})


def read_recipe(catalog_dir: Path, recipe_id: str) -> Recipe:
    recipe_dir = catalog_dir / recipe_id
    if not recipe_dir.is_dir():
        raise RecipeValidationError(f"recipe directory not found: {recipe_dir}")
    recipe = load_recipe_toml(recipe_dir / "recipe.toml")
    threshold = recipe.sdd.threshold
    if threshold and threshold not in CEREMONY_LEVELS:
        raise RecipeValidationError(
            f"recipe {recipe_id}: invalid sdd.threshold '{threshold}' (allowed: {', '.join(sorted(CEREMONY_LEVELS))})"
        )
    return recipe


def recipe_to_dict(recipe: Recipe) -> dict:
    init = None
    if recipe.init is not None:
        init = {
            "prompt": recipe.init.prompt,
            "description": recipe.init.description,
            "needs_manifest": recipe.init.needs_manifest,
            "needs_mcp": list(recipe.init.needs_mcp),
        }

    return {
        "id": recipe.id,
        "name": recipe.name,
        "description": recipe.description,
        "version": recipe.version,
        "author": recipe.author,
        "license": recipe.license,
        "sdd": {"threshold": recipe.sdd.threshold},
        "init": init,
        "provides": {
            "skills": [
                {"id": s.id, "source": s.source, "url": s.url, "path": s.path}
                for s in recipe.skills
            ],
            "commands": [
                {"id": c.id, "path": c.path}
                for c in recipe.commands
            ],
            "mcp": [
                {"id": m.id, **m.config}
                for m in recipe.mcp
            ],
            "templates": [
                {"source": t.source, "target": t.target, "condition": t.condition}
                for t in recipe.templates
            ],
            "docs": [
                {"source": d.source, "target": d.target}
                for d in recipe.docs
            ],
        },
    }


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <catalog_dir> <recipe_id>", file=sys.stderr)
        return 2

    catalog_dir = Path(sys.argv[1])
    recipe_id = sys.argv[2]

    try:
        recipe = read_recipe(catalog_dir, recipe_id)
    except RecipeValidationError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(json.dumps(recipe_to_dict(recipe)))
    return 0


if __name__ == "__main__":
    sys.exit(main())

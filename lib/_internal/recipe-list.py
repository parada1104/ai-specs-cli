#!/usr/bin/env python3
"""List available and installed recipes.

Usage:
  recipe-list.py <project_root>
"""

from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path
from typing import Any


def _load_toml_read():
    module_path = Path(__file__).with_name("toml-read.py")
    spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _load_recipe_read():
    module_path = Path(__file__).with_name("recipe-read.py")
    spec = importlib.util.spec_from_file_location("recipe_read_internal", module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _resolve_catalog_dir(project_root: Path) -> Path:
    ai_specs_home = os.environ.get("AI_SPECS_HOME")
    if ai_specs_home:
        home_catalog = Path(ai_specs_home) / "catalog" / "recipes"
        if home_catalog.is_dir():
            return home_catalog

    # Consumer projects declare recipes in ai-specs.toml; the recipe catalog is owned by the CLI.
    return Path(__file__).resolve().parents[2] / "catalog" / "recipes"


def list_recipes(project_root: Path) -> list[dict[str, str]]:
    """Scan catalog/recipes/ and compare against manifest [recipes.*]."""
    catalog_dir = _resolve_catalog_dir(project_root)
    manifest_path = project_root / "ai-specs" / "ai-specs.toml"

    toml_read = _load_toml_read()
    recipe_read = _load_recipe_read()

    # Read manifest recipes
    manifest_recipes: dict[str, dict[str, Any]] = {}
    if manifest_path.is_file():
        try:
            data = toml_read.load_toml(manifest_path)
            manifest_recipes = toml_read.read_recipes(data)
        except Exception:
            pass

    # Scan catalog
    results: list[dict[str, str]] = []
    if catalog_dir.is_dir():
        for entry in sorted(catalog_dir.iterdir()):
            if not entry.is_dir() or entry.name.startswith("."):
                continue
            recipe_id = entry.name
            try:
                recipe = recipe_read.read_recipe(catalog_dir, recipe_id)
                recipe_dict = recipe_read.recipe_to_dict(recipe)
                status = "available"
                if recipe_id in manifest_recipes:
                    status = "installed" if manifest_recipes[recipe_id].get("enabled") else "disabled"
                results.append({
                    "id": recipe_id,
                    "name": recipe_dict["name"],
                    "version": recipe_dict["version"],
                    "status": status,
                })
            except Exception as exc:
                results.append({
                    "id": recipe_id,
                    "name": "",
                    "version": "",
                    "status": f"error ({exc})",
                })

    return results


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <project_root>", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1])
    manifest_path = project_root / "ai-specs" / "ai-specs.toml"

    if not manifest_path.is_file():
        print("Proyecto no inicializado. Ejecuta: ai-specs init", file=sys.stderr)
        return 1

    recipes = list_recipes(project_root)
    if not recipes:
        print("No hay recipes disponibles.")
        return 0

    for r in recipes:
        status = r.get("status", "available")
        name = r.get("name", "")
        version = r.get("version", "")
        print(f"[{status:12s}]  {r['id']:24s}  {version:8s}  {name}")

    return 0


if __name__ == "__main__":
    sys.exit(main())

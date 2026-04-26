#!/usr/bin/env python3
"""Add a recipe to the manifest.

Usage:
  recipe-add.py <project_root> <recipe_id>
"""

from __future__ import annotations

import importlib.util
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


def add_recipe(project_root: Path, recipe_id: str) -> int:
    """Validate recipe exists and append [recipes.<id>] to ai-specs.toml."""
    manifest_path = project_root / "ai-specs" / "ai-specs.toml"
    catalog_dir = project_root / "catalog" / "recipes"

    if not manifest_path.is_file():
        print("Proyecto no inicializado. Ejecuta: ai-specs init", file=sys.stderr)
        return 1

    # Validate recipe exists in catalog
    recipe_read = _load_recipe_read()
    try:
        recipe = recipe_read.read_recipe(catalog_dir, recipe_id)
    except Exception as exc:
        print(f"Recipe '{recipe_id}' no encontrada en catalog/recipes/: {exc}", file=sys.stderr)
        return 1

    # Check if already in manifest
    toml_read = _load_toml_read()
    try:
        data = toml_read.load_toml(manifest_path)
        manifest_recipes = toml_read.read_recipes(data)
        if recipe_id in manifest_recipes:
            print(
                f"Recipe '{recipe_id}' ya está en el manifest. "
                "Usa ai-specs sync para materializar.",
                file=sys.stderr,
            )
            return 1
    except Exception:
        pass

    # Append to manifest
    recipe_dict = recipe_read.recipe_to_dict(recipe)
    version = recipe_dict["version"]

    section = f"\n[recipes.{recipe_id}]\nenabled = true\nversion = \"{version}\"\n"

    manifest_text = manifest_path.read_text(encoding="utf-8")
    if not manifest_text.endswith("\n"):
        manifest_text += "\n"
    manifest_text += section
    manifest_path.write_text(manifest_text, encoding="utf-8")

    print(f"Recipe '{recipe_id}' agregada al manifest.")
    print("Próximo sync materializará:")

    provides = recipe_dict.get("provides", {})
    skills = provides.get("skills", [])
    commands = provides.get("commands", [])
    mcp = provides.get("mcp", [])
    templates = provides.get("templates", [])
    docs = provides.get("docs", [])

    if skills:
        print(f"  - skills: {', '.join(s['id'] for s in skills)}")
    if commands:
        print(f"  - commands: {', '.join(c['id'] for c in commands)}")
    if mcp:
        print(f"  - mcp: {', '.join(m['id'] for m in mcp)}")
    if templates:
        for t in templates:
            print(f"  - template: {t['source']} → {t['target']}")
    if docs:
        for d in docs:
            print(f"  - doc: {d['source']} → {d['target']}")
    if not any([skills, commands, mcp, templates, docs]):
        print("  (ninguna primitive declarada)")

    return 0


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <project_root> <recipe_id>", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1])
    recipe_id = sys.argv[2]
    return add_recipe(project_root, recipe_id)


if __name__ == "__main__":
    sys.exit(main())

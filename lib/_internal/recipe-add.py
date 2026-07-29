#!/usr/bin/env python3
"""Add a recipe to the manifest.

Usage:
  recipe-add.py <project_root> <recipe_id>
"""

from __future__ import annotations

import importlib.util
import os
import sys
import tomllib
from pathlib import Path
from typing import Any



def _load_sibling(name: str):
    """Load a same-directory _internal module by absolute path."""
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module

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


def _load_toml_write():
    module_path = Path(__file__).with_name("toml_write.py")
    spec = importlib.util.spec_from_file_location("toml_write_internal", module_path)
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


def add_recipe(project_root: Path, recipe_id: str) -> int:
    """Validate recipe exists and append [recipes.<id>] to ai-specs.toml."""
    manifest_path = project_root / "ai-specs" / "ai-specs.toml"
    catalog_dir = _resolve_catalog_dir(project_root)

    if not manifest_path.is_file():
        print("Proyecto no inicializado. Ejecuta: ai-specs init", file=sys.stderr)
        return 1

    util = _load_sibling("util")
    if util.is_internal_test_recipe(recipe_id):
        print(util.internal_test_recipe_message(recipe_id), file=sys.stderr)
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
    section = f"\n[recipes.{recipe_id}]\nenabled = true\n"

    # Append config placeholders so the user knows what needs configuration
    if recipe.config_schema.fields:
        toml_write = _load_toml_write()
        section += f"\n[recipes.{recipe_id}.config]\n"
        for key in sorted(recipe.config_schema.fields):
            field = recipe.config_schema.fields[key]
            if field.required:
                section += f'{key} = ""  # REQUIRED\n'
            elif field.default is not None:
                section += f"{key} = {toml_write.toml_value(field.default)}\n"
            else:
                section += f'# {key} = ""  # optional\n'

    original_text = manifest_path.read_text(encoding="utf-8")
    manifest_text = original_text
    if not manifest_text.endswith("\n"):
        manifest_text += "\n"
    manifest_text += section
    manifest_path.write_text(manifest_text, encoding="utf-8")

    # Guard: the appended section must keep the manifest as valid TOML. If it
    # does not, restore the original so a malformed write never reaches sync.
    try:
        tomllib.loads(manifest_text)
    except tomllib.TOMLDecodeError as exc:
        manifest_path.write_text(original_text, encoding="utf-8")
        print(
            f"Error: agregar '{recipe_id}' produciría un manifest TOML inválido "
            f"({exc}). No se modificó ai-specs.toml.",
            file=sys.stderr,
        )
        return 1

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


    # Guidance: what to do next
    has_config = bool(recipe.config_schema.fields)
    has_mcp_env = any(
        bool((mcp.config or {}).get("env"))
        for mcp in (recipe.mcp or [])
    )

    # If interactive, run config wizard + env var prompts now
    tty = sys.stdin.isatty() and sys.stdout.isatty()
    if tty and (has_config or has_mcp_env):
        print()
        import questionary
        if not questionary.confirm("¿Configurar ahora?", default=True).ask():
            print("Podés configurar después con: ai-specs configure-recipes")
            return 0
        if has_config:
            try:
                cw = _load_sibling("config_wizard")
                manifest = project_root / "ai-specs" / "ai-specs.toml"
                cw.configure_selected_recipes(project_root, [recipe_id], manifest)
            except Exception:
                print("  ! no se pudo abrir el asistente de configuración")

        if has_mcp_env:
            try:
                env = _load_sibling("env_scaffold")
                if env.collect_env_vars(project_root):
                    env.offer_harness_env(project_root)
            except Exception:
                print("  ! no se pudieron configurar las variables de entorno")
    else:
        # Non-TTY: print guidance
        next_steps = []
        if has_config:
            next_steps.append("Configurar valores requeridos: ai-specs configure-recipes")
        if has_mcp_env:
            next_steps.append("Configurar variables de entorno MCP: ai-specs configure-recipes")
        if next_steps:
            print()
            print("Siguientes pasos:")
            for step in next_steps:
                print(f"  - {step}")
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

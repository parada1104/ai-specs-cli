#!/usr/bin/env python3
"""Orchestrate recipe materialization during ai-specs sync.

Usage:
  recipe-materialize.py <project_root> <ai_specs_home>

Reads [recipes.*] from ai-specs.toml, validates, detects conflicts,
materializes bundled assets, vendors dep skills, applies templates,
and writes ai-specs/.recipe-mcp.json for downstream mcp-render.py.

Exit 0 on success, 1 on validation/conflict error.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

import importlib.util

# Load toml-read helper
_toml_read_module = None

def _load_toml_read() -> Any:
    global _toml_read_module
    if _toml_read_module is None:
        module_path = Path(__file__).with_name("toml-read.py")
        spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load toml-read.py at {module_path}")
        _toml_read_module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(_toml_read_module)
    return _toml_read_module


def load_recipes_from_manifest(project_root: Path) -> dict[str, dict[str, Any]]:
    mod = _load_toml_read()
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    data = mod.load_toml(toml_path)
    return mod.read_recipes(data)


def fail(msg: str) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(1)


def warn(msg: str) -> None:
    print(f"  ! {msg}", file=sys.stderr)


# --- Load recipe schema helper ------------------------------------------------
_recipe_schema_module = None

def _load_recipe_schema() -> Any:
    global _recipe_schema_module
    if _recipe_schema_module is None:
        module_path = Path(__file__).with_name("recipe_schema.py")
        spec = importlib.util.spec_from_file_location("recipe_schema_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load recipe_schema.py at {module_path}")
        _recipe_schema_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _recipe_schema_module
        spec.loader.exec_module(_recipe_schema_module)
    return _recipe_schema_module


def read_recipe(catalog_dir: Path, recipe_id: str) -> Any:
    schema = _load_recipe_schema()
    recipe_dir = catalog_dir / recipe_id
    if not recipe_dir.is_dir():
        raise schema.RecipeValidationError(f"recipe directory not found: {recipe_dir}")
    return schema.load_recipe_toml(recipe_dir / "recipe.toml")


# --- Conflict detection -------------------------------------------------------
_conflict_module = None

def _load_conflict() -> Any:
    global _conflict_module
    if _conflict_module is None:
        module_path = Path(__file__).with_name("recipe-conflicts.py")
        spec = importlib.util.spec_from_file_location("recipe_conflicts_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load recipe-conflicts.py at {module_path}")
        _conflict_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _conflict_module
        spec.loader.exec_module(_conflict_module)
    return _conflict_module


def check_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any]:
    mod = _load_conflict()
    return mod.check_recipe_conflicts(catalog_dir, recipe_ids)


# --- Version pinning validation ----------------------------------------------
def validate_version_pin(recipe_id: str, manifest_version: str, recipe: Any) -> None:
    if manifest_version != recipe.version:
        raise RuntimeError(
            f"recipe '{recipe_id}' version mismatch: manifest pins "
            f"'{manifest_version}' but catalog has '{recipe.version}'"
        )


# --- Materialize helpers ------------------------------------------------------
def materialize_bundled_skill(recipe_dir: Path, skill_id: str, project_root: Path) -> None:
    src = recipe_dir / "skills" / skill_id
    dest = project_root / "ai-specs" / "skills" / skill_id
    if not src.is_dir():
        raise RuntimeError(f"bundled skill not found: {src}")
    if dest.exists():
        shutil.rmtree(dest)
    shutil.copytree(src, dest)
    print(f"    ✓ bundled skill {skill_id}")


def materialize_dep_skill(skill: Any, project_root: Path) -> None:
    # Reuse vendor-skills.py logic via import
    vendor_path = Path(__file__).with_name("vendor-skills.py")
    spec = importlib.util.spec_from_file_location("vendor_skills_internal", vendor_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load vendor-skills.py at {vendor_path}")
    vendor_mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vendor_mod)

    dep = {
        "id": skill.id,
        "source": skill.url,
    }
    if skill.path:
        dep["path"] = skill.path
    vendor_mod.sync_dep_target(dep, project_root)
    print(f"    ✓ dep skill {skill.id}")


def materialize_command(recipe_dir: Path, cmd: Any, project_root: Path) -> None:
    src = recipe_dir / cmd.path
    dest = project_root / "ai-specs" / "commands" / f"{cmd.id}.md"
    if not src.is_file():
        raise RuntimeError(f"command source not found: {src}")
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)
    print(f"    ✓ command {cmd.id}")


def materialize_template(recipe_dir: Path, tpl: Any, project_root: Path) -> None:
    src = recipe_dir / tpl.source
    dest = project_root / tpl.target
    if not src.is_file():
        raise RuntimeError(f"template source not found: {src}")
    if tpl.condition == "not_exists":
        if dest.exists():
            print(f"    · template skipped (exists) {tpl.target}")
            return
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)
    print(f"    ✓ template {tpl.target}")


def materialize_doc(recipe_dir: Path, doc: Any, project_root: Path) -> None:
    src = recipe_dir / doc.source
    dest = project_root / doc.target
    if not src.is_file():
        raise RuntimeError(f"doc source not found: {src}")
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)
    print(f"    ✓ doc {doc.target}")


# --- Recipe vs user-local skill warning --------------------------------------
def warn_user_local_conflicts(project_root: Path, recipe: Any) -> None:
    user_skills_dir = project_root / "ai-specs" / "skills"
    for skill in recipe.skills:
        user_skill = user_skills_dir / skill.id
        if user_skill.is_dir():
            warn(
                f"recipe '{recipe.name}' provides skill '{skill.id}' which already exists "
                f"in ai-specs/skills/. Recipe version takes precedence."
            )


# --- MCP merge ---------------------------------------------------------------
def build_recipe_mcp(catalog_dir: Path, recipe_ids: list[str], manifest_mcp: dict[str, Any]) -> dict[str, Any]:
    """Merge recipe MCP presets. Recipe values take precedence over manifest."""
    merged: dict[str, Any] = dict(manifest_mcp)
    for rid in recipe_ids:
        recipe = read_recipe(catalog_dir, rid)
        for mcp in recipe.mcp:
            if mcp.id in merged:
                warn(
                    f"recipe '{recipe.name}' overrides mcp.id='{mcp.id}' from project manifest"
                )
            merged[mcp.id] = mcp.config
    return merged


# --- Main ---------------------------------------------------------------------
def materialize_recipes(project_root: Path, ai_specs_home: Path) -> int:
    catalog_dir = ai_specs_home / "catalog" / "recipes"
    toml_path = project_root / "ai-specs" / "ai-specs.toml"

    recipes = load_recipes_from_manifest(project_root)
    enabled = {rid: cfg for rid, cfg in recipes.items() if cfg.get("enabled")}
    if not enabled:
        print("  (no [recipes.*] enabled — skipping)")
        return 0

    # Conflict detection across recipes
    conflicts = check_conflicts(catalog_dir, list(enabled.keys()))
    if conflicts:
        for c in conflicts:
            fail(
                f"recipe conflict: {c.primitive_type}.id='{c.primitive_id}' "
                f"claimed by {', '.join(sorted(c.recipes))}. "
                f"Resolve manually in ai-specs.toml."
            )
        return 1

    # Load manifest MCP for merge
    mod = _load_toml_read()
    manifest_data = mod.load_toml(toml_path)
    manifest_mcp = mod.read_mcp(manifest_data)

    recipe_mcp: dict[str, Any] = dict(manifest_mcp)

    for rid, cfg in enabled.items():
        print(f"  ▸ recipe {rid}")
        recipe = read_recipe(catalog_dir, rid)
        validate_version_pin(rid, cfg.get("version", ""), recipe)
        warn_user_local_conflicts(project_root, recipe)

        recipe_dir = catalog_dir / rid

        # Skills (bundled then deps)
        for skill in recipe.skills:
            if skill.source == "bundled":
                materialize_bundled_skill(recipe_dir, skill.id, project_root)
            elif skill.source == "dep":
                materialize_dep_skill(skill, project_root)
            else:
                raise RuntimeError(f"unknown skill source '{skill.source}' for skill '{skill.id}'")

        # Commands
        for cmd in recipe.commands:
            materialize_command(recipe_dir, cmd, project_root)

        # MCP presets (accumulate for merge)
        for mcp in recipe.mcp:
            if mcp.id in recipe_mcp:
                warn(
                    f"recipe '{recipe.name}' overrides mcp.id='{mcp.id}' from project manifest"
                )
            recipe_mcp[mcp.id] = mcp.config

        # Templates
        for tpl in recipe.templates:
            materialize_template(recipe_dir, tpl, project_root)

        # Docs
        for doc in recipe.docs:
            materialize_doc(recipe_dir, doc, project_root)

    # Write merged MCP for downstream mcp-render.py
    recipe_mcp_path = project_root / "ai-specs" / ".recipe-mcp.json"
    recipe_mcp_path.write_text(json.dumps(recipe_mcp, indent=2) + "\n")
    print(f"  ✓ wrote {recipe_mcp_path} ({len(recipe_mcp)} server(s))")

    return 0


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <project_root> <ai_specs_home>", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1]).resolve()
    ai_specs_home = Path(sys.argv[2]).resolve()

    try:
        return materialize_recipes(project_root, ai_specs_home)
    except Exception as exc:
        fail(str(exc))
        return 1


if __name__ == "__main__":
    sys.exit(main())

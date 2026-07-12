"""Minimal ai-specs project fixtures for eval scenarios."""

from __future__ import annotations

import re
from pathlib import Path


def recipe_version(catalog_root: Path, recipe_id: str) -> str:
    text = (catalog_root / "recipes" / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    if not match:
        raise ValueError(f"no version in {recipe_id}/recipe.toml")
    return match.group(1)


def write_manifest(
    root: Path,
    *,
    recipe_id: str,
    version: str,
    extra_recipes: str = "",
) -> Path:
    ai_specs = root / "ai-specs"
    ai_specs.mkdir(parents=True, exist_ok=True)
    (ai_specs / "skills").mkdir(exist_ok=True)
    (ai_specs / "commands").mkdir(exist_ok=True)
    manifest = ai_specs / "ai-specs.toml"
    manifest.write_text(
        "[project]\nname = 'eval-fixture'\n\n"
        "[agents]\nenabled = ['claude']\n\n"
        f'[recipes.{recipe_id}]\nenabled = true\nversion = "{version}"\n'
        + extra_recipes
    )
    return manifest

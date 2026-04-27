#!/usr/bin/env python3
"""Detect primitive ID collisions across recipes.

Usage (library):
  from recipe_conflicts import ConflictRegistry, ConflictError

Usage (CLI):
  recipe-conflicts.py <catalog_dir> <recipe_id>...

Exits 0 if no conflicts, 1 if conflicts found (prints JSON to stderr).
"""

from __future__ import annotations

import importlib.util
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


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


class ConflictError(Exception):
    def __init__(self, conflicts: list[Conflict]):
        self.conflicts = conflicts
        super().__init__(f"{len(conflicts)} conflict(s) detected")

    def to_dicts(self) -> list[dict[str, str]]:
        return [{"type": c.primitive_type, "id": c.primitive_id, "recipes": list(c.recipes)} for c in self.conflicts]


@dataclass
class Conflict:
    primitive_type: str
    primitive_id: str
    recipes: set[str] = field(default_factory=set)
    severity: str = "fatal"


class ConflictRegistry:
    """Tracks claimed primitive IDs across recipes and reports collisions."""

    def __init__(self) -> None:
        self._claimed: dict[str, dict[str, str]] = {}
        # _claimed[primitive_type][primitive_id] = recipe_name

    def claim(self, primitive_type: str, primitive_id: str, recipe_name: str) -> None:
        pt = self._claimed.setdefault(primitive_type, {})
        if primitive_id in pt:
            raise ConflictError([
                Conflict(
                    primitive_type=primitive_type,
                    primitive_id=primitive_id,
                    recipes={pt[primitive_id], recipe_name},
                )
            ])
        pt[primitive_id] = recipe_name

    def register_recipe(self, recipe: Recipe) -> None:
        for skill in recipe.skills:
            self.claim("skill", skill.id, recipe.name)
        for cmd in recipe.commands:
            self.claim("command", cmd.id, recipe.name)
        for mcp in recipe.mcp:
            self.claim("mcp", mcp.id, recipe.name)


def check_recipe_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Conflict]:
    """Load recipes and return any conflicts. Empty list means no conflicts."""
    registry = ConflictRegistry()
    all_conflicts: list[Conflict] = []
    for rid in recipe_ids:
        recipe_dir = catalog_dir / rid
        if not recipe_dir.is_dir():
            raise RecipeValidationError(f"recipe directory not found: {recipe_dir}")
        try:
            recipe = load_recipe_toml(recipe_dir / "recipe.toml")
        except RecipeValidationError:
            raise
        try:
            registry.register_recipe(recipe)
        except ConflictError as exc:
            all_conflicts.extend(exc.conflicts)
    return all_conflicts


def check_capability_conflicts(
    catalog_dir: Path, recipe_ids: list[str], explicit_bindings: list[dict[str, str]]
) -> list[Conflict]:
    """Detect capability ambiguity and duplicate explicit bindings.

    Returns conflicts with severity:
      - "warning" for auto-bind ambiguity (>1 provider, no explicit binding)
      - "fatal" for duplicate explicit bindings
    """
    enabled_ids = set(recipe_ids)
    # Load enabled recipes and their capabilities
    cap_providers: dict[str, set[str]] = {}
    for rid in recipe_ids:
        recipe_dir = catalog_dir / rid
        if not recipe_dir.is_dir():
            continue
        try:
            recipe = load_recipe_toml(recipe_dir / "recipe.toml")
        except RecipeValidationError:
            continue
        for cap in recipe.capabilities:
            cap_providers.setdefault(cap.id, set()).add(rid)

    # Track explicitly bound capabilities
    bound_caps: set[str] = set()
    bound_recipes: dict[str, str] = {}  # capability -> recipe
    for binding in explicit_bindings:
        cap = binding.get("capability", "")
        rec = binding.get("recipe", "")
        if cap in bound_caps:
            return [
                Conflict(
                    primitive_type="capability",
                    primitive_id=cap,
                    recipes={bound_recipes[cap], rec},
                    severity="fatal",
                )
            ]
        bound_caps.add(cap)
        bound_recipes[cap] = rec

    conflicts: list[Conflict] = []
    for cap_id, providers in cap_providers.items():
        if cap_id in bound_caps:
            continue
        if len(providers) > 1:
            conflicts.append(
                Conflict(
                    primitive_type="capability",
                    primitive_id=cap_id,
                    recipes=providers,
                    severity="warning",
                )
            )
    return conflicts


def main() -> int:
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <catalog_dir> <recipe_id>...", file=sys.stderr)
        return 2

    catalog_dir = Path(sys.argv[1])
    recipe_ids = sys.argv[2:]

    try:
        conflicts = check_recipe_conflicts(catalog_dir, recipe_ids)
    except RecipeValidationError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if conflicts:
        payload = [{"type": c.primitive_type, "id": c.primitive_id, "recipes": sorted(c.recipes)} for c in conflicts]
        print(json.dumps(payload), file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())

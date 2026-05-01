#!/usr/bin/env python3
"""Multi-source skill resolution with precedence rules.

Usage:
  skill-resolution.py <project_root> [--json]

Scans three skill sources:
  1. ai-specs/skills/{id}/          — local (highest precedence)
  2. .recipe/{recipe-id}/skills/{id}/ — recipe-bundled (middle)
  3. .deps/{dep-id}/skills/{id}/      — vendored dependency (lowest)

Returns a dict mapping skill_id -> (source_type, abs_path).
When the same skill ID appears in multiple recipes or deps within the same
tier, first-seen wins and a warning is emitted to stderr.

If --json is passed, prints JSON to stdout.
Otherwise, prints a human-readable table.
"""

from __future__ import annotations

import json
import sys
import tomllib
from pathlib import Path
from typing import Optional


def warn(msg: str) -> None:
    print(f"  ! {msg}", file=sys.stderr)


def _scan_local_skills(project_root: Path) -> dict[str, Path]:
    skills_dir = project_root / "ai-specs" / "skills"
    result: dict[str, Path] = {}
    if not skills_dir.is_dir():
        return result
    for child in sorted(skills_dir.iterdir()):
        if child.is_dir() and (child / "SKILL.md").is_file():
            result[child.name] = child
    return result


def _scan_recipe_skills(project_root: Path) -> dict[str, Path]:
    recipe_dir = project_root / "ai-specs" / ".recipe"
    result: dict[str, Path] = {}
    if not recipe_dir.is_dir():
        return result
    for recipe_child in sorted(recipe_dir.iterdir()):
        if not recipe_child.is_dir():
            continue
        skills_dir = recipe_child / "skills"
        if not skills_dir.is_dir():
            continue
        for skill_child in sorted(skills_dir.iterdir()):
            if skill_child.is_dir() and (skill_child / "SKILL.md").is_file():
                skill_id = skill_child.name
                if skill_id in result:
                    warn(
                        f"skill '{skill_id}' found in multiple recipes; "
                        f"using first-seen from '{result[skill_id].parents[1].name}'"
                    )
                else:
                    result[skill_id] = skill_child
    return result


def _scan_dep_skills(project_root: Path) -> dict[str, Path]:
    deps_dir = project_root / "ai-specs" / ".deps"
    result: dict[str, Path] = {}
    if not deps_dir.is_dir():
        return result
    for dep_child in sorted(deps_dir.iterdir()):
        if not dep_child.is_dir():
            continue
        skills_dir = dep_child / "skills"
        if not skills_dir.is_dir():
            continue
        for skill_child in sorted(skills_dir.iterdir()):
            if skill_child.is_dir() and (skill_child / "SKILL.md").is_file():
                skill_id = skill_child.name
                if skill_id in result:
                    warn(
                        f"skill '{skill_id}' found in multiple deps; "
                        f"using first-seen from '{result[skill_id].parents[1].name}'"
                    )
                else:
                    result[skill_id] = skill_child
    return result


def _get_recipe_id_for_skill(skill_path: Path, project_root: Path) -> str | None:
    """Infer recipe-id from a recipe skill path like .recipe/{id}/skills/{skill}/"""
    try:
        rel = skill_path.relative_to(project_root / "ai-specs" / ".recipe")
        parts = rel.parts
        if len(parts) >= 3 and parts[1] == "skills":
            return parts[0]
    except ValueError:
        pass
    return None


def load_skill_config(project_root: Path, skill_id: str, bundled_config: dict | None = None) -> dict:
    """Load config for a skill, merging recipe overrides if applicable.

    Returns a dict with override values taking precedence over bundled defaults.
    Missing override files are handled gracefully (no error, no warning).
    """
    resolved = collect_skills(project_root)
    if skill_id not in resolved:
        raise RuntimeError(f"required skill '{skill_id}' not found in any source")

    source_type, skill_path = resolved[skill_id]
    result = dict(bundled_config or {})

    if source_type == "recipe":
        recipe_id = _get_recipe_id_for_skill(skill_path, project_root)
        if recipe_id:
            override_path = project_root / "ai-specs" / ".recipe" / recipe_id / "overrides" / "config.toml"
            if override_path.is_file():
                with override_path.open("rb") as f:
                    overrides = tomllib.load(f)
                if isinstance(overrides, dict):
                    result.update(overrides)

    return result


def resolve_skill_template(project_root: Path, skill_id: str, template_name: str) -> Path | None:
    """Resolve a template path for a skill, preferring recipe overrides.

    If the skill comes from a recipe and an identically-named template exists in
    .recipe/{recipe-id}/overrides/templates/, that override is returned.
    Otherwise the bundled template in the skill directory is returned.
    If neither exists, returns None.
    """
    resolved = collect_skills(project_root)
    if skill_id not in resolved:
        raise RuntimeError(f"required skill '{skill_id}' not found in any source")

    source_type, skill_path = resolved[skill_id]

    if source_type == "recipe":
        recipe_id = _get_recipe_id_for_skill(skill_path, project_root)
        if recipe_id:
            override_template = project_root / "ai-specs" / ".recipe" / recipe_id / "overrides" / "templates" / template_name
            if override_template.is_file():
                return override_template

    bundled_template = skill_path / template_name
    if bundled_template.is_file():
        return bundled_template

    return None


def collect_skills(project_root: Path) -> dict[str, tuple[str, Path]]:
    """Resolve skills across three sources with precedence.

    Returns {skill_id: (source_type, abs_path)} where source_type is one of
    'local', 'recipe', or 'dep'.
    """
    resolved: dict[str, tuple[str, Path]] = {}

    # Tier 1: local (highest precedence)
    for skill_id, path in _scan_local_skills(project_root).items():
        resolved[skill_id] = ("local", path)

    # Tier 2: recipe (middle precedence)
    for skill_id, path in _scan_recipe_skills(project_root).items():
        if skill_id not in resolved:
            resolved[skill_id] = ("recipe", path)

    # Tier 3: dep (lowest precedence)
    for skill_id, path in _scan_dep_skills(project_root).items():
        if skill_id not in resolved:
            resolved[skill_id] = ("dep", path)

    return resolved


def resolve_skill(project_root: Path, skill_id: str) -> tuple[str, Path]:
    """Resolve a single skill ID. Raises RuntimeError if not found."""
    resolved = collect_skills(project_root)
    if skill_id not in resolved:
        raise RuntimeError(f"required skill '{skill_id}' not found in any source")
    return resolved[skill_id]


def main() -> int:
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <project_root> [--json]", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1]).resolve()
    use_json = "--json" in sys.argv

    resolved = collect_skills(project_root)

    if use_json:
        # JSON needs serializable values — convert Path to str
        payload = {k: {"source": v[0], "path": str(v[1])} for k, v in resolved.items()}
        print(json.dumps(payload, indent=2))
    else:
        print(f"Resolved {len(resolved)} skill(s) from {project_root}")
        for skill_id, (source, path) in sorted(resolved.items()):
            print(f"  {source:7}  {skill_id:<30}  {path}")

    return 0


if __name__ == "__main__":
    sys.exit(main())

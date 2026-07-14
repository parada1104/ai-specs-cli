"""Minimal ai-specs project fixtures for eval scenarios."""

from __future__ import annotations

import re
import shutil
from pathlib import Path


RUNTIME_SKILL_DIRS = {
    "claude": ".claude/skills",
    "opencode": ".opencode/skills",
    "cursor": ".cursor/skills",
    "pi": ".pi/skills",
    "omp": ".pi/skills",  # omp shares pi skill discovery
}


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
    agents: list[str] | None = None,
) -> Path:
    ai_specs = root / "ai-specs"
    ai_specs.mkdir(parents=True, exist_ok=True)
    (ai_specs / "skills").mkdir(exist_ok=True)
    (ai_specs / "commands").mkdir(exist_ok=True)
    enabled = agents or ["claude"]
    agents_lit = "[" + ", ".join(f"'{a}'" for a in enabled) + "]"
    manifest = ai_specs / "ai-specs.toml"
    manifest.write_text(
        "[project]\nname = 'eval-fixture'\n\n"
        f"[agents]\nenabled = {agents_lit}\n\n"
        f'[recipes.{recipe_id}]\nenabled = true\nversion = "{version}"\n'
        + extra_recipes
    )
    return manifest


def seed_project_files(root: Path) -> None:
    """Seed a tiny app so 'implement X' prompts have a concrete target."""
    (root / "pyproject.toml").write_text(
        '[project]\nname = "eval-app"\nversion = "0.0.1"\n'
    )
    src = root / "src" / "forms"
    src.mkdir(parents=True, exist_ok=True)
    (src / "signup.py").write_text(
        '"""Signup form handler (intentionally no validation)."""\n\n'
        "def signup(email: str, password: str) -> dict:\n"
        "    return {'email': email, 'password': password}\n"
    )


def setup_runtime_skills(
    root: Path,
    runtime: str,
    recipe_id: str,
    *,
    catalog_root: Path | None = None,
) -> Path:
    """Copy recipe SKILL.md into the runtime discovery path."""
    catalog = catalog_root or (Path(__file__).resolve().parents[3] / "catalog")
    src = (
        catalog
        / "recipes"
        / recipe_id
        / "skills"
        / recipe_id
        / "SKILL.md"
    )
    if not src.is_file():
        # Fall back to materialized path
        src = (
            root
            / "ai-specs"
            / ".recipe"
            / recipe_id
            / "skills"
            / recipe_id
            / "SKILL.md"
        )
    if not src.is_file():
        raise FileNotFoundError(f"skill not found for {recipe_id}: tried catalog + materialize")

    rel = RUNTIME_SKILL_DIRS.get(runtime)
    if not rel:
        raise ValueError(f"no skill dir mapping for runtime {runtime}")

    dest_dir = root / rel / recipe_id
    dest_dir.mkdir(parents=True, exist_ok=True)
    dest = dest_dir / "SKILL.md"
    shutil.copy2(src, dest)

    # Minimal brief so the agent sees workflow rules when AGENTS.md is loaded.
    agents = root / "AGENTS.md"
    if not agents.exists():
        agents.write_text(
            "# Eval fixture\n\n"
            f"Enabled recipe: `{recipe_id}`.\n\n"
            "## Workflow\n\n"
            "- Classify change depth (full/standard/light) before production edits.\n"
            "- Write planning artifacts, present the plan, wait for authorization.\n"
            "- Do not implement production code during planning.\n"
        )
    return dest

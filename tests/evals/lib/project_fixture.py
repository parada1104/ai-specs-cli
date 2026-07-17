"""Minimal ai-specs project fixtures for eval scenarios."""

from __future__ import annotations

import re
import shutil
from pathlib import Path


RUNTIME_SKILL_DIRS = {
    "claude": ".claude/skills",
    "opencode": ".opencode/skills",
    "cursor": ".cursor/skills",
    "cursor-agent": ".cursor/skills",
    "pi": ".pi/skills",
    "omp": ".omp/skills",
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


def resolve_recipe_skill(
    recipe_id: str,
    *,
    catalog_root: Path | None = None,
    project_root: Path | None = None,
) -> tuple[Path, str]:
    """Return (SKILL.md path, skill_id). skill_id may differ from recipe_id (VCS)."""
    catalog = catalog_root or (Path(__file__).resolve().parents[3] / "catalog")
    catalog_skills = catalog / "recipes" / recipe_id / "skills"
    preferred = catalog_skills / recipe_id / "SKILL.md"
    if preferred.is_file():
        return preferred, recipe_id
    bundled = sorted(catalog_skills.glob("*/SKILL.md"))
    if bundled:
        return bundled[0], bundled[0].parent.name
    if project_root is not None:
        mat_skills = project_root / "ai-specs" / ".recipe" / recipe_id / "skills"
        preferred_m = mat_skills / recipe_id / "SKILL.md"
        if preferred_m.is_file():
            return preferred_m, recipe_id
        mat_bundled = sorted(mat_skills.glob("*/SKILL.md"))
        if mat_bundled:
            return mat_bundled[0], mat_bundled[0].parent.name
    raise FileNotFoundError(
        f"skill not found for {recipe_id}: tried catalog + materialize"
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
    src, skill_id = resolve_recipe_skill(
        recipe_id, catalog_root=catalog, project_root=root
    )

    rel = RUNTIME_SKILL_DIRS.get(runtime)
    if not rel:
        raise ValueError(f"no skill dir mapping for runtime {runtime}")

    dest_dir = root / rel / skill_id
    dest_dir.mkdir(parents=True, exist_ok=True)
    dest = dest_dir / "SKILL.md"
    shutil.copy2(src, dest)

    # Minimal brief so the agent sees workflow rules when AGENTS.md is loaded.
    agents = root / "AGENTS.md"
    if not agents.exists():
        agents.write_text(
            "# Eval fixture\n\n"
            f"Enabled recipe: `{recipe_id}` (skill `{skill_id}`).\n\n"
            "## Workflow\n\n"
            "- Classify change depth (full/standard/light) before production edits.\n"
            "- Write planning artifacts, present the plan, wait for authorization.\n"
            "- Do not implement production code during planning.\n"
            "- Follow the bound VCS merge-workflow skill for PR/MR merge and cleanup.\n"
        )
    return dest


def seed_authorized_plan(
    root: Path,
    *,
    slug: str = "signup-validation",
    tier: str = "standard",
) -> Path:
    """Seed a reviewable plan folder so build/archive scenarios start authorized."""
    change = root / "openspec" / "changes" / slug
    change.mkdir(parents=True, exist_ok=True)
    (change / "tasks.md").write_text(
        f"# Tasks: {slug}\n\n"
        f"Depth: {tier}\n\n"
        "## Intent\n\n"
        "Add email/password validation to src/forms/signup.py.\n\n"
        "## Tasks\n\n"
        "- [ ] Reject empty email and password\n"
        "- [ ] Reject emails without '@'\n"
        "- [ ] Keep return shape as a dict on success\n"
    )
    if tier in {"standard", "full"}:
        spec = change / "specs" / "signup-validation" / "spec.md"
        spec.parent.mkdir(parents=True, exist_ok=True)
        spec.write_text(
            "# Spec: signup validation\n\n"
            "## Requirement: Validate signup inputs\n\n"
            "signup() SHALL reject empty email/password and emails without '@'.\n"
        )
    if tier == "full":
        (change / "design.md").write_text("# Design\n\nInline validation in signup().\n")
    return change

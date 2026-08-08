"""Minimal ai-specs project fixtures for eval scenarios."""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from pathlib import Path


RUNTIME_SKILL_DIRS = {
    "claude": ".claude/skills",
    "opencode": ".opencode/skills",
    "cursor": ".cursor/skills",
    "cursor-agent": ".cursor/skills",
    "pi": ".pi/skills",
    "omp": ".omp/skills",
}

RUNTIME_COMMAND_DIRS = {
    "claude": ".claude/commands",
    "opencode": ".opencode/commands",
    "cursor": ".cursor/commands",
    "cursor-agent": ".cursor/commands",
    "pi": ".pi/commands",
    "omp": ".omp/commands",
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

def setup_bundled_skills(
    root: Path,
    runtime: str,
    names: list[str] | tuple[str, ...],
    *,
    bundled_root: Path | None = None,
) -> list[Path]:
    """Copy CLI-bundled literacy skills into a runtime discovery directory.

    This is additive to ``setup_runtime_skills``: catalog recipe skills and
    always-on bundled skills have different source trees.
    """
    source_root = bundled_root or (Path(__file__).resolve().parents[3] / "bundled-skills")
    rel = RUNTIME_SKILL_DIRS.get(runtime)
    if not rel:
        raise ValueError(f"no skill dir mapping for runtime {runtime}")
    copied: list[Path] = []
    for name in sorted(names):
        source = source_root / name / "SKILL.md"
        if not source.is_file():
            raise FileNotFoundError(f"bundled skill not found: {source}")
        dest_dir = root / rel / name
        dest_dir.mkdir(parents=True, exist_ok=True)
        dest = dest_dir / "SKILL.md"
        shutil.copy2(source, dest)
        copied.append(dest)
    return copied


def setup_runtime_commands(
    root: Path,
    runtime: str,
    recipe_id: str,
    *,
    catalog_root: Path | None = None,
) -> list[Path]:
    """Copy recipe [[provides.commands]] .md files into the runtime discovery
    path, mirroring the basename-preserving copy `sync-agent.sh` performs
    (`cp "$src" "$dest/$(basename "$src")"`). Without this, a live eval agent
    never sees the recipe's actual slash commands (e.g. `/worktree-new`) —
    only whatever a SKILL.md happens to describe in prose."""
    catalog = catalog_root or (Path(__file__).resolve().parents[3] / "catalog")
    commands_src_dir = catalog / "recipes" / recipe_id / "commands"
    if not commands_src_dir.is_dir():
        return []

    rel = RUNTIME_COMMAND_DIRS.get(runtime)
    if not rel:
        raise ValueError(f"no command dir mapping for runtime {runtime}")

    dest_dir = root / rel
    dest_dir.mkdir(parents=True, exist_ok=True)
    copied: list[Path] = []
    for src in sorted(commands_src_dir.glob("*.md")):
        dest = dest_dir / src.name
        shutil.copy2(src, dest)
        copied.append(dest)
    return copied


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

def seed_monorepo_apps(
    root: Path,
    apps: tuple[str, ...] = ("admin-dashboard", "api"),
) -> None:
    """Seed an ``apps/<name>/`` tree (monorepo-apps shape; no ``.gitmodules``)."""
    for name in apps:
        app_dir = root / "apps" / name
        app_dir.mkdir(parents=True, exist_ok=True)
        (app_dir / "README.md").write_text(f"{name}\n")


def _git(repo: Path, *args: str) -> str:
    env = dict(os.environ)
    env.update(
        {
            "GIT_AUTHOR_NAME": "eval",
            "GIT_AUTHOR_EMAIL": "eval@ai-specs.local",
            "GIT_COMMITTER_NAME": "eval",
            "GIT_COMMITTER_EMAIL": "eval@ai-specs.local",
        }
    )
    return subprocess.run(
        ["git", *args],
        cwd=repo,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    ).stdout


def _make_bare_submodule_source(sources_dir: Path, *, label: str) -> Path:
    """Create a bare repo suitable as a local ``git submodule add`` URL."""
    sources_dir.mkdir(parents=True, exist_ok=True)
    src = sources_dir / f"{label}-src"
    if src.exists():
        shutil.rmtree(src)
    src.mkdir()
    _git(src, "init", "-q", "-b", "main")
    (src / "README.md").write_text(f"{label}\n")
    _git(src, "add", "-A")
    _git(src, "commit", "-qm", f"init {label}")
    bare = sources_dir / f"{label}.git"
    if bare.exists():
        shutil.rmtree(bare)
    subprocess.run(
        ["git", "clone", "-q", "--bare", str(src), str(bare)],
        check=True,
        capture_output=True,
        text=True,
    )
    return bare


def add_initialized_submodule(
    repo_root: Path,
    *,
    path: str,
    name: str | None = None,
    label: str | None = None,
    sources_dir: Path | None = None,
) -> Path:
    """Add a real initialized submodule via a local file-path remote (no network).

    Satisfies ``detect_submodules`` in ``lib/_internal/util.py``:
    - ``.gitmodules`` is present and registers ``path``
    - ``git submodule status`` reports the entry with a non-``-`` prefix
      (``' '`` / ``'+'`` / ``'U'`` count as initialized; ``'-'`` does not)

    Returns the submodule working-tree path under ``repo_root``.
    """
    if not (repo_root / ".git").exists():
        raise ValueError(f"not a git repo: {repo_root}")

    slug = label or Path(path).name.replace("/", "-")
    remotes = sources_dir or (repo_root / "_eval_submodule_remotes")
    bare = _make_bare_submodule_source(remotes, label=slug)

    # Keep fixture remotes out of the project index when they live under root.
    gi = repo_root / ".gitignore"
    ignore_line = "_eval_submodule_remotes/"
    existing = gi.read_text() if gi.is_file() else ""
    if ignore_line not in existing.splitlines():
        suffix = "" if not existing or existing.endswith("\n") else "\n"
        gi.write_text(existing + suffix + ignore_line + "\n")
        try:
            _git(repo_root, "add", ".gitignore")
            _git(repo_root, "commit", "-qm", "ignore eval submodule remotes")
        except subprocess.CalledProcessError:
            pass

    add_args = ["-c", "protocol.file.allow=always", "submodule", "add"]
    if name is not None:
        add_args.extend(["--name", name])
    add_args.extend([str(bare), path])
    _git(repo_root, *add_args)
    _git(repo_root, "commit", "-qm", f"add submodule {path}")

    # Confirm initialized (non-'-') prefix — exact contract detect_submodules uses.
    status = subprocess.run(
        ["git", "-C", str(repo_root), "submodule", "status", "--", path],
        capture_output=True,
        text=True,
        check=False,
    )
    line = next((ln for ln in status.stdout.splitlines() if ln.strip()), "")
    if not line or line[0] == "-":
        raise RuntimeError(
            f"submodule {path!r} not initialized after add; status={status.stdout!r} "
            f"stderr={status.stderr!r}"
        )
    return repo_root / path


def add_initialized_submodules(
    repo_root: Path,
    modules: list[dict[str, str]],
    *,
    sources_dir: Path | None = None,
) -> list[Path]:
    """Add multiple initialized submodules. Each item: ``path`` + optional ``name``/``label``."""
    remotes = sources_dir or (repo_root / "_eval_submodule_remotes")
    out: list[Path] = []
    for mod in modules:
        out.append(
            add_initialized_submodule(
                repo_root,
                path=mod["path"],
                name=mod.get("name"),
                label=mod.get("label") or Path(mod["path"]).name,
                sources_dir=remotes,
            )
        )
    return out

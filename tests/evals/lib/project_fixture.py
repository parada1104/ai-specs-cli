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

# Execution modes the plan-build recipe resolves from its manifest config.
MODES = ("odd", "openspec")
# The branch and base ref a seeded fixture repo works on, exposed to the runner:
# required_git_config substitutes <branch>, required_commit_trailer needs a range.
# The branch's last segment is the change ODD pairs the plan with, so it matches
# the default slug `signup-validation`.
FIXTURE_BRANCH = "feat/signup-validation"
FIXTURE_BASE_REF = "development"


def recipe_version(catalog_root: Path, recipe_id: str) -> str:
    text = (catalog_root / "recipes" / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    if not match:
        raise ValueError(f"no version in {recipe_id}/recipe.toml")
    return match.group(1)


def _with_mode(extra_recipes: str, mode: str) -> str:
    """Insert the recipe's `mode` config without defining its table twice."""
    header = "[recipes.plan-build-flow.config]"
    line = f'mode = "{mode}"'
    if header not in extra_recipes:
        return f"\n{header}\n{line}\n" + extra_recipes
    return extra_recipes.replace(header, f"{header}\n{line}", 1)


def write_manifest(
    root: Path,
    *,
    recipe_id: str,
    version: str,
    extra_recipes: str = "",
    agents: list[str] | None = None,
    mode: str = "openspec",
) -> Path:
    if mode not in MODES:
        raise ValueError(f"mode must be one of {MODES}: {mode!r}")
    ai_specs = root / "ai-specs"
    ai_specs.mkdir(parents=True, exist_ok=True)
    (ai_specs / "skills").mkdir(exist_ok=True)
    (ai_specs / "commands").mkdir(exist_ok=True)
    enabled = agents or ["claude"]
    agents_lit = "[" + ", ".join(f"'{a}'" for a in enabled) + "]"
    if recipe_id == "plan-build-flow":
        extra_recipes = _with_mode(extra_recipes, mode)
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
            "- The change tier is derived from the artifacts the branch carries "
            "(none / light / standard): never declared and never estimated from size.\n"
            "- none: no plan, no delta, no change folder; light: a plan alone, or a "
            "delta alone; standard: a plan and a delta.\n"
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
    mode: str = "openspec",
) -> Path:
    """Seed an authorized plan so build/archive scenarios start from a valid one.

    The plan carries the graded grammar (`## Specs` / `## Tasks` / `## Verification`
    / `## Log`) and no declared tier: the tier is derived from these artifacts.
    `tier` is kept for callers and only decides whether a delta is seeded (light).
    """
    if mode not in MODES:
        raise ValueError(f"mode must be one of {MODES}: {mode!r}")
    if mode == "odd":
        change = root / "odd" / "changes" / slug
        plan_path = root / "odd" / "tasks" / f"{slug}.md"
    else:
        change = root / "openspec" / "changes" / slug
        plan_path = change / "tasks.md"
    request = (
        "Necesito implementar validación de input en el formulario de signup "
        "(src/forms/signup.py)."
    )
    plan_path.parent.mkdir(parents=True, exist_ok=True)
    plan_path.write_text(
        f"# {slug}\n\n"
        f"Plan and delta for email/password validation in `src/forms/signup.py`.\n\n"
        "## Specs\n\n"
        f'- **S1.** Reject an empty email or password — *"{request}"*\n'
        f'- **S2.** Reject an email without `@` — *"{request}"*\n\n'
        "## Tasks\n\n"
        "| ID | Spec | Tarea | Ruta | Commit |\n"
        "|----|------|-------|------|--------|\n"
        "| T1 | S1, S2 | Validate signup inputs and test the rejection paths "
        "| worker | pending |\n\n"
        "## Verification\n\n"
        "- **S1: PASS** — `./src/forms/signup.py`, exit 0\n"
        "- **S2: PASS** — `./src/forms/signup.py`, exit 0\n\n"
        "## Log\n\n"
        f"- **L1** — {request}\n"
    )
    if tier != "light":
        spec = change / "specs" / "forms" / "spec.md"
        spec.parent.mkdir(parents=True, exist_ok=True)
        spec.write_text(
            "## ADDED Requirements\n\n"
            "### Requirement: Validate signup inputs\n\n"
            "signup() SHALL reject an empty email or password and an email without `@`.\n"
        )
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


def seed_fixture_branch(
    root: Path,
    *,
    branch: str = FIXTURE_BRANCH,
    base_ref: str = FIXTURE_BASE_REF,
) -> tuple[str, str]:
    """Put a committed fixture repo on `branch` over `base_ref`; return both.

    Call after the baseline commit: `base_ref` is created at that commit and the
    working branch checks out over it, so a scenario's commits form
    `<base_ref>..HEAD` for `required_commit_trailer` and `branch.<branch>.*`
    reads work for `required_git_config`.
    """
    _git(root, "branch", "-f", base_ref, "HEAD")
    _git(root, "checkout", "-q", "-B", branch)
    return branch, base_ref


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

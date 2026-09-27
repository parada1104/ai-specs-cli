"""Black-box external-dirs / cache-layout / skill-resolution tests.

Every test drives ``bin/ai-specs`` verbs (init / sync / sync-agent /
refresh-bundled / doctor) through the process boundary via ``_blackbox``.
No test may import ``lib/_internal`` modules. Assertions preserve the
original contract intents (cache dir layout, external dir placement, skill
resolution precedence, warnings, leftovers migration) through the CLI.

Internal-only surfaces with no CLI-observable equivalent are kept as
process-boundary probes of the isolated home's own lib copy, each marked
with a distinct ``# TRIAGE:`` comment.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    cache_project_dir,
    invoke,
    isolated_home,
    populate_catalog,
)

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize/doctor derive cache and catalog roots from their own
    realpath, so a symlinked lib would resolve back into the repository and
    let the CLI touch repo cache state. A real copy keeps every lookup and
    write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    """One shared isolated install root per command sequence (memoized by
    the project's parent dir, which is unique per temp workspace)."""
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = _make_home(base)
    return _HOMES[key]


def _new_project(name: str = "prj") -> tuple[tempfile.TemporaryDirectory, Path]:
    tmp = tempfile.TemporaryDirectory(prefix="ai-specs-ext-")
    root = Path(tmp.name) / name
    root.mkdir(parents=True)
    return tmp, root


def _write_manifest(root: Path, sections: str = "") -> None:
    (root / "ai-specs" / "ai-specs.toml").write_text(
        "[project]\nname = 'ext-dirs-fixture'\n\n"
        "[agents]\nenabled = ['cursor']\n\n" + sections
    )


def _make_dep_repo(tmp: Path, name: str, body: str | None = None) -> Path:
    """Local git repo shipping one SKILL.md (offline vendor source)."""
    repo = tmp if name == "." else tmp / name
    repo.mkdir(parents=True, exist_ok=True)
    (repo / "SKILL.md").write_text(
        body if body is not None else f"# {name}\n"
    )
    subprocess.run(["git", "init", "-q", str(repo)], check=True, text=True,
                   capture_output=True, input="")
    subprocess.run(["git", "-C", str(repo), "config", "user.name", "Fixture"],
                   check=True, text=True, capture_output=True, input="")
    subprocess.run(["git", "-C", str(repo), "config", "user.email", "f@example.com"],
                   check=True, text=True, capture_output=True, input="")
    subprocess.run(["git", "-C", str(repo), "add", "."], check=True, text=True,
                   capture_output=True, input="")
    subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "init"],
                   check=True, text=True, capture_output=True, input="")
    return repo


def _recipe_toml(
    recipe_id: str,
    *,
    skills: tuple[str, ...] = (),
    dep_skills: tuple[tuple[str, str], ...] = (),
    commands: tuple[str, ...] = (),
) -> str:
    lines = [
        "[recipe]",
        f'id = "{recipe_id}"',
        f'name = "{recipe_id}"',
        'description = "external-dirs fixture recipe"',
        'version = "1.0.0"',
    ]
    provides = []
    if skills or dep_skills:
        entries = [f'{{ id = "{s}", source = "bundled" }}' for s in skills]
        entries += [
            f'{{ id = "{s}", source = "dep", url = "{u}" }}'
            for s, u in dep_skills
        ]
        provides.append("skills = [\n    " + ",\n    ".join(entries) + ",\n]")
    if commands:
        entries = [f'{{ id = "{c}", path = "commands/{c}.md" }}' for c in commands]
        provides.append("commands = [\n    " + ",\n    ".join(entries) + ",\n]")
    if provides:
        lines.append("[provides]")
        lines.extend(provides)
    return "\n".join(lines) + "\n"


def _seed_recipe(
    home: Path,
    recipe_id: str,
    *,
    skills: tuple[str, ...] = (),
    dep_skills: tuple[tuple[str, str], ...] = (),
    commands: tuple[str, ...] = (),
    files: dict[str, str] | None = None,
) -> Path:
    """Seed a fresh-unique recipe into the isolated home's catalog.

    Fresh ids only: an existing repo recipe id would write through the
    catalog symlink into the repository.
    """
    rdir = populate_catalog(home, recipe_id, toml=_recipe_toml(
        recipe_id, skills=skills, dep_skills=dep_skills, commands=commands))
    for rel, content in (files or {}).items():
        path = rdir / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
    return rdir


def _recipe_cache_skill(project: Path, home: Path, recipe: str, skill: str) -> Path:
    return cache_project_dir(project, home) / ".recipe" / recipe / "skills" / skill


def _cache_dep_skill(project: Path, home: Path, dep: str) -> Path:
    return cache_project_dir(project, home) / ".deps" / dep / "skills" / dep


def _inproject_dep_skill(project: Path, dep: str) -> Path:
    return project / "ai-specs" / ".deps" / dep / "skills" / dep


def _cache_command(project: Path, home: Path, cmd: str) -> Path:
    return cache_project_dir(project, home) / "commands" / f"{cmd}.md"


def _resolved_skill(project: Path, home: Path, skill: str) -> Path:
    return cache_project_dir(project, home) / "resolved-skills" / skill / "SKILL.md"


def _skill_resolution_probe(project: Path, home: Path, calls: str) -> subprocess.CompletedProcess:
    """Process-boundary probe used only by # TRIAGE tests below.

    Runs the isolated home's OWN ``skill-resolution.py`` copy in a hermetic
    subprocess; ``calls`` is python code that must print JSON to stdout.
    stdin is closed (input='') so the probe can never block.
    """
    sr = home / "lib" / "_internal" / "skill-resolution.py"
    script = (
        "import json, sys\n"
        "from pathlib import Path\n"
        f"src = open({str(sr)!r}).read()\n"
        "mod = type(sys)('skill_resolution_probe')\n"
        f"mod.__file__ = {str(sr)!r}\n"
        "exec(compile(src, mod.__file__, 'exec'), mod.__dict__)\n"
        f"project = Path({str(project)!r})\n"
        f"home = Path({str(home)!r})\n"
        + calls + "\n"
    )
    with tempfile.TemporaryDirectory(prefix="ai-specs-probe-") as tmp:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(Path(tmp) / "home"),
            "TMPDIR": tmp,
            "AI_SPECS_HOME": str(home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        (Path(tmp) / "home").mkdir()
        return subprocess.run(
            [sys.executable, "-"], input=script, env=env, text=True,
            capture_output=True, check=False,
        )


def _git_project() -> tuple[tempfile.TemporaryDirectory, Path]:
    tmp = tempfile.TemporaryDirectory(prefix="ai-specs-ext-")
    root = Path(tmp.name) / "prj"
    root.mkdir(parents=True)
    subprocess.run(["git", "init", "-q", str(root)], check=True, text=True,
                   capture_output=True, input="")
    subprocess.run(["git", "-C", str(root), "config", "user.email", "t@example.com"],
                   check=True, text=True, capture_output=True, input="")
    subprocess.run(["git", "-C", str(root), "config", "user.name", "t"],
                   check=True, text=True, capture_output=True, input="")
    return tmp, root


class InitExternalDirsTests(unittest.TestCase):
    def test_init_does_not_create_in_project_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "init", "--no-tui", cli_home=_home_for(target))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse((target / "ai-specs" / ".recipe").exists())
            self.assertFalse((target / "ai-specs" / ".deps").exists())

    def test_init_idempotent_without_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            home = _home_for(target)
            result = invoke(target, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(target, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue((target / "ai-specs" / "ai-specs.toml").is_file())
            self.assertFalse((target / "ai-specs" / ".recipe").exists())

    def test_gitignore_omits_in_project_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "init", "--no-tui", cli_home=_home_for(target))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            gitignore = (target / ".gitignore").read_text()
            self.assertNotIn("ai-specs/.recipe/", gitignore)
            self.assertNotIn("ai-specs/.deps/", gitignore)

    def test_gitignore_ignores_recipes_except_overrides(self):
        """recipes/ is CLI-owned; only declared overrides are committed."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run(["git", "init", "-q", str(target)], check=True, text=True,
                           capture_output=True, input="")
            result = invoke(target, "init", "--no-tui", cli_home=_home_for(target))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            ai_specs = target / "ai-specs"
            # A bundled recipe doc should be ignored; a declared override committed.
            (ai_specs / "recipes" / "demo").mkdir(parents=True)
            (ai_specs / "recipes" / "demo" / "README.md").write_text("bundled doc\n")
            (ai_specs / "recipes" / "demo" / "overrides").mkdir()
            (ai_specs / "recipes" / "demo" / "overrides" / "config.toml").write_text("x = 1\n")
            (ai_specs / ".deps" / "mydep").mkdir(parents=True)
            (ai_specs / ".deps" / "mydep" / "SKILL.md").write_text("# mydep\n")

            def ignored(rel: str) -> bool:
                r = subprocess.run(
                    ["git", "check-ignore", "-q", rel],
                    cwd=target, capture_output=True, input="",
                )
                return r.returncode == 0

            self.assertTrue(ignored("ai-specs/recipes/demo/README.md"))
            self.assertFalse(ignored("ai-specs/recipes/demo/overrides/config.toml"))
            self.assertTrue(ignored("ai-specs/.deps/mydep/SKILL.md"))

    def test_gitignore_idempotent_no_origin_entries(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            home = _home_for(target)
            result = invoke(target, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(target, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            gitignore = (target / ".gitignore").read_text()
            lines = [ln.strip() for ln in gitignore.splitlines()]
            self.assertEqual(lines.count("ai-specs/.recipe/"), 0)
            self.assertEqual(lines.count("ai-specs/.deps/"), 0)

    def test_gitignore_committable_relocated_recipe_templates(self):
        """Relocated conditional templates/bin under overrides/ are committable."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run(["git", "init", "-q", str(target)], check=True, text=True,
                           capture_output=True, input="")
            result = invoke(target, "init", "--no-tui", cli_home=_home_for(target))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            ai_specs = target / "ai-specs"

            trello_ovr = (
                ai_specs / "recipes" / "trello-mcp-workflow" / "overrides" / "templates"
            )
            trello_ovr.mkdir(parents=True)
            (trello_ovr / "card-feature.md").write_text("# feature\n")

            wt_ovr = ai_specs / "recipes" / "worktree-flow" / "overrides" / "bin"
            wt_ovr.mkdir(parents=True)
            (wt_ovr / "worktree-cleanup.sh").write_text("#!/bin/sh\n")

            trello_bare = ai_specs / "recipes" / "trello-mcp-workflow" / "templates"
            trello_bare.mkdir(parents=True)
            (trello_bare / "card-feature.md").write_text("# bare\n")

            wt_bare = ai_specs / "recipes" / "worktree-flow" / "bin"
            wt_bare.mkdir(parents=True)
            (wt_bare / "worktree-cleanup.sh").write_text("#!/bin/sh\n")

            def ignored(rel: str) -> bool:
                r = subprocess.run(
                    ["git", "check-ignore", "-q", rel],
                    cwd=target, capture_output=True, input="",
                )
                return r.returncode == 0

            self.assertFalse(
                ignored(
                    "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md"
                )
            )
            self.assertFalse(
                ignored(
                    "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
                )
            )
            self.assertTrue(
                ignored(
                    "ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md"
                )
            )
            self.assertTrue(
                ignored("ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh")
            )


class CatalogConditionalTemplateTargetLintTests(unittest.TestCase):
    """Catalog-only guard: not_exists templates under ai-specs/recipes/ use overrides/."""

    EXPECTED_TARGETS = {
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-bug.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-spike.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-epic.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-handoff.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-decision.md",
        "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh",
    }

    def test_not_exists_recipe_template_targets_use_overrides(self):
        found = []
        for recipe_toml in sorted(CATALOG.glob("*/recipe.toml")):
            recipe_id = recipe_toml.parent.name
            if recipe_id == "test-fixture" or recipe_id.startswith("test-"):
                continue
            text = recipe_toml.read_text()
            parts = re.split(r"\n\[\[provides\.templates\]\]\n", text)
            for part in parts[1:]:
                block = re.split(r"\n\[\[", part, maxsplit=1)[0]
                cond_m = re.search(r'(?m)^condition\s*=\s*"([^"]+)"\s*$', block)
                tgt_m = re.search(r'(?m)^target\s*=\s*"([^"]+)"\s*$', block)
                if not tgt_m:
                    continue
                target = tgt_m.group(1)
                condition = cond_m.group(1) if cond_m else None
                if condition != "not_exists":
                    continue
                if not target.startswith("ai-specs/recipes/"):
                    continue
                found.append(target)
                self.assertIn(
                    "/overrides/",
                    target,
                    f"{recipe_id}: not_exists target must nest under overrides/: {target}",
                )

        self.assertEqual(set(found), self.EXPECTED_TARGETS)


class VendorSkillsPathTests(unittest.TestCase):
    """toml-declared [[deps]] vendor in-project (ai-specs/.deps/), never into
    ai-specs/skills/ nor the CLI cache — observed through `ai-specs sync`."""

    def _project_with_dep(self, tmp: Path) -> Path:
        project = tmp / "project"
        project.mkdir()
        dep_repo = _make_dep_repo(tmp, "my-dep")
        result = invoke(project, "init", "--no-tui", cli_home=_home_for(project))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        _write_manifest(
            project,
            "[[deps]]\n"
            'id = "my-dep"\n'
            f'source = "{dep_repo.as_posix()}"\n',
        )
        return project

    def test_vendor_writes_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = self._project_with_dep(tmp_path)
            home = _home_for(project)
            result = invoke(project, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # toml-deps ([[deps]]) are project-governed → in-project ai-specs/.deps/
            skill = _inproject_dep_skill(project, "my-dep") / "SKILL.md"
            self.assertTrue(skill.is_file())
            self.assertIn("name: my-dep", skill.read_text())
            # and NOT staged under the CLI cache
            self.assertFalse((_cache_dep_skill(project, home, "my-dep") / "SKILL.md").is_file())

    def test_vendor_does_not_write_to_ai_specs_skills(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = self._project_with_dep(tmp_path)
            home = _home_for(project)
            result = invoke(project, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse((project / "ai-specs" / "skills" / "my-dep").exists())


class RecipeMaterializePathTests(unittest.TestCase):
    """Recipe materialization lands in the per-project cache — observed
    through `ai-specs sync` with a fresh-unique recipe in the isolated home."""

    def _project_with_recipe(self, home: Path, recipe_id: str, **kwargs) -> Path:
        _seed_recipe(home, recipe_id, **kwargs)
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        _write_manifest(
            root,
            f"[recipes.{recipe_id}]\nenabled = true\nversion = \"1.0.0\"\n",
        )
        return root

    def test_materializes_bundled_skill_to_recipe_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            root = self._project_with_recipe(
                home, "ext-mat-skill", skills=("ext-skill-a",),
                files={"skills/ext-skill-a/SKILL.md": "# ext-skill-a\n"},
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            skill_dir = _recipe_cache_skill(root, home, "ext-mat-skill", "ext-skill-a")
            self.assertTrue(skill_dir.is_dir())
            self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command_to_cache(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            root = self._project_with_recipe(
                home, "ext-mat-cmd", commands=("ext-command-a",),
                files={"commands/ext-command-a.md": "# ext-command-a\n"},
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            cmd = _cache_command(root, home, "ext-command-a")
            self.assertTrue(cmd.is_file())

    def test_warns_when_recipe_command_overwrites_existing_managed_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            root = self._project_with_recipe(
                home, "ext-mat-cmd-ow", commands=("ext-command-ow",),
                files={"commands/ext-command-ow.md": "# new managed\n"},
            )
            # Pre-seed the managed cache copy the recipe is about to replace.
            cmd = _cache_command(root, home, "ext-command-ow")
            cmd.parent.mkdir(parents=True, exist_ok=True)
            cmd.write_text("# previous managed\n")

            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("overwrites existing managed command", result.stderr)
            self.assertNotEqual(cmd.read_text(), "# previous managed\n")

    def test_materializes_recipe_dep_skill_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            home = _make_home(tmp_path / "home-base")
            dep_repo = _make_dep_repo(tmp_path, "dep-skill")
            root = self._project_with_recipe(
                home, "ext-mat-dep",
                dep_skills=(("dep-skill", dep_repo.as_posix()),),
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            dep_skill = _cache_dep_skill(root, home, "dep-skill") / "SKILL.md"
            self.assertTrue(dep_skill.is_file())
            self.assertFalse((root / "ai-specs" / "skills" / "dep-skill").exists())

    def test_local_skills_untouched_by_materialization(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            root = self._project_with_recipe(
                home, "ext-mat-local", skills=("ext-skill-local",),
                files={"skills/ext-skill-local/SKILL.md": "# ext-skill-local\n"},
            )
            local_skill = root / "ai-specs" / "skills" / "local-only"
            local_skill.mkdir(parents=True)
            (local_skill / "SKILL.md").write_text("local")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual((local_skill / "SKILL.md").read_text(), "local")


class BundledLeftoverCleanupTests(unittest.TestCase):
    """`ai-specs sync` deletes materialized bundled-skill copies from the
    project surface, but never genuine local skills or customized copies."""

    def _project(self) -> tuple[Path, Path]:
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return root, home

    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root, home = self._project()
        skills = root / "ai-specs" / "skills"
        bundled_src = home / "bundled-skills"

        # 1. Materialized bundled copy (byte-identical to CLI source) → remove.
        leftover = skills / "harness-lifecycle"
        leftover.mkdir()
        (leftover / "SKILL.md").write_text(
            (bundled_src / "harness-lifecycle" / "SKILL.md").read_text()
        )

        # 2. Genuine local skill (no bundled counterpart) → keep.
        local = skills / "my-local-skill"
        local.mkdir()
        (local / "SKILL.md").write_text("# my-local-skill\n")

        # 3. Customized copy of a bundled skill (content differs) → keep + warn.
        customized = skills / "skill-creator"
        customized.mkdir()
        (customized / "SKILL.md").write_text("# skill-creator (locally edited)\n")

        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local skill must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        """Migration: a copy from an older CLI (differs from current source) but
        recorded untouched in the legacy lock is safe to remove."""
        root, home = self._project()
        old = root / "ai-specs" / "skills" / "skill-creator"
        old.mkdir(parents=True)
        old_content = "# skill-creator (older CLI version, untouched)\n"
        (old / "SKILL.md").write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            f'[skills."skill-creator"]\n"SKILL.md" = "{h}"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root, home = self._project()
        edited = root / "ai-specs" / "skills" / "skill-creator"
        edited.mkdir(parents=True)
        (edited / "SKILL.md").write_text("# genuinely edited by the user\n")
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            '[skills."skill-creator"]\n"SKILL.md" = "0000000000000000"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")


class BundledCommandLeftoverCleanupTests(unittest.TestCase):
    """`ai-specs sync` deletes materialized bundled-command copies from the
    project surface, but never genuine local commands or customized copies."""

    def _project(self) -> tuple[Path, Path]:
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return root, home

    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root, home = self._project()
        commands = root / "ai-specs" / "commands"
        bundled_src = home / "bundled-commands"

        # 1. Materialized bundled copy (byte-identical to CLI source) → remove.
        leftover = commands / "rules-audit.md"
        leftover.write_text((bundled_src / "rules-audit.md").read_text())

        # 2. Genuine local command (no bundled counterpart) → keep.
        local = commands / "my-local-command.md"
        local.write_text("# my-local-command\n")

        # 3. Customized copy of a bundled command (content differs) → keep + warn.
        customized = commands / "skills-as-rules.md"
        customized.write_text("# skills-as-rules (locally edited)\n")

        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local command must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        """Migration: a copy from an older CLI (differs from current source) but
        recorded untouched in the legacy lock is safe to remove."""
        root, home = self._project()
        old = root / "ai-specs" / "commands" / "rules-audit.md"
        old_content = "# rules-audit (older CLI version, untouched)\n"
        old.write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            f'[commands]\n"rules-audit.md" = "{h}"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root, home = self._project()
        edited = root / "ai-specs" / "commands" / "rules-audit.md"
        edited.write_text("# genuinely edited by the user\n")
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            '[commands]\n"rules-audit.md" = "0000000000000000"\n'
        )
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")

    def test_no_bundled_counterpart_is_untouched(self):
        root, home = self._project()
        only_local = root / "ai-specs" / "commands" / "totally-local.md"
        only_local.write_text("# no bundled counterpart\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(only_local.exists())


class RecipeCommandLeftoverCleanupTests(unittest.TestCase):
    """Recipe-managed command copies migrate out of ai-specs/commands safely
    (observed through `ai-specs sync` / `ai-specs refresh-bundled`)."""

    def _project_with_recipe_command(
        self, home: Path, recipe_id: str, cmd_id: str, cmd_content: str,
    ) -> Path:
        _seed_recipe(
            home, recipe_id, commands=(cmd_id,),
            files={f"commands/{cmd_id}.md": cmd_content},
        )
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        _write_manifest(
            root,
            f"[recipes.{recipe_id}]\nenabled = true\nversion = \"1.0.0\"\n",
        )
        return root

    def test_removes_untouched_recipe_copy_and_merge_stays_silent(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            content = "# recipe command\n"
            root = self._project_with_recipe_command(
                home, "ext-cmd-clean", "pr-create", content,
            )
            local = root / "ai-specs" / "commands" / "pr-create.md"
            local.write_text(content)

            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            self.assertFalse(local.exists())
            self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)
            # The cache now carries the recipe-managed copy.
            self.assertEqual(_cache_command(root, home, "pr-create").read_text(), content)

    def test_preserves_customized_recipe_copy_with_local_warning(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            root = self._project_with_recipe_command(
                home, "ext-cmd-custom", "pr-create", "# recipe command\n",
            )
            local = root / "ai-specs" / "commands" / "pr-create.md"
            local.write_text("# customized locally\n")

            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            self.assertTrue(local.exists())
            self.assertIn("local/customized", result.stderr)

    def test_refresh_migrates_cached_recipe_copy(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            result = invoke(root, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            managed = _cache_command(root, home, "pr-create")
            managed.parent.mkdir(parents=True, exist_ok=True)
            content = "# recipe-managed command\n"
            managed.write_text(content)
            local = root / "ai-specs" / "commands" / "pr-create.md"
            local.write_text(content)

            result = invoke(root, "refresh-bundled", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(local.exists())
            self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)

    def test_sync_migrates_first_recipe_copy_before_cache_exists(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            result = invoke(root, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            _write_manifest(
                root,
                "[recipes.tdd-flow]\nenabled = true\n\n"
                "[recipes.tdd-flow.config]\ntest_command = \"python3 -m unittest\"\n",
            )
            local = root / "ai-specs" / "commands" / "tdd.md"
            content = (ROOT / "catalog" / "recipes" / "tdd-flow" / "commands" / "tdd.md").read_text()
            local.write_text(content)
            managed = _cache_command(root, home, "tdd")
            self.assertFalse(managed.exists())

            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(local.exists())
            self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)
            self.assertEqual(managed.read_text(), content)

    def test_removes_untouched_recipe_copy_via_legacy_lock_hash(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            result = invoke(root, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            local = root / "ai-specs" / "commands" / "pr-create.md"
            content = "# older recipe command\n"
            local.write_text(content)
            digest = hashlib.sha256(content.encode()).hexdigest()
            (root / "ai-specs" / ".ai-specs.lock").write_text(
                f'[commands]\n"pr-create.md" = "{digest}"\n'
            )
            # No managed cache copy exists (fresh cache) — same shape as the
            # original test, which removed the managed dir before migrating.

            result = invoke(root, "refresh-bundled", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(local.exists())


class TrackedBundledCommandLeftoverTests(unittest.TestCase):
    """`ai-specs doctor` WARNs when git still tracks bundled commands whose
    working-tree copy is gone; doctor never mutates the index."""

    def _doctor_lines(self, root: Path, home: Path) -> list[str]:
        result = invoke(root, "doctor", cli_home=home)
        lines = [
            ln for ln in result.stdout.splitlines()
            if "tracked-bundled-leftover" in ln
        ]
        return lines

    def test_finds_tracked_command_with_missing_working_tree_copy(self):
        tmp, root = _git_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# leftover\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True,
                       capture_output=True, input="")
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"],
                       check=True, capture_output=True, input="")
        (commands / "rules-audit.md").unlink()

        lines = self._doctor_lines(root, home)
        self.assertTrue(lines, "doctor must report tracked-bundled-leftover")
        self.assertTrue(
            any("rules-audit" in ln for ln in lines),
            f"tracked-bundled-leftover must name rules-audit: {lines}",
        )

    def test_empty_when_working_tree_copy_exists(self):
        tmp, root = _git_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# still here\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True,
                       capture_output=True, input="")
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"],
                       check=True, capture_output=True, input="")

        self.assertEqual(self._doctor_lines(root, home), [])

    def test_empty_when_not_a_git_work_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "prj"
            (root / "ai-specs" / "commands").mkdir(parents=True)
            home = _home_for(root)
            self.assertEqual(self._doctor_lines(root, home), [])


class SkillResolutionTests(unittest.TestCase):
    """Multi-source skill resolution precedence, observed through the CLI:
    `ai-specs sync` / `ai-specs sync-agent --all` flatten the resolved skill
    set into the cache ``resolved-skills/`` tree, and tier warnings surface
    on stderr."""

    def _project(self) -> tuple[tempfile.TemporaryDirectory, Path, Path]:
        tmp, root = _new_project()
        home = _home_for(root)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return tmp, root, home

    def _write_local_skill(self, root: Path, name: str, body: str) -> None:
        d = root / "ai-specs" / "skills" / name
        d.mkdir(parents=True, exist_ok=True)
        (d / "SKILL.md").write_text(body)

    def _recipe_project(
        self, home: Path, recipe_id: str, skill: str, body: str,
        root: Path | None = None,
    ) -> Path:
        """Seed a fresh-unique recipe providing `skill` and enable it."""
        _seed_recipe(
            home, recipe_id, skills=(skill,),
            files={f"skills/{skill}/SKILL.md": body},
        )
        if root is None:
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            result = invoke(root, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        _write_manifest(
            root,
            f"[recipes.{recipe_id}]\nenabled = true\nversion = \"1.0.0\"\n",
        )
        return root

    def _dep_project(
        self, root: Path, tmp: Path, dep_id: str, body: str,
        sections: str = "",
    ) -> Path:
        repo = _make_dep_repo(tmp, f"repo-{dep_id}", body=body)
        _write_manifest(
            root,
            sections
            + "[[deps]]\n"
            f'id = "{dep_id}"\n'
            f'source = "{repo.as_posix()}"\n',
        )
        return root

    def test_bundled_fallback_when_no_other_source(self):
        tmp, root, home = self._project()
        with tmp:
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "harness-lifecycle")
            self.assertTrue(resolved.is_file())
            self.assertEqual(
                resolved.read_text(),
                (home / "bundled-skills" / "harness-lifecycle" / "SKILL.md").read_text(),
                "bundled content must win when no local/recipe/dep source exists",
            )

    def test_dep_precedence_over_bundled(self):
        tmp, root, home = self._project()
        with tmp:
            self._dep_project(root, Path(tmp.name), "harness-lifecycle", "# dep harness\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "harness-lifecycle").read_text()
            vendored = _inproject_dep_skill(root, "harness-lifecycle") / "SKILL.md"
            self.assertEqual(resolved, vendored.read_text())
            self.assertNotEqual(
                resolved,
                (home / "bundled-skills" / "harness-lifecycle" / "SKILL.md").read_text(),
                "dep tier must beat the bundled tier",
            )

    def test_local_precedence_over_bundled(self):
        tmp, root, home = self._project()
        with tmp:
            self._write_local_skill(root, "skill-creator", "# local skill-creator\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "skill-creator").read_text()
            self.assertEqual(resolved, "# local skill-creator\n")
            self.assertNotEqual(
                resolved,
                (home / "bundled-skills" / "skill-creator" / "SKILL.md").read_text(),
                "local tier must beat the bundled tier",
            )

    def test_local_precedence_over_recipe(self):
        tmp, root, home = self._project()
        with tmp:
            root = self._recipe_project(
                home, "ext-res-local-recipe", "shared", "# recipe shared\n", root=root,
            )
            self._write_local_skill(root, "shared", "# local shared\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "shared").read_text()
            self.assertEqual(resolved, "# local shared\n")
            self.assertNotIn("# recipe shared", resolved)

    def test_recipe_precedence_over_dep(self):
        tmp, root, home = self._project()
        with tmp:
            root = self._recipe_project(
                home, "ext-res-recipe-dep", "shared", "# recipe shared\n", root=root,
            )
            root = self._dep_project(
                root, Path(tmp.name), "shared", "# dep shared\n",
                sections="[recipes.ext-res-recipe-dep]\nenabled = true\nversion = \"1.0.0\"\n\n",
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "shared").read_text()
            self.assertIn("# recipe shared", resolved)
            self.assertNotIn("# dep shared", resolved)

    def test_local_precedence_over_all(self):
        tmp, root, home = self._project()
        with tmp:
            root = self._recipe_project(
                home, "ext-res-local-all", "shared", "# recipe shared\n", root=root,
            )
            root = self._dep_project(
                root, Path(tmp.name), "shared", "# dep shared\n",
                sections="[recipes.ext-res-local-all]\nenabled = true\nversion = \"1.0.0\"\n\n",
            )
            self._write_local_skill(root, "shared", "# local shared\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "shared").read_text()
            self.assertEqual(resolved, "# local shared\n")
            self.assertNotIn("# recipe shared", resolved)
            self.assertNotIn("# dep shared", resolved)

    def test_dep_fallback_when_no_other_source(self):
        tmp, root, home = self._project()
        with tmp:
            self._dep_project(root, Path(tmp.name), "only-dep", "# only-dep body\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "only-dep")
            self.assertTrue(resolved.is_file())
            self.assertEqual(
                resolved.read_text(),
                (_inproject_dep_skill(root, "only-dep") / "SKILL.md").read_text(),
            )

    def test_inproject_toml_dep_resolves_as_dep(self):
        tmp, root, home = self._project()
        with tmp:
            self._dep_project(root, Path(tmp.name), "only-toml-dep", "# only-toml-dep\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            vendored = _inproject_dep_skill(root, "only-toml-dep") / "SKILL.md"
            self.assertTrue(vendored.is_file())
            resolved = _resolved_skill(root, home, "only-toml-dep")
            self.assertEqual(resolved.read_text(), vendored.read_text())

    def test_first_seen_recipe_wins_with_warning(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            result = invoke(root, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # Stage the recipe tier directly in the per-project cache: two
            # recipe dirs claiming the same skill id. A real `sync` refuses
            # this state (primitive conflict), so resolution is exercised
            # through `sync-agent`, which only reads the staged tiers.
            for recipe_id, body in (
                ("ext-res-dup-a", "# dup from ext-res-dup-a\n"),
                ("ext-res-dup-b", "# dup from ext-res-dup-b\n"),
            ):
                d = _recipe_cache_skill(root, home, recipe_id, "dup")
                d.mkdir(parents=True)
                (d / "SKILL.md").write_text(body)

            # sync-agent runs recipe materialize unless --recipe-mcp is
            # passed; an empty preset file keeps the staged tiers untouched.
            mcp_file = root.parent / "empty-recipe-mcp.json"
            mcp_file.write_text("{}")
            result = invoke(root, "sync-agent", "--all", "--recipe-mcp", str(mcp_file),
                            cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("dup", result.stderr)
            self.assertIn("ext-res-dup-a", result.stderr)
            resolved = _resolved_skill(root, home, "dup").read_text()
            self.assertEqual(resolved, "# dup from ext-res-dup-a\n")

    def test_first_seen_dep_wins_with_warning(self):
        tmp, root, home = self._project()
        with tmp:
            repos = {
                "d1": _make_dep_repo(Path(tmp.name), "repo-d1", body="# d1 body\n"),
                "d2": _make_dep_repo(Path(tmp.name), "repo-d2", body="# d2 body\n"),
            }
            _write_manifest(
                root,
                "[[deps]]\n" f'id = "d1"\n' f'source = "{repos["d1"].as_posix()}"\n\n'
                "[[deps]]\n" f'id = "d2"\n' f'source = "{repos["d2"].as_posix()}"\n',
            )
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            # Stage the same skill id inside two dep dirs (dep skill ids are
            # dep ids when vendored, so the duplicate-id state is staged).
            for dep_id, body in (("d1", "# dup from d1\n"), ("d2", "# dup from d2\n")):
                dup = root / "ai-specs" / ".deps" / dep_id / "skills" / "dup"
                dup.mkdir(parents=True)
                (dup / "SKILL.md").write_text(body)

            mcp_file = root.parent / "empty-recipe-mcp.json"
            mcp_file.write_text("{}")
            result = invoke(root, "sync-agent", "--all", "--recipe-mcp", str(mcp_file),
                            cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("dup", result.stderr)
            self.assertIn("d1", result.stderr)
            resolved = _resolved_skill(root, home, "dup").read_text()
            self.assertEqual(resolved, "# dup from d1\n")

    def test_missing_skill_raises(self):
        # TRIAGE: ai-specs sync — single-skill resolution (resolve_skill) has
        # no CLI surface; the failure contract is probed against the isolated
        # home's own skill-resolution.py copy in a subprocess.
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            probe = _skill_resolution_probe(root, home, (
                "try:\n"
                "    mod.resolve_skill(project, 'missing', cli_home=home)\n"
                "    print(json.dumps({'error': None}))\n"
                "except RuntimeError as exc:\n"
                "    print(json.dumps({'error': str(exc)}))\n"
            ))
            self.assertEqual(probe.returncode, 0, probe.stderr)
            payload = json.loads(probe.stdout)
            self.assertIsNotNone(payload["error"])
            self.assertIn("missing", payload["error"])

    def test_local_override_silent_no_warning(self):
        tmp, root, home = self._project()
        with tmp:
            root = self._recipe_project(
                home, "ext-res-silent", "shared", "# recipe shared\n", root=root,
            )
            self._write_local_skill(root, "shared", "# local shared\n")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            resolved = _resolved_skill(root, home, "shared").read_text()
            self.assertEqual(resolved, "# local shared\n")
            # No warning should be emitted for local override
            self.assertNotIn("found in multiple", result.stderr)
            self.assertNotIn("using first-seen", result.stderr)

    def test_local_precedence_does_not_backfill_files_from_recipe(self):
        # TRIAGE: ai-specs sync — resolve_skill_template has no CLI surface;
        # the no-backfill contract is probed against the isolated home's own
        # skill-resolution.py copy in a subprocess.
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            tmp2, root = _new_project()
            self.addCleanup(tmp2.cleanup)
            # Local skill wins; recipe tier holds an asset the local skill lacks.
            local = root / "ai-specs" / "skills" / "shared"
            local.mkdir(parents=True)
            (local / "SKILL.md").write_text("# local shared")
            staged = _recipe_cache_skill(root, home, "ext-res-backfill", "shared")
            staged.mkdir(parents=True)
            (staged / "SKILL.md").write_text("# recipe shared")
            (staged / "assets").mkdir()
            (staged / "assets" / "helper.md").write_text("recipe asset")
            probe = _skill_resolution_probe(root, home, (
                "p = mod.resolve_skill_template(project, 'shared', 'assets/helper.md', cli_home=home)\n"
                "print(json.dumps(str(p) if p else None))\n"
            ))
            self.assertEqual(probe.returncode, 0, probe.stderr)
            self.assertIsNone(json.loads(probe.stdout))


class OverrideLoadingTests(unittest.TestCase):
    """Recipe-skill override loading (config.toml merge, template override).

    # TRIAGE: ai-specs sync — load_skill_config / resolve_skill_template have
    # no CLI surface (no verb or rendered artifact consumes them); each test
    # probes the isolated home's own skill-resolution.py copy in a hermetic
    # subprocess and asserts on its JSON output.
    """

    def _project(self) -> tuple[Path, Path]:
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
        return root, home

    def _stage_recipe_skill(
        self, root: Path, home: Path, recipe: str, skill: str,
        files: dict[str, str] | None = None,
    ) -> Path:
        d = _recipe_cache_skill(root, home, recipe, skill)
        d.mkdir(parents=True, exist_ok=True)
        (d / "SKILL.md").write_text(f"# {skill}")
        for rel, content in (files or {}).items():
            p = d / rel
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(content)
        return d

    def _overrides_file(self, root: Path, recipe: str, rel: str, content: str) -> Path:
        p = root / "ai-specs" / "recipes" / recipe / "overrides" / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)
        return p

    def test_override_config_merged(self):
        root, home = self._project()
        self._stage_recipe_skill(root, home, "ext-ovr-merged", "my-skill")
        self._overrides_file(root, "ext-ovr-merged", "config.toml", "timeout = 99\n")
        probe = _skill_resolution_probe(root, home, (
            "cfg = mod.load_skill_config(project, 'my-skill', {'timeout': 30}, cli_home=home)\n"
            "print(json.dumps(cfg))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        self.assertEqual(json.loads(probe.stdout)["timeout"], 99)

    def test_override_config_missing_uses_defaults(self):
        root, home = self._project()
        self._stage_recipe_skill(root, home, "ext-ovr-default", "my-skill")
        probe = _skill_resolution_probe(root, home, (
            "cfg = mod.load_skill_config(project, 'my-skill', {'timeout': 30}, cli_home=home)\n"
            "print(json.dumps(cfg))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        self.assertEqual(json.loads(probe.stdout)["timeout"], 30)

    def test_override_config_isolated_between_recipes(self):
        root, home = self._project()
        self._stage_recipe_skill(root, home, "ext-ovr-iso-a", "shared-skill")
        self._stage_recipe_skill(root, home, "ext-ovr-iso-b", "shared-skill")
        self._stage_recipe_skill(root, home, "ext-ovr-iso-b", "other-skill")
        self._overrides_file(root, "ext-ovr-iso-a", "config.toml", "timeout = 99\n")
        # For ext-ovr-iso-b's other-skill, the override from ext-ovr-iso-a
        # must not apply; first-seen resolution picks ext-ovr-iso-a for
        # shared-skill.
        probe = _skill_resolution_probe(root, home, (
            "cfg = mod.load_skill_config(project, 'shared-skill', {'timeout': 30}, cli_home=home)\n"
            "cfg_b = mod.load_skill_config(project, 'other-skill', {'timeout': 30}, cli_home=home)\n"
            "print(json.dumps({'shared': cfg, 'other': cfg_b}))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        payload = json.loads(probe.stdout)
        self.assertEqual(payload["shared"]["timeout"], 99)
        self.assertEqual(payload["other"]["timeout"], 30)

    def test_override_template_preferred(self):
        root, home = self._project()
        self._stage_recipe_skill(
            root, home, "ext-ovr-tpl", "my-skill",
            files={"template.md": "bundled"},
        )
        self._overrides_file(
            root, "ext-ovr-tpl", "templates/template.md", "override",
        )
        probe = _skill_resolution_probe(root, home, (
            "p = mod.resolve_skill_template(project, 'my-skill', 'template.md', cli_home=home)\n"
            "print(json.dumps(str(p) if p else None))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        resolved = json.loads(probe.stdout)
        self.assertIsNotNone(resolved)
        self.assertEqual(Path(resolved).read_text(), "override")

    def test_override_template_fallback_to_bundled(self):
        root, home = self._project()
        self._stage_recipe_skill(
            root, home, "ext-ovr-tpl-fb", "my-skill",
            files={"template.md": "bundled"},
        )
        probe = _skill_resolution_probe(root, home, (
            "p = mod.resolve_skill_template(project, 'my-skill', 'template.md', cli_home=home)\n"
            "print(json.dumps(str(p) if p else None))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        resolved = json.loads(probe.stdout)
        self.assertIsNotNone(resolved)
        self.assertEqual(Path(resolved).read_text(), "bundled")

    def test_override_template_missing_returns_none(self):
        root, home = self._project()
        self._stage_recipe_skill(root, home, "ext-ovr-tpl-miss", "my-skill")
        probe = _skill_resolution_probe(root, home, (
            "p = mod.resolve_skill_template(project, 'my-skill', 'nonexistent.md', cli_home=home)\n"
            "print(json.dumps(str(p) if p else None))\n"
        ))
        self.assertEqual(probe.returncode, 0, probe.stderr)
        self.assertIsNone(json.loads(probe.stdout))


class OrphanCleanupTests(unittest.TestCase):
    """Orphaned origin dirs are cleaned by `ai-specs sync`; referenced recipe
    dirs survive materialization."""

    def _project(self, sections: str = "") -> tuple[Path, Path]:
        tmp, root = _new_project()
        self.addCleanup(tmp.cleanup)
        home = _home_for(root)
        result = invoke(root, "init", "--no-tui", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        if sections:
            _write_manifest(root, sections)
        return root, home

    def test_orphan_recipe_directory_removed(self):
        root, home = self._project()
        orphan = root / "ai-specs" / ".recipe" / "old-recipe"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(orphan.exists())

    def test_orphan_dep_directory_removed(self):
        root, home = self._project()
        orphan = root / "ai-specs" / ".deps" / "old-dep"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(orphan.exists())

    def test_referenced_recipe_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            _seed_recipe(
                home, "ext-orphan-ref", skills=("ext-skill-ref",),
                files={"skills/ext-skill-ref/SKILL.md": "# ext-skill-ref\n"},
            )
            root, _ = self._project(
                "[recipes.ext-orphan-ref]\nenabled = true\nversion = \"1.0.0\"\n",
            )
            recipe_dir = cache_project_dir(root, home) / ".recipe" / "ext-orphan-ref"
            recipe_dir.mkdir(parents=True)
            (recipe_dir / "keep.txt").write_text("keep")
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            # keep.txt is wiped when skill materialize replaces the skill tree,
            # but the recipe dir remains and the skill is materialized.
            self.assertTrue(
                _recipe_cache_skill(root, home, "ext-orphan-ref", "ext-skill-ref").is_dir()
            )


class ResyncIdempotencyTests(unittest.TestCase):
    def test_sync_is_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            home = _home_for(target)
            result = invoke(target, "init", "--no-tui", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = invoke(target, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            first = self._hash_tree(target)
            result = invoke(target, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            second = self._hash_tree(target)
            self.assertEqual(first, second)

    def _hash_tree(self, root: Path) -> str:
        hashes = []
        for p in sorted(root.rglob("*")):
            if p.is_file() and ".git" not in str(p):
                hashes.append(f"{p.relative_to(root)}:{hashlib.sha1(p.read_bytes()).hexdigest()}")
        return "\n".join(hashes)


class CommandRelocationMigrationSmokeTest(unittest.TestCase):
    """End-to-end: a pre-upgrade project (bundled commands committed under
    ai-specs/commands/, legacy lock with [commands]/[opted-out]) cleanly
    migrates on the next `sync` — byte-identical bundled copies removed,
    customizations preserved with a warning, genuine local commands
    untouched, lock trimmed to [meta] (+ [agents.*]), and the merged/fan-out
    command set still includes the bundled command from the cache."""

    def test_pre_upgrade_project_migrates_cleanly_on_sync(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "prj"
        target.mkdir()
        home = _home_for(target)
        subprocess.run(["git", "init", "-q", str(target)], check=True,
                       capture_output=True, input="")
        subprocess.run(
            ["git", "-C", str(target), "config", "user.email", "t@example.com"],
            check=True, capture_output=True, input="",
        )
        subprocess.run(
            ["git", "-C", str(target), "config", "user.name", "t"],
            check=True, capture_output=True, input="",
        )

        # `ai-specs init` on a current CLI never materializes bundled commands;
        # simulate the pre-upgrade (0.16.0-era) committed state by hand.
        result = invoke(target, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        commands_dir = target / "ai-specs" / "commands"
        bundled_src = home / "bundled-commands"

        # 1. Byte-identical committed bundled copy → must be removed as a leftover.
        rules_audit_content = (bundled_src / "rules-audit.md").read_text()
        (commands_dir / "rules-audit.md").write_text(rules_audit_content)

        # 2. Customized bundled copy (content differs) → must be preserved + warned.
        (commands_dir / "skills-as-rules.md").write_text(
            "# skills-as-rules (customized by this project)\n"
        )

        # 3. Genuine local command (no bundled counterpart) → must be untouched.
        (commands_dir / "my-local-command.md").write_text("# my-local-command\n")

        # 4. Legacy lock with [commands]/[opted-out] (0.16.0-era schema).
        rules_audit_hash = hashlib.sha256(rules_audit_content.encode("utf-8")).hexdigest()
        (target / "ai-specs" / ".ai-specs.lock").write_text(
            '[meta]\ncli_version = "0.16.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            "[commands]\n"
            f'"rules-audit.md" = "{rules_audit_hash}"\n'
            '"skills-as-rules.md" = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"\n\n'
            "[opted-out]\n"
            'files = ["commands/some-other-file.md"]\n'
        )

        subprocess.run(["git", "-C", str(target), "add", "-A"], check=True,
                       capture_output=True, input="")
        subprocess.run(
            ["git", "-C", str(target), "commit", "-qm", "pre-upgrade snapshot"],
            check=True, capture_output=True, input="",
        )

        _write_manifest(target)  # default agents first (unused; rewritten below)
        (target / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'migration-fixture'\n\n"
            "[agents]\nenabled = ['cursor', 'opencode']\n"
        )
        result = invoke(target, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        out = result.stdout + result.stderr

        # Byte-identical bundled copy removed.
        self.assertFalse((commands_dir / "rules-audit.md").exists())

        # Customized copy preserved, with a warning printed.
        self.assertTrue((commands_dir / "skills-as-rules.md").exists())
        self.assertIn("customized", out)

        # Genuine local command untouched.
        self.assertEqual(
            (commands_dir / "my-local-command.md").read_text(), "# my-local-command\n"
        )

        # Lock trimmed to [meta] (+ [agents.*]) — no [commands]/[opted-out].
        lock_text = (target / "ai-specs" / ".ai-specs.lock").read_text()
        self.assertIn("[meta]", lock_text)
        self.assertNotIn("[commands]", lock_text)
        self.assertNotIn("[opted-out]", lock_text)

        # Merged/fan-out command set still includes the bundled command (from
        # cache) alongside the customized and genuine local ones.
        for rel in (
            ".cursor/commands/rules-audit.md",
            ".cursor/commands/skills-as-rules.md",
            ".cursor/commands/my-local-command.md",
            ".opencode/commands/rules-audit.md",
            ".opencode/commands/skills-as-rules.md",
            ".opencode/commands/my-local-command.md",
        ):
            self.assertTrue((target / rel).is_file(), rel)
        self.assertEqual(
            (target / ".cursor" / "commands" / "skills-as-rules.md").read_text(),
            "# skills-as-rules (customized by this project)\n",
            "the customized local copy must win over the bundled/cache tier in fan-out",
        )


if __name__ == "__main__":
    unittest.main()

"""Black-box worktree-flow recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema validation, config defaults/validation, stamped
gate/post-merge hooks, materialized skill/commands/scripts, rendered brief
policies, and golden skill/command content) through the CLI process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + exact id + materialization plan) and via
  ``tomllib`` reads of the catalog recipe.toml.
- Config defaults, overrides, and enum rejection are observable via ``sync``:
  the stamped ``worktree-gate.sh`` hook carries the resolved gate_mode /
  gate_scope / repo_topology, values outside a declared enum fail sync naming
  the value, and the rendered AGENTS.md brief states the configured policy.
- Materialization is observable via ``sync``: bundled skill + commands under
  the per-project CLI cache, the cleanup launcher under
  ``ai-specs/recipes/worktree-flow/overrides/bin/``, and the managed
  post-merge hook under ``.git/hooks/post-merge``.
- Golden content checks read the recipe surfaces directly (read-only).
"""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_DIR = CATALOG / "worktree-flow"
RECIPE_ID = "worktree-flow"

UNCONDITIONAL_WORKTREE_RULE = (
    "Create a dedicated worktree for changes that write artifacts or modify code."
)
PROJECT_RUNTIME_FLOW_WORKTREE_RULE = (
    "Artifact phases and implementation phases run in a dedicated worktree "
    "when they write files."
)


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _make_manifest(root: Path, name: str = "fixture") -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = ['claude']\n"
    )


def _recipe_toml() -> dict:
    return tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())


def _run_quiet(argv: list[str], **kwargs):
    kwargs.setdefault("stdin", subprocess.DEVNULL)
    return subprocess.run(argv, capture_output=True, text=True, **kwargs)


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-worktree-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id, cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {recipe_id} failed: {result.stdout}{result.stderr}",
        )
        return result

    def sync(self):
        return invoke(self.root, "sync", cli_home=self.home)


    def set_config(self, old: str, new: str) -> None:
        """Override a recipe default written by `recipe add` in the manifest."""
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text().replace(old, new))

    def agents_md(self) -> str:
        return (self.root / "AGENTS.md").read_text()

    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)

    def stamped_gate_hook(self) -> Path:
        return (
            self.root / "ai-specs" / "recipes" / RECIPE_ID
            / "hooks" / "worktree-gate.sh"
        )

    def post_merge_hook(self) -> Path:
        return self.root / ".git" / "hooks" / "post-merge"


class WorktreeFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_recipe_validates(self):
        """`recipe add` validates the recipe by its exact id; the catalog
        declaration carries the worktree-isolation capability."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "worktree-flow must pass schema validation and add by its exact id",
        )
        raw = _recipe_toml()
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        cap_ids = {c["id"] for c in raw["capabilities"]}
        self.assertIn("worktree-isolation", cap_ids)

    def test_sync_defaults_to_always(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        hook = self.stamped_gate_hook()
        self.assertTrue(hook.is_file())
        content = hook.read_text()
        self.assertIn('stamped_gate_mode="always"', content)

    def test_sync_materializes_gate_mode_into_hook(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_mode = "always"', 'gate_mode = "ask"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.stamped_gate_hook().read_text()
        self.assertIn('stamped_gate_mode="ask"', content)
        self.assertNotIn("__WORKTREE_GATE_MODE__", content)

    def test_sync_rejects_invalid_gate_mode(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_mode = "always"', 'gate_mode = "bogus"')
        result = self.sync()
        combined = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1)
        self.assertIn("bogus", combined)
        self.assertRegex(combined, r"always.*ask.*off|always \| ask \| off")

    def test_materializes_skill_commands_and_script(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill = self.cache() / ".recipe" / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md"
        self.assertTrue(skill.is_file(), "bundled skill should materialize")
        for cmd in ("worktree-new", "worktree-clean"):
            path = self.cache() / "commands" / f"{cmd}.md"
            self.assertTrue(path.is_file(), f"command {cmd} should materialize")
        script = (
            self.root / "ai-specs" / "recipes" / RECIPE_ID / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertTrue(script.is_file(), "cleanup script should materialize")

    def test_sync_defaults_repo_topology_to_auto(self):
        """With no project override, sync resolves repo_topology to the
        declared `auto` default — observable in the stamped gate hook."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.stamped_gate_hook().read_text()
        self.assertIn('stamped_repo_topology="auto"', content)

    def test_sync_rejects_invalid_repo_topology(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('repo_topology = "auto"', 'repo_topology = "nested"')
        result = self.sync()
        combined = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1)
        self.assertIn("nested", combined)
        self.assertRegex(
            combined,
            r"auto.*standalone.*monorepo-apps.*monorepo-submodules"
            r"|auto \| standalone \| monorepo-apps \| monorepo-submodules",
        )

    def test_sync_materializes_with_repo_topology_default(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        skill = self.cache() / ".recipe" / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md"
        self.assertTrue(skill.is_file(), "skill should materialize with default topology")
        for cmd in ("worktree-new", "worktree-clean"):
            path = self.cache() / "commands" / f"{cmd}.md"
            self.assertTrue(path.is_file(), f"command {cmd} should materialize")
        script = (
            self.root / "ai-specs" / "recipes" / RECIPE_ID / "overrides" / "bin"
            / "worktree-cleanup.sh"
        )
        self.assertTrue(script.is_file(), "cleanup script should materialize")

    def test_gate_scope_defaults_to_auto_and_is_independent(self):
        """gate_scope stays `auto` (its own default) even when gate_mode and
        repo_topology are configured — the resolved values land in the stamp."""
        self.recipe_add(RECIPE_ID)
        self.set_config('repo_topology = "auto"', 'repo_topology = "monorepo-submodules"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.stamped_gate_hook().read_text()
        self.assertIn('stamped_gate_scope="auto"', content)
        self.assertIn('stamped_gate_mode="always"', content)
        self.assertIn('stamped_repo_topology="monorepo-submodules"', content)

    def test_gate_scope_materializes_stamp(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_scope = "auto"', 'gate_scope = "superrepo"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.stamped_gate_hook().read_text()
        self.assertIn('stamped_gate_scope="superrepo"', content)
        self.assertIn('stamped_repo_topology="auto"', content)
        self.assertNotIn("__WORKTREE_REPO_TOPOLOGY__", content)
        self.assertNotIn("__WORKTREE_GATE_SCOPE__", content)

    def test_gate_scope_rejects_invalid_value(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_scope = "auto"', 'gate_scope = "super-repo"')
        result = self.sync()
        combined = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1)
        self.assertIn("super-repo", combined)
        self.assertIn("auto | superrepo | subrepo", combined)

    def test_stale_gate_hook_is_preserved_with_refresh_guidance(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        hook = self.stamped_gate_hook()
        hook.write_text("custom legacy hook\n")
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(hook.read_text(), "custom legacy hook\n")

    # --- Rendered brief gate-mode policies (card AImzsLWw) ------------------

    def test_rendered_brief_states_always_policy(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = self.agents_md()
        self.assertIn("`gate_mode = always`", text)
        self.assertIn("`always` requires a dedicated worktree", text)

    def test_rendered_brief_states_ask_policy_without_bypass(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_mode = "always"', 'gate_mode = "ask"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = self.agents_md()
        self.assertIn("`gate_mode = ask`", text)
        self.assertIn("ask the user to choose a destination", text)
        self.assertIn("feature branch in the current checkout", text)
        self.assertIn("explicit protected-branch override", text)
        self.assertNotIn("WORKTREE_GATE_MODE=off", text)
        self.assertNotIn(UNCONDITIONAL_WORKTREE_RULE, text)

    def test_rendered_brief_states_off_policy(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('gate_mode = "always"', 'gate_mode = "off"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = self.agents_md()
        self.assertIn("`gate_mode = off`", text)
        self.assertIn("where the user directs", text)

    # --- Managed VCS post-merge close hook (card AImzsLWw, T3) ---------------

    POST_MERGE_TARGET = ".git/hooks/post-merge"

    def test_sync_materializes_post_merge_hook(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        hook = self.post_merge_hook()
        self.assertTrue(hook.is_file(), "managed post-merge hook should materialize")
        self.assertTrue(
            os.access(hook, os.X_OK), "post-merge hook must be executable for Git"
        )
        content = hook.read_text()
        self.assertIn("worktree-cleanup.sh", content)
        self.assertIn("exit 0", content)
        self.assertNotIn("__WORKTREE_", content)

    def test_post_merge_hook_fails_open_without_launcher(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        proc = _run_quiet(["bash", str(self.post_merge_hook())], cwd=self.root)
        self.assertEqual(
            proc.returncode, 0, "hook must fail open and never break the merge"
        )

    def test_sync_preserves_existing_post_merge_hook(self):
        hook = self.post_merge_hook()
        hook.parent.mkdir(parents=True)
        hook.write_text("#!/bin/sh\necho user hook\n")
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(hook.read_text(), "#!/bin/sh\necho user hook\n")

    # --- Post-merge hook stamps configured cleanup config (T4) ---------------

    def test_post_merge_hook_stamps_configured_cleanup_config(self):
        self.recipe_add(RECIPE_ID)
        self.set_config('worktrees_dir = ".worktrees"', 'worktrees_dir = "custom-wt"')
        self.set_config('integration_branch = "main"', 'integration_branch = "trunk"')
        self.set_config('repo_topology = "auto"', 'repo_topology = "monorepo-submodules"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.post_merge_hook().read_text()
        self.assertNotIn("__WORKTREE_", content)
        self.assertIn('worktrees_dir="custom-wt"', content)
        self.assertIn('integration_branch="trunk"', content)
        self.assertIn('topology="monorepo-submodules"', content)

    def test_post_merge_hook_stamps_catalog_defaults(self):
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self.post_merge_hook().read_text()
        self.assertNotIn("__WORKTREE_", content)
        self.assertIn('worktrees_dir=".worktrees"', content)
        self.assertIn('integration_branch="main"', content)
        self.assertIn('topology="auto"', content)

    def test_post_merge_hook_materializes_in_linked_worktree(self):
        """A linked worktree's `.git` is a gitfile, not a directory: the managed
        hook must resolve to the shared hooks dir instead of crashing on a
        bogus `.git/hooks` directory."""
        hold = tempfile.TemporaryDirectory()
        self.addCleanup(hold.cleanup)
        main = Path(hold.name) / "main"
        main.mkdir()

        def git(*args):
            subprocess.run(
                ["git", "-C", str(main), *args],
                check=True,
                capture_output=True,
                text=True,
                stdin=subprocess.DEVNULL,
            )

        git("init", "-q")
        git("config", "user.email", "t@t.t")
        git("config", "user.name", "t")
        (main / "README.md").write_text("main\n")
        git("add", "-A")
        git("commit", "-qm", "init")
        git("checkout", "-q", "-B", "main")
        wt = Path(hold.name) / "wt"
        git("worktree", "add", "-q", "-b", "feat", str(wt))

        _make_manifest(wt)
        result = invoke(wt, "recipe", "add", RECIPE_ID, cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        synced = invoke(wt, "sync", cli_home=self.home)
        self.assertEqual(synced.returncode, 0, synced.stdout + synced.stderr)
        hook = main / ".git" / "hooks" / "post-merge"
        self.assertTrue(hook.is_file(), "hook must materialize in the shared git dir")
        self.assertTrue(os.access(hook, os.X_OK))


    def test_post_merge_hook_passes_stamped_config_to_launcher(self):
        subprocess.run(["git", "init", "-q", str(self.root)], check=True,
                       capture_output=True, stdin=subprocess.DEVNULL)
        self.recipe_add(RECIPE_ID)
        self.set_config('worktrees_dir = ".worktrees"', 'worktrees_dir = "custom-wt"')
        self.set_config('integration_branch = "main"', 'integration_branch = "trunk"')
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        launcher = (
            self.root / "ai-specs" / "recipes" / RECIPE_ID / "overrides"
            / "bin" / "worktree-cleanup.sh"
        )
        captured = self.root / "captured-args.txt"
        launcher.write_text(
            "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > " + str(captured) + "\n"
        )
        launcher.chmod(0o755)

        proc = _run_quiet(["bash", str(self.post_merge_hook())], cwd=self.root)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        args = captured.read_text().splitlines()
        self.assertIn("--dir", args)
        self.assertEqual(args[args.index("--dir") + 1], "custom-wt")
        self.assertIn("--base", args)
        self.assertEqual(args[args.index("--base") + 1], "trunk")


class WorktreeFlowGoldenContentTests(unittest.TestCase):
    """Golden content checks over the recipe's own catalog surfaces (read-only)."""

    POST_MERGE_TARGET = ".git/hooks/post-merge"

    def test_skill_mentions_sdd_artifact_phases(self):
        text = (RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md").read_text()
        self.assertIn("SDD artifact phases", text)

    def test_repo_topology_config_is_documented_as_deprecated_alias(self):
        """T4 — `[project].repo_topology` owns topology; the recipe key is legacy."""
        text = (RECIPE_DIR / "recipe.toml").read_text()
        self.assertIn("[config.repo_topology]", text)
        self.assertIn("[project].repo_topology", text)
        self.assertIn("deprecated", text.lower())

    def test_worktree_new_documents_submodule_create_contract(self):
        """Doc-content only — live git worktree add under submodules is manual/agent."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("git -C", text)
        self.assertIn("worktrees_dir", text)
        self.assertIn("<subrepo>-<slug>", text)
        self.assertIn("show-toplevel", text)
        self.assertIn("longest", text.lower())
        self.assertIn("submodule update --init", text)

    def test_worktree_new_documents_superrepo_context_requires_explicit_subrepo(self):
        """1.2 — RED: a superrepo-context request must not infer a subrepo."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("explicit", text)
        self.assertIn("hard-error", text.lower().replace("hard error", "hard-error"))
        self.assertIn("not infer", text.lower())
        self.assertIn("git worktree add", text)

    def test_worktree_new_documents_owner_vs_planning_root(self):
        """1.2 — RED: owner root and planning root are distinct request facts."""
        text = (RECIPE_DIR / "commands" / "worktree-new.md").read_text()
        self.assertIn("planning root", text.lower())
        self.assertIn("owner", text.lower())
        self.assertIn("superproject", text)
        self.assertIn("planning", text.lower())

    def test_worktree_new_is_generated_markdown_not_executable_helper(self):
        """1.2 — RED: /worktree-new is generated Markdown; no executable helper."""
        cmd = RECIPE_DIR / "commands" / "worktree-new.md"
        self.assertTrue(cmd.is_file())
        self.assertNotIn("#!/", cmd.read_text(), "command must stay Markdown, not a script")
        self.assertFalse(
            (RECIPE_DIR / "bin" / "worktree-new").exists(),
            "no executable /worktree-new helper may be added",
        )

    def test_skill_md_documents_request_context_and_no_helper(self):
        """1.2 — RED: SKILL.md create block pins request-context + no helper."""
        skill = (RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md").read_text()
        self.assertIn("resolve_request_context", skill)
        self.assertIn("planning_root", skill)
        self.assertIn("explicit", skill)
        self.assertNotIn("bin/worktree-new", skill)

    def test_brief_workflow_rules_require_which_repo_check(self):
        rules = _recipe_toml()["provides"]["brief"]["workflow_rules"]
        joined = " ".join(rules)
        self.assertIn("which", joined.lower())
        self.assertIn("show-toplevel", joined)
        self.assertIn("monorepo-submodules", joined)

    def test_skill_md_create_block_matches_worktree_new_contract(self):
        """SKILL.md create block must stay byte-consistent with worktree-new.md."""
        skill = (RECIPE_DIR / "skills" / "worktree-flow" / "SKILL.md").read_text()
        # Must use super_root-scoped rev-parse (not bare show-toplevel).
        self.assertIn('git -C "$super_root" rev-parse --show-toplevel', skill)
        # Must use configurable worktrees_dir placeholder, not hardcoded .worktrees
        # as the create destination (default may still be mentioned as prose).
        self.assertIn("<worktrees_dir>", skill)
        # Hardcoded ".worktrees/<subrepo>" create path recreates the original bug.
        self.assertNotIn("$super_abs/.worktrees/", skill)
        self.assertNotIn("git worktree add .worktrees/", skill)

    def test_recipe_brief_fragment_is_config_aware(self):
        raw = (RECIPE_DIR / "recipe.toml").read_text()
        self.assertIn("{config.gate_mode}", raw)
        self.assertNotIn(UNCONDITIONAL_WORKTREE_RULE, raw)
        self.assertNotIn("WORKTREE_GATE_MODE=off", raw)

    def test_project_manifest_runtime_flow_has_no_unconditional_worktree_rule(self):
        text = (ROOT / "ai-specs" / "ai-specs.toml").read_text()
        self.assertNotIn(PROJECT_RUNTIME_FLOW_WORKTREE_RULE, text)

    def test_recipe_registers_managed_post_merge_template(self):
        raw = _recipe_toml()
        targets = {t["target"]: t for t in raw["provides"]["templates"]}
        self.assertIn(self.POST_MERGE_TARGET, targets)
        entry = targets[self.POST_MERGE_TARGET]
        self.assertEqual(entry["source"], "templates/post-merge.sh")
        self.assertEqual(entry["condition"], "not_exists")


if __name__ == "__main__":
    unittest.main()

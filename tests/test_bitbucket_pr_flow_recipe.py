"""Black-box bitbucket-pr-flow recipe tests: every test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema declarations, materialized artifacts, binding
semantics, golden skill/command content) through the CLI process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + "The next sync will materialize:" plan) and via
  ``sync`` (materialized artifacts under the per-project CLI cache).
- The on-sync ``validate-config`` hook is observable by enabling a fresh-id
  copy of the recipe with a required config field: sync fails naming the
  missing field; configuring the field makes sync succeed.
- Capability binding semantics are observable via ``sync``: two providers of
  ``vcs-pr-flow`` without an explicit binding emit a capability-ambiguity
  warning; an explicit ``[[bindings]]`` entry resolves it.
"""
from __future__ import annotations

import json
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
from _blackbox import cache_project_dir, invoke, isolated_home, populate_catalog  # noqa: E402
from _change_paths import change_artifact  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "bitbucket-pr-flow"


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


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-bitbucket-")
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
        result = invoke(self.root, "sync", cli_home=self.home)
        return result

    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)

    def resolved_bindings(self) -> dict:
        """Bindings map from the resolved-config JSON at the materialize
        process boundary (the isolated home's OWN lib copy — never a repo
        import). This is the only observable the [[bindings]] capability
        selection controls: cache artifacts materialize per enabled recipe
        regardless of the binding, so the map is the honest stand-in for the
        old white-box resolve_bindings() assertion.
        """
        out = self.base / "resolved-config.json"
        (self.base / "home").mkdir(parents=True, exist_ok=True)
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.base / "home"),
            "TMPDIR": str(self.base),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        argv = [
            sys.executable,
            str(self.home / "lib" / "_internal" / "recipe-materialize.py"),
            str(self.root), str(self.home), "--resolved-config-out", str(out),
        ]
        proc = subprocess.run(argv, cwd=ROOT, env=env, text=True,
                              capture_output=True, check=False, input="")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertTrue(out.is_file(), proc.stdout + proc.stderr)
        return json.loads(out.read_text()).get("bindings", {})


class BitbucketPrFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
    # --- Phase 1: Manifest and Binding ---

    def test_recipe_validates_and_declares_vcs_pr_flow(self):
        """Recipe is valid and declares vcs-pr-flow capability.

        ``recipe add`` validates the schema and the exact id; pairing the
        recipe with the sibling vcs-pr-flow provider makes sync surface the
        capability declaration as an ambiguity warning naming both recipes.
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "bitbucket-pr-flow must pass schema validation and add by its exact id",
        )
        self.recipe_add("git-pr-flow")
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        self.assertIn(
            "capability ambiguity: capability.id='vcs-pr-flow' "
            "declared by bitbucket-pr-flow, git-pr-flow",
            sync.stderr,
            "bitbucket-pr-flow must declare the vcs-pr-flow capability",
        )

    def test_recipe_has_no_provider_config(self):
        """Config must not declare provider — recipe id is the provider identity."""
        self.recipe_add(RECIPE_ID)
        manifest = (self.root / "ai-specs" / "ai-specs.toml").read_text()
        self.assertIn(
            f"[recipes.{RECIPE_ID}.config]", manifest,
            "recipe add must write the config section for config-field recipes",
        )
        config_section = manifest.split(f"[recipes.{RECIPE_ID}.config]", 1)[1]
        self.assertNotIn(
            "provider",
            config_section,
            "provider config field must not exist on sibling VCS recipes",
        )

    def test_recipe_declares_development_base_branch_default(self):
        """Config declares base_branch=development as default."""
        self.recipe_add(RECIPE_ID)
        manifest = (self.root / "ai-specs" / "ai-specs.toml").read_text()
        self.assertIn(
            'base_branch = "development"',
            manifest,
            "base_branch config field must exist with default 'development'",
        )
        self.assertNotIn(
            'base_branch = ""  # REQUIRED',
            manifest,
            "base_branch must not be a required config field",
        )

    def test_recipe_declares_validate_config_hook(self):
        """Recipe declares on-sync validate-config hook.

        Observable through sync with a fresh-id copy of the recipe carrying a
        required config field: the hook fails the sync naming the missing
        field, and configuring the field lets the same sync succeed.
        """
        hookcheck_id = "bitbucket-pr-flow-hookcheck"
        toml = (CATALOG / RECIPE_ID / "recipe.toml").read_text()
        modified = toml.replace(f'id = "{RECIPE_ID}"', f'id = "{hookcheck_id}"', 1)
        modified += (
            "\n[config.mandatory_field]\nrequired = true\ntype = \"string\"\n"
            'help_text = "test required field"\n'
        )
        populate_catalog(self.home, hookcheck_id, modified)
        # Bundled primitives resolve under the recipe's own catalog dir; link
        # the real recipe's skills/commands into the fresh-id copy so the only
        # behavioral delta is the added required config field.
        fresh_dir = self.home / "catalog" / "recipes" / hookcheck_id
        for asset in ("skills", "commands", "README.md"):
            (fresh_dir / asset).symlink_to(CATALOG / RECIPE_ID / asset)
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{hookcheck_id}]\nenabled = true\n"
        )
        failed = self.sync()
        self.assertNotEqual(
            failed.returncode, 0,
            "on-sync validate-config hook must reject a missing required field",
        )
        self.assertIn(
            "missing required config field 'mandatory_field'",
            failed.stderr,
            "validate-config hook must name the missing required field",
        )
        text = manifest.read_text()
        text += (
            f"\n[recipes.{hookcheck_id}.config]\n"
            'mandatory_field = "present"\n'
        )
        manifest.write_text(text)
        ok = self.sync()
        self.assertEqual(
            ok.returncode, 0,
            f"satisfied validate-config hook must pass: {ok.stdout}{ok.stderr}",
        )

    def test_recipe_declares_bundled_skill(self):
        """Recipe declares bundled bitbucket-merge-workflow skill."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "The next sync will materialize:", result.stdout,
            "recipe add must print the declared primitives",
        )
        self.assertIn(
            "- skills: bitbucket-merge-workflow", result.stdout,
            "bitbucket-merge-workflow skill must be declared by the recipe",
        )

    def test_recipe_declares_bb_pr_create_command(self):
        """Recipe declares bb-pr-create command."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "- commands: bb-pr-create", result.stdout,
            "bb-pr-create command must be declared by the recipe",
        )

    def test_recipe_declares_readme_doc(self):
        """Recipe declares README.md doc provision."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "- doc: README.md → ai-specs/recipes/bitbucket-pr-flow/README.md",
            result.stdout,
            "README doc provision must be declared by the recipe",
        )

    def test_recipe_identifies_php_bb_cli(self):
        """Manifest targets PHP bb-cli homepage, binary bb, recipe 1.3.0, host 1.4.1."""
        recipe_path = CATALOG / RECIPE_ID / "recipe.toml"
        text = recipe_path.read_text()
        data = tomllib.loads(text)
        self.assertEqual(data["recipe"]["version"], "1.3.0")
        self.assertEqual(data["deps"]["cli"][0]["binary"], "bb")
        self.assertEqual(
            data["deps"]["cli"][0]["install_url"],
            "https://bb-cli.github.io",
        )
        self.assertEqual(data["deps"]["cli"][0]["version_check"], "bb --version")
        self.assertEqual(data["deps"]["cli"][0]["min_version"], "1.4.1")
        self.assertNotEqual(
            data["recipe"]["version"],
            data["deps"]["cli"][0]["min_version"],
            "recipe version 1.3.0 must stay distinct from host min_version 1.4.1",
        )
        self.assertNotIn("paulvanderlei", text)
        self.assertNotIn("@pilatos", text)

    # --- Phase 2: Materialization ---

    def test_brief_surfaces_postmerge_sync_and_cleanup(self):
        """The always-on brief must surface both a post-merge base-sync rule
        (git pull --ff-only) and a post-merge cleanup rule."""
        self.recipe_add(RECIPE_ID)
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        agents = (self.root / "AGENTS.md").read_text()
        self.assertTrue(
            any("ff-only" in line.lower() for line in agents.splitlines()),
            "post-merge base-sync workflow_rule missing (git pull --ff-only)",
        )
        self.assertTrue(
            any(
                "worktree" in line.lower() and "merged" in line.lower()
                for line in agents.splitlines()
            ),
            "post-merge cleanup workflow_rule missing",
        )

    def _synced_project(self):
        """Project with bitbucket-pr-flow added and synced; returns the sync result."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def test_materialize_produces_skill(self):
        """Sync materializes the bundled bitbucket-merge-workflow skill."""
        self._synced_project()
        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

    def test_materialize_produces_command(self):
        """Sync materializes the bb-pr-create command."""
        self._synced_project()
        cmd = self.cache() / "commands" / "bb-pr-create.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

    def test_materialize_produces_readme(self):
        """Sync materializes the README doc."""
        self._synced_project()
        doc = self.root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_touch_github_assets(self):
        """Sync does not modify git-pr-flow recipe assets."""
        self._synced_project()
        github_skill = (
            self.cache() / ".recipe" / "git-pr-flow"
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertFalse(
            github_skill.exists(),
            "git-pr-flow assets must not be materialized when only bitbucket-pr-flow is enabled"
        )


class BitbucketPrFlowBindingTests(_CliFixtureMixin, unittest.TestCase):
    """Provider binding semantics: ambiguity and explicit binding.

    Both real catalog recipes (git-pr-flow and bitbucket-pr-flow) declare the
    vcs-pr-flow capability; the original in-process resolve_bindings calls are
    replaced by the binding behavior sync renders.
    """

    def _enable_both(self) -> None:
        self.recipe_add("git-pr-flow")
        self.recipe_add(RECIPE_ID)

    def test_dual_vcs_pr_flow_providers_stay_unbound_without_binding(self):
        """When both git-pr-flow and bitbucket-pr-flow are enabled without bindings, vcs-pr-flow stays unbound."""
        self._enable_both()
        sync = self.sync()
        self.assertEqual(
            sync.returncode, 0,
            "unbound capability ambiguity is a warning, not a fatal conflict",
        )
        self.assertIn(
            "capability ambiguity: capability.id='vcs-pr-flow' "
            "declared by bitbucket-pr-flow, git-pr-flow",
            sync.stderr,
            "vcs-pr-flow must stay unbound (auto-bind is forbidden with two providers)",
        )
        self.assertIn(
            "Add an explicit [[bindings]] entry to resolve",
            sync.stderr,
            "ambiguity warning must guide toward an explicit binding",
        )

    def test_explicit_binding_selects_bitbucket(self):
        """Explicit binding to bitbucket-pr-flow selects it for vcs-pr-flow."""
        self._enable_both()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text()
            + '\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "bitbucket-pr-flow"\n'
        )
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        self.assertNotIn(
            "capability ambiguity: capability.id='vcs-pr-flow'",
            sync.stderr,
            "an explicit [[bindings]] entry must resolve the vcs-pr-flow ambiguity",
        )
        # Binding-discriminating observable (old white-box assertion:
        # bindings['vcs-pr-flow'] == 'bitbucket-pr-flow'). Cache-file
        # presence is NOT discriminating — every enabled recipe materializes
        # its primitives regardless of the binding — so the assertion must
        # read the resolved-config bindings map, the surface the binding
        # actually controls.
        self.assertEqual(
            self.resolved_bindings().get("vcs-pr-flow"),
            "bitbucket-pr-flow",
            "the explicit [[bindings]] entry must select bitbucket-pr-flow "
            "for the vcs-pr-flow capability",
        )
        # In-test discrimination proof: flipping the binding to the sibling
        # provider flips the resolved selection — this assertion would fail
        # if the observable were binding-insensitive.
        manifest.write_text(
            manifest.read_text().replace(
                'recipe = "bitbucket-pr-flow"', 'recipe = "git-pr-flow"'
            )
        )
        flipped = self.resolved_bindings()
        self.assertEqual(
            flipped.get("vcs-pr-flow"), "git-pr-flow",
            "the flipped binding must select git-pr-flow (discrimination proof)",
        )
        self.assertNotIn(
            "bitbucket-pr-flow", flipped.values(),
            "the flipped binding must no longer select bitbucket-pr-flow",
        )


class BitbucketPrFlowGoldenContentTests(unittest.TestCase):
    """Phase 3: golden text checks for skill and command content."""

    @classmethod
    def setUpClass(cls):
        cls.skill_path = (
            CATALOG / RECIPE_ID / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        cls.command_path = CATALOG / RECIPE_ID / "commands" / "bb-pr-create.md"
        cls.skill_text = cls.skill_path.read_text()
        cls.command_text = cls.command_path.read_text()
        cls.readme_text = (CATALOG / RECIPE_ID / "README.md").read_text()
        cls.recipe_text = (CATALOG / RECIPE_ID / "recipe.toml").read_text()
        cls.catalog_doc = (ROOT / "docs" / "recipes-catalog.md").read_text()
        cls.schema_doc = (ROOT / "docs" / "recipe-schema.md").read_text()

    def _live_surfaces(self) -> dict[str, str]:
        return {
            "recipe.toml": self.recipe_text,
            "README.md": self.readme_text,
            "bb-pr-create.md": self.command_text,
            "SKILL.md": self.skill_text,
            "docs/recipes-catalog.md": self.catalog_doc,
            "docs/recipe-schema.md": self.schema_doc,
        }

    def _fenced_executable(self, text: str) -> str:
        return "\n".join(
            m.group(1)
            for m in re.finditer(r"```(?:bash|sh)\n(.*?)```", text, re.DOTALL)
        )

    def _active_command_lines(self, text: str) -> list[str]:
        lines = []
        for raw in self._fenced_executable(text).splitlines():
            stripped = raw.strip()
            if not stripped or stripped.startswith("#"):
                continue
            lines.append(stripped)
        return lines

    # --- Skill golden content ---

    def test_skill_checks_bb_installed(self):
        """Skill checks that bb is installed via command -v bb."""
        self.assertIn("command -v bb", self.skill_text)

    def test_skill_checks_bb_auth(self):
        """Skill checks bb authentication via bb auth show."""
        self.assertIn("bb auth show", self.skill_text)

    def test_skill_has_positive_php_bb_cli_identity_guard(self):
        """Skill blocks a foreign bb using bb --version and PHP bb-cli / brew guidance."""
        self.assertIn("bb --version", self.skill_text)
        normalized = self.skill_text.replace("`", "")
        self.assertRegex(normalized, r"(?i)not PHP bb-cli")
        self.assertIn("brew install bb-cli", self.skill_text)
        self.assertIn("https://bb-cli.github.io", self.skill_text)

    def test_command_has_positive_php_bb_cli_identity_guard(self):
        """Command blocks a foreign bb using bb --version and PHP bb-cli / brew guidance."""
        self.assertIn("bb --version", self.command_text)
        normalized = self.command_text.replace("`", "")
        self.assertRegex(normalized, r"(?i)not PHP bb-cli")
        self.assertIn("brew install bb-cli", self.command_text)
        self.assertIn("https://bb-cli.github.io", self.command_text)

    def test_agent_auth_checks_emit_only_username(self):
        """Agent-facing auth checks capture bb auth show and emit only Username."""
        for name, text in (
            ("SKILL.md", self.skill_text),
            ("bb-pr-create.md", self.command_text),
        ):
            with self.subTest(surface=name):
                auth_lines = [
                    ln
                    for ln in self._active_command_lines(text)
                    if "bb auth show" in ln
                ]
                self.assertTrue(
                    auth_lines,
                    f"{name} must capture bb auth show in an executable check",
                )
                for ln in auth_lines:
                    self.assertNotEqual(
                        ln,
                        "bb auth show",
                        f"{name} must not print raw bb auth show output",
                    )
                    self.assertNotIn("AppPassword", ln)
                self.assertRegex(
                    text,
                    r"(?i)(missing|empty|absent).{0,40}Username|Username.{0,40}(missing|empty|absent)",
                )
                self.assertRegex(text, r"(?i)multiple Username")

    def test_skill_deletes_feature_remote_branch_after_merge(self):
        """Feature heads get an explicit remote-branch delete; protected heads stay excluded."""
        self.assertRegex(
            self.skill_text,
            r"git push\s+\$REMOTE\s+--delete|git push\s+--delete",
        )
        self.assertIn("protected", self.skill_text.lower())
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)

    def test_apply_progress_omits_absolute_host_and_worktree_paths(self):
        """Hardening apply-progress wording must not embed host or worktree absolutes."""
        progress = change_artifact(ROOT, "bitbucket-bb-cli-alignment", "apply-progress.md")
        self.assertTrue(progress.is_file(), f"missing {progress}")
        text = progress.read_text()
        self.assertIsNone(
            re.search(r"(?m)(/Users/|/home/|/opt/homebrew/)", text),
            "apply-progress.md must not embed absolute host or Homebrew paths",
        )

    def test_skill_uses_explicit_push(self):
        """Skill uses explicit git push -u $REMOTE before PR creation."""
        self.assertIn("git push -u $REMOTE", self.skill_text)

    def test_skill_uses_bb_pr_create_with_required_flags(self):
        """Skill creates PR with verified PHP argv (positional source/dest + title/description)."""
        self.assertIn("bb pr create", self.skill_text)
        self.assertIn("--title", self.skill_text)
        self.assertIn("--description", self.skill_text)
        create_lines = [
            ln for ln in self._active_command_lines(self.skill_text) if ln.startswith("bb pr create")
        ]
        self.assertTrue(create_lines, "Skill must contain an executable bb pr create example")
        for ln in create_lines:
            self.assertNotIn("--source", ln)
            self.assertNotIn("--destination", ln)
            self.assertNotIn("--body", ln)

    def test_skill_merge_omits_close_source_flag_and_protects_heads(self):
        """Protected-head policy remains; PHP merge does not take --close-source-branch."""
        self.assertIn("never pass --close-source-branch", self.skill_text.lower())
        self.assertIn("Head branch class", self.skill_text)
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)
        self.assertIn("release/v", self.skill_text)
        merge_lines = [
            ln for ln in self._active_command_lines(self.skill_text) if ln.startswith("bb pr merge")
        ]
        self.assertTrue(merge_lines, "Skill must contain an executable bb pr merge example")
        for ln in merge_lines:
            self.assertNotIn("--close-source-branch", ln)
            self.assertNotIn("--strategy", ln)

    def test_skill_merge_uses_php_merge_method(self):
        """Skill merge command is PHP `bb pr merge` with id only."""
        merge_pos = self.skill_text.find("bb pr merge")
        self.assertGreater(merge_pos, 0, "Skill must contain bb pr merge")
        merge_line = self.skill_text[merge_pos:self.skill_text.find("\n", merge_pos)]
        self.assertNotIn("--strategy squash", merge_line)
        self.assertNotIn("--close-source-branch", merge_line)

    def test_skill_merge_pins_approved_sha(self):
        """Approval-SHA policy stays; retrieval is an open gap (show is comments, not JSON)."""
        self.assertIn("APPROVED_SHA", self.skill_text)
        self.assertIn("CURRENT_SHA", self.skill_text)
        self.assertIn("bb pr show", self.skill_text)
        self.assertNotIn("bb pr view", self.skill_text)
        lowered = self.skill_text.lower()
        self.assertTrue(
            "open verification gap" in lowered or "open-verification gap" in lowered,
            "SHA retrieval must be labeled an open verification gap",
        )

    def test_skill_worktree_cleanup_uses_absolute_path(self):
        """Skill worktree cleanup does not assume cwd is repo root."""
        self.assertNotIn(
            "git worktree remove .worktrees/",
            self.skill_text,
            "Skill must not use relative .worktrees/ path for worktree removal"
        )

    def test_skill_does_not_auto_merge(self):
        """Skill does not include auto-merge flags."""
        self.assertNotIn("auto-merge", self.skill_text.lower())

    # --- Command golden content ---

    def test_command_checks_bb_installed(self):
        """Command checks that bb is installed via command -v bb."""
        self.assertIn("command -v bb", self.command_text)

    def test_command_checks_bb_auth(self):
        """Command checks bb authentication via bb auth show."""
        self.assertIn("bb auth show", self.command_text)



    def test_skill_does_not_require_openspec_change_folder(self):
        """VCS-only projects merge without an OpenSpec change folder."""
        self.assertNotIn("openspec/changes/<slug>/", self.skill_text)

    def test_skill_does_not_own_archive_tail(self):
        """Archive-tail stays with Plan Build; the VCS skill never archives."""
        self.assertNotIn("archive and record SDD/OpenSpec artifacts", self.skill_text)

    def test_skill_does_not_invoke_premerge_guardian(self):
        """The artifact guardian is Plan Build-owned, not a VCS precondition."""
        self.assertNotIn("premerge_guardian.py", self.skill_text)

    def test_skill_points_artifact_ownership_at_plan_build(self):
        """A concise note hands planning/promotion/archive to Plan Build."""
        self.assertIn("Artifact ownership", self.skill_text)
        self.assertIn("plan-build-flow", self.skill_text)

    def test_skill_invokes_tracker_ledger_host_before_merge(self):
        """Tracker authorization is graded by the shell host's direct mode."""
        self.assertIn("tracker-card-gate.sh", self.skill_text)
        self.assertIn("--root <planning-root>", self.skill_text)
        self.assertIn("--checkpoint pre-merge", self.skill_text)
        self.assertNotIn("tracker_ledger_host.py", self.skill_text)
        self.assertNotIn("premerge_guardian.py", self.skill_text)

    def test_command_uses_explicit_push(self):
        """Command uses explicit git push -u $REMOTE before PR creation."""
        self.assertIn("git push -u $REMOTE", self.command_text)

    def test_command_uses_bb_pr_create_with_required_flags(self):
        """Command creates PR with verified PHP argv."""
        self.assertIn("bb pr create", self.command_text)
        self.assertIn("--title", self.command_text)
        self.assertIn("--description", self.command_text)
        create_lines = [
            ln
            for ln in self._active_command_lines(self.command_text)
            if ln.startswith("bb pr create")
        ]
        self.assertTrue(create_lines, "Command must contain an executable bb pr create example")
        for ln in create_lines:
            self.assertNotIn("--source", ln)
            self.assertNotIn("--destination", ln)
            self.assertNotIn("--body", ln)

    def test_command_does_not_include_merge(self):
        """Command is create-only and does not include merge steps."""
        self.assertNotIn("bb pr merge", self.command_text)
        self.assertNotIn("--close-source-branch", self.command_text)

    def test_command_does_not_auto_merge(self):
        """Command does not include auto-merge flags."""
        self.assertNotIn("auto-merge", self.command_text.lower())

    def test_command_push_before_create_order(self):
        """Command places git push before bb pr create."""
        push_pos = self.command_text.find("git push -u $REMOTE")
        create_pos = self.command_text.find("bb pr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before bb pr create in the command"
        )

    def test_skill_push_before_create_order(self):
        """Skill places git push before bb pr create."""
        push_pos = self.skill_text.find("git push -u $REMOTE")
        create_pos = self.skill_text.find("bb pr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before bb pr create in the skill"
        )

    # --- Runtime blocker messages ---

    def test_skill_install_blocker_message(self):
        """Skill contains exact install blocker message when bb is missing."""
        self.assertIn(
            "bb` is not installed",
            self.skill_text,
            "Skill must contain install blocker message"
        )
        self.assertIn(
            "https://bb-cli.github.io",
            self.skill_text,
            "Skill install blocker must include PHP bb-cli installation URL"
        )

    def test_skill_auth_blocker_message(self):
        """Skill contains exact auth blocker message when bb is unauthenticated."""
        self.assertIn(
            "bb` is not authenticated",
            self.skill_text,
            "Skill must contain auth blocker message"
        )
        self.assertIn(
            "bb auth save",
            self.skill_text,
            "Skill auth blocker must include PHP remediation command"
        )
        self.assertNotIn("bb auth login", self.skill_text)

    def test_skill_preflight_before_push_order(self):
        """Skill checks bb install and auth BEFORE git push."""
        install_check_pos = self.skill_text.find("command -v bb")
        auth_check_pos = self.skill_text.find("bb auth show")
        push_pos = self.skill_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v bb in the skill"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER bb auth show in the skill"
        )

    def test_skill_stops_after_pr_create_reports_url(self):
        """Skill STOPs after PR creation and reports the PR URL."""
        create_pos = self.skill_text.find("bb pr create")
        stop_pos = self.skill_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER bb pr create in the skill"
        )
        self.assertIn(
            "Report the PR URL",
            self.skill_text,
            "Skill must instruct to report the PR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.skill_text,
            "Skill must explicitly say not to merge after PR creation"
        )

    def test_command_install_blocker_message(self):
        """Command contains exact install blocker message when bb is missing."""
        self.assertIn(
            "bb` is not installed",
            self.command_text,
            "Command must contain install blocker message"
        )
        self.assertIn(
            "https://bb-cli.github.io",
            self.command_text,
            "Command install blocker must include PHP bb-cli installation URL"
        )

    def test_command_auth_blocker_message(self):
        """Command contains exact auth blocker message when bb is unauthenticated."""
        self.assertIn(
            "bb` is not authenticated",
            self.command_text,
            "Command must contain auth blocker message"
        )
        self.assertIn(
            "bb auth save",
            self.command_text,
            "Skill auth blocker must include PHP remediation command"
        )
        self.assertNotIn("bb auth login", self.command_text)

    def test_command_preflight_before_push_order(self):
        """Command checks bb install and auth BEFORE git push."""
        install_check_pos = self.command_text.find("command -v bb")
        auth_check_pos = self.command_text.find("bb auth show")
        push_pos = self.command_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v bb in the command"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER bb auth show in the command"
        )

    def test_command_stops_after_pr_create_reports_url(self):
        """Command STOPs after PR creation and reports the PR URL."""
        create_pos = self.command_text.find("bb pr create")
        stop_pos = self.command_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER bb pr create in the command"
        )
        self.assertIn(
            "Report the PR URL",
            self.command_text,
            "Command must instruct to report the PR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.command_text,
            "Command must explicitly say not to merge after PR creation"
        )

    # --- Dynamic remote resolution ---

    def test_skill_uses_dynamic_remote_resolution(self):
        """Skill resolves the Bitbucket remote dynamically instead of hardcoding origin."""
        self.assertIn("REMOTE=$(git remote", self.skill_text)
        self.assertIn("git push -u $REMOTE", self.skill_text)

    def test_command_uses_dynamic_remote_resolution(self):
        """Command resolves the Bitbucket remote dynamically instead of hardcoding origin."""
        self.assertIn("REMOTE=$(git remote", self.command_text)
        self.assertIn("git push -u $REMOTE", self.command_text)

    def test_live_surfaces_php_identity_and_forbidden_typescript(self):
        """Live Bitbucket surfaces identify PHP bb-cli and drop TypeScript identity/verbs."""
        for name, text in self._live_surfaces().items():
            with self.subTest(surface=name):
                self.assertNotIn("paulvanderlei", text)
                self.assertNotIn("@pilatos", text)
                self.assertNotIn("bb auth login", text)
                self.assertNotIn("bb pr view", text)
                self.assertFalse(
                    bool(re.search(r"brew install bb(?!-cli)\b", text)),
                    f"{name} must not propose brew install bb",
                )

    def test_live_surfaces_php_auth_and_pr_verbs(self):
        """Auth/PR verbs on recipe surfaces are PHP save/show/create/show/merge."""
        for name in ("README.md", "bb-pr-create.md", "SKILL.md"):
            text = self._live_surfaces()[name]
            with self.subTest(surface=name):
                self.assertIn("bb auth show", text)
                self.assertIn("bb auth save", text)
                self.assertIn("bb pr create", text)
                self.assertIn("bb pr show", text)
        self.assertIn("bb pr merge", self.skill_text)

    def test_unverified_flags_absent_from_executable_examples(self):
        """TypeScript create/show/merge flags are not executable examples."""
        for name, text in self._live_surfaces().items():
            if name in ("recipe.toml", "docs/recipe-schema.md", "docs/recipes-catalog.md"):
                continue
            with self.subTest(surface=name):
                for ln in self._active_command_lines(text):
                    if ln.startswith("bb pr create"):
                        self.assertNotIn("--source", ln)
                        self.assertNotIn("--destination", ln)
                        self.assertNotIn("--body", ln)
                    if ln.startswith("bb pr show"):
                        self.assertNotIn("--json", ln)
                        self.assertNotIn("--jq", ln)
                    if ln.startswith("bb pr merge"):
                        self.assertNotIn("--strategy", ln)
                        self.assertNotIn("--close-source-branch", ln)
                    self.assertNotIn("bb auth login", ln)
                    self.assertNotIn("bb pr view", ln)


class BitbucketPrFlowDualProviderTests(_CliFixtureMixin, unittest.TestCase):
    """End-to-end dual provider materialization with explicit bindings."""

    def _enable_dual(self, binding_recipe: str) -> None:
        """Enable both git-pr-flow and bitbucket-pr-flow with an explicit binding."""
        self.recipe_add("git-pr-flow")
        self.recipe_add(RECIPE_ID)
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text()
            + f'\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "{binding_recipe}"\n'
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_dual_provider_bitbucket_bound_materializes_both(self):
        """When bound to bitbucket-pr-flow, both recipes materialize their assets (different IDs)."""
        self._enable_dual("bitbucket-pr-flow")

        bitbucket_skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        bitbucket_cmd = self.cache() / "commands" / "bb-pr-create.md"
        self.assertTrue(bitbucket_skill.is_file(), f"missing bitbucket skill at {bitbucket_skill}")
        self.assertTrue(bitbucket_cmd.is_file(), f"missing bitbucket command at {bitbucket_cmd}")

        github_skill = (
            self.cache() / ".recipe" / "git-pr-flow"
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        github_cmd = self.cache() / "commands" / "pr-create.md"
        self.assertTrue(github_skill.is_file(), f"missing github skill at {github_skill}")
        self.assertTrue(github_cmd.is_file(), f"missing github command at {github_cmd}")

    def test_dual_provider_github_bound_materializes_both(self):
        """When bound to git-pr-flow, both recipes materialize their assets (different IDs)."""
        self._enable_dual("git-pr-flow")

        github_skill = (
            self.cache() / ".recipe" / "git-pr-flow"
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        github_cmd = self.cache() / "commands" / "pr-create.md"
        self.assertTrue(github_skill.is_file(), f"missing github skill at {github_skill}")
        self.assertTrue(github_cmd.is_file(), f"missing github command at {github_cmd}")

        bitbucket_skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        bitbucket_cmd = self.cache() / "commands" / "bb-pr-create.md"
        self.assertTrue(bitbucket_skill.is_file(), f"missing bitbucket skill at {bitbucket_skill}")
        self.assertTrue(bitbucket_cmd.is_file(), f"missing bitbucket command at {bitbucket_cmd}")


if __name__ == "__main__":
    unittest.main()

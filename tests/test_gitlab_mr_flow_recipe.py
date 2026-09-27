"""Black-box gitlab-mr-flow recipe tests: every test drives ``bin/ai-specs``.

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

import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "gitlab-mr-flow"


def extract_awk(text: str, anchor: str) -> list[str]:
    """Return every single-quoted awk program in ``text`` containing ``anchor``."""
    found = [
        match.group(1)
        for match in re.finditer(r"awk\s+'([^']*)'", text, re.S)
        if anchor in match.group(1)
    ]
    if not found:
        raise AssertionError(f"no awk snippet containing {anchor!r}")
    return found


def run_awk(script: str, sample: str) -> str:
    proc = subprocess.run(
        ["awk", script], input=sample, capture_output=True, text=True
    )
    assert proc.returncode == 0, f"awk failed: {proc.stderr}"
    return proc.stdout


def _normalize(script: str) -> str:
    return "\n".join(line.strip() for line in script.strip().splitlines())


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
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-gitlab-")
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


class GitlabMrFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
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
            "gitlab-mr-flow must pass schema validation and add by its exact id",
        )
        self.recipe_add("git-pr-flow")
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        self.assertIn(
            "capability ambiguity: capability.id='vcs-pr-flow' "
            "declared by git-pr-flow, gitlab-mr-flow",
            sync.stderr,
            "gitlab-mr-flow must declare the vcs-pr-flow capability",
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
        hookcheck_id = "gitlab-mr-flow-hookcheck"
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
        """Recipe declares bundled gitlab-merge-workflow skill."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "The next sync will materialize:", result.stdout,
            "recipe add must print the declared primitives",
        )
        self.assertIn(
            "- skills: gitlab-merge-workflow", result.stdout,
            "gitlab-merge-workflow skill must be declared by the recipe",
        )

    def test_recipe_declares_mr_create_command(self):
        """Recipe declares mr-create command."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "- commands: mr-create", result.stdout,
            "mr-create command must be declared by the recipe",
        )

    def test_recipe_declares_readme_doc(self):
        """Recipe declares README.md doc provision."""
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            "- doc: README.md → ai-specs/recipes/gitlab-mr-flow/README.md",
            result.stdout,
            "README doc provision must be declared by the recipe",
        )

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
        """Project with gitlab-mr-flow added and synced; returns the sync result."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def test_materialize_produces_skill(self):
        """Sync materializes the bundled gitlab-merge-workflow skill."""
        self._synced_project()
        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

    def test_materialize_produces_command(self):
        """Sync materializes the mr-create command."""
        self._synced_project()
        cmd = self.cache() / "commands" / "mr-create.md"
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
            "git-pr-flow assets must not be materialized when only gitlab-mr-flow is enabled"
        )


class GitlabMrFlowBindingTests(_CliFixtureMixin, unittest.TestCase):
    """Provider binding semantics: ambiguity and explicit binding.

    Both real catalog recipes (git-pr-flow and gitlab-mr-flow) declare the
    vcs-pr-flow capability; the original in-process resolve_bindings calls are
    replaced by the binding behavior sync renders.
    """

    def _enable_both(self) -> None:
        self.recipe_add("git-pr-flow")
        self.recipe_add(RECIPE_ID)

    def test_dual_vcs_pr_flow_providers_stay_unbound_without_binding(self):
        """When both git-pr-flow and gitlab-mr-flow are enabled without bindings, vcs-pr-flow stays unbound."""
        self._enable_both()
        sync = self.sync()
        self.assertEqual(
            sync.returncode, 0,
            "unbound capability ambiguity is a warning, not a fatal conflict",
        )
        self.assertIn(
            "capability ambiguity: capability.id='vcs-pr-flow' "
            "declared by git-pr-flow, gitlab-mr-flow",
            sync.stderr,
            "vcs-pr-flow must stay unbound (auto-bind is forbidden with two providers)",
        )
        self.assertIn(
            "Add an explicit [[bindings]] entry to resolve",
            sync.stderr,
            "ambiguity warning must guide toward an explicit binding",
        )

    def test_explicit_binding_selects_gitlab(self):
        """Explicit binding to gitlab-mr-flow selects it for vcs-pr-flow."""
        self._enable_both()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text()
            + '\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "gitlab-mr-flow"\n'
        )
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        self.assertNotIn(
            "capability ambiguity: capability.id='vcs-pr-flow'",
            sync.stderr,
            "an explicit [[bindings]] entry must resolve the vcs-pr-flow ambiguity",
        )


class GitlabMrFlowGoldenContentTests(unittest.TestCase):
    """Phase 3: golden text checks for skill and command content."""

    @classmethod
    def setUpClass(cls):
        cls.skill_path = (
            CATALOG / RECIPE_ID / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        cls.command_path = CATALOG / RECIPE_ID / "commands" / "mr-create.md"
        cls.skill_text = cls.skill_path.read_text()
        cls.command_text = cls.command_path.read_text()

    # --- Skill golden content ---

    def test_skill_checks_glab_installed(self):
        """Skill checks that glab is installed via command -v glab."""
        self.assertIn("command -v glab", self.skill_text)

    def test_skill_checks_glab_auth(self):
        """Skill checks glab authentication via glab auth status."""
        self.assertIn("glab auth status", self.skill_text)

    def test_skill_uses_explicit_push(self):
        """Skill uses explicit git push -u $REMOTE before MR creation."""
        self.assertIn("git push -u $REMOTE", self.skill_text)

    def test_skill_uses_glab_mr_create_with_required_flags(self):
        """Skill creates MR with glab mr create and required flags."""
        self.assertIn("glab mr create", self.skill_text)
        self.assertIn("--source-branch", self.skill_text)
        self.assertIn("--target-branch", self.skill_text)
        self.assertIn("--title", self.skill_text)
        self.assertIn("--description", self.skill_text)
        self.assertIn("--yes", self.skill_text)

    def test_skill_merge_removes_source_branch(self):
        """Skill merge command includes --remove-source-branch for feature heads."""
        self.assertIn("--remove-source-branch", self.skill_text)
        self.assertIn("never pass --remove-source-branch", self.skill_text.lower())
        self.assertIn("Head branch class", self.skill_text)
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)
        self.assertIn("release/v", self.skill_text)

    def test_skill_merge_uses_yes_flag(self):
        """Skill merge command includes --yes to skip interactive prompt."""
        # Find the merge command context (after "glab mr merge")
        merge_pos = self.skill_text.find("glab mr merge")
        self.assertGreater(merge_pos, 0, "Skill must contain glab mr merge")
        merge_line = self.skill_text[merge_pos:self.skill_text.find("\n", merge_pos)]
        self.assertIn("--yes", merge_line)

    def test_skill_merge_pins_approved_sha(self):
        """Skill captures and pins the approved MR head SHA before merging."""
        self.assertIn("APPROVED_SHA", self.skill_text)
        self.assertIn("glab mr view", self.skill_text)
        merge_pos = self.skill_text.find("glab mr merge")
        self.assertGreater(merge_pos, 0, "Skill must contain glab mr merge")
        merge_line = self.skill_text[merge_pos:self.skill_text.find("\n", merge_pos)]
        self.assertIn("--sha", merge_line)

    def test_skill_worktree_cleanup_uses_absolute_path(self):
        """Skill worktree cleanup does not assume cwd is repo root."""
        self.assertNotIn(
            "git worktree remove .worktrees/",
            self.skill_text,
            "Skill must not use relative .worktrees/ path for worktree removal"
        )

    def test_skill_does_not_use_fill(self):
        """Skill does not use --fill (implicit push is forbidden)."""
        self.assertNotIn("--fill", self.skill_text)

    def test_skill_does_not_auto_merge(self):
        """Skill does not include auto-merge flags."""
        self.assertNotIn("--merge-when-pipeline-succeeds", self.skill_text)
        self.assertNotIn("auto-merge", self.skill_text.lower())

    # --- Command golden content ---

    def test_command_checks_glab_installed(self):
        """Command checks that glab is installed via command -v glab."""
        self.assertIn("command -v glab", self.command_text)

    def test_command_checks_glab_auth(self):
        """Command checks glab authentication via glab auth status."""
        self.assertIn("glab auth status", self.command_text)



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
        """Command uses explicit git push -u $REMOTE before MR creation."""
        self.assertIn("git push -u $REMOTE", self.command_text)

    def test_command_uses_glab_mr_create_with_required_flags(self):
        """Command creates MR with glab mr create and required flags."""
        self.assertIn("glab mr create", self.command_text)
        self.assertIn("--source-branch", self.command_text)
        self.assertIn("--target-branch", self.command_text)
        self.assertIn("--title", self.command_text)
        self.assertIn("--description", self.command_text)
        self.assertIn("--yes", self.command_text)

    def test_command_does_not_include_merge(self):
        """Command is create-only and does not include merge steps."""
        self.assertNotIn("glab mr merge", self.command_text)
        self.assertNotIn("--remove-source-branch", self.command_text)

    def test_command_does_not_use_fill(self):
        """Command does not use --fill (implicit push is forbidden)."""
        self.assertNotIn("--fill", self.command_text)

    def test_command_does_not_auto_merge(self):
        """Command does not include auto-merge flags."""
        self.assertNotIn("--merge-when-pipeline-succeeds", self.command_text)
        self.assertNotIn("auto-merge", self.command_text.lower())

    def test_command_push_before_create_order(self):
        """Command places git push before glab mr create."""
        push_pos = self.command_text.find("git push -u $REMOTE")
        create_pos = self.command_text.find("glab mr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before glab mr create in the command"
        )

    def test_skill_push_before_create_order(self):
        """Skill places git push before glab mr create."""
        push_pos = self.skill_text.find("git push -u $REMOTE")
        create_pos = self.skill_text.find("glab mr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before glab mr create in the skill"
        )

    # --- Runtime blocker messages (verify-report remediation) ---

    def test_skill_install_blocker_message(self):
        """Skill contains exact install blocker message when glab is missing."""
        self.assertIn(
            "glab` is not installed",
            self.skill_text,
            "Skill must contain install blocker message"
        )
        self.assertIn(
            "https://gitlab.com/gitlab-org/cli",
            self.skill_text,
            "Skill install blocker must include installation URL"
        )

    def test_skill_auth_blocker_message(self):
        """Skill contains exact auth blocker message when glab is unauthenticated."""
        self.assertIn(
            "glab` is not authenticated",
            self.skill_text,
            "Skill must contain auth blocker message"
        )
        self.assertIn(
            "glab auth login",
            self.skill_text,
            "Skill auth blocker must include remediation command"
        )

    def test_skill_preflight_before_push_order(self):
        """Skill checks glab install and auth BEFORE git push."""
        install_check_pos = self.skill_text.find("command -v glab")
        auth_check_pos = self.skill_text.find("glab auth status")
        push_pos = self.skill_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v glab in the skill"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER glab auth status in the skill"
        )

    def test_skill_stops_after_mr_create_reports_url(self):
        """Skill STOPs after MR creation and reports the MR URL."""
        create_pos = self.skill_text.find("glab mr create")
        stop_pos = self.skill_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER glab mr create in the skill"
        )
        self.assertIn(
            "Report the MR URL",
            self.skill_text,
            "Skill must instruct to report the MR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.skill_text,
            "Skill must explicitly say not to merge after MR creation"
        )

    def test_command_install_blocker_message(self):
        """Command contains exact install blocker message when glab is missing."""
        self.assertIn(
            "glab` is not installed",
            self.command_text,
            "Command must contain install blocker message"
        )
        self.assertIn(
            "https://gitlab.com/gitlab-org/cli",
            self.command_text,
            "Command install blocker must include installation URL"
        )

    def test_command_auth_blocker_message(self):
        """Command contains exact auth blocker message when glab is unauthenticated."""
        self.assertIn(
            "glab` is not authenticated",
            self.command_text,
            "Command must contain auth blocker message"
        )
        self.assertIn(
            "glab auth login",
            self.command_text,
            "Command auth blocker must include remediation command"
        )

    def test_command_preflight_before_push_order(self):
        """Command checks glab install and auth BEFORE git push."""
        install_check_pos = self.command_text.find("command -v glab")
        auth_check_pos = self.command_text.find("glab auth status")
        push_pos = self.command_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v glab in the command"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER glab auth status in the command"
        )

    def test_command_stops_after_mr_create_reports_url(self):
        """Command STOPs after MR creation and reports the MR URL."""
        create_pos = self.command_text.find("glab mr create")
        stop_pos = self.command_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER glab mr create in the command"
        )
        self.assertIn(
            "Report the MR URL",
            self.command_text,
            "Command must instruct to report the MR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.command_text,
            "Command must explicitly say not to merge after MR creation"
        )

    # --- jq preflight (R4 finding) ---

    def test_skill_checks_jq_installed(self):
        """Skill checks that jq is installed via command -v jq."""
        self.assertIn("command -v jq", self.skill_text)

    def test_command_checks_jq_installed(self):
        """Command checks that jq is installed via command -v jq."""
        self.assertIn("command -v jq", self.command_text)

    def test_skill_jq_blocker_message(self):
        """Skill contains jq blocker message when jq is missing."""
        self.assertIn(
            "jq` is not installed",
            self.skill_text,
            "Skill must contain jq install blocker message"
        )
        self.assertIn(
            "https://jqlang.github.io/jq/download/",
            self.skill_text,
            "Skill jq blocker must include installation URL"
        )

    def test_command_jq_blocker_message(self):
        """Command contains jq blocker message when jq is missing."""
        self.assertIn(
            "jq` is not installed",
            self.command_text,
            "Command must contain jq install blocker message"
        )
        self.assertIn(
            "https://jqlang.github.io/jq/download/",
            self.command_text,
            "Command jq blocker must include installation URL"
        )

    def test_skill_jq_preflight_before_push_order(self):
        """Skill checks jq BEFORE git push."""
        jq_check_pos = self.skill_text.find("command -v jq")
        push_pos = self.skill_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, jq_check_pos,
            "git push must appear AFTER command -v jq in the skill"
        )

    def test_command_jq_preflight_before_push_order(self):
        """Command checks jq BEFORE git push."""
        jq_check_pos = self.command_text.find("command -v jq")
        push_pos = self.command_text.find("git push -u $REMOTE")
        self.assertGreater(
            push_pos, jq_check_pos,
            "git push must appear AFTER command -v jq in the command"
        )

    # --- Dynamic remote resolution (R4 finding) ---

    def test_skill_uses_dynamic_remote_resolution(self):
        """Skill resolves the GitLab remote dynamically instead of hardcoding origin."""
        self.assertIn("REMOTE=$(git remote", self.skill_text)
        self.assertIn("git push -u $REMOTE", self.skill_text)

    def test_command_uses_dynamic_remote_resolution(self):
        """Command resolves the GitLab remote dynamically instead of hardcoding origin."""
        self.assertIn("REMOTE=$(git remote", self.command_text)
        self.assertIn("git push -u $REMOTE", self.command_text)


class GitlabMrFlowDualProviderTests(_CliFixtureMixin, unittest.TestCase):
    """End-to-end dual provider materialization with explicit bindings."""

    def _enable_dual(self, binding_recipe: str) -> None:
        """Enable both git-pr-flow and gitlab-mr-flow with an explicit binding."""
        self.recipe_add("git-pr-flow")
        self.recipe_add(RECIPE_ID)
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text()
            + f'\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "{binding_recipe}"\n'
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_dual_provider_gitlab_bound_materializes_both(self):
        """When bound to gitlab-mr-flow, both recipes materialize their assets (different IDs)."""
        self._enable_dual("gitlab-mr-flow")

        # GitLab assets should exist
        gitlab_skill = (
            self.cache() / ".recipe" / "gitlab-mr-flow"
            / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        gitlab_cmd = self.cache() / "commands" / "mr-create.md"
        self.assertTrue(gitlab_skill.is_file(), f"missing gitlab skill at {gitlab_skill}")
        self.assertTrue(gitlab_cmd.is_file(), f"missing gitlab command at {gitlab_cmd}")

        # GitHub assets should also exist (different IDs, no conflict)
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

        # GitHub assets should exist
        github_skill = (
            self.cache() / ".recipe" / "git-pr-flow"
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        github_cmd = self.cache() / "commands" / "pr-create.md"
        self.assertTrue(github_skill.is_file(), f"missing github skill at {github_skill}")
        self.assertTrue(github_cmd.is_file(), f"missing github command at {github_cmd}")

        # GitLab assets should also exist (different IDs, no conflict)
        gitlab_skill = (
            self.cache() / ".recipe" / "gitlab-mr-flow"
            / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        gitlab_cmd = self.cache() / "commands" / "mr-create.md"
        self.assertTrue(gitlab_skill.is_file(), f"missing gitlab skill at {gitlab_skill}")
        self.assertTrue(gitlab_cmd.is_file(), f"missing gitlab command at {gitlab_cmd}")


class GitlabMrFlowAccountExtractionTests(unittest.TestCase):
    """``glab auth status`` has no active-account marker, so the preflight may
    only trust the extracted login when exactly one is listed."""

    @classmethod
    def setUpClass(cls):
        cls.command_text = (
            CATALOG / RECIPE_ID / "commands" / "mr-create.md"
        ).read_text()
        cls.skill_text = (
            CATALOG / RECIPE_ID / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        ).read_text()
        cls.awk = extract_awk(cls.command_text, "Logged in to .* as")[0]

    def _active(self, status: str) -> str:
        return run_awk(self.awk, status).strip()

    def test_single_annotated_login_returns_account(self):
        status = (
            "gitlab.com\n"
            "  \u2713 Logged in to gitlab.com as solo "
            "(/home/u/.config/glab-cli/config.yml)\n"
        )
        self.assertEqual(self._active(status), "solo")

    def test_multiple_logins_return_empty(self):
        status = (
            "gitlab.com\n"
            "  \u2713 Logged in to gitlab.com as alice "
            "(/home/u/.config/glab-cli/config.yml)\n"
            "  \u2713 Logged in to gitlab.com as bob "
            "(/home/u/.config/glab-cli/config.yml)\n"
        )
        self.assertEqual(self._active(status), "")

    def test_multiple_hosts_return_empty(self):
        status = (
            "gitlab.com\n"
            "  \u2713 Logged in to gitlab.com as alice "
            "(/home/u/.config/glab-cli/config.yml)\n"
            "gitlab.example.com\n"
            "  \u2713 Logged in to gitlab.example.com as bob "
            "(/home/u/config.yml)\n"
        )
        self.assertEqual(self._active(status), "")

    def test_unrelated_status_lines_are_ignored(self):
        status = (
            "gitlab.com\n"
            "  \u2713 Logged in to gitlab.com as solo (/home/u/config.yml)\n"
            "  \u2713 API calls found at https://gitlab.com/api/v4\n"
        )
        self.assertEqual(self._active(status), "solo")

    def test_requires_exactly_one_login_in_awk(self):
        """No literal offsets and no per-match printing: the count decides."""
        self.assertNotIn("RSTART", self.awk)
        self.assertNotIn("RLENGTH", self.awk)
        self.assertIn("END", self.awk)
        self.assertIn("n == 1", self.awk)

    def test_all_copies_share_the_same_extraction(self):
        scripts = [
            _normalize(script)
            for text in (self.command_text, self.skill_text)
            for script in extract_awk(text, "Logged in to .* as")
        ]
        self.assertGreaterEqual(len(scripts), 2, scripts)
        self.assertEqual(len(set(scripts)), 1, scripts)


if __name__ == "__main__":
    unittest.main()

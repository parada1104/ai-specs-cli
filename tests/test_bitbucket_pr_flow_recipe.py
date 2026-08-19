import json
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "bitbucket-pr-flow"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import isolated_home, invoke, temp_project
from _cache_paths import cache_command, recipe_skill_dir, recipe_root


def _recipe_toml() -> dict:
    with (CATALOG / RECIPE_ID / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)


def _version() -> str:
    return _recipe_toml()["recipe"]["version"]


def _recipe_version(rid: str) -> str:
    with (CATALOG / rid / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)["recipe"]["version"]


def _enable_recipes(root: Path, *recipe_ids: str) -> Path:
    """Append the given recipes to an isolated project manifest (version pinned)."""
    manifest = root / "ai-specs" / "ai-specs.toml"
    for rid in recipe_ids:
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write(
                f'\n[recipes.{rid}]\nenabled = true\nversion = "{_recipe_version(rid)}"\n'
            )
    return root


class BitbucketPrFlowRecipeTests(unittest.TestCase):
    """Phase 1/2: manifest declarations and materialization via the CLI."""

    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="bbprf-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def _invoke(self, root: Path, *args: str):
        return invoke(root, *args, cli_home=self.home)

    def _sync(self, root: Path):
        r = self._invoke(root, "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    def _project(self) -> Path:
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        return _enable_recipes(root, RECIPE_ID)

    def _agents(self, root: Path) -> str:
        return (root / "AGENTS.md").read_text()

    # --- Phase 1: Manifest and Binding ---

    def test_recipe_validates_and_declares_vcs_pr_flow(self):
        """Recipe is valid and declares vcs-pr-flow capability."""
        root = self._project()
        result = self._invoke(root, "recipe", "list")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(RECIPE_ID, result.stdout)
        self.assertIn(_version(), result.stdout)

        self._sync(root)
        agents = self._agents(root)
        self.assertIn(
            "VCS/PR provider: Bitbucket (`bb` CLI); base branch: `development`",
            agents,
        )

        data = _recipe_toml()
        cap_ids = [c["id"] for c in data.get("capabilities", [])]
        self.assertIn("vcs-pr-flow", cap_ids)

    def test_recipe_has_no_provider_config(self):
        """Config must not declare provider — recipe id is the provider identity."""
        root = self._project()
        result = self._invoke(root, "recipe", "list")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(RECIPE_ID, result.stdout)

        cfg = self._invoke(root, "recipe", "configure", RECIPE_ID, "--inspect", "--json")
        self.assertEqual(cfg.returncode, 0, cfg.stderr)
        inspect = json.loads(cfg.stdout)
        field_keys = [f["key"] for f in inspect["schema"]["fields"]]
        self.assertNotIn(
            "provider",
            field_keys,
            "provider config field must not exist on sibling VCS recipes",
        )

    def test_recipe_declares_development_base_branch_default(self):
        """Config declares base_branch=development as default."""
        root = self._project()
        cfg = self._invoke(root, "recipe", "configure", RECIPE_ID, "--inspect", "--json")
        self.assertEqual(cfg.returncode, 0, cfg.stderr)
        inspect = json.loads(cfg.stdout)
        base = next(f for f in inspect["schema"]["fields"] if f["key"] == "base_branch")
        self.assertFalse(base["required"])
        self.assertEqual(base["default"], "development")

        self._sync(root)
        self.assertIn("base branch: `development`", self._agents(root))

    def test_recipe_declares_validate_config_hook(self):
        """Recipe declares on-sync validate-config hook; hook executes on sync."""
        data = _recipe_toml()
        hook_pairs = [(h["event"], h["action"]) for h in data.get("hooks", [])]
        self.assertIn(("on-sync", "validate-config"), hook_pairs)

        root = self._project()
        self._sync(root)

    def test_recipe_declares_bundled_skill(self):
        """Recipe declares bundled bitbucket-merge-workflow skill."""
        root = self._project()
        self._sync(root)
        skill = (
            recipe_skill_dir(root, RECIPE_ID, "bitbucket-merge-workflow", cli_home=self.home)
            / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        data = _recipe_toml()
        skill_ids = [(s["id"], s.get("source")) for s in data["provides"]["skills"]]
        self.assertIn(("bitbucket-merge-workflow", "bundled"), skill_ids)

    def test_recipe_declares_bb_pr_create_command(self):
        """Recipe declares bb-pr-create command."""
        root = self._project()
        self._sync(root)
        cmd = cache_command(root, "bb-pr-create", cli_home=self.home)
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        data = _recipe_toml()
        cmd_ids = [c["id"] for c in data["provides"]["commands"]]
        self.assertIn("bb-pr-create", cmd_ids)

    def test_recipe_declares_readme_doc(self):
        """Recipe declares README.md doc provision."""
        root = self._project()
        self._sync(root)
        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

        data = _recipe_toml()
        doc_targets = [d["target"] for d in data["provides"]["docs"]]
        self.assertIn("ai-specs/recipes/bitbucket-pr-flow/README.md", doc_targets)

    # --- Phase 2: Materialization ---

    def test_brief_surfaces_postmerge_sync_and_cleanup(self):
        """The always-on brief must surface both a post-merge base-sync rule
        (git pull --ff-only) and a post-merge cleanup rule."""
        root = self._project()
        self._sync(root)
        agents = self._agents(root)
        self.assertIn("git pull --ff-only", agents)
        self.assertTrue(
            any("worktree" in line and "merged" in line for line in agents.splitlines()),
            "post-merge cleanup workflow_rule missing",
        )

        data = _recipe_toml()
        brief = data["provides"].get("brief")
        self.assertIsNotNone(brief)
        rules = brief.get("workflow_rules") or []
        self.assertTrue(
            any("ff-only" in r.lower() for r in rules),
            "post-merge base-sync workflow_rule missing (git pull --ff-only)",
        )
        self.assertTrue(
            any("worktree" in r.lower() and "merged" in r.lower() for r in rules),
            "post-merge cleanup workflow_rule missing",
        )

    def test_materialize_produces_skill(self):
        """Sync materializes the bundled bitbucket-merge-workflow skill."""
        root = self._project()
        self._sync(root)
        skill = (
            recipe_skill_dir(root, RECIPE_ID, "bitbucket-merge-workflow", cli_home=self.home)
            / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

    def test_materialize_produces_command(self):
        """Sync materializes the bb-pr-create command."""
        root = self._project()
        self._sync(root)
        cmd = cache_command(root, "bb-pr-create", cli_home=self.home)
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

    def test_materialize_produces_readme(self):
        """Sync materializes the README doc."""
        root = self._project()
        self._sync(root)
        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_touch_github_assets(self):
        """Sync does not modify git-pr-flow recipe assets."""
        root = self._project()
        self._sync(root)
        github_skill = (
            recipe_root(root, "git-pr-flow", cli_home=self.home)
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertFalse(
            github_skill.exists(),
            "git-pr-flow assets must not be materialized when only bitbucket-pr-flow is enabled",
        )


class BitbucketPrFlowBindingTests(unittest.TestCase):
    """Provider binding semantics: ambiguity and explicit binding."""

    AMBIGUITY = (
        "capability ambiguity: capability.id='vcs-pr-flow' declared by "
        "bitbucket-pr-flow, git-pr-flow. Add an explicit [[bindings]] entry to resolve."
    )

    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="bbprf-bind-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def _invoke(self, root: Path, *args: str):
        return invoke(root, *args, cli_home=self.home)

    def _sync(self, root: Path):
        r = self._invoke(root, "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    def _dual_project(self, binding: str | None = None) -> Path:
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = _enable_recipes(root, "git-pr-flow", RECIPE_ID)
        if binding:
            manifest = root / "ai-specs" / "ai-specs.toml"
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write(
                    f'\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "{binding}"\n'
                )
        return root

    def test_dual_vcs_pr_flow_providers_stay_unbound_without_binding(self):
        """When both git-pr-flow and bitbucket-pr-flow are enabled without bindings,
        vcs-pr-flow stays unbound (non-fatal ambiguity) and no provider is selected."""
        root = self._dual_project()
        r = self._sync(root)
        self.assertIn(self.AMBIGUITY, r.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertNotIn("VCS/PR provider", agents)

    def test_explicit_binding_selects_bitbucket(self):
        """Explicit binding to bitbucket-pr-flow selects it for vcs-pr-flow."""
        root = self._dual_project(binding=RECIPE_ID)
        r = self._sync(root)
        self.assertNotIn("capability ambiguity", r.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn(
            "VCS/PR provider: Bitbucket (`bb` CLI); base branch: `development`",
            agents,
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

    # --- Skill golden content ---

    def test_skill_checks_bb_installed(self):
        """Skill checks that bb is installed via command -v bb."""
        self.assertIn("command -v bb", self.skill_text)

    def test_skill_checks_bb_auth(self):
        """Skill checks bb authentication via bb auth show."""
        self.assertIn("bb auth show", self.skill_text)

    def test_skill_uses_explicit_push(self):
        """Skill uses explicit git push -u $REMOTE before PR creation."""
        self.assertIn("git push -u $REMOTE", self.skill_text)

    def test_skill_uses_bb_pr_create_with_required_flags(self):
        """Skill creates PR with bb pr create and required flags."""
        self.assertIn("bb pr create", self.skill_text)
        self.assertIn("--source", self.skill_text)
        self.assertIn("--destination", self.skill_text)
        self.assertIn("--title", self.skill_text)
        self.assertIn("--body", self.skill_text)

    def test_skill_merge_closes_source_branch(self):
        """Skill merge command includes --close-source-branch for feature heads."""
        self.assertIn("--close-source-branch", self.skill_text)
        self.assertIn("never pass --close-source-branch", self.skill_text.lower())
        self.assertIn("Head branch class", self.skill_text)
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)
        self.assertIn("release/v", self.skill_text)

    def test_skill_merge_uses_squash_strategy(self):
        """Skill merge command uses squash strategy."""
        merge_pos = self.skill_text.find("bb pr merge")
        self.assertGreater(merge_pos, 0, "Skill must contain bb pr merge")
        merge_line = self.skill_text[merge_pos:self.skill_text.find("\n", merge_pos)]
        self.assertIn("--strategy squash", merge_line)

    def test_skill_merge_pins_approved_sha(self):
        """Skill captures and verifies the approved PR source commit before merging."""
        self.assertIn("APPROVED_SHA", self.skill_text)
        self.assertIn("bb pr view", self.skill_text)
        self.assertIn("CURRENT_SHA", self.skill_text)

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



    def test_skill_requires_pre_merge_archive_before_merge(self):
        """Skill archives SDD/OpenSpec artifacts before provider merge."""
        merge_pos = self.skill_text.find("bb pr merge")
        archive_pos = self.skill_text.find("archive and record SDD/OpenSpec artifacts")
        self.assertGreater(archive_pos, 0)
        self.assertGreater(merge_pos, 0)
        self.assertLess(archive_pos, merge_pos)

    def test_command_uses_explicit_push(self):
        """Command uses explicit git push -u $REMOTE before PR creation."""
        self.assertIn("git push -u $REMOTE", self.command_text)

    def test_command_uses_bb_pr_create_with_required_flags(self):
        """Command creates PR with bb pr create and required flags."""
        self.assertIn("bb pr create", self.command_text)
        self.assertIn("--source", self.command_text)
        self.assertIn("--destination", self.command_text)
        self.assertIn("--title", self.command_text)
        self.assertIn("--body", self.command_text)

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
            "https://bitbucket-cli.paulvanderlei.com/getting-started/installation/",
            self.skill_text,
            "Skill install blocker must include installation URL"
        )

    def test_skill_auth_blocker_message(self):
        """Skill contains exact auth blocker message when bb is unauthenticated."""
        self.assertIn(
            "bb` is not authenticated",
            self.skill_text,
            "Skill must contain auth blocker message"
        )
        self.assertIn(
            "bb auth login",
            self.skill_text,
            "Skill auth blocker must include remediation command"
        )

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
            "https://bitbucket-cli.paulvanderlei.com/getting-started/installation/",
            self.command_text,
            "Command install blocker must include installation URL"
        )

    def test_command_auth_blocker_message(self):
        """Command contains exact auth blocker message when bb is unauthenticated."""
        self.assertIn(
            "bb` is not authenticated",
            self.command_text,
            "Command must contain auth blocker message"
        )
        self.assertIn(
            "bb auth login",
            self.command_text,
            "Command auth blocker must include remediation command"
        )

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


class BitbucketPrFlowDualProviderTests(unittest.TestCase):
    """End-to-end dual provider materialization with explicit bindings."""

    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="bbprf-dual-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def _invoke(self, root: Path, *args: str):
        return invoke(root, *args, cli_home=self.home)

    def _sync(self, root: Path):
        r = self._invoke(root, "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    def _make_dual_project(self, binding_recipe: str) -> Path:
        """Create a project with both git-pr-flow and bitbucket-pr-flow enabled, with explicit binding."""
        td, root = temp_project(name="dual-fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = _enable_recipes(root, "git-pr-flow", RECIPE_ID)
        manifest = root / "ai-specs" / "ai-specs.toml"
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write(
                f'\n[[bindings]]\ncapability = "vcs-pr-flow"\nrecipe = "{binding_recipe}"\n'
            )
        return root

    def test_dual_provider_bitbucket_bound_materializes_both(self):
        """When bound to bitbucket-pr-flow, both recipes materialize their assets (different IDs)."""
        root = self._make_dual_project(RECIPE_ID)
        self._sync(root)

        bitbucket_skill = (
            recipe_root(root, RECIPE_ID, cli_home=self.home)
            / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        bitbucket_cmd = cache_command(root, "bb-pr-create", cli_home=self.home)
        self.assertTrue(bitbucket_skill.is_file(), f"missing bitbucket skill at {bitbucket_skill}")
        self.assertTrue(bitbucket_cmd.is_file(), f"missing bitbucket command at {bitbucket_cmd}")

        github_skill = (
            recipe_root(root, "git-pr-flow", cli_home=self.home)
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        github_cmd = cache_command(root, "pr-create", cli_home=self.home)
        self.assertTrue(github_skill.is_file(), f"missing github skill at {github_skill}")
        self.assertTrue(github_cmd.is_file(), f"missing github command at {github_cmd}")

    def test_dual_provider_github_bound_materializes_both(self):
        """When bound to git-pr-flow, both recipes materialize their assets (different IDs)."""
        root = self._make_dual_project("git-pr-flow")
        self._sync(root)

        github_skill = (
            recipe_root(root, "git-pr-flow", cli_home=self.home)
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        github_cmd = cache_command(root, "pr-create", cli_home=self.home)
        self.assertTrue(github_skill.is_file(), f"missing github skill at {github_skill}")
        self.assertTrue(github_cmd.is_file(), f"missing github command at {github_cmd}")

        bitbucket_skill = (
            recipe_root(root, RECIPE_ID, cli_home=self.home)
            / "skills" / "bitbucket-merge-workflow" / "SKILL.md"
        )
        bitbucket_cmd = cache_command(root, "bb-pr-create", cli_home=self.home)
        self.assertTrue(bitbucket_skill.is_file(), f"missing bitbucket skill at {bitbucket_skill}")
        self.assertTrue(bitbucket_cmd.is_file(), f"missing bitbucket command at {bitbucket_cmd}")


if __name__ == "__main__":
    unittest.main()

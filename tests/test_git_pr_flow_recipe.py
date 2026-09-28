"""Black-box git-pr-flow recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema validation and declared primitives, materialized
artifacts, guardian placement, golden skill content, account-extraction awk)
through the CLI process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + "The next sync will materialize:" plan) and via
  ``sync`` (the vcs-pr-flow capability ambiguity warning pairing this recipe
  with the sibling provider, plus materialized artifacts under the per-project
  CLI cache).
- The canonical pre-merge guardian is observable through the isolated CLI-home
  layout (lib/_internal copy), never through in-process imports.
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
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "git-pr-flow"


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
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-gitpr-")
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


class GitPrFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
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

    def test_recipe_validates_and_declares_capability(self):
        """Recipe validates under its exact id and declares the vcs-pr-flow
        capability plus its bundled skill and command.

        ``recipe add`` validates the schema and prints the declared primitives;
        pairing the recipe with the sibling vcs-pr-flow provider makes sync
        surface the capability declaration as an ambiguity warning naming both.
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "git-pr-flow must pass schema validation and add by its exact id",
        )
        self.assertIn(
            "- skills: git-merge-workflow", result.stdout,
            "bundled git-merge-workflow skill must be declared by the recipe",
        )
        self.assertIn(
            "- commands: pr-create", result.stdout,
            "pr-create command must be declared by the recipe",
        )
        self.recipe_add("gitlab-mr-flow")
        sync = self.sync()
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        self.assertIn(
            "capability ambiguity: capability.id='vcs-pr-flow' "
            "declared by git-pr-flow, gitlab-mr-flow",
            sync.stderr,
            "git-pr-flow must declare the vcs-pr-flow capability",
        )

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
        """Project with git-pr-flow added and synced; returns the sync result."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def test_materialize_produces_skill_command_and_doc(self):
        """Sync materializes the bundled skill, command, and README doc."""
        self._synced_project()

        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = self.cache() / "commands" / "pr-create.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = self.root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_stage_premerge_guardian_into_project(self):
        """Sync never stages the Plan Build guardian into the project."""
        self._synced_project()
        helper = self.root / "ai-specs" / "bin" / "premerge_guardian.py"
        self.assertFalse(helper.exists(), f"unexpected in-project guardian at {helper}")
        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        text = skill.read_text()
        # W4: the VCS skill must not carry or invoke the Plan Build artifact
        # guardian; the guardian stays a CLI-home, Plan Build-only helper.
        self.assertNotIn("premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)

    def test_canonical_guardian_lives_in_cli_home(self):
        """The canonical guardian ships in the CLI home's lib/_internal, never
        in the project. Observable through the isolated CLI-home layout the
        CLI actually runs against."""
        canon = self.home / "lib" / "_internal" / "premerge_guardian.py"
        self.assertTrue(canon.is_file())
        self.assertIn("pre-merge guardian", canon.read_text().lower())


class GitPrFlowGoldenContentTests(unittest.TestCase):
    """Golden text checks for pre-merge archive guidance."""

    @classmethod
    def setUpClass(cls):
        cls.skill_path = (
            CATALOG / RECIPE_ID / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        cls.skill_text = cls.skill_path.read_text()

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

    def test_skill_requires_native_post_merge_cleanup_sequence(self):
        """Skill delegates ordered branch/worktree cleanup to the Go command."""
        self.assertIn("worktree-cleanup.sh", self.skill_text)
        self.assertIn("git pull --ff-only", self.skill_text)
        self.assertIn("base sync is deliberately LAST", self.skill_text)

    def test_skill_documents_the_implemented_cleanup_order(self):
        """The prose order must match what the Go command actually does.

        Asserting that each phrase merely appears somewhere cannot catch a
        reordered sequence, and the order is the whole point of this step: the
        remote branch is deleted before the local one so an unreachable remote
        leaves a retry handle behind, and the base sync runs last.
        """
        owns = self.skill_text.find("The cleanup command owns, in this order,")
        self.assertGreater(owns, 0, "cleanup ownership sentence is missing")
        # The prose is hard-wrapped, so phrases straddle line breaks.
        sentence = " ".join(self.skill_text[owns : owns + 400].split())
        steps = [
            "merged-worktree removal",
            "remote branch deletion",
            "local branch removal",
        ]
        positions = [sentence.find(step) for step in steps]
        for step, pos in zip(steps, positions):
            self.assertGreater(pos, -1, f"cleanup step not documented: {step}")
        self.assertEqual(
            positions,
            sorted(positions),
            f"documented cleanup order does not match the implementation: {steps}",
        )
        self.assertLess(
            self.skill_text.find("local branch removal"),
            self.skill_text.find("git pull --ff-only"),
            "base sync must be documented after branch cleanup",
        )

    def test_skill_rejects_path_presence_as_merge_evidence(self):
        """Stale-branch deletion needs content proof, never a matching name."""
        self.assertNotIn("path-presence proof", self.skill_text)
        self.assertIn(
            "A same-named path existing on the base is not evidence",
            self.skill_text,
        )

    def test_skill_never_recommends_delete_branch(self):
        """Provider-side source deletion is forbidden in this worktree layout."""
        self.assertNotIn("--delete-branch", self.skill_text)
        self.assertIn("without asking the hosting provider to delete", self.skill_text)

    def test_skill_classifies_protected_heads(self):
        """Skill retains protected-head classification and guardrails."""
        self.assertIn("Head branch class", self.skill_text)
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)
        self.assertIn("Protected heads", self.skill_text)

    def test_skill_preflight_checks_delete_branch_on_merge(self):
        """Skill warns when GitHub auto-deletes heads on merge."""
        self.assertIn("delete_branch_on_merge", self.skill_text)
        self.assertIn(
            "gh api -X PATCH repos/$REPO -f delete_branch_on_merge=false",
            self.skill_text,
        )

    def test_skill_prefers_release_head_for_main(self):
        """Skill documents release/* heads for shipping to main."""
        self.assertIn("release/v", self.skill_text)


class GitPrFlowAccountExtractionTests(unittest.TestCase):
    """The account-match awk must read the ``Active account: true`` entry and
    return the bare account token, not annotation noise or literal offsets."""

    @classmethod
    def setUpClass(cls):
        cls.command_text = (
            CATALOG / RECIPE_ID / "commands" / "pr-create.md"
        ).read_text()
        cls.skill_text = (
            CATALOG / RECIPE_ID / "skills" / "git-merge-workflow" / "SKILL.md"
        ).read_text()
        cls.awk = extract_awk(cls.command_text, "Active account: true")[0]

    def _active(self, status: str) -> str:
        lines = run_awk(self.awk, status).strip().splitlines()
        return lines[0] if lines else ""

    def test_single_annotated_account_returns_clean_name(self):
        status = (
            "github.com\n"
            "  \u2713 Logged in to github.com account solo (keyring)\n"
            "  - Active account: true\n"
            "  - Git operations protocol: https\n"
        )
        self.assertEqual(self._active(status), "solo")

    def test_unannotated_login_returns_account(self):
        status = (
            "github.com\n"
            "  \u2713 Logged in to github.com account solo\n"
            "  - Active account: true\n"
        )
        self.assertEqual(self._active(status), "solo")

    def test_multi_account_returns_the_active_one(self):
        status = (
            "github.com\n"
            "  \u2713 Logged in to github.com account alice (keyring)\n"
            "  - Active account: false\n"
            "\n"
            "  \u2713 Logged in to github.com account bob (keyring)\n"
            "  - Active account: true\n"
        )
        self.assertEqual(self._active(status), "bob")

    def test_without_active_marker_returns_nothing(self):
        status = (
            "github.com\n"
            "  \u2713 Logged in to github.com account alice (keyring)\n"
            "  - Active account: false\n"
        )
        self.assertEqual(self._active(status), "")

    def test_multi_host_returns_the_first_active_account(self):
        status = (
            "github.com\n"
            "  \u2713 Logged in to github.com account alice (keyring)\n"
            "  - Active account: true\n"
            "\n"
            "ghe.example.com\n"
            "  \u2713 Logged in to ghe.example.com account bob (keyring)\n"
            "  - Active account: true\n"
        )
        self.assertEqual(self._active(status), "alice")

    def test_uses_field_based_extraction_not_literal_offsets(self):
        """Regression: offsets drifted with the match text; require the token
        following ``account`` instead."""
        self.assertNotIn("RSTART", self.awk)
        self.assertNotIn("RLENGTH", self.awk)
        self.assertIn('$i == "account"', self.awk)

    def test_all_copies_share_the_same_extraction(self):
        """Command and skill both read, then re-read after ``gh auth switch``;
        every copy must stay in sync."""
        scripts = [
            _normalize(script)
            for text in (self.command_text, self.skill_text)
            for script in extract_awk(text, "Active account: true")
        ]
        self.assertGreaterEqual(len(scripts), 2, scripts)
        self.assertEqual(len(set(scripts)), 1, scripts)


if __name__ == "__main__":
    unittest.main()

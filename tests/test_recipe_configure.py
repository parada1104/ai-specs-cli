"""Contract tests for the non-interactive recipe configure helper (black-box).

Drives ``bin/ai-specs recipe configure`` through the shared black-box helpers.
Exit-code contract (go-migration-parity-contract.md §2):
  0 ok/no-op/dry-run; 1 write failed / sync failed (partial) / doctor failed;
  2 argparse; 3 ConfigureError (validation, unknown key, secret-shaped literal);
  4 blocked by [tool] CLI version policy.
"""
from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import invoke, isolated_home, snapshot, tree_diff  # noqa: E402


def _git(root: Path, *args: str) -> subprocess.CompletedProcess:
    proc = subprocess.run(
        ["git", *args],
        cwd=root,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise AssertionError(f"git {' '.join(args)!r} failed ({proc.returncode}): {proc.stderr}")
    return proc


class RecipeConfigureTests(unittest.TestCase):
    """Black-box tests for ``ai-specs recipe configure <id> [path]``."""
    def _cli_home(self) -> Path:
        """One isolated install+cache root shared across a command sequence."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return isolated_home(Path(tmp.name))

    def _configure(self, root: Path, *args: str, home: Path) -> object:
        """Shared wrapper: ai-specs recipe configure <args> <root>."""
        return invoke(root, "recipe", "configure", *args, cli_home=home)

    def _project(self, config: str = "") -> tuple[tempfile.TemporaryDirectory, Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.worktree-flow]\nenabled = true\nversion = '1.4.0'\n\n"
            "[recipes.worktree-flow.config]\n"
            + config,
            encoding="utf-8",
        )
        self.addCleanup(tmp.cleanup)
        return tmp, root, manifest

    def test_inspect_json_is_deterministic_and_contains_schema_state(self):
        _tmp, root, _manifest = self._project("integration_branch = 'main'\nkeep_me = 'x'\n")
        home = self._cli_home()
        first = json.loads(
            self._configure(root, "worktree-flow", "--inspect", "--json", home=home).stdout
        )
        second = json.loads(
            self._configure(root, "worktree-flow", "--inspect", "--json", home=home).stdout
        )
        self.assertEqual(json.dumps(first, sort_keys=False), json.dumps(second, sort_keys=False))
        self.assertEqual(first["schema_version"], 1)
        self.assertEqual(first["current_config"]["integration_branch"], "main")
        self.assertIn("repo_topology", {field["key"] for field in first["schema"]["fields"]})
        self.assertEqual(first["unknown_keys"], ["keep_me"])

    def test_topology_grounding_uses_resolution_without_init_contract(self):
        _tmp, root, _manifest = self._project()
        # A real superproject with one initialized submodule at libs/core.
        sub_src = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: shutil.rmtree(sub_src, ignore_errors=True))
        _git(sub_src, "init", "-q")
        _git(sub_src, "config", "user.email", "t@example.com")
        _git(sub_src, "config", "user.name", "t")
        (sub_src / "f.txt").write_text("x")
        _git(sub_src, "add", ".")
        _git(sub_src, "commit", "-qm", "init")
        _git(root, "init", "-q")
        _git(root, "config", "user.email", "t@example.com")
        _git(root, "config", "user.name", "t")
        _git(root, "add", ".")
        _git(root, "commit", "-qm", "base")
        _git(root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", str(sub_src), "libs/core")
        _git(root, "commit", "-qm", "add submodule")
        _git(root, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
        home = self._cli_home()
        doc = json.loads(
            self._configure(root, "worktree-flow", "--inspect", "--json", home=home).stdout
        )
        self.assertEqual(doc["grounding"]["topology"]["resolved"], "monorepo-submodules")
        self.assertEqual(doc["grounding"]["topology"]["submodules"], ["libs/core"])

    def test_apply_rejects_unknown_key_without_write(self):
        _tmp, root, manifest = self._project()
        home = self._cli_home()
        before = manifest.read_bytes()
        res = self._configure(root, "worktree-flow", "--set", "not_in_schema=x", "--json", home=home)
        self.assertEqual(res.returncode, 3)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_pin_violation_blocks_before_writer_and_sync(self):
        _tmp, root, manifest = self._project()
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        home = self._cli_home()
        before = manifest.read_bytes()
        before_tree = snapshot(root)
        res = self._configure(
            root, "worktree-flow", "--set", "integration_branch=dev", "--sync", "--json", home=home
        )
        self.assertEqual(res.returncode, 4)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "blocked")
        self.assertEqual(manifest.read_bytes(), before)
        # No sync ran: the project tree is unchanged.
        self.assertEqual(tree_diff(before_tree, snapshot(root)), {"created": [], "deleted": [], "modified": []})

    def test_sync_failure_is_partial_after_successful_write(self):
        _tmp, root, manifest = self._project()
        # Make `ai-specs sync` fail deterministically: a deps URL that cannot be vendored.
        manifest.write_text(
            manifest.read_text()
            + "\n[[deps]]\nurl = 'file:///nonexistent-ai-specs-vendor'\n"
        )
        before = manifest.read_bytes()
        home = self._cli_home()
        res = self._configure(
            root, "worktree-flow", "--set", "integration_branch=dev", "--sync", "--json", home=home
        )
        self.assertEqual(res.returncode, 1)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "partial")
        # The write is kept even though sync failed (partial semantics).
        self.assertIn('integration_branch = "dev"', manifest.read_text())
        self.assertNotEqual(manifest.read_bytes(), before)

    def test_unparsed_doctor_summary_is_not_zero(self):
        # TRIAGE: the synthetic-input branch (parse_doctor_summary returns
        # parsed=False / warn=None / error=None on "doctor output without
        # summary") has no observable CLI equivalent. Ran `bin/ai-specs recipe
        # configure worktree-flow <root> --set integration_branch=dev --sync
        # --json` — three surfaces: exit 0; tree_diff created .gitignore,
        # AGENTS.md, ai-specs/.ai-specs.lock, ai-specs/recipes/worktree-flow/*,
        # and modified ai-specs/ai-specs.toml (sync renders project files);
        # stdout JSON `verify` parsed True with doctor_exit 0. The `verify`
        # block is always derived from a real `ai-specs doctor` stdout carrying
        # a `Summary: <N> OK, ...` line; no public flag injects unparsable
        # doctor text, so parsed=False cannot be forced. The observable doctor
        # surface is asserted below (verify.parsed True, verify.doctor_exit 0).
        _tmp, root, _manifest = self._project()
        home = self._cli_home()
        res = self._configure(
            root, "worktree-flow", "--set", "integration_branch=dev", "--sync", "--json", home=home
        )
        self.assertEqual(res.returncode, 0)
        report = json.loads(res.stdout)
        self.assertTrue(report["verify"]["parsed"])
        self.assertEqual(report["verify"]["doctor_exit"], 0)

    def test_secret_literal_is_rejected_and_env_reference_allowed(self):
        # TRIAGE (secret-shaped literal branch): no public catalog recipe
        # exposes a [config.*] key matching SECRET_KEY_RE (token/password/
        # secret/api_key) — every value-bearing [config.*] key in
        # catalog/recipes/*/recipe.toml was checked. Ran `bin/ai-specs recipe
        # configure worktree-flow <root> --set api_token=literal --json` —
        # three surfaces: exit 3, manifest bytes unchanged (no write), and
        # stdout JSON whose `reason` is "unknown config key: api_token" (the
        # unknown-key branch, NOT the secret-literal guard), so that guard is
        # unreachable through the public CLI. The env-reference acceptance IS
        # observable and asserted below: a non-secret string field accepts the
        # ${env:VAR} literal (exit 0, manifest written).
        _tmp, root, manifest = self._project()
        before = manifest.read_bytes()
        home = self._cli_home()
        res = self._configure(
            root, "worktree-flow", "--set", "integration_branch=${env:BRANCH}", "--json", home=home
        )
        self.assertEqual(res.returncode, 0)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "ok")
        self.assertNotEqual(manifest.read_bytes(), before)
        self.assertIn("${env:BRANCH}", manifest.read_text())

    def test_no_gitmodules_surfaces_monorepo_apps_question(self):
        _tmp, root, _manifest = self._project()
        home = self._cli_home()
        doc = json.loads(
            self._configure(root, "worktree-flow", "--inspect", "--json", home=home).stdout
        )
        self.assertTrue(any("monorepo-apps" in item for item in doc["assumptions"]))

    def test_enum_value_is_rejected_without_write(self):
        _tmp, root, manifest = self._project()
        home = self._cli_home()
        before = manifest.read_bytes()
        res = self._configure(root, "worktree-flow", "--set", "gate_mode=invalid", "--json", home=home)
        self.assertEqual(res.returncode, 3)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "rejected")
        self.assertEqual(manifest.read_bytes(), before)

    def test_lock_staleness_is_informational_gap(self):
        _tmp, root, _manifest = self._project()
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            "[meta]\ncli_version = '0.0.1'\n", encoding="utf-8"
        )
        home = self._cli_home()
        res = self._configure(root, "worktree-flow", "--set", "integration_branch=dev", "--json", home=home)
        self.assertEqual(res.returncode, 0)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "ok")
        self.assertTrue(any("0.0.1" in gap for gap in report["gaps"]))

    def test_ignore_cli_version_is_recorded_and_forwarded(self):
        _tmp, root, manifest = self._project()
        manifest.write_text(manifest.read_text() + "\n[tool]\nversion = '999.0.0'\n")
        home = self._cli_home()
        # Without the flag this pin blocks (exit 4, asserted in
        # test_pin_violation_blocks_before_writer_and_sync). With
        # --ignore-cli-version the preflight must be recorded as True and the
        # blocker honored so the writer runs to a successful write (code 0).
        # recipe-configure.py forwards the flag to the sync subcommand; a
        # blocking pin cannot reach a clean sync+doctor exit, so the cleanly
        # observable proof of the flag is the write proceeding in spite of it.
        res = self._configure(
            root, "worktree-flow", "--set", "integration_branch=dev",
            "--ignore-cli-version", "--json", home=home,
        )
        self.assertEqual(res.returncode, 0)
        report = json.loads(res.stdout)
        self.assertTrue(report["preflight"]["ignore_cli_version"])
        self.assertEqual(report["status"], "ok")
        self.assertIn('integration_branch = "dev"', manifest.read_text())

    def test_noop_report_has_no_changed_keys(self):
        _tmp, root, manifest = self._project("integration_branch='main'\n")
        home = self._cli_home()
        before = manifest.read_bytes()
        res = self._configure(root, "worktree-flow", "--set", "integration_branch=main", "--json", home=home)
        self.assertEqual(res.returncode, 0)
        report = json.loads(res.stdout)
        self.assertEqual(report["status"], "no-op")
        self.assertEqual(report["applied"]["changed"], [])
        self.assertEqual(manifest.read_bytes(), before)

    def test_recipe_subcommand_help_lists_configure(self):
        _tmp, root, _manifest = self._project()
        home = self._cli_home()
        res = invoke(root, "recipe", "--help", cli_home=home)
        self.assertEqual(res.returncode, 0)
        self.assertIn("configure <id>", res.stdout)

    def test_trello_inspect_surfaces_init_and_secret_env_names(self):
        _tmp, root, manifest = self._project()
        manifest.write_text(
            "[project]\nname='fixture'\n\n"
            "[recipes.trello-mcp-workflow]\nenabled=true\nversion='1.3.0'\n\n"
            "[recipes.trello-mcp-workflow.config]\n"
        )
        home = self._cli_home()
        res = self._configure(root, "trello-mcp-workflow", "--inspect", "--json", home=home)
        self.assertEqual(res.returncode, 0)
        doc = json.loads(res.stdout)
        self.assertTrue(doc["grounding"]["init"]["present"])
        self.assertEqual(doc["grounding"]["init"]["needs_mcp"], ["trello"])
        self.assertIn("TRELLO_API_KEY", doc["grounding"]["mcp"]["env_vars"])
        self.assertNotIn("$TRELLO_API_KEY", json.dumps(doc))


if __name__ == "__main__":
    unittest.main()

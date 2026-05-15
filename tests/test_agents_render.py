"""Tests for lib/_internal/agents-render.py.

Covers the sub_agents OFF and ON branches, orphan detection, idempotency,
the .new sidecar pattern for user-modified files, fallback for harnesses
without native subagent support, and lock-file round trips.
"""

from __future__ import annotations

import hashlib
import importlib.util
import io
import shutil
import sys
import tempfile
import textwrap
import unittest
from contextlib import redirect_stdout, redirect_stderr
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
AGENTS_RENDER_PATH = ROOT / "lib" / "_internal" / "agents-render.py"
BUNDLED_AGENTS_CLAUDE = ROOT / "bundled-agents" / "claude"

SDD_NAMES = (
    "sdd-explore",
    "sdd-proposal",
    "sdd-artifacts",
    "sdd-apply",
    "sdd-verify",
    "sdd-archive",
)


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class AgentsRenderTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_internal")

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.project = Path(tmp.name) / "project"
        (self.project / "ai-specs").mkdir(parents=True)

    def write_manifest(self, text: str) -> Path:
        path = self.project / "ai-specs" / "ai-specs.toml"
        path.write_text(textwrap.dedent(text).lstrip("\n"))
        return path

    def claude_dir(self) -> Path:
        return self.project / ".claude" / "agents"

    def run_render(self) -> tuple[int, str, str]:
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = self.mod.render(self.project, ROOT)
        return code, out.getvalue(), err.getvalue()

    def assert_six_subagents_present(self) -> None:
        for name in SDD_NAMES:
            self.assertTrue(
                (self.claude_dir() / f"{name}.md").is_file(),
                f"expected materialized {name}.md",
            )

    # --- ON branch -----------------------------------------------------------

    def test_on_with_claude_only_materializes_six_files(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = true
            """
        )
        code, _stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assert_six_subagents_present()

    def test_on_byte_identical_to_bundled_source(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = true
            """
        )
        self.run_render()
        for name in SDD_NAMES:
            mat = self.claude_dir() / f"{name}.md"
            src = BUNDLED_AGENTS_CLAUDE / f"{name}.md"
            self.assertEqual(
                mat.read_bytes(),
                src.read_bytes(),
                f"materialized {name}.md must match bundled source byte-for-byte",
            )

    def test_on_mixed_harnesses_only_materializes_claude_and_logs_fallback(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude", "opencode"]
            [sdd]
            sub_agents = true
            """
        )
        code, stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assert_six_subagents_present()
        self.assertFalse((self.project / ".opencode" / "agents").exists())
        self.assertIn("opencode", stdout)
        self.assertIn("inline", stdout)

    def test_on_unsupported_harness_only_writes_nothing(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["opencode"]
            [sdd]
            sub_agents = true
            """
        )
        code, stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assertFalse(self.claude_dir().exists())
        self.assertIn("opencode", stdout)

    # --- OFF branch ----------------------------------------------------------

    def test_off_does_not_create_claude_agents(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            """
        )
        code, _stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assertFalse(self.claude_dir().exists())

    def test_off_explicit_false_does_not_create_claude_agents(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = false
            """
        )
        code, _stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assertFalse(self.claude_dir().exists())

    def test_off_with_existing_files_warns_about_orphans_without_deleting(self):
        # First, materialize files with sub_agents on.
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = true
            """
        )
        self.run_render()
        self.assert_six_subagents_present()

        # Then turn off and rerun.
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = false
            """
        )
        code, stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assert_six_subagents_present()  # NOT deleted
        self.assertIn("sub_agents is off", stdout)
        self.assertIn("subagent files exist", stdout)
        for name in SDD_NAMES:
            self.assertIn(name, stdout)

    # --- Invalid input -------------------------------------------------------

    def test_non_boolean_sub_agents_fails_with_clear_message(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = "true"
            """
        )
        code, _stdout, stderr = self.run_render()
        self.assertNotEqual(code, 0)
        self.assertIn("[sdd].sub_agents", stderr)
        self.assertIn("boolean", stderr)

    # --- Idempotency ---------------------------------------------------------

    def test_idempotent_two_runs_produce_identical_files_and_lock(self):
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = true
            """
        )
        self.run_render()
        lock_path = self.project / "ai-specs" / ".ai-specs.lock"
        first_lock = lock_path.read_bytes()
        first_hashes = {
            n: (self.claude_dir() / f"{n}.md").read_bytes() for n in SDD_NAMES
        }
        code, stdout, _stderr = self.run_render()
        self.assertEqual(code, 0)
        self.assertEqual(lock_path.read_bytes(), first_lock)
        for name in SDD_NAMES:
            self.assertEqual(
                (self.claude_dir() / f"{name}.md").read_bytes(),
                first_hashes[name],
            )
        # Second run should report up-to-date, not re-install.
        self.assertNotIn("installed", stdout)

    # --- User-modified file → sidecar ---------------------------------------

    def test_user_modified_file_with_upstream_change_produces_new_sidecar(self):
        # First pass: materialize the catalog as-is.
        self.write_manifest(
            """
            [project]
            name = "fixture"
            [agents]
            enabled = ["claude"]
            [sdd]
            sub_agents = true
            """
        )
        self.run_render()

        # Simulate user customization.
        target = self.claude_dir() / "sdd-explore.md"
        target.write_text(target.read_text() + "\n\n# user edit\n")

        # Simulate upstream change by patching the bundled source in a copy
        # of the CLI tree. We can't mutate the real one, so we copy the tree
        # and run render against the copy.
        with tempfile.TemporaryDirectory() as cli_copy_dir:
            cli_copy = Path(cli_copy_dir) / "cli"
            shutil.copytree(ROOT / "bundled-agents", cli_copy / "bundled-agents")
            shutil.copytree(ROOT / "lib", cli_copy / "lib")
            # Mutate the upstream copy.
            mutated = cli_copy / "bundled-agents" / "claude" / "sdd-explore.md"
            mutated.write_text(mutated.read_text() + "\n\n# upstream moved\n")

            out, err = io.StringIO(), io.StringIO()
            with redirect_stdout(out), redirect_stderr(err):
                code = self.mod.render(self.project, cli_copy)
            self.assertEqual(code, 0)

        # The user's edit must survive, and a sidecar must surface the upstream.
        self.assertIn("user edit", target.read_text())
        sidecar = target.with_name("sdd-explore.md.new")
        self.assertTrue(sidecar.is_file(), "expected .new sidecar to be written")
        self.assertIn("upstream moved", sidecar.read_text())

    # --- Read-helper unit tests ---------------------------------------------

    def test_read_sub_agents_flag_strict(self):
        self.assertFalse(self.mod.read_sub_agents_flag({}))
        self.assertFalse(self.mod.read_sub_agents_flag({"sdd": {}}))
        self.assertTrue(self.mod.read_sub_agents_flag({"sdd": {"sub_agents": True}}))
        with self.assertRaises(ValueError):
            self.mod.read_sub_agents_flag({"sdd": {"sub_agents": "yes"}})

    def test_detect_orphans_empty_when_no_claude_dir(self):
        self.assertEqual(self.mod.detect_orphans(self.project), [])

    def test_detect_orphans_returns_present_subagents(self):
        d = self.claude_dir()
        d.mkdir(parents=True)
        (d / "sdd-explore.md").write_text("x")
        (d / "sdd-archive.md").write_text("x")
        (d / "unrelated.md").write_text("x")
        orphans = self.mod.detect_orphans(self.project)
        names = sorted(p.name for p in orphans)
        self.assertEqual(names, ["sdd-archive.md", "sdd-explore.md"])


if __name__ == "__main__":
    unittest.main()

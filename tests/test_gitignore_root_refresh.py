"""Black-box tests for the root .gitignore managed agent-block refresh.

Every test drives ``bin/ai-specs sync -v``, whose verbose step replay prints
the ``✓ refreshed|appended root .gitignore (agent block)`` action line, and
asserts the filesystem effect on the project's root ``.gitignore``. No test
may import ``lib/_internal`` modules.

``init`` also appends the managed block, so these tests hand-write the project
manifest and never run init: the sync step must be the only writer of the
managed block.
"""
from __future__ import annotations

import shutil
import tempfile
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, temp_project  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

STALE_BLOCK_GITIGNORE = (
    "node_modules/\n"
    "\n"
    "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---\n"
    ".claude/\n"
    ".cursor/\n"
    "# --- end ai-specs ---\n"
    "\n"
    "dist/\n"
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


class GitignoreRootRefreshTests(unittest.TestCase):
    def _sync_project(self, gitignore_body: str | None) -> tuple[Path, object]:
        """Hand-written manifest project (no init) plus ``sync -v``.

        Returns (project root, CLIResult). The CLI only ever touches the temp
        project and the temp install root.
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = _make_home(Path(tmp.name))
        td, root = temp_project(name="gitignore-fixture")
        self.addCleanup(td.cleanup)
        if gitignore_body is not None:
            (root / ".gitignore").write_text(gitignore_body, encoding="utf-8")
        result = invoke(root, "sync", "-v", cli_home=home, tmpdir=Path(tmp.name))
        return root, result

    def test_refresh_updates_stale_agent_block_with_pi(self):
        root, result = self._sync_project(STALE_BLOCK_GITIGNORE)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("refreshed root .gitignore (agent block)", result.stdout)
        text = (root / ".gitignore").read_text()
        self.assertIn(".pi/", text)
        self.assertIn(".omp/", text)
        self.assertIn("node_modules/", text)
        self.assertIn("dist/", text)
        self.assertEqual(text.count("# --- end ai-specs ---"), 1)

    def test_refresh_appends_block_when_missing(self):
        root, result = self._sync_project("*.log\n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("appended root .gitignore (agent block)", result.stdout)
        text = (root / ".gitignore").read_text()
        self.assertTrue(text.startswith("*.log\n"))
        self.assertIn(".pi/", text)
        self.assertIn(
            "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---",
            text,
        )

    def test_root_template_ignores_harness_env_secrets(self):
        """JD-2/JD-7: consumer root gitignore must ignore harness env + migration bak.

        Surfaced through the synced artifact: with no pre-existing .gitignore,
        sync appends the managed block and every harness-env ignore from
        templates/gitignore-root.tmpl must land in the project's root .gitignore.
        """
        root, result = self._sync_project(None)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = (root / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("ai-specs.env", text)
        self.assertIn("ai-specs/.env", text)
        self.assertIn("ai-specs/.env.bak", text)
        self.assertIn("ai-specs/.envrc.bak", text)
        self.assertIn(".env", text.splitlines())
        self.assertIn(".envrc", text.splitlines())

    def test_refresh_appends_ai_specs_env_ignore(self):
        root, result = self._sync_project(None)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("appended root .gitignore (agent block)", result.stdout)
        text = (root / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("ai-specs.env", text)
        self.assertIn("ai-specs/.env", text)
        self.assertIn("ai-specs/.env.bak", text)
        self.assertIn("ai-specs/.envrc.bak", text)


if __name__ == "__main__":
    unittest.main()

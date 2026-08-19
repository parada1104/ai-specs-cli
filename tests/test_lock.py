"""Black-box round-trip tests for the ai-specs lock helper.

Regression: write_lock emits recipe/dep skill hashes as a 3-level table
(`[recipes."<id>".skills."<skill>"]`), but load_lock read them back one level
too shallow, leaving a stray "skills" key. On the next write that mismatch was
serialized with Python repr (`"{'SKILL.md': '...'}"`) and then re-nested into
invalid TOML, breaking `ai-specs sync` for any project with multiple
recipe-bundled skills.

Converted to black-box: every test drives `bin/ai-specs <verb>` as a
subprocess via `invoke` and asserts on the emitted `ai-specs/.ai-specs.lock`.
Legacy-fixture tests reconstruct the historical failure state by writing the
legacy lock bytes into the project BEFORE `sync`, then assert how `sync`
normalizes the emitted lock (drops hash/commands/opted-out sections, preserves
agents provenance, restamps `[meta]`). The CLI no longer tracks content hashes,
so `[skills.]`/`[recipes.]`/`[deps.]` sections are never present in an emitted
lock.
"""

import tempfile
import tomllib
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home, temp_project


class LockRoundTripTests(unittest.TestCase):
    """Lock normalization observed through `bin/ai-specs sync`."""

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for sequences)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _version(self, root: Path) -> str:
        """Installed CLI version reported by `ai-specs version` (meta restamp target)."""
        if getattr(self, "_cached_version", None) is None:
            v = invoke(root, "version", cli_home=self._cli_home())
            self._cached_version = v.stdout.strip()
        return self._cached_version

    def _sync(self, root: Path):
        """Single shared helper wrapping the CLI sync invocation for this class."""
        return invoke(root, "sync", cli_home=self._cli_home())

    def _make_project(self) -> tuple:
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        return td, root

    def test_sync_writes_observable_lock_provenance(self):
        td, root = self._make_project()
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        self.assertTrue(lock_path.is_file())
        lock = tomllib.loads(lock_path.read_text())
        self.assertIn("meta", lock)
        self.assertIn("managed", lock)
        self.assertIn("AGENTS.md", lock["managed"])
        self.assertEqual(lock["managed"]["AGENTS.md"]["kind"], "runtime-brief")
        self.assertEqual(lock["managed"]["AGENTS.md"]["policy"], "never-force")

    def test_skill_recipe_dep_hashes_not_emitted(self):
        """The lock is a provenance stamp: content hashes are no longer tracked."""
        td, root = self._make_project()
        # Enable a skill-bearing recipe (plan-build-flow provides a bundled skill)
        # and sync; the emitted lock must contain NO skills/recipes/deps sections.
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.plan-build-flow]\nenabled = true\n"
        )
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        text = lock_path.read_text()
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        self.assertNotIn("[deps.", text)

        reloaded = tomllib.loads(text)
        self.assertNotIn("skills", reloaded)
        self.assertNotIn("recipes", reloaded)
        self.assertNotIn("deps", reloaded)

    def test_legacy_hash_sections_dropped_on_rewrite(self):
        """A lock written by an older CLI (with hash sections) is normalized."""
        td, root = self._make_project()
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[skills."skill-creator"]\n"SKILL.md" = "zzz"\n\n'
            '[recipes."worktree-flow".skills."worktree-flow"]\n"SKILL.md" = "aaa"\n\n'
            '[deps."my-dep".skills."my-dep"]\n"SKILL.md" = "eee"\n'
        )
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)

        text = lock_path.read_text()
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        self.assertNotIn("[deps.", text)

        reloaded = tomllib.loads(lock_path.read_text())
        self.assertIn("meta", reloaded)
        self.assertEqual(reloaded["meta"]["cli_version"], self._version(root))
        self.assertNotIn("skills", reloaded)
        self.assertNotIn("recipes", reloaded)
        self.assertNotIn("deps", reloaded)

    def test_legacy_commands_opted_out_dropped_on_write(self):
        """[commands]/[opted-out] were the last non-[meta]/[agents.*] legacy
        sections; both are dropped unconditionally on the next sync."""
        td, root = self._make_project()
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "0.12.2"\nsynced_at = "2026-06-23T12:00:00Z"\n\n'
            '[commands]\n"rules-audit.md" = "cmdhash"\n\n'
            '[opted-out]\nfiles = ["commands/skills-as-rules.md"]\n'
        )
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)

        text = lock_path.read_text()
        self.assertIn("[meta]", text)
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)

        reloaded = tomllib.loads(lock_path.read_text())
        self.assertIn("meta", reloaded)
        self.assertEqual(reloaded["meta"]["cli_version"], self._version(root))
        self.assertNotIn("commands", reloaded)
        self.assertNotIn("opted_out", reloaded)

    def test_legacy_lock_with_commands_opted_out_sections_normalized(self):
        """A lock written by a prior CLI version (with [commands]/[opted-out])
        is normalized on the next write — sections silently dropped, no crash."""
        td, root = self._make_project()
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[commands]\n"rules-audit.md" = "cmdhash"\n\n'
            '[opted-out]\nfiles = ["commands/skills-as-rules.md"]\n'
        )
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)

        text = lock_path.read_text()
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)

        reloaded = tomllib.loads(lock_path.read_text())
        self.assertIn("meta", reloaded)
        self.assertEqual(reloaded["meta"]["cli_version"], self._version(root))
        self.assertNotIn("commands", reloaded)
        self.assertNotIn("opted_out", reloaded)

    def test_legacy_commands_opted_out_dropped_agents_preserved(self):
        """Combined legacy case: [commands]/[opted-out] dropped, [agents.*] kept.

        A lock that still has both the pre-relocation hash sections AND a
        populated [agents.*] section must normalize both correctly on rewrite —
        drop the legacy sections, preserve agents provenance unchanged.
        """
        td, root = self._make_project()
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[commands]\n"rules-audit.md" = "cmdhash"\n\n'
            '[opted-out]\nfiles = ["commands/skills-as-rules.md"]\n\n'
            '[agents."claude"]\n"AGENTS.md" = "agenthash"\n'
        )
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)

        text = lock_path.read_text()
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)
        self.assertIn('[agents."claude"]', text)
        self.assertIn('"AGENTS.md" = "agenthash"', text)

        reloaded = tomllib.loads(lock_path.read_text())
        self.assertIn("agents", reloaded)
        self.assertEqual(reloaded["agents"]["claude"]["AGENTS.md"], "agenthash")
        self.assertIn("meta", reloaded)
        self.assertEqual(reloaded["meta"]["cli_version"], self._version(root))
        self.assertNotIn("commands", reloaded)
        self.assertNotIn("opted_out", reloaded)


if __name__ == "__main__":
    unittest.main()

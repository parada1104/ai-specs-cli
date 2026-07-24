"""Round-trip tests for the ai-specs lock helper.

Regression: write_lock emits recipe/dep skill hashes as a 3-level table
(`[recipes."<id>".skills."<skill>"]`), but load_lock read them back one level
too shallow, leaving a stray "skills" key. On the next write that mismatch was
serialized with Python repr (`"{'SKILL.md': '...'}"`) and then re-nested into
invalid TOML, breaking `ai-specs sync` for any project with multiple
recipe-bundled skills.
"""

import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
LOCK_PATH = ROOT / "lib" / "_internal" / "lock.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class LockRoundTripTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.lock = load_module(LOCK_PATH, "lock_internal")

    def _lock_path(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name) / ".ai-specs.lock"

    def test_skill_recipe_dep_hashes_not_emitted(self):
        """The lock is a provenance stamp: content hashes are no longer tracked."""
        path = self._lock_path()
        lock = self.lock.load_lock(path)
        lock["skills"]["skill-creator"] = {"SKILL.md": "zzz"}
        self.lock.set_recipe_skill_hashes(
            lock, "worktree-flow", "worktree-flow", {"SKILL.md": "aaa"}
        )
        self.lock.set_dep_skill_hashes(lock, "my-dep", "my-dep", {"SKILL.md": "eee"})
        self.lock.write_lock(path, lock)

        text = path.read_text()
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        self.assertNotIn("[deps.", text)

        reloaded = self.lock.load_lock(path)
        self.assertEqual(reloaded["skills"], {})
        self.assertEqual(reloaded["recipes"], {})
        self.assertEqual(reloaded["deps"], {})

    def test_legacy_hash_sections_dropped_on_rewrite(self):
        """A lock written by an older CLI (with hash sections) is normalized."""
        path = self._lock_path()
        path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[skills."skill-creator"]\n"SKILL.md" = "zzz"\n\n'
            '[recipes."worktree-flow".skills."worktree-flow"]\n"SKILL.md" = "aaa"\n\n'
            '[deps."my-dep".skills."my-dep"]\n"SKILL.md" = "eee"\n'
        )
        lock = self.lock.load_lock(path)
        self.lock.write_lock(path, lock)

        text = path.read_text()
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        self.assertNotIn("[deps.", text)
        self.assertIn('cli_version = "0.14.0"', text)

    def test_legacy_commands_opted_out_dropped_on_write(self):
        """[commands]/[opted-out] were the last non-[meta]/[agents.*] legacy
        sections; both are dropped unconditionally now that commands no longer
        materialize in-project (no per-file hash or delete-memory needed)."""
        path = self._lock_path()
        lock = self.lock.load_lock(path)
        lock["meta"] = {"cli_version": "0.12.2", "synced_at": "2026-06-23T12:00:00Z"}
        lock["commands"] = {"rules-audit.md": "cmdhash"}
        lock["opted_out"] = ["commands/skills-as-rules.md"]
        self.lock.write_lock(path, lock)

        text = path.read_text()
        self.assertIn("[meta]", text)
        self.assertIn('cli_version = "0.12.2"', text)
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)

        reloaded = self.lock.load_lock(path)
        self.assertEqual(reloaded["meta"]["cli_version"], "0.12.2")
        self.assertNotIn("commands", reloaded)
        self.assertNotIn("opted_out", reloaded)

    def test_legacy_lock_with_commands_opted_out_sections_normalized(self):
        """A lock written by a prior CLI version (with [commands]/[opted-out])
        is normalized on the next write — sections silently dropped, no crash."""
        path = self._lock_path()
        path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[commands]\n"rules-audit.md" = "cmdhash"\n\n'
            '[opted-out]\nfiles = ["commands/skills-as-rules.md"]\n'
        )
        lock = self.lock.load_lock(path)
        self.lock.write_lock(path, lock)

        text = path.read_text()
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)
        self.assertIn('cli_version = "0.14.0"', text)

    def test_legacy_commands_opted_out_dropped_agents_preserved(self):
        """Combined legacy case: [commands]/[opted-out] dropped, [agents.*] kept.

        A lock that still has both the pre-relocation hash sections AND a
        populated [agents.*] section must normalize both correctly on rewrite —
        drop the legacy sections, preserve agents provenance unchanged.
        """
        path = self._lock_path()
        path.write_text(
            '[meta]\ncli_version = "0.14.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            '[commands]\n"rules-audit.md" = "cmdhash"\n\n'
            '[opted-out]\nfiles = ["commands/skills-as-rules.md"]\n\n'
            '[agents."claude"]\n"AGENTS.md" = "agenthash"\n'
        )
        lock = self.lock.load_lock(path)
        self.assertEqual(lock["agents"]["claude"]["AGENTS.md"], "agenthash")
        self.lock.write_lock(path, lock)

        text = path.read_text()
        self.assertNotIn("[commands]", text)
        self.assertNotIn("[opted-out]", text)
        self.assertIn('[agents."claude"]', text)
        self.assertIn('"AGENTS.md" = "agenthash"', text)
        self.assertIn('cli_version = "0.14.0"', text)

        reloaded = self.lock.load_lock(path)
        self.assertEqual(reloaded["agents"]["claude"]["AGENTS.md"], "agenthash")
        self.assertNotIn("commands", reloaded)
        self.assertNotIn("opted_out", reloaded)



if __name__ == "__main__":
    unittest.main()

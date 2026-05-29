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

    def test_recipe_skill_hashes_round_trip(self):
        path = self._lock_path()
        lock = self.lock.load_lock(path)
        self.lock.set_recipe_skill_hashes(
            lock, "worktree-flow", "worktree-flow", {"SKILL.md": "aaa"}
        )
        self.lock.write_lock(path, lock)

        reloaded = self.lock.load_lock(path)
        self.assertEqual(
            reloaded["recipes"]["worktree-flow"]["worktree-flow"],
            {"SKILL.md": "aaa"},
        )

    def test_multiple_recipes_survive_repeated_write_load(self):
        path = self._lock_path()

        lock = self.lock.load_lock(path)
        self.lock.set_recipe_skill_hashes(
            lock, "worktree-flow", "worktree-flow", {"SKILL.md": "aaa"}
        )
        self.lock.write_lock(path, lock)

        # Simulate the next bundled skill from a different recipe: load, mutate,
        # write — this is where the round-trip used to corrupt the file.
        lock = self.lock.load_lock(path)
        self.lock.set_recipe_skill_hashes(
            lock, "git-pr-flow", "git-merge-workflow", {"SKILL.md": "bbb"}
        )
        self.lock.write_lock(path, lock)

        # And a third with multiple files.
        lock = self.lock.load_lock(path)
        self.lock.set_recipe_skill_hashes(
            lock,
            "session-context",
            "context-precedence",
            {"SKILL.md": "ccc", "assets/extra.md": "ddd"},
        )
        self.lock.write_lock(path, lock)

        # Must still be valid TOML after all the re-writes.
        with path.open("rb") as fh:
            tomllib.load(fh)  # raises if corrupted

        final = self.lock.load_lock(path)
        self.assertEqual(
            final["recipes"]["worktree-flow"]["worktree-flow"], {"SKILL.md": "aaa"}
        )
        self.assertEqual(
            final["recipes"]["git-pr-flow"]["git-merge-workflow"], {"SKILL.md": "bbb"}
        )
        self.assertEqual(
            final["recipes"]["session-context"]["context-precedence"],
            {"SKILL.md": "ccc", "assets/extra.md": "ddd"},
        )

    def test_dep_skill_hashes_round_trip(self):
        path = self._lock_path()
        lock = self.lock.load_lock(path)
        self.lock.set_dep_skill_hashes(lock, "my-dep", "my-dep", {"SKILL.md": "eee"})
        self.lock.write_lock(path, lock)

        reloaded = self.lock.load_lock(path)
        self.assertEqual(reloaded["deps"]["my-dep"]["my-dep"], {"SKILL.md": "eee"})


if __name__ == "__main__":
    unittest.main()

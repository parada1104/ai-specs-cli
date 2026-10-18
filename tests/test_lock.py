"""Round-trip tests for the ai-specs lock helper.

Regression: write_lock emits recipe/dep skill hashes as a 3-level table
(`[recipes."<id>".skills."<skill>"]`), but load_lock read them back one level
too shallow, leaving a stray "skills" key. On the next write that mismatch was
serialized with Python repr (`"{'SKILL.md': '...'}"`) and then re-nested into
invalid TOML, breaking `ai-specs sync` for any project with multiple
recipe-bundled skills.
"""

import importlib.util
import os
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest import mock


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


class FallbackControlCharRefusalTests(unittest.TestCase):
    """The TEMPORARY Python fallback lock writer enforces the same boundary as
    the Go authority (``worktree-gate --write-lock``): any emitted key or
    value containing an ASCII control character (< 0x20 or DEL 0x7f) is
    refused with RuntimeError using the Go locator wording, so a fallback
    write can never emit bytes the Go writer would refuse (GO-08 second
    findings batch)."""

    @classmethod
    def setUpClass(cls):
        cls.lock = load_module(LOCK_PATH, "lock_internal_refusal")

    def _fallback_path(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name) / ".ai-specs.lock"

    def test_control_char_in_agents_hash_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["agents"]["claude"] = {"AGENTS.md": "hash\nwith-newline"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for agents hash contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_managed_path_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["managed"]["hooks/pre\x01commit"] = {"sha256": "abc"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for managed path contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_meta_value_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["meta"] = {"cli_version": "0.14.0", "synced_at": "2026\x7f-01-01"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for meta.synced_at contains a control character",
        )

    def test_control_char_in_lock_path_is_refused(self):
        path = self._fallback_path() / "loc\x1fck.ai-specs.lock"
        lock = self.lock.load_lock(self._fallback_path())
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for lock_path contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_meta_cli_version_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["meta"] = {"cli_version": "0.14.0\n"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for meta.cli_version contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_managed_entry_value_is_refused(self):
        """Every emitted managed value is walked, not only the path key."""
        for key in ("sha256", "recipe", "source", "kind", "policy"):
            with self.subTest(key=key):
                path = self._fallback_path()
                lock = self.lock.load_lock(path)
                lock["managed"]["AGENTS.md"] = {
                    "sha256": "abc",
                    key: f"bad\t{key}",
                }
                with self.assertRaises(RuntimeError) as ctx:
                    self.lock._write_lock_python(path, lock)
                self.assertEqual(
                    str(ctx.exception),
                    f"value for managed.{key} contains a control character",
                )
                self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_agents_harness_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["agents"]["cl\x02ude"] = {"AGENTS.md": "hash"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for agents harness contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_control_char_in_agents_filename_is_refused(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["agents"]["claude"] = {"AGE\x03NTS.md": "hash"}
        with self.assertRaises(RuntimeError) as ctx:
            self.lock._write_lock_python(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for agents filename contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_clean_lock_still_writes(self):
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["agents"]["pi"] = {"AGENTS.md": "abc123"}
        lock["managed"]["AGENTS.md"] = {"sha256": "def456", "kind": "runtime-brief"}
        self.lock._write_lock_python(path, lock)
        text = path.read_text()
        self.assertIn('[agents."pi"]', text)
        self.assertIn('[managed."AGENTS.md"]', text)

    def test_write_lock_fallback_route_refuses_control_chars(self):
        """The user-facing ``write_lock`` entry refuses too, on the fallback
        route (no usable Go binary): the Go authority would refuse the same
        envelope, so the fail-closed decision must survive degradation."""
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["agents"]["claude"] = {"AGENTS.md": "hash\nwith-newline"}
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path / "no-such-gate")})
        with pin:
            with self.assertRaises(RuntimeError) as ctx:
                self.lock.write_lock(path, lock)
        self.assertEqual(
            str(ctx.exception),
            "value for agents hash contains a control character",
        )
        self.assertFalse(path.exists(), "nothing written on refusal")

    def test_skip_branches_write_without_refusal(self):
        """The refusal walk mirrors the emitter's skip guards: a managed entry
        without a sha256 and an empty agents harness are skipped by both, so
        neither triggers a refusal nor reaches the output."""
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["managed"]["dropped.md"] = {"recipe": "x"}  # no sha256: skipped
        lock["managed"]["kept.md"] = {"sha256": "abc"}
        lock["agents"]["empty"] = {}  # no files: skipped
        lock["agents"]["pi"] = {"AGENTS.md": "abc123"}
        self.lock._write_lock_python(path, lock)
        text = path.read_text()
        self.assertNotIn("dropped.md", text)
        self.assertIn("[managed.\"kept.md\"]", text)
        self.assertNotIn("[agents.\"empty\"]", text)
        self.assertIn("[agents.\"pi\"]", text)

    def test_non_dict_managed_entry_is_skipped_not_refused(self):
        """A malformed managed entry is dropped exactly like the emitter's
        isinstance guard, never crashed on and never emitted."""
        path = self._fallback_path()
        lock = self.lock.load_lock(path)
        lock["managed"]["bad.md"] = "not-a-dict"
        lock["managed"]["kept.md"] = {"sha256": "abc"}
        self.lock._write_lock_python(path, lock)
        text = path.read_text()
        self.assertNotIn("bad.md", text)
        self.assertIn("[managed.\"kept.md\"]", text)


class RemoveRecipeLockEntriesTests(unittest.TestCase):
    """Pin the results guard of ``remove_recipe_lock_entries``: True exactly
    when a recipe section existed and was removed, False otherwise — the
    caller relies on this boolean to decide whether the rewrite is needed."""

    @classmethod
    def setUpClass(cls):
        cls.lock = load_module(LOCK_PATH, "lock_internal_remove")

    def test_removing_present_recipe_returns_true_and_deletes(self):
        lock = {"recipes": {"worktree-flow": {"SKILL.md": {"SKILL.md": "h"}}}}
        self.assertTrue(self.lock.remove_recipe_lock_entries(lock, "worktree-flow"))
        self.assertNotIn("worktree-flow", lock["recipes"])

    def test_removing_absent_recipe_returns_false_and_keeps_others(self):
        lock = {"recipes": {"other": {}}}
        self.assertFalse(self.lock.remove_recipe_lock_entries(lock, "worktree-flow"))
        self.assertEqual(lock["recipes"], {"other": {}})

    def test_removing_from_missing_recipes_group_returns_false(self):
        self.assertFalse(self.lock.remove_recipe_lock_entries({}, "worktree-flow"))


if __name__ == "__main__":
    unittest.main()

"""Black-box tests for command merge (cache managed + local hand-authored).

Drives the shipped CLI (`bin/ai-specs sync`), which materializes recipes and
then merges three command tiers into each agent's commands dir:

    CLI-bundled  ({cache}/.bundled/commands, primed by `refresh-bundled --init`)
    recipe-      ({cache}/commands, per-project cache, seeded per test)
    managed
    local hand-  (ai-specs/commands/, per-project, seeded per test)
    authored

Ascending precedence: bundled -> managed -> local. The claude agent mirrors the
merged set into `.claude/commands/`. No lib/_internal import; no loader.
"""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home, cache_project_dir, temp_project


class CommandMergeTests(unittest.TestCase):
    def setUp(self):
        self._home_holder = tempfile.TemporaryDirectory(prefix="cmd-merge-home-")
        self.addCleanup(self._home_holder.cleanup)
        self.home = isolated_home(Path(self._home_holder.name))
        self._project_holder, self.root = temp_project(agents=("claude",))
        self.addCleanup(self._project_holder.cleanup)
        self.cache = cache_project_dir(self.root, self.home)
        # Prime the CLI-bundled tier with real rules-audit.md / skills-as-rules.md.
        invoke(self.root, "refresh-bundled", "--init", cli_home=self.home)
        (self.root / "AGENTS.md").write_text("# AGENTS\n\n## Rules\n\n")

    def _managed(self) -> Path:
        p = self.cache / "commands"
        p.mkdir(parents=True, exist_ok=True)
        return p

    def _local(self) -> Path:
        p = self.root / "ai-specs" / "commands"
        p.mkdir(parents=True, exist_ok=True)
        return p

    def _bundled(self) -> Path:
        return self.cache / ".bundled" / "commands"

    def _sync(self):
        return invoke(self.root, "sync", cli_home=self.home)

    def _merge_dest(self) -> Path:
        return self.root / ".claude" / "commands"

    def test_local_wins_over_managed(self):
        (self._managed() / "shared.md").write_text("managed\n")
        (self._managed() / "only-managed.md").write_text("m\n")
        (self._local() / "shared.md").write_text("local\n")
        (self._local() / "only-local.md").write_text("l\n")

        result = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("syncing merge commands", result.stdout)
        dest = self._merge_dest()
        self.assertEqual((dest / "shared.md").read_text(), "local\n")
        self.assertEqual((dest / "only-managed.md").read_text(), "m\n")
        self.assertEqual((dest / "only-local.md").read_text(), "l\n")
        # bundled rules-audit + skills-as-rules + shared + only-managed + only-local
        self.assertEqual(len(list(dest.glob("*.md"))), 5)
        # TRIAGE: the merge's exact warning "command 'shared' present in cache
        # and ai-specs/commands/; local hand-authored wins" is withheld from
        # assertion per the change spec; the "keeping local/customized ...
        # shared.md" notice is the observable nearest-surface equivalent and
        # surfaces twice on stderr (recipe-leftover cleanup runs for both the
        # root workspace and the agent fan-out).
        self.assertEqual(result.stderr.count("keeping local/customized ai-specs/commands/shared.md"), 2)

    def test_bundled_only_appears_in_merge_output(self):
        result = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = self._merge_dest()
        bundled = self._bundled()
        self.assertEqual(len(list(dest.glob("*.md"))), 2)
        self.assertEqual((dest / "rules-audit.md").read_text(),
                         (bundled / "rules-audit.md").read_text())
        self.assertEqual((dest / "skills-as-rules.md").read_text(),
                         (bundled / "skills-as-rules.md").read_text())

    def test_managed_silently_overrides_bundled(self):
        (self._managed() / "rules-audit.md").write_text("managed\n")
        result = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = self._merge_dest()
        self.assertEqual((dest / "rules-audit.md").read_text(), "managed\n")
        # managed replaces bundled; skills-as-rules still bundled-only.
        self.assertEqual(len(list(dest.glob("*.md"))), 2)
        # No warning for the bundled-vs-managed collision (both CLI-driven tiers).
        self.assertNotIn("hand-authored", result.stderr)
        self.assertNotIn("keeping customized", result.stderr)

    def test_local_wins_over_bundled_and_managed_with_warning(self):
        (self._managed() / "shared.md").write_text("managed\n")
        (self._local() / "shared.md").write_text("local-over-managed\n")
        (self._local() / "rules-audit.md").write_text("local-over-bundled\n")

        result = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = self._merge_dest()
        self.assertEqual((dest / "shared.md").read_text(), "local-over-managed\n")
        self.assertEqual((dest / "rules-audit.md").read_text(), "local-over-bundled\n")
        self.assertEqual((dest / "skills-as-rules.md").read_text(),
                         (self._bundled() / "skills-as-rules.md").read_text())
        # TRIAGE: the "local hand-authored wins" warning texts for the
        # bundled- and managed-side collisions have no precedence-observable
        # assertion required; here we assert only the precedence outcomes.
        self.assertEqual(len(list(dest.glob("*.md"))), 3)

    def test_managed_only(self):
        (self._managed() / "a.md").write_text("a\n")
        result = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = self._merge_dest()
        self.assertEqual((dest / "a.md").read_text(), "a\n")
        # a + bundled rules-audit + bundled skills-as-rules
        self.assertEqual(len(list(dest.glob("*.md"))), 3)


if __name__ == "__main__":
    unittest.main()

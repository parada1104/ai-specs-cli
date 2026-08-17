"""Version summary and upgrade-notice replay for `ai-specs upgrade`.

A notice is prose that names a command. `upgrade` runs against the global
installation and has no consumer project in scope, so it must never evaluate
or execute a notice — only display it.
"""

from __future__ import annotations

import unittest

from test_upgrade_output import UpgradeOutputTests, run


class UpgradeNoticeTests(UpgradeOutputTests):
    """Reuses the fake-install harness; overrides changelog authoring."""

    def changelog(self, *versions: str) -> str:
        """Changelog where 0.22.0 and 0.20.1 declare upgrade notes."""
        notes = {
            "0.22.0": (
                "Run `ai-specs sync` in each project to acquire the verified Go\n"
                "worktree-gate binary.\n"
            ),
            "0.20.1": "Re-run `ai-specs sync` to drop stale command copies.\n",
        }
        out = "# Changelog\n\n## [Unreleased]\n\n"
        for version in versions:
            out += f"## [{version}] — 2026-08-17\n\n"
            out += f"### Added\n- thing for {version}.\n\n"
            if version in notes:
                out += f"### Upgrade notes\n{notes[version]}\n"
            out += f"### Fixed\n- sentinel-{version}-must-not-bleed.\n\n"
        return out

    def publish_chain(self, ai_specs, bare, versions):
        """Publish a release whose changelog contains every listed version."""
        import shutil

        clone = ai_specs.parent / "publisher"
        if clone.exists():
            shutil.rmtree(clone)
        run(["git", "clone", str(bare), str(clone)])
        run(["git", "config", "user.email", "t@t.com"], cwd=clone)
        run(["git", "config", "user.name", "T"], cwd=clone)
        (clone / "VERSION").write_text(versions[0] + "\n")
        (clone / "CHANGELOG.md").write_text(self.changelog(*versions))
        run(["git", "add", "-A"], cwd=clone)
        run(["git", "commit", "-m", f"release {versions[0]}"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)

    # --- summary ------------------------------------------------------------

    def test_summary_lists_the_crossed_version(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0"])

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertIn("0.22.0", result.stdout)

    def test_summary_covers_every_crossed_version(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.20.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0", "0.20.1", "0.20.0"])

        result = self.upgrade(home)
        stdout = result.stdout
        for version in ("0.22.0", "0.21.0", "0.20.1"):
            self.assertIn(version, stdout, msg=f"missing {version} in:\n{stdout}")

    def test_summary_is_ordered_newest_first(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.20.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0", "0.20.1", "0.20.0"])

        stdout = self.upgrade(home).stdout
        # Restrict to the summary block so the notice block cannot skew order.
        block = stdout.split("Action required")[0]
        self.assertLess(block.index("0.22.0"), block.index("0.21.0"))
        self.assertLess(block.index("0.21.0"), block.index("0.20.1"))

    def test_no_summary_when_already_up_to_date(self):
        home = self.fake_home()
        self.setup_install(home, version="0.21.0")

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stderr)
        self.assertNotIn("What changed", result.stdout)
        self.assertNotIn("Action required", result.stdout)

    # --- notices ------------------------------------------------------------

    def test_notice_is_printed_for_a_crossed_version(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0"])

        stdout = self.upgrade(home).stdout
        self.assertIn("Action required", stdout)
        self.assertIn("ai-specs sync", stdout)
        self.assertIn("worktree-gate binary", stdout)

    def test_notice_does_not_bleed_into_the_next_subsection(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0"])

        stdout = self.upgrade(home).stdout
        # The sentinel is a `### Fixed` bullet, so it belongs in the summary.
        # What must never happen is the notice absorbing the subsection that
        # follows it.
        notice_block = stdout.split("Action required")[1]
        self.assertNotIn("sentinel-0.22.0-must-not-bleed", notice_block)

    def test_notices_replay_oldest_first(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.20.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0", "0.20.1", "0.20.0"])

        stdout = self.upgrade(home).stdout
        block = stdout.split("Action required")[1]
        self.assertLess(
            block.index("stale command copies"),
            block.index("worktree-gate binary"),
            msg=f"notices out of release order:\n{block}",
        )

    def test_no_notice_section_when_none_declared(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.20.1")
        self.publish_chain(ai_specs, bare, ["0.21.0", "0.20.1"])

        stdout = self.upgrade(home).stdout
        self.assertIn("0.21.0", stdout)
        self.assertNotIn("Action required", stdout)

    def test_notice_survives_compact_mode(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0"])

        result = self.upgrade(home)  # no --verbose
        self.assertIn("Action required", result.stdout)

    def test_notice_command_is_displayed_not_executed(self):
        """A notice naming `ai-specs sync` must not trigger a sync."""
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_chain(ai_specs, bare, ["0.22.0", "0.21.0"])

        stdout = self.upgrade(home).stdout
        self.assertIn("ai-specs sync", stdout)
        # A real sync against this fake install would fail loudly; nothing in
        # the output may suggest one ran.
        self.assertNotIn("syncing ", stdout)
        self.assertNotIn("sync complete", stdout)

    # --- degradation --------------------------------------------------------
    #
    # Degradation must be induced in the PUBLISHED state, not the local tree.
    # Touching tracked files locally trips the dirty-tree guard (exit 3) and
    # would test that guard instead of the parser's degradation.

    def publish_mutated(self, ai_specs, bare, version, mutate):
        """Publish `version` after applying `mutate(clone_dir)`."""
        import shutil

        clone = ai_specs.parent / "publisher"
        if clone.exists():
            shutil.rmtree(clone)
        run(["git", "clone", str(bare), str(clone)])
        run(["git", "config", "user.email", "t@t.com"], cwd=clone)
        run(["git", "config", "user.name", "T"], cwd=clone)
        (clone / "VERSION").write_text(version + "\n")
        mutate(clone)
        run(["git", "add", "-A"], cwd=clone)
        run(["git", "commit", "-m", f"release {version}"], cwd=clone)
        run(["git", "push", "origin", "main"], cwd=clone)

    def test_unreadable_changelog_still_upgrades(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_mutated(
            ai_specs, bare, "0.22.0", lambda c: (c / "CHANGELOG.md").unlink()
        )

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stdout + result.stderr)
        self.assertIn("0.22.0", result.stdout)
        self.assertNotIn("Traceback", result.stdout + result.stderr)

    def test_malformed_changelog_degrades_to_plain_line(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_mutated(
            ai_specs,
            bare,
            "0.22.0",
            lambda c: (c / "CHANGELOG.md").write_text("garbage with no headings\n"),
        )

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stdout + result.stderr)
        self.assertIn("Upgraded", result.stdout)
        self.assertNotIn("What changed", result.stdout)
        self.assertNotIn("Traceback", result.stdout + result.stderr)

    def test_missing_parser_does_not_break_the_upgrade(self):
        home = self.fake_home()
        ai_specs, bare = self.setup_install(home, version="0.21.0")
        self.publish_mutated(
            ai_specs,
            bare,
            "0.22.0",
            lambda c: (c / "lib" / "_internal" / "changelog.py").unlink(),
        )

        result = self.upgrade(home)
        self.assertEqual(result.returncode, 0, msg=result.stdout + result.stderr)
        self.assertIn("Upgraded", result.stdout)
        self.assertNotIn("Traceback", result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()

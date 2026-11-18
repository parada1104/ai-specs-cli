"""Regression tests for environment artifacts at the parity boundary."""

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from . import parity


class HarnessNormalizationTests(unittest.TestCase):
    def test_staged_install_modes_do_not_follow_checkout_permissions(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            source = base / "checkout"
            commands = source / "bundled-commands"
            commands.mkdir(parents=True)
            doc = commands / "demo.md"
            doc.write_text("# demo\n")
            run = source / "bin" / "ai-specs"
            run.parent.mkdir()
            run.write_text("#!/bin/sh\n")
            commands.chmod(0o700)
            doc.chmod(0o600)
            run.chmod(0o700)

            with patch.object(parity.bb, "ROOT", source):
                home = parity.make_home(base / "scratch")
            tree = parity.snapshot_tree(home, {})
            self.assertEqual(tree["bundled-commands"][:2], ("dir", "0o755"))
            self.assertEqual(tree["bundled-commands/demo.md"][:2], ("file", "0o644"))
            self.assertEqual(tree["bin/ai-specs"][:2], ("file", "0o755"))
            self.assertEqual((home / "bundled-commands/demo.md").read_text(), "# demo\n")
            self.assertEqual(doc.stat().st_mode & 0o777, 0o600)

    def test_doctor_gate_normalizes_only_the_version_of_a_cached_binary(self):
        line = ("  OK     worktree-gate    Go binary {version} at "
                "<TEMP>/cache/bin/worktree-gate/{version}/darwin-arm64/worktree-gate; "
                "selftest passed (cache <CACHE_KB> KiB)\n")
        adjacent = "  OK     another-check    Go binary 0.25.2 at <TEMP>/another-check\n"
        for version in ("0.25.0", "0.25.2"):
            with self.subTest(version=version):
                self.assertEqual(
                    parity.apply_normalizations("stdout", line.format(version=version) + adjacent,
                                                {"argv": ("doctor",)}),
                    line.format(version="<CLI_VERSION>") + adjacent,
                )
        sync_text = line.format(version="0.25.2")
        self.assertEqual(parity.apply_normalizations("stdout", sync_text, {"argv": ("sync",)}),
                         sync_text)

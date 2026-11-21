"""Scenario: an installed v0.24.0 upgrades to this tree and keeps working.

`tests/scenarios/upgrade_from_0_24.sh` installs the real v0.24.0 release from a
local bare clone, promotes this tree's committed content to `main`, runs the
installed old `ai-specs upgrade`, and then asserts the next verb self-heals
(#534) and the project keeps working. This module is only a thin runner: the
scenario itself is a plain bash script so any backend (local now, a Space
later) can run the same file.

The script exits 0 on pass, 1 on an assertion failure, and 77 when a
prerequisite is missing (no git/curl/go, or a shallow clone without the
v0.24.0 tag) — that 77 becomes a skip here. Set AI_SPECS_SKIP_SCENARIOS=1 to
skip the run entirely, e.g. in a constrained CI job.
"""
from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

from test_install_binary_entry import _platform_asset, _Server

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "tests" / "scenarios" / "upgrade_from_0_24.sh"
BASH = shutil.which("bash") or "/bin/bash"
TIMEOUT_SECONDS = 300
TAIL_CHARS = 4000


def _tail(text: str | None) -> str:
    return (text or "")[-TAIL_CHARS:]


class UpgradeScenarioRunTests(unittest.TestCase):
    def test_upgrade_from_0_24_keeps_working(self) -> None:
        if os.environ.get("AI_SPECS_SKIP_SCENARIOS") == "1":
            self.skipTest("AI_SPECS_SKIP_SCENARIOS=1")
        self.assertTrue(SCRIPT.is_file(), f"missing scenario script: {SCRIPT}")
        result = subprocess.run(
            [BASH, str(SCRIPT)],
            cwd=str(ROOT),
            capture_output=True,
            text=True,
            timeout=TIMEOUT_SECONDS,
        )
        if result.returncode == 77:
            lines = (result.stderr or result.stdout).strip().splitlines()
            self.skipTest(lines[-1] if lines else "scenario prerequisite missing")
        self.assertEqual(
            result.returncode,
            0,
            f"scenario failed (rc={result.returncode})\n"
            f"--- stdout tail ---\n{_tail(result.stdout)}\n"
            f"--- stderr tail ---\n{_tail(result.stderr)}",
        )


class UpgradeReleaseBaseTests(unittest.TestCase):
    """JD-A-005: the native `upgrade` honors AI_SPECS_RELEASE_BASE.

    A binary install (no checkout .git, so no git step) takes its asset from the
    release base. Pointing that at a local mirror proves the CLI route passes it
    to the engine instead of always using the default GitHub URL; the bytes the
    entry ends up holding cannot come from anywhere but the mirror.
    """

    def test_upgrade_downloads_from_the_local_mirror(self) -> None:
        from gate_cli import gate_binary

        binary = gate_binary()
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        with tempfile.TemporaryDirectory(prefix="ai-specs-upgrade-mirror-") as tmp:
            base = Path(tmp).resolve()
            userhome = base / "home"
            userhome.mkdir()
            home = userhome / ".ai-specs"
            (home / "bin").mkdir(parents=True)
            (home / "VERSION").write_text("1.0.0\n", encoding="utf-8")
            (home / "bin" / "SHA256SUMS").write_text(
                f"{hashlib.sha256(payload).hexdigest()}  {asset}\n",
                encoding="utf-8")
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            entry = userhome / ".local" / "bin" / "ai-specs"
            entry.parent.mkdir(parents=True)
            entry.write_bytes(binary.read_bytes())
            entry.chmod(0o755)

            proc = subprocess.run(
                [str(entry), "upgrade"],
                cwd=str(base),
                env={
                    "HOME": str(userhome),
                    "AI_SPECS_HOME": str(home),
                    "AI_SPECS_RELEASE_BASE": server.base_url,
                    "PATH": os.environ.get("PATH", ""),
                    "LC_ALL": "C",
                    "LANG": "C",
                },
                capture_output=True,
                text=True,
                timeout=120,
            )

            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(entry.read_bytes(), payload,
                             "the verified asset must become the entry")
            self.assertEqual((home / "dist" / "ai-specs").read_bytes(), payload,
                             "the launcher copy must come from the same base")
            stamp = (home / "dist" / "VERSION").read_text().split()
            self.assertEqual((stamp[0], len(stamp)), ("1.0.0", 3),
                             "the launcher copy stamp binds version and bytes")


class UpgradeScenarioStaticTests(unittest.TestCase):
    def test_script_is_plain_bash_and_has_no_target_version_literal(self) -> None:
        self.assertTrue(SCRIPT.is_file(), f"missing scenario script: {SCRIPT}")
        # The scenario is invoked as `bash <path>`, so it needs no exec bit.
        syntax = subprocess.run(
            [BASH, "-n", str(SCRIPT)], capture_output=True, text=True
        )
        self.assertEqual(syntax.returncode, 0, syntax.stderr)
        source = SCRIPT.read_text(encoding="utf-8")
        default = re.search(r"FROM_TAG:-v?(\d+\.\d+\.\d+)", source)
        if default is None:
            self.fail("scenario must declare its FROM_TAG default")
        literals = set(re.findall(r"\b\d+\.\d+\.\d+\b", source))
        self.assertEqual(
            literals - {default.group(1)},
            set(),
            "scenario must read versions from the tree, not hard-code them",
        )


if __name__ == "__main__":
    unittest.main()

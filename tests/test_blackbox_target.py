"""Pin the black-box seam to the native Go binary (Go 20 / U2d T1, T2).

``_blackbox.invoke`` is the process boundary every launcher-driven module uses.
It must drive the Go route built by ``_go_cli.cli()``, not the bash launcher.
The removed ``CLI`` constant must have no consumer left, and every consumer
must import and collect against the helper's contract.
"""
from __future__ import annotations

import re
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import _blackbox  # noqa: E402
import _go_cli  # noqa: E402

TESTS_DIR = Path(__file__).resolve().parent


class BlackboxTargetsNativeBinary(unittest.TestCase):
    def test_invoke_drives_the_go_binary_not_the_launcher(self):
        self.assertEqual(_blackbox.cli_path(), _go_cli.cli())
        self.assertNotEqual(_blackbox.cli_path(), _go_cli.legacy_cli())

    def test_version_through_invoke_reports_the_repo_version(self):
        with tempfile.TemporaryDirectory() as td:
            got = _blackbox.invoke(Path(td), "version", append_root=False)
        expected = (_blackbox.ROOT / "VERSION").read_text().strip()
        self.assertEqual(got.returncode, 0)
        self.assertEqual(got.stdout.strip(), expected)


class BlackboxContractCollection(unittest.TestCase):
    """Every consumer of the helper must import and collect against the contract."""

    def test_no_consumer_imports_the_removed_cli_constant(self):
        offenders = []
        for path in sorted(TESTS_DIR.glob("test_*.py")):
            for match in re.finditer(r"^from _blackbox import (.+)$", path.read_text(), re.M):
                names = {n.strip().split(" as ")[0] for n in match.group(1).split("#")[0].split(",")}
                if "CLI" in names:
                    offenders.append(path.name)
        self.assertEqual(offenders, [])

    def test_vault_canonical_store_module_collects_without_import_error(self):
        suite = unittest.TestLoader().loadTestsFromName("test_vault_canonical_store_recipe")
        failed = [t for t in _flatten(suite) if type(t).__name__ == "_FailedTest"]
        self.assertEqual(failed, [])


def _flatten(suite):
    for item in suite:
        if isinstance(item, unittest.TestSuite):
            yield from _flatten(item)
        else:
            yield item


if __name__ == "__main__":
    unittest.main()

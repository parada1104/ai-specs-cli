"""Static contract guard: install.sh must not chmod lib/ globs.

The old install.sh ran chmod +x on lib/*.sh and lib/_internal/*.py on every
install. Those files are committed as mode 100644 in git, so the chmod makes
git report them as modified (mode-only dirt) when core.fileMode=true — causing
subsequent update runs to fail or silently skip the pull.

These tests assert the offending lines have been removed.
"""
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"


class InstallNoLibChmodTests(unittest.TestCase):
    def test_install_sh_does_not_chmod_lib_sh_glob(self):
        """install.sh must not contain chmod targeting lib/*.sh."""
        text = INSTALL_SH.read_text()
        # The problematic pattern was: "$AI_SPECS_HOME/lib/"*.sh
        self.assertNotIn(
            'lib/"*.sh',
            text,
            "install.sh must not chmod lib/*.sh globs — this makes tracked 100644 "
            "files appear modified in git (mode-only dirt).",
        )

    def test_install_sh_does_not_chmod_internal_py_glob(self):
        """install.sh must not contain chmod targeting lib/_internal/*.py."""
        text = INSTALL_SH.read_text()
        self.assertNotIn(
            '_internal/"*.py',
            text,
            "install.sh must not chmod lib/_internal/*.py globs — those .py files "
            "are 100644 in git and do not need the executable bit.",
        )

    def test_install_sh_no_longer_symlinks_the_entrypoint(self):
        """The entry is an INSTALLED BINARY now, not a symlink into the checkout.

        GO-20 U1b-2b replaced the legacy `ln -sf "$AI_SPECS_HOME/bin/ai-specs"
        "$INSTALL_BIN/ai-specs"` with an atomic install of the compiled binary (a
        verified release asset, or a local `go build`). The intent of the old
        `chmod +x bin/ai-specs` assertion survives where it is observable: the
        functional suite asserts the installed entry exists, is executable and
        runs. What this guard can still pin statically is that the legacy
        symlink is gone, so the flip cannot silently regress.
        """
        text = INSTALL_SH.read_text()
        self.assertNotIn(
            'ln -sf "$AI_SPECS_HOME/bin/ai-specs"',
            text,
            "install.sh must install the binary as the entry; symlinking the "
            "launcher into the checkout is the pre-flip behaviour.",
        )
        self.assertIn(
            "go build",
            text,
            "install.sh keeps the local-build fallback for platforms without a "
            "release asset.",
        )


if __name__ == "__main__":
    unittest.main()

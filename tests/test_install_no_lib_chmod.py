"""Static contract guard: install.sh must not chmod lib/ globs.

The old install.sh ran chmod +x on lib/*.sh and lib/_internal/*.py on every
install. Those files are committed as mode 100644 in git, so the chmod makes
git report them as modified (mode-only dirt) when core.fileMode=true — causing
subsequent update runs to fail or silently skip the pull.

These tests assert the offending lines have been removed.
"""
import re
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

    def test_install_sh_still_chmods_bin_ai_specs(self):
        """install.sh must keep chmod +x for bin/ai-specs (the entrypoint)."""
        text = INSTALL_SH.read_text()
        # Accept both single-line and continuation-line forms.
        single_line = re.search(r'chmod\b[^\n]*bin/ai-specs', text)
        continuation = re.search(r'chmod\s+\+x\s*\\\n\s*["\']?\S*/bin/ai-specs', text)
        self.assertTrue(
            single_line or continuation,
            "install.sh must still chmod +x bin/ai-specs (the entrypoint).",
        )


if __name__ == "__main__":
    unittest.main()

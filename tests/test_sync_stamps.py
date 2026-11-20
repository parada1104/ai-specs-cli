"""S8: sync keeps an explicit [recipes.<id>.config] value and points at the wizard."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import ROOT, isolated_home, populate_catalog

RECIPE = (
    "[recipe]\nid = 'demo'\nname = 'Demo'\ndescription = 'Fixture'\nversion = '1.0.0'\n"
    "[config.reconcile]\nscope_field = 'workflow'\n"
    "[config.workflow]\nrequired = true\ndefault = 'feature'\n"
)

NOTICE = (
    "  ! recipe 'Demo': config 'workflow' differs from the declared value; "
    "kept your value, review it with `ai-specs configure-recipes`"
)


class TestSyncReconcileStamps(unittest.TestCase):
    def test_explicit_config_value_is_kept_and_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            tmp = Path(directory)
            binary = tmp / 'ai-specs'
            subprocess.run(['go', 'build', '-o', str(binary), './cmd/ai-specs'], cwd=ROOT, check=True)
            home = isolated_home(tmp, catalog=False)
            populate_catalog(home, 'demo', RECIPE)
            root = tmp / 'project'
            (root / 'ai-specs').mkdir(parents=True)
            manifest = root / 'ai-specs/ai-specs.toml'
            manifest.write_text(
                "[project]\nname = 'demo'\n[brief]\nrender = false\n"
                "[agents]\nenabled = ['claude']\n[recipes.demo]\nenabled = true\n"
            )
            (root / 'AGENTS.md').write_text('# Brief\n')

            def run():
                return subprocess.run(
                    [str(binary), 'sync', str(root)], text=True, capture_output=True,
                    env={**os.environ, 'HOME': str(tmp), 'AI_SPECS_HOME': str(home)},
                )

            # A value the project never set is still stamped, silently.
            first = run()
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertNotIn(NOTICE, first.stderr)
            self.assertIn('workflow = "feature"', manifest.read_text())

            # The user's explicit value survives byte-identically, with one notice.
            authored = manifest.read_text().replace('workflow = "feature"', 'workflow = "mine"', 1)
            self.assertIn('workflow = "mine"', authored)
            manifest.write_text(authored)
            for _ in range(2):
                synced = run()
                self.assertEqual(synced.returncode, 0, synced.stderr)
                self.assertEqual(manifest.read_text(), authored)
                self.assertEqual([line for line in synced.stderr.splitlines() if line == NOTICE],
                                 [NOTICE], synced.stderr)
                self.assertNotIn('not stamped', synced.stderr)

            # Agreeing with the recipe is not a divergence: no notice.
            manifest.write_text(authored.replace('workflow = "mine"', 'workflow = "feature"'))
            again = run()
            self.assertEqual(again.returncode, 0, again.stderr)
            self.assertNotIn(NOTICE, again.stderr)


if __name__ == '__main__':
    unittest.main()

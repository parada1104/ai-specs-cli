"""Exercise stale legacy recipe command diagnostics at the CLI boundary."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import ROOT, cache_project_dir, isolated_home, populate_catalog


class TestSyncCommands(unittest.TestCase):
    def test_preserves_and_warns_for_untracked_recipe_command(self):
        with tempfile.TemporaryDirectory() as directory:
            tmp = Path(directory)
            binary = tmp / 'ai-specs'
            subprocess.run(['go', 'build', '-o', str(binary), './cmd/ai-specs'], cwd=ROOT, check=True)
            home = isolated_home(tmp, catalog=False)
            populate_catalog(home, 'demo', "[recipe]\nid='demo'\nname='Demo'\ndescription='Fixture'\nversion='1.0.0'\n[provides]\ncommands=[{id='example', path='example.md'}]\n")
            root = tmp / 'project'
            (root / 'ai-specs').mkdir(parents=True)
            (root / 'ai-specs/ai-specs.toml').write_text("[project]\nname='demo'\n[brief]\nrender=false\n[agents]\nenabled=['claude']\n[recipes.demo]\nenabled=false\n")
            (root / 'AGENTS.md').write_text('# Brief\n')
            stale = root / '.claude/commands/example.md'
            stale.parent.mkdir(parents=True)
            stale.write_text('# Legacy\n')
            cached = cache_project_dir(root, home) / 'commands/example.md'
            cached.parent.mkdir(parents=True)
            cached.write_text('# Legacy\n')
            message = '.claude/commands/example.md looks like a stale ai-specs command but has no ownership record; delete it if you did not write it'

            def run(command):
                return subprocess.run([str(binary), command, str(root)], text=True, capture_output=True,
                                      env={**os.environ, 'HOME': str(tmp), 'AI_SPECS_HOME': str(home)})

            for _ in range(2):
                synced = run('sync')
                self.assertEqual(synced.returncode, 0, synced.stderr)
                self.assertTrue(synced.stdout.endswith('\n✓ ai-specs sync complete\n'), synced.stdout)
                self.assertEqual(synced.stderr, f'warning: {message}\n')
                self.assertFalse(cached.exists())
                self.assertEqual(stale.read_text(), '# Legacy\n')
                doctor = run('doctor')
                self.assertEqual(doctor.returncode, 0, doctor.stdout + doctor.stderr)
                self.assertEqual(doctor.stderr, '')
                self.assertIn(f'WARN   commands         {message}\n', doctor.stdout)
                self.assertNotIn('stale command(s): example.md', doctor.stdout)

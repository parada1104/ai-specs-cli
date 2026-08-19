"""Black-box recipe validation + materialization tests for the tdd-flow recipe.

Converted from direct lib/_internal module loads to bin/ai-specs subprocess
invocations with observable output/filesystem checks, per the
blackbox-test-conversion delta. No project tests run here; the parent validates
the suite.
"""

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_ID = "tdd-flow"
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import cache_command, recipe_root
from _blackbox import isolated_home, invoke, temp_project

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "tdd-flow"
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import cache_command, recipe_root
from _blackbox import isolated_home, invoke, temp_project


class TddFlowRecipeTests(unittest.TestCase):
    def test_recipe_validates_and_declares_capability(self):
        # TRIAGE: the recipe's declared capability id ("test-runner") and the
        # bundled skill "source" label are not exposed by `recipe list` or
        # `recipe configure --inspect`, so they have no black-box observable
        # equivalent here and remain uncovered by this converted test.
        home_tmp = tempfile.TemporaryDirectory(prefix="tf-home-")
        self.addCleanup(home_tmp.cleanup)
        home = isolated_home(Path(home_tmp.name))
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)

        # The recipe is listed as available under its id.
        lst = invoke(root, "recipe", "list", cli_home=home)
        self.assertEqual(lst.returncode, 0)
        self.assertIn(RECIPE_ID, lst.stdout)

        # Declared skill/command are surfaced by `recipe add`'s materialize plan.
        add = invoke(root, "recipe", "add", RECIPE_ID, cli_home=home)
        self.assertEqual(add.returncode, 0)
        self.assertIn("skills: tdd-flow", add.stdout)
        self.assertIn("commands: tdd", add.stdout)

        # `recipe configure --inspect --json` exposes the config schema:
        # test_command is an optional string field with no default.
        conf = invoke(root, "recipe", "configure", RECIPE_ID, "--inspect", "--json", cli_home=home)
        self.assertEqual(conf.returncode, 0)
        data = json.loads(conf.stdout)
        self.assertEqual(data["recipe"]["id"], RECIPE_ID)
        fields = {f["key"]: f for f in data["schema"]["fields"]}
        self.assertIn("test_command", fields)
        self.assertFalse(fields["test_command"]["required"])
        self.assertEqual(fields["test_command"]["type"], "str")
        self.assertIsNone(fields["test_command"]["default"])

    def test_materialize_produces_skill_command_and_doc(self):
        home_tmp = tempfile.TemporaryDirectory(prefix="tf-materialize-")
        self.addCleanup(home_tmp.cleanup)
        home = isolated_home(Path(home_tmp.name))
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)

        add = invoke(root, "recipe", "add", RECIPE_ID, cli_home=home)
        self.assertEqual(add.returncode, 0)
        sync = invoke(root, "sync", cli_home=home)
        self.assertEqual(sync.returncode, 0, sync.stderr)

        skill = (
            recipe_root(root, RECIPE_ID, home)
            / "skills" / "tdd-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = cache_command(root, "tdd", home)
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")


if __name__ == "__main__":
    unittest.main()

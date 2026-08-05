from __future__ import annotations

import importlib.util
import io
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class OverrideOwnershipTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.lock = load_module(ROOT / "lib/_internal/lock.py", "lock_override_tests")
        cls.util = load_module(ROOT / "lib/_internal/util.py", "util_override_tests")
        cls.materialize = load_module(
            ROOT / "lib/_internal/recipe-materialize.py", "materialize_override_tests"
        )
        cls.schema = load_module(ROOT / "lib/_internal/recipe_schema.py", "schema_override_tests")
        cls.doctor = load_module(ROOT / "lib/_internal/doctor.py", "doctor_override_tests")

    def test_managed_lock_round_trip_preserves_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            lock_path = Path(tmp) / "ai-specs/.ai-specs.lock"
            lock = self.lock.load_lock(lock_path)
            self.lock.set_managed_override(
                lock,
                "ai-specs/recipes/example/overrides/card.md",
                "a" * 64,
                recipe="example",
                source="templates/card.md",
                kind="template",
                policy="auto",
            )
            self.lock.write_lock(lock_path, lock)
            loaded = self.lock.load_lock(lock_path)
            self.assertEqual(
                loaded["managed"]["ai-specs/recipes/example/overrides/card.md"]["sha256"],
                "a" * 64,
            )
            self.assertEqual(loaded["managed"]["ai-specs/recipes/example/overrides/card.md"]["policy"], "auto")
            text = lock_path.read_text()
            self.assertIn('[managed."ai-specs/recipes/example/overrides/card.md"]', text)
            self.assertNotIn("[skills.", text)
            self.assertNotIn("[recipes.", text)

    def test_classifier_covers_ownership_states(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            src = root / "catalog.md"
            dest = root / "dest.md"
            src.write_text("catalog")
            self.assertEqual(self.util.classify_managed_override(dest, None, src), "missing")
            dest.write_text("catalog")
            self.assertEqual(self.util.classify_managed_override(dest, None, src), "untracked")
            managed = {"sha256": self.util.sha256_bytes(dest.read_bytes())}
            self.assertEqual(self.util.classify_managed_override(dest, managed, src), "managed_current")
            src.write_text("evolved")
            self.assertEqual(self.util.classify_managed_override(dest, managed, src), "managed_stale")
            dest.write_text("user edit")
            self.assertEqual(self.util.classify_managed_override(dest, managed, src), "user_modified")

    def _template(self, source: str = "template.md", target: str = "out/template.md", policy: str = "auto"):
        return SimpleNamespace(source=source, target=target, condition="not_exists", update_policy=policy)

    def test_materialize_seeds_and_refreshes_managed_templates(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            recipe = root / "recipe"
            recipe.mkdir()
            (recipe / "template.md").write_text("v1")
            tpl = self._template()
            self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            dest = root / tpl.target
            lock = self.lock.load_lock(root / "ai-specs/.ai-specs.lock")
            self.assertEqual(dest.read_text(), "v1")
            self.assertEqual(lock["managed"][tpl.target]["sha256"], self.lock.sha256_of(dest))
            (recipe / "template.md").write_text("v2")
            self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            self.assertEqual(dest.read_text(), "v2")
            self.assertEqual(
                self.lock.load_lock(root / "ai-specs/.ai-specs.lock")["managed"][tpl.target]["sha256"],
                self.lock.sha256_of(dest),
            )

    def test_materialize_preserves_user_modified_and_untracked_diverged(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            recipe = root / "recipe"
            recipe.mkdir()
            (recipe / "template.md").write_text("catalog")
            tpl = self._template()
            self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            dest = root / tpl.target
            dest.write_text("user edit")
            stream = io.StringIO()
            with patch("sys.stderr", stream):
                self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            self.assertEqual(dest.read_text(), "user edit")
            self.assertIn("user-modified", stream.getvalue())
            dest.unlink()
            (root / "ai-specs/.ai-specs.lock").unlink()
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text("custom before migration")
            stream = io.StringIO()
            with patch("sys.stderr", stream):
                self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            self.assertEqual(dest.read_text(), "custom before migration")
            self.assertIn("metadata", stream.getvalue())
            self.assertNotIn(tpl.target, self.lock.load_lock(root / "ai-specs/.ai-specs.lock").get("managed", {}))

    def test_untracked_matching_catalog_seeds_without_rewrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            recipe = root / "recipe"
            recipe.mkdir()
            (recipe / "template.md").write_text("same")
            dest = root / "out/template.md"
            dest.parent.mkdir(parents=True)
            dest.write_text("same")
            tpl = self._template()
            self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            self.assertEqual(dest.read_text(), "same")
            self.assertEqual(
                self.lock.load_lock(root / "ai-specs/.ai-specs.lock")["managed"][tpl.target]["sha256"],
                self.lock.sha256_of(dest),
            )

    def test_confirm_policy_preserves_managed_stale(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            recipe = root / "recipe"
            recipe.mkdir()
            (recipe / "template.md").write_text("v1")
            tpl = self._template(policy="confirm")
            self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            (recipe / "template.md").write_text("v2")
            stream = io.StringIO()
            with patch("sys.stderr", stream):
                self.materialize.materialize_template(recipe, tpl, root, recipe_id="example")
            self.assertEqual((root / tpl.target).read_text(), "v1")
            self.assertIn("managed-stale", stream.getvalue())

    def test_template_policy_is_validated_and_defaults_auto(self):
        with tempfile.TemporaryDirectory() as tmp:
            recipe_dir = Path(tmp)
            path = recipe_dir / "recipe.toml"
            path.write_text(
                '[recipe]\nid="r"\nname="R"\ndescription="D"\nversion="1"\n'
                '[[provides.templates]]\nsource="x"\ntarget="y"\nupdate_policy="never-force"\n'
            )
            recipe = self.schema.load_recipe_toml(path)
            self.assertEqual(recipe.templates[0].update_policy, "never-force")
            path.write_text(
                '[recipe]\nid="r"\nname="R"\ndescription="D"\nversion="1"\n'
                '[[provides.templates]]\nsource="x"\ntarget="y"\n'
            )
            self.assertEqual(self.schema.load_recipe_toml(path).templates[0].update_policy, "auto")
            path.write_text(
                '[recipe]\nid="r"\nname="R"\ndescription="D"\nversion="1"\n'
                '[[provides.templates]]\nsource="x"\ntarget="y"\nupdate_policy="sometimes"\n'
            )
            with self.assertRaises(self.schema.RecipeValidationError) as ctx:
                self.schema.load_recipe_toml(path)
            self.assertIn("update_policy", str(ctx.exception))

    def test_doctor_warns_user_modified_but_not_managed_auto_stale(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "project"
            catalog = Path(tmp) / "catalog" / "recipes" / "example"
            catalog.mkdir(parents=True)
            (catalog / "recipe.toml").write_text(
                '[recipe]\nid="example"\nname="Example"\ndescription="D"\nversion="1"\n'
                '[[provides.templates]]\nsource="template.md"\ntarget="out/template.md"\n'
            )
            (catalog / "template.md").write_text("catalog-v2")
            (root / "ai-specs").mkdir(parents=True)
            (root / "ai-specs/ai-specs.toml").write_text('[recipes.example]\nenabled = true\n')
            dest = root / "out/template.md"
            dest.parent.mkdir(parents=True)
            dest.write_text("user edit")
            lock = self.lock.load_lock(root / "ai-specs/.ai-specs.lock")
            self.lock.set_managed_override(lock, "out/template.md", self.util.sha256_bytes(b"catalog-v1"))
            self.lock.write_lock(root / "ai-specs/.ai-specs.lock", lock)
            with patch.object(self.doctor, "AI_SPECS_HOME", Path(tmp)):
                doctor = self.doctor.Doctor(root)
                doctor._check_stale_template_overrides()
            self.assertEqual(len(doctor.checks), 1)
            self.assertIn("user-modified", doctor.checks[0].message)

    def test_hook_materialization_remains_unconditional(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            recipe = root / "recipe"
            (recipe / "hooks").mkdir(parents=True)
            (recipe / "hooks/gate.sh").write_text("v1")
            hook = SimpleNamespace(script="hooks/gate.sh")
            self.materialize.materialize_hook_script(recipe, hook, root, "example")
            (recipe / "hooks/gate.sh").write_text("v2")
            self.materialize.materialize_hook_script(recipe, hook, root, "example")
            self.assertEqual((root / "ai-specs/recipes/example/hooks/gate.sh").read_text(), "v2")


if __name__ == "__main__":
    unittest.main()

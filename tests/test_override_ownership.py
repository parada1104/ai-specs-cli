from __future__ import annotations

import importlib.util
import io
import os
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
            warning = stream.getvalue()
            self.assertIn("missing", warning)
            self.assertIn("preserving existing file", warning)
            self.assertIn("leave it unchanged", warning)
            self.assertIn("remove it and run sync again", warning)
            self.assertNotIn("user-managed", warning.lower())
            self.assertNotIn("customized", warning.lower())
            self.assertNotIn(tpl.target, self.lock.load_lock(root / "ai-specs/.ai-specs.lock").get("managed", {}))

    def test_doctor_describes_untracked_divergence_neutrally(self):
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
            dest.write_text("local bytes without metadata")

            with patch.object(self.doctor, "AI_SPECS_HOME", Path(tmp)):
                doctor = self.doctor.Doctor(root)
                doctor._check_stale_template_overrides()

            self.assertEqual(len(doctor.checks), 1)
            warning = doctor.checks[0].message
            self.assertIn("missing ownership metadata", warning)
            self.assertIn("preserve", warning)
            self.assertIn("remove", warning)
            self.assertIn("sync", warning)
            self.assertNotIn("user-managed", warning.lower())
            self.assertNotIn("user-owned", warning.lower())

    def test_doctor_silences_legacy_catalog_seed_with_rendered_config(self):
        """A raw catalog seed is adopted by sync even when rendering changes bytes."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "project"
            catalog = Path(tmp) / "catalog" / "recipes" / "example"
            catalog.mkdir(parents=True)
            (catalog / "recipe.toml").write_text(
                '[recipe]\nid="example"\nname="Example"\ndescription="D"\nversion="1"\n'
                '[[provides.templates]]\nsource="template.md"\ntarget="out/template.md"\n'
            )
            legacy = "topology=__WORKTREE_REPO_TOPOLOGY__\n"
            (catalog / "template.md").write_text(legacy)
            (root / "ai-specs").mkdir(parents=True)
            (root / "ai-specs/ai-specs.toml").write_text(
                '[recipes.example]\nenabled = true\n'
                '[recipes.example.config]\nrepo_topology = "worktree"\n'
            )
            dest = root / "out/template.md"
            dest.parent.mkdir(parents=True)
            dest.write_text(legacy)

            with patch.object(self.doctor, "AI_SPECS_HOME", Path(tmp)):
                doctor = self.doctor.Doctor(root)
                doctor._check_stale_template_overrides()

            self.assertEqual(doctor.checks, [])

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

    def _hook(self, script: str = "hooks/gate.sh"):
        return SimpleNamespace(script=script)

    def _hook_project(self, gate_bytes: bytes = b"v1\n") -> tuple[Path, Path]:
        """Fake home (catalog) + project enabling a runtime-hook recipe."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name)
        recipe_dir = home / "catalog" / "recipes" / "wt-hook"
        (recipe_dir / "hooks").mkdir(parents=True)
        (recipe_dir / "hooks" / "gate.sh").write_bytes(gate_bytes)
        (recipe_dir / "recipe.toml").write_text(
            '[recipe]\n'
            'id = "wt-hook"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[provides.hooks]]\n'
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            'blocking = true\n'
        )
        proj_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(proj_tmp.cleanup)
        project_root = Path(proj_tmp.name)
        ai_specs = project_root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.wt-hook]\nenabled = true\nversion = '1.0'\n"
        )
        return project_root, home

    def _hook_lock_entry(self, project_root: Path) -> dict | None:
        lock = self.lock.load_lock(project_root / "ai-specs/.ai-specs.lock")
        return (lock.get("managed") or {}).get(
            "ai-specs/recipes/wt-hook/hooks/gate.sh"
        )

    def test_gate_baseline_match_refreshes_and_records_baseline(self):
        """3.1 — RED: baseline match refreshes the generated gate."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        self.materialize.materialize_recipes(project_root, home)
        gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
        entry = self._hook_lock_entry(project_root)
        self.assertEqual(entry["kind"], "gate")
        self.assertEqual(entry["policy"], "auto")
        self.assertEqual(entry["sha256"], self.lock.sha256_of(gate))
        self.assertEqual(gate.read_bytes(), b"v1\n")

        # Catalog evolves to v2; a matching baseline force-refreshes.
        (home / "catalog/recipes/wt-hook/hooks/gate.sh").write_bytes(b"v2\n")
        stream = io.StringIO()
        with patch("sys.stderr", stream):
            self.materialize.materialize_recipes(project_root, home)
        self.assertEqual(gate.read_bytes(), b"v2\n")
        self.assertNotIn("user-modified", stream.getvalue())
        self.assertEqual(
            self._hook_lock_entry(project_root)["sha256"], self.lock.sha256_of(gate)
        )

    def test_gate_byte_mismatch_preserves_with_warning(self):
        """3.1 — RED: byte mismatch preserves the customized gate."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        self.materialize.materialize_recipes(project_root, home)
        gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
        gate.write_bytes(b"# custom user gate\n")
        (home / "catalog/recipes/wt-hook/hooks/gate.sh").write_bytes(b"v2\n")
        stream = io.StringIO()
        with patch("sys.stderr", stream):
            self.materialize.materialize_recipes(project_root, home)
        self.assertEqual(gate.read_bytes(), b"# custom user gate\n")
        warning = stream.getvalue()
        self.assertIn("gate.sh", warning)
        self.assertIn("user-modified", warning)
        self.assertIn("refresh", warning.lower())

    def test_gate_missing_provenance_preserves_without_seeding(self):
        """3.1 — RED: no baseline means preserve + warn, no seeding."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
        gate.parent.mkdir(parents=True, exist_ok=True)
        gate.write_bytes(b"# pre-existing without provenance\n")
        stream = io.StringIO()
        with patch("sys.stderr", stream):
            self.materialize.materialize_recipes(project_root, home)
        self.assertEqual(gate.read_bytes(), b"# pre-existing without provenance\n")
        warning = stream.getvalue()
        self.assertIn("gate.sh", warning)
        self.assertIn("provenance", warning.lower())
        self.assertIsNone(self._hook_lock_entry(project_root),
                          "a baseline must not be seeded when the CLI did not "
                          "render the gate")

    def _customize_then_refresh(
        self, project_root: Path, home: Path, custom: bytes, gate_bytes: bytes = b"v2\n"
    ) -> tuple[Path, bytes]:
        """Customize the gate, run an explicit refresh, return (gate, new_bytes)."""
        gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
        gate.write_bytes(custom)
        (home / "catalog/recipes/wt-hook/hooks/gate.sh").write_bytes(gate_bytes)
        self.materialize.materialize_recipes(project_root, home, refresh_gates=True)
        return gate, gate_bytes

    def test_explicit_refresh_backs_up_pre_refresh_bytes_immutably(self):
        """3.2 — RED: refresh saves exact pre-refresh bytes to the cache backup."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        self.materialize.materialize_recipes(project_root, home)
        custom = b"# customized user gate\n"
        gate, _ = self._customize_then_refresh(project_root, home, custom)
        self.assertEqual(gate.read_bytes(), b"v2\n")
        self.assertEqual(self._hook_lock_entry(project_root)["sha256"],
                         self.lock.sha256_of(gate))
        pc = load_module(ROOT / "lib/_internal/project-cache.py", "project_cache_oo")
        rel = "ai-specs/recipes/wt-hook/hooks/gate.sh"
        backup = pc.gate_backup_path(project_root, rel, self.lock.sha256_bytes(custom), cli_home=home)
        self.assertTrue(backup.is_file(), f"backup missing at {backup}")
        self.assertEqual(backup.read_bytes(), custom)
        self.assertNotIn("cache", str(project_root),
                         "backup must live in the CLI cache, not the project")

    def test_repeated_refresh_is_collision_safe(self):
        """3.2 — RED: repeated refreshes keep the original snapshot intact."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        self.materialize.materialize_recipes(project_root, home)
        pc = load_module(ROOT / "lib/_internal/project-cache.py", "project_cache_oo2")
        rel = "ai-specs/recipes/wt-hook/hooks/gate.sh"
        custom_a = b"# custom A\n"
        _, _ = self._customize_then_refresh(project_root, home, custom_a)
        backup_a = pc.gate_backup_path(project_root, rel, self.lock.sha256_bytes(custom_a), cli_home=home)
        self.assertTrue(backup_a.is_file())
        custom_b = b"# custom B\n"
        self._customize_then_refresh(project_root, home, custom_b)
        backup_b = pc.gate_backup_path(project_root, rel, self.lock.sha256_bytes(custom_b), cli_home=home)
        self.assertNotEqual(backup_a, backup_b, "distinct content must not collide")
        self.assertTrue(backup_a.is_file(), "original snapshot must remain intact")
        self.assertTrue(backup_b.is_file())
        self.assertEqual(backup_a.read_bytes(), custom_a)

    def test_failed_backup_write_leaves_gate_unchanged(self):
        """3.2 — RED: a failed backup aborts the refresh atomically."""
        project_root, home = self._hook_project(gate_bytes=b"v1\n")
        self.materialize.materialize_recipes(project_root, home)
        gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
        custom = b"# custom gate\n"
        gate.write_bytes(custom)
        before_lock = (project_root / "ai-specs/.ai-specs.lock").read_bytes()
        (home / "catalog/recipes/wt-hook/hooks/gate.sh").write_bytes(b"v9\n")

        pc = load_module(ROOT / "lib/_internal/project-cache.py", "project_cache_oo3")
        # Force the backup target to collide with an existing FILE so mkdir fails.
        rel = "ai-specs/recipes/wt-hook/hooks/gate.sh"
        bad = pc.gate_backup_path(project_root, rel, self.lock.sha256_bytes(custom), cli_home=home)
        bad.parent.parent.mkdir(parents=True, exist_ok=True)
        bad.parent.write_text("blocking file\n")  # rel-key dir position is a file

        with patch.object(self.materialize._load_project_cache(), "gate_backup_path",
                          return_value=bad):
            with self.assertRaises(Exception):
                self.materialize.materialize_recipes(project_root, home, refresh_gates=True)
        self.assertEqual(gate.read_bytes(), custom,
                         "gate must remain unchanged when the backup write fails")
        self.assertEqual(
            (project_root / "ai-specs/.ai-specs.lock").read_bytes(), before_lock,
            "lock must not be partially updated on refresh failure",
        )

    def test_refresh_absent_or_disabled_provider_parity(self):
        """3.2 — RED: refresh behaves identically with external orchestration
        absent or disabled."""
        import subprocess
        outcomes = []
        for extra_env in ({}, {"GENTLE_AI_MODE": "disabled", "GENTLE_AI_ABSENT": "1"}):
            project_root, home = self._hook_project(gate_bytes=b"v1\n")
            env = dict(os.environ, AI_SPECS_HOME=str(home))
            env.update(extra_env)
            proc = subprocess.run(
                [
                    "python3", str(ROOT / "lib/_internal/recipe-materialize.py"),
                    str(project_root), str(home),
                ],
                capture_output=True, text=True, env=env, check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            gate = project_root / "ai-specs/recipes/wt-hook/hooks/gate.sh"
            gate.write_bytes(b"# custom\n")
            (home / "catalog/recipes/wt-hook/hooks/gate.sh").write_bytes(b"v3\n")
            proc = subprocess.run(
                [
                    "python3", str(ROOT / "lib/_internal/recipe-materialize.py"),
                    str(project_root), str(home), "--refresh-gates",
                ],
                capture_output=True, text=True, env=env, check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            outcomes.append(gate.read_bytes())
        self.assertEqual(len(set(outcomes)), 1,
                         "absent and disabled external orchestration must behave "
                         "identically")

    def test_gate_provenance_policy_is_documented(self):
        """The changed recipe docs preserve the gate provenance contract."""
        docs = (
            ROOT / "catalog/recipes/worktree-flow/README.md",
            ROOT / "catalog/recipes/trello-mcp-workflow/README.md",
            ROOT / "docs/recipes-catalog.md",
        )
        surface = "\n".join(path.read_text().lower() for path in docs)
        for phrase in (
            "gate provenance",
            "records a baseline",
            "byte mismatch",
            "missing baseline",
            "ai-specs sync --refresh-gates",
            "cache-only immutable backup",
            "runtime hook scripts are no longer rewritten unconditionally",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, surface)
        self.assertNotIn("always rewritten", surface)


if __name__ == "__main__":
    unittest.main()

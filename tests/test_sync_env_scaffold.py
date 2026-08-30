"""Black-box: ai-specs sync regenerates harness env example + .envrc warnings."""
from __future__ import annotations

import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
VAULT_RECIPE = ROOT / "catalog" / "recipes" / "vault-canonical-store" / "recipe.toml"


def _vault_version() -> str:
    with open(VAULT_RECIPE, "rb") as fh:
        return tomllib.load(fh)["recipe"]["version"]


def _sync_env() -> dict[str, str]:
    return {
        **os.environ,
        "AI_SPECS_HOME": str(ROOT),
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
    }


class SyncEnvScaffoldTests(unittest.TestCase):
    def _init_project(self, project: Path) -> None:
        subprocess.run(
            [str(CLI), "init", str(project)],
            check=True,
            text=True,
            capture_output=True,
            env=_sync_env(),
        )

    def _enable_vault(self, project: Path) -> None:
        toml_path = project / "ai-specs" / "ai-specs.toml"
        text = toml_path.read_text(encoding="utf-8")
        if "[recipes.vault-canonical-store]" not in text:
            text += (
                f"\n[recipes.vault-canonical-store]\n"
                f'enabled = true\nversion = "{_vault_version()}"\n'
            )
            toml_path.write_text(text, encoding="utf-8")

    def _run_sync(self, project: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(CLI), "sync", str(project)],
            text=True,
            capture_output=True,
            env=_sync_env(),
        )

    def test_sync_regenerates_env_example_for_enabled_recipe(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self._init_project(project)
            # First sync without vault — example should not list CANONICAL_VAULT_PATH
            proc0 = self._run_sync(project)
            self.assertEqual(proc0.returncode, 0, proc0.stderr)
            example = project / "ai-specs.env.example"
            self.assertTrue(example.is_file())
            self.assertNotIn("CANONICAL_VAULT_PATH", example.read_text(encoding="utf-8"))

            self._enable_vault(project)
            proc = self._run_sync(project)
            self.assertEqual(proc.returncode, 0, proc.stderr + proc.stdout)
            text = example.read_text(encoding="utf-8")
            self.assertIn("CANONICAL_VAULT_PATH=", text)
            self.assertIn("vault-canonical", text)

    def test_sync_creates_envrc_managed_block(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self._init_project(project)
            self.assertFalse((project / ".envrc").exists())
            proc = self._run_sync(project)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            envrc = (project / ".envrc").read_text(encoding="utf-8")
            self.assertIn("# managed-by: ai-specs (do not remove block)", envrc)
            self.assertIn("dotenv_if_exists .env", envrc)
            self.assertIn("dotenv_if_exists ai-specs.env", envrc)
            self.assertIn("# end managed-by: ai-specs", envrc)

    def test_sync_preserves_custom_envrc(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self._init_project(project)
            (project / ".envrc").write_text("use nix\n", encoding="utf-8")
            proc = self._run_sync(project)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            text = (project / ".envrc").read_text(encoding="utf-8")
            self.assertTrue(text.startswith("use nix\n"))
            self.assertIn("# managed-by: ai-specs (do not remove block)", text)

    def test_sync_warns_missing_env_values_nonfatal(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self._init_project(project)
            self._enable_vault(project)
            (project / "ai-specs.env").write_text(
                "UNRELATED=1\n",
                encoding="utf-8",
            )
            app_env = project / ".env"
            app_env.write_text("APP=keep\n", encoding="utf-8")
            proc = self._run_sync(project)
            self.assertEqual(proc.returncode, 0, proc.stderr + proc.stdout)
            combined = proc.stdout + proc.stderr
            self.assertIn(
                "CANONICAL_VAULT_PATH sin valor en ai-specs.env",
                combined,
            )
            self.assertIn("configure-recipes", combined)
            self.assertEqual(app_env.read_text(encoding="utf-8"), "APP=keep\n")
            self.assertEqual(
                (project / "ai-specs.env").read_text(encoding="utf-8"),
                "UNRELATED=1\n",
            )

    def test_sync_does_not_create_ai_specs_env_or_nested_examples(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self._init_project(project)
            self._enable_vault(project)
            app_env = project / ".env"
            app_env.write_text("APP=keep\n", encoding="utf-8")
            self.assertFalse((project / "ai-specs.env").exists())
            proc = self._run_sync(project)
            self.assertEqual(proc.returncode, 0, proc.stderr + proc.stdout)
            self.assertFalse((project / "ai-specs.env").exists())
            self.assertEqual(app_env.read_text(encoding="utf-8"), "APP=keep\n")
            self.assertFalse((project / "ai-specs" / ".env.example").exists())
            self.assertFalse((project / "ai-specs" / ".envrc.example").exists())
            self.assertTrue((project / "ai-specs.env.example").is_file())


if __name__ == "__main__":
    unittest.main()

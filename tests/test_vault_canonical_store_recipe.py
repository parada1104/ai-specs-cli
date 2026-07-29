"""Tests for the vault-canonical-store catalog recipe."""
from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, deps_skill_dir  # noqa: E402

RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_DIR = ROOT / "catalog" / "recipes" / "vault-canonical-store"
CLI = ROOT / "bin" / "ai-specs"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
KEPANO_URL = "https://github.com/kepano/obsidian-skills.git"
KEPANO_SKILLS = (
    ("obsidian-markdown", "skills/obsidian-markdown"),
    ("obsidian-bases", "skills/obsidian-bases"),
    ("json-canvas", "skills/json-canvas"),
    ("obsidian-cli", "skills/obsidian-cli"),
    ("defuddle", "skills/defuddle"),
)


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class VaultCanonicalStoreRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.mat = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal_vcs"
        )

    def test_recipe_validates_and_provides_canonical_store(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "vault-canonical-store")
        self.assertIn("canonical-store", {c.id for c in recipe.capabilities})
        skill_ids = {s.id for s in recipe.skills}
        self.assertIn("vault-context", skill_ids)

    def test_recipe_declares_kepano_dep_skills(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        by_id = {s.id: s for s in recipe.skills}
        self.assertEqual(by_id["vault-context"].source, "bundled")
        for skill_id, subpath in KEPANO_SKILLS:
            self.assertIn(skill_id, by_id, f"missing kepano skill {skill_id}")
            skill = by_id[skill_id]
            self.assertEqual(skill.source, "dep", skill_id)
            self.assertEqual(skill.url, KEPANO_URL, skill_id)
            self.assertEqual(skill.path, subpath, skill_id)

    def test_recipe_mcp_uses_env_owned_wrapper_not_path_arg(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(len(recipe.mcp), 1)
        mcp = recipe.mcp[0]
        self.assertEqual(mcp.id, "vault-canonical")
        self.assertEqual(mcp.config.get("command"), "bash")
        args = mcp.config.get("args") or []
        self.assertEqual(
            args,
            ["ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"],
        )
        # Path must NOT appear as an MCP argv placeholder — wrapper reads env.
        joined = " ".join(str(a) for a in args)
        self.assertNotIn("CANONICAL_VAULT_PATH", joined)
        self.assertNotIn("server-filesystem", joined)
        env = mcp.config.get("env") or {}
        self.assertEqual(list(env.keys()), ["CANONICAL_VAULT_PATH"])
        self.assertNotIn("OBSIDIAN", str(env))
        wrapper = RECIPE_DIR / "templates" / "vault-fs-mcp.sh"
        self.assertTrue(wrapper.is_file())
        self.assertIn("server-filesystem@2025.7.1", wrapper.read_text())
        # zod pinned to 3.x: the package inherits zod from the SDK (now 4.x) and its
        # zod-to-json-schema@3 emits empty inputSchemas for zod 4 definitions.
        self.assertIn("zod@3", wrapper.read_text())
        self.assertIn("CANONICAL_VAULT_PATH", wrapper.read_text())
        self.assertNotIn("OBSIDIAN_VAULT_PATH", wrapper.read_text())

    def test_recipe_version_is_1_2_0(self):
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            version = tomllib.load(fh)["recipe"]["version"]
        self.assertEqual(version, "1.2.0")

    def test_materializes_vault_context_skill(self):
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            version = tomllib.load(fh)["recipe"]["version"]
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp) / "project"
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.vault-canonical-store]\nenabled = true\nversion = \"{version}\"\n"
            )
            env = {
                **os.environ,
                "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            }
            with mock.patch.dict(os.environ, env, clear=False):
                self.assertEqual(self.mat.materialize_recipes(project_root, ROOT), 0)
            skill = (
                recipe_root(project_root, "vault-canonical-store")
                / "skills"
                / "vault-context"
                / "SKILL.md"
            )
            self.assertTrue(skill.is_file())

    def test_materializes_kepano_dep_skills_from_fixture(self):
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            version = tomllib.load(fh)["recipe"]["version"]
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp) / "project"
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.vault-canonical-store]\nenabled = true\nversion = \"{version}\"\n"
            )
            env = {
                **os.environ,
                "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            }
            with mock.patch.dict(os.environ, env, clear=False):
                self.assertEqual(self.mat.materialize_recipes(project_root, ROOT), 0)
            for skill_id, _ in KEPANO_SKILLS:
                skill_md = deps_skill_dir(project_root, skill_id) / "SKILL.md"
                self.assertTrue(skill_md.is_file(), f"missing dep skill {skill_md}")

    def test_vault_context_cross_links_obsidian_skills(self):
        text = (RECIPE_DIR / "skills" / "vault-context" / "SKILL.md").read_text()
        for needle in (
            "obsidian-markdown",
            "obsidian-bases",
            "json-canvas",
            "obsidian-cli",
            "defuddle",
        ):
            self.assertIn(needle, text)
        self.assertIn("Do not hardcode", text)

    def test_readme_documents_mcp_and_spaced_paths(self):
        readme = (RECIPE_DIR / "README.md").read_text()
        self.assertIn("vault-canonical", readme)
        self.assertIn("CANONICAL_VAULT_PATH", readme)
        self.assertIn("Mobile Documents", readme)
        for skill_id, _ in KEPANO_SKILLS:
            self.assertIn(skill_id, readme)


class VaultCanonicalMcpSyncTests(unittest.TestCase):
    """Spaced-path MCP arg rendering across agents for the vault preset."""

    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-vault-mcp-"))
        workspace = tmp / "workspace"
        workspace.mkdir()
        (workspace / "packages" / "a").mkdir(parents=True)
        (workspace / "packages" / "b").mkdir(parents=True)
        return workspace

    def test_sync_vault_mcp_uses_wrapper_across_agents(self):
        workspace = self.make_workspace()
        try:
            with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
                version = tomllib.load(fh)["recipe"]["version"]
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
            (workspace / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name = 'fixture-vault-mcp'\n\n"
                "[agents]\n"
                "enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']\n\n"
                "[recipes.vault-canonical-store]\n"
                "enabled = true\n"
                f'version = "{version}"\n'
                "[recipes.vault-canonical-store.config]\n"
                "vault_scope = 'nnodes/proyectos/fixture'\n"
            )
            env = {
                **os.environ,
                "AI_SPECS_HOME": str(ROOT),
                "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
                "CANONICAL_VAULT_PATH": "/tmp/Mobile Documents/vault scope",
            }
            subprocess.run(
                [str(CLI), "sync", str(workspace)],
                check=True,
                text=True,
                env=env,
            )

            wrapper = (
                workspace
                / "ai-specs"
                / "recipes"
                / "vault-canonical-store"
                / "bin"
                / "vault-fs-mcp.sh"
            )
            self.assertTrue(wrapper.is_file(), f"missing materialized wrapper {wrapper}")

            script_arg = "ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"
            targets = {
                "claude": workspace / ".mcp.json",
                "cursor": workspace / ".cursor" / "mcp.json",
                "omp": workspace / ".omp" / "mcp.json",
            }
            for agent, path in targets.items():
                self.assertTrue(path.is_file(), f"missing {agent} mcp at {path}")
                parsed = json.loads(path.read_text())
                servers = parsed.get("mcpServers") or parsed.get("mcp") or {}
                self.assertIn("vault-canonical", servers, agent)
                cfg = servers["vault-canonical"]
                self.assertEqual(cfg.get("command"), "bash", agent)
                args = cfg.get("args") or []
                self.assertEqual(args, [script_arg], agent)
                # Spaced path must not be baked into rendered MCP args.
                joined = " ".join(args)
                self.assertNotIn("Mobile", joined)
                self.assertNotIn("${CANONICAL_VAULT_PATH}", joined)
                env_block = cfg.get("env") or {}
                self.assertIn("CANONICAL_VAULT_PATH", env_block)

            opencode = json.loads((workspace / "opencode.json").read_text())
            demo = opencode["mcp"]["vault-canonical"]
            cmd = demo["command"]
            self.assertEqual(cmd, ["bash", script_arg])
            self.assertNotIn("$CANONICAL_VAULT_PATH", cmd)
            env_block = demo.get("environment") or {}
            self.assertEqual(env_block.get("CANONICAL_VAULT_PATH"), "{env:CANONICAL_VAULT_PATH}")
        finally:
            shutil.rmtree(workspace.parent)


if __name__ == "__main__":
    unittest.main()

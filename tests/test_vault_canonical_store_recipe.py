"""Tests for the vault-canonical-store catalog recipe."""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RECIPE_ID = "vault-canonical-store"
RECIPE_DIR = ROOT / "catalog" / "recipes" / RECIPE_ID
RECIPE_VERSION = "1.2.0"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
KEPANO_URL = "https://github.com/kepano/obsidian-skills.git"
KEPANO_SKILLS = (
    ("obsidian-markdown", "skills/obsidian-markdown"),
    ("obsidian-bases", "skills/obsidian-bases"),
    ("json-canvas", "skills/json-canvas"),
    ("obsidian-cli", "skills/obsidian-cli"),
    ("defuddle", "skills/defuddle"),
)

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import isolated_home  # noqa: E402
from _cache_paths import deps_skill_dir, recipe_skill_dir  # noqa: E402


def _recipe_toml() -> dict:
    with (RECIPE_DIR / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)


class _CliTestCase(unittest.TestCase):
    """Shared isolated CLI home (one home per scenario) and hermetic launcher.

    ``sync`` materializes the kepano dep skills by cloning from
    ``AI_SPECS_VENDOR_FIXTURE_ROOT`` (run.sh exports the same seam), so the
    launcher always carries the fixture root; a shared ``isolated_home`` keeps
    cache state visible across every command in a scenario.
    """

    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="vcs-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def _env(self, root: Path, extra: dict | None = None) -> dict:
        env = {
            **os.environ,
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(root),
            "TMPDIR": str(root),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "LC_ALL": "C",
            "LANG": "C",
        }
        if extra:
            env.update(extra)
        return env

    def _invoke(self, root: Path, *args: str, env_extra: dict | None = None):
        env = self._env(root, env_extra)
        return subprocess.run(
            [str(CLI), *args, str(root)],
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )

    def _sync(self, root: Path, env_extra: dict | None = None):
        r = self._invoke(root, "sync", env_extra=env_extra)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    def _project(self, config_extra: str = "") -> Path:
        td = tempfile.TemporaryDirectory(prefix="vcs-project-")
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture'\n\n"
            "[agents]\n"
            "enabled = ['claude']\n\n"
            f"[recipes.{RECIPE_ID}]\n"
            "enabled = true\n"
            f'version = "{RECIPE_VERSION}"\n'
            + config_extra
        )
        return root


class VaultCanonicalStoreRecipeTests(_CliTestCase):
    def test_recipe_validates_and_provides_canonical_store(self):
        root = self._project()
        result = self._invoke(root, "recipe", "list")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(RECIPE_ID, result.stdout)
        self.assertIn(RECIPE_VERSION, result.stdout)

        data = _recipe_toml()
        self.assertEqual(data["recipe"]["id"], RECIPE_ID)
        cap_ids = [c["id"] for c in data.get("capabilities", [])]
        self.assertIn("canonical-store", cap_ids)
        skill_ids = {s["id"] for s in data["provides"]["skills"]}
        self.assertIn("vault-context", skill_ids)

    def test_recipe_declares_kepano_dep_skills(self):
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        root = self._project()
        self._sync(root)

        by_id = {s["id"]: s for s in _recipe_toml()["provides"]["skills"]}
        self.assertEqual(by_id["vault-context"]["source"], "bundled")
        for skill_id, subpath in KEPANO_SKILLS:
            self.assertIn(skill_id, by_id, f"missing kepano skill {skill_id}")
            skill = by_id[skill_id]
            self.assertEqual(skill["source"], "dep", skill_id)
            self.assertEqual(skill["url"], KEPANO_URL, skill_id)
            self.assertEqual(skill["path"], subpath, skill_id)
            skill_md = deps_skill_dir(root, skill_id, cli_home=self.home) / "SKILL.md"
            self.assertTrue(skill_md.is_file(), f"missing dep skill {skill_md}")

    def test_recipe_mcp_uses_env_owned_wrapper_not_path_arg(self):
        root = self._project()
        cfg = self._invoke(root, "recipe", "configure", RECIPE_ID, "--inspect", "--json")
        self.assertEqual(cfg.returncode, 0, cfg.stderr)
        inspect = json.loads(cfg.stdout)
        self.assertEqual(inspect["recipe"]["id"], RECIPE_ID)

        data = _recipe_toml()
        mcp_blocks = data["provides"]["mcp"]
        self.assertEqual(len(mcp_blocks), 1)
        mcp = mcp_blocks[0]
        self.assertEqual(mcp["id"], "vault-canonical")
        self.assertEqual(mcp.get("command"), "bash")
        args = mcp.get("args") or []
        self.assertEqual(
            args,
            ["ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"],
        )
        # Path must NOT appear as an MCP argv placeholder — wrapper reads env.
        joined = " ".join(str(a) for a in args)
        self.assertNotIn("CANONICAL_VAULT_PATH", joined)
        self.assertNotIn("server-filesystem", joined)
        env = mcp.get("env") or {}
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
        root = self._project()
        result = self._invoke(root, "recipe", "list")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("vault-canonical-store", result.stdout)
        self.assertIn(RECIPE_VERSION, result.stdout)
        self.assertEqual(_recipe_toml()["recipe"]["version"], RECIPE_VERSION)

    def test_materializes_vault_context_skill(self):
        root = self._project()
        self._sync(root)
        skill = recipe_skill_dir(root, RECIPE_ID, "vault-context", cli_home=self.home) / "SKILL.md"
        self.assertTrue(skill.is_file(), f"missing vault-context skill at {skill}")

    def test_materializes_kepano_dep_skills_from_fixture(self):
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        root = self._project()
        self._sync(root)
        for skill_id, _ in KEPANO_SKILLS:
            skill_md = deps_skill_dir(root, skill_id, cli_home=self.home) / "SKILL.md"
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


class VaultCanonicalMcpSyncTests(_CliTestCase):
    """Spaced-path MCP arg rendering across agents for the vault preset."""

    def test_sync_vault_mcp_uses_wrapper_across_agents(self):
        td = tempfile.TemporaryDirectory(prefix="ai-specs-vault-mcp-")
        self.addCleanup(td.cleanup)
        workspace = Path(td.name) / "workspace"
        (workspace / "packages" / "a").mkdir(parents=True)
        (workspace / "packages" / "b").mkdir(parents=True)
        (workspace / "ai-specs" / "skills").mkdir(parents=True)
        (workspace / "ai-specs" / "commands").mkdir()
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture-vault-mcp'\n\n"
            "[agents]\n"
            "enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']\n\n"
            "[recipes.vault-canonical-store]\n"
            "enabled = true\n"
            f'version = "{RECIPE_VERSION}"\n'
            "[recipes.vault-canonical-store.config]\n"
            "vault_scope = 'nnodes/proyectos/fixture'\n"
        )
        self._sync(
            workspace,
            env_extra={"CANONICAL_VAULT_PATH": "/tmp/Mobile Documents/vault scope"},
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


if __name__ == "__main__":
    unittest.main()
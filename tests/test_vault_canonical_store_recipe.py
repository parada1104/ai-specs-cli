"""Black-box vault-canonical-store recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema declarations, dep-skill declarations, env-owned MCP
wrapper, materialized artifacts, MCP arg rendering across agents) through the
CLI process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + exact id + plan) and via ``tomllib`` reads of
  the catalog recipe.toml.
- Materialization is observable via ``sync``: bundled skill under the cache's
  ``.recipe/`` tree, dep skills under the cache's ``.deps/`` tree, and the
  wrapper template under the project harness tree. Dep skills resolve offline
  through AI_SPECS_VENDOR_FIXTURE_ROOT (kepano fixture checkout).
- MCP arg rendering is observable via ``sync`` across all five enabled agents.
"""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import CLI, cache_project_dir, invoke, isolated_home, normalize_output  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "vault-canonical-store"
RECIPE_ID = "vault-canonical-store"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
KEPANO_URL = "https://github.com/kepano/obsidian-skills.git"
KEPANO_SKILLS = (
    ("obsidian-markdown", "skills/obsidian-markdown"),
    ("obsidian-bases", "skills/obsidian-bases"),
    ("json-canvas", "skills/json-canvas"),
    ("obsidian-cli", "skills/obsidian-cli"),
    ("defuddle", "skills/defuddle"),
)
SCRIPT_ARG = "ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _make_manifest(root: Path, name: str = "fixture") -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = ['claude']\n"
    )


def invoke_env(project_root: Path, *args: str, cli_home: Path,
               extra_env: dict[str, str]) -> tuple[int, str, str]:
    """Run bin/ai-specs with extra env vars (stdin closed) — like
    ``_blackbox.invoke`` but for env the helper does not forward (offline
    fixture root, MCP env). Returns (returncode, stdout, stderr)."""
    tmp = tempfile.TemporaryDirectory(prefix="ai-specs-env-")
    try:
        temp = Path(tmp.name)
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(temp / "home"),
            "TMPDIR": str(temp),
            "AI_SPECS_HOME": str(cli_home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
            **extra_env,
        }
        (temp / "home").mkdir(parents=True, exist_ok=True)
        proc = subprocess.run(
            [str(CLI), *args, str(project_root)],
            cwd=ROOT, env=env, text=True, capture_output=True, check=False,
            input="",
        )
        roots = (project_root, cli_home, temp)
        return proc.returncode, normalize_output(proc.stdout, roots), normalize_output(proc.stderr, roots)
    finally:
        tmp.cleanup()


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-vault-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id, cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {recipe_id} failed: {result.stdout}{result.stderr}",
        )
        return result

    def sync_with_fixture_env(self):
        """Sync with the offline kepano fixture root (dep skills vendor locally)."""
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        rc, out, err = invoke_env(
            self.root, "sync", cli_home=self.home,
            extra_env={"AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE)},
        )
        return (rc, out + "\n", err + "\n")

    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)


class VaultCanonicalStoreRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_recipe_validates_and_provides_canonical_store(self):
        """Recipe validates through the CLI and declares canonical-store +
        vault-context.

        ``recipe add`` runs schema validation and adds by the exact id; the
        declared capability/skill sets are read from the catalog declaration.
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "vault-canonical-store must pass schema validation and add by its exact id",
        )
        self.assertIn(
            "- mcp: vault-canonical", result.stdout,
            "recipe add plan must declare the vault-canonical MCP",
        )
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        self.assertIn("canonical-store", {c["id"] for c in raw["capabilities"]})
        skill_ids = {s["id"] for s in raw["provides"]["skills"]}
        self.assertIn("vault-context", skill_ids)

    def test_recipe_declares_kepano_dep_skills(self):
        """The kepano obsidian skills are declared as dep skills from the kepano URL."""
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        by_id = {s["id"]: s for s in raw["provides"]["skills"]}
        self.assertEqual(by_id["vault-context"]["source"], "bundled")
        for skill_id, subpath in KEPANO_SKILLS:
            self.assertIn(skill_id, by_id, f"missing kepano skill {skill_id}")
            skill = by_id[skill_id]
            self.assertEqual(skill["source"], "dep", skill_id)
            self.assertEqual(skill["url"], KEPANO_URL, skill_id)
            self.assertEqual(skill["path"], subpath, skill_id)

    def test_recipe_mcp_uses_env_owned_wrapper_not_path_arg(self):
        """MCP argv keeps the vault path out of args; the wrapper reads env.

        Declared MCP config and wrapper template are read from the catalog
        recipe (validated by ``recipe add``).
        """
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        mcps = raw["provides"]["mcp"]
        self.assertEqual(len(mcps), 1)
        mcp = mcps[0]
        self.assertEqual(mcp["id"], "vault-canonical")
        self.assertEqual(mcp.get("command"), "bash")
        args = mcp.get("args") or []
        self.assertEqual(args, [SCRIPT_ARG])
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
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["version"], "1.2.0")

    def test_materializes_vault_context_skill(self):
        """Sync materializes the bundled vault-context skill (offline fixture)."""
        self.recipe_add(RECIPE_ID)
        rc, out, err = self.sync_with_fixture_env()
        self.assertEqual(rc, 0, out + err)
        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "vault-context" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

    def test_materializes_kepano_dep_skills_from_fixture(self):
        """Sync vendors all declared kepano dep skills from the offline fixture."""
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        self.recipe_add(RECIPE_ID)
        rc, out, err = self.sync_with_fixture_env()
        self.assertEqual(rc, 0, out + err)
        for skill_id, _ in KEPANO_SKILLS:
            skill_md = self.cache() / ".deps" / skill_id / "skills" / skill_id / "SKILL.md"
            self.assertTrue(skill_md.is_file(), f"missing dep skill {skill_md}")

    def test_vault_context_cross_links_obsidian_skills(self):
        """The vault-context skill cross-links every kepano skill."""
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
        """The README documents the MCP and the spaced-vault-path handling."""
        readme = (RECIPE_DIR / "README.md").read_text()
        self.assertIn("vault-canonical", readme)
        self.assertIn("CANONICAL_VAULT_PATH", readme)
        self.assertIn("Mobile Documents", readme)
        for skill_id, _ in KEPANO_SKILLS:
            self.assertIn(skill_id, readme)


class VaultCanonicalMcpSyncTests(_CliFixtureMixin, unittest.TestCase):
    """Spaced-path MCP arg rendering across agents for the vault preset."""

    def _synced_workspace(self) -> Path:
        """Init a workspace, enable the recipe for all five agents, and sync
        with CANONICAL_VAULT_PATH set (spaced path)."""
        result = invoke(self.root, "init", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture-vault-mcp'\n\n"
            "[agents]\n"
            "enabled = ['claude', 'cursor', 'opencode', 'pi', 'omp']\n\n"
            f"[recipes.{RECIPE_ID}]\n"
            "enabled = true\n\n"
            f"[recipes.{RECIPE_ID}.config]\n"
            "vault_scope = 'nnodes/proyectos/fixture'\n"
        )
        self.assertTrue(
            KEPANO_FIXTURE.is_dir(),
            f"missing offline fixture at {KEPANO_FIXTURE}",
        )
        rc, out, err = invoke_env(
            self.root, "sync", cli_home=self.home,
            extra_env={
                "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
                "CANONICAL_VAULT_PATH": "/tmp/Mobile Documents/vault scope",
            },
        )
        self.assertEqual(rc, 0, out + err)
        return self.root

    def test_sync_vault_mcp_uses_wrapper_across_agents(self):
        workspace = self._synced_workspace()

        wrapper = (
            workspace / "ai-specs" / "recipes" / RECIPE_ID
            / "bin" / "vault-fs-mcp.sh"
        )
        self.assertTrue(wrapper.is_file(), f"missing materialized wrapper {wrapper}")

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
            self.assertEqual(args, [SCRIPT_ARG], agent)
            # Spaced path must not be baked into rendered MCP args.
            joined = " ".join(args)
            self.assertNotIn("Mobile", joined)
            self.assertNotIn("${CANONICAL_VAULT_PATH}", joined)
            env_block = cfg.get("env") or {}
            self.assertIn("CANONICAL_VAULT_PATH", env_block)

        opencode = json.loads((workspace / "opencode.json").read_text())
        demo = opencode["mcp"]["vault-canonical"]
        cmd = demo["command"]
        self.assertEqual(cmd, ["bash", SCRIPT_ARG])
        self.assertNotIn("$CANONICAL_VAULT_PATH", cmd)
        env_block = demo.get("environment") or {}
        self.assertEqual(env_block.get("CANONICAL_VAULT_PATH"), "{env:CANONICAL_VAULT_PATH}")


if __name__ == "__main__":
    unittest.main()

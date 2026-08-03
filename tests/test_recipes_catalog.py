"""Guard docs/recipes-catalog.md against drift from catalog recipe.toml manifests."""

from __future__ import annotations

import re
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG_DOC = ROOT / "docs" / "recipes-catalog.md"
RECIPES_DIR = ROOT / "catalog" / "recipes"

# Recipe ids with bundled MCP presets — must appear in the "Installs MCP" column.
MCP_RECIPES: dict[str, str] = {
    "trello-mcp-workflow": "trello",
    "vault-canonical-store": "vault-canonical",
    "playwright-mcp": "playwright",
}

# User-facing config keys that must be documented in the per-recipe section.
CONFIG_KEYS_IN_CATALOG: dict[str, list[str]] = {
    "worktree-flow": ["WORKTREE_GATE_PROTECTED"],
    "plan-build-flow": ["artifact_store_default"],
    "trello-mcp-workflow": [
        "board_id",
        "forbidden_tools",
        "card_validation_required",
    ],
}


def _catalog_recipe_dirs() -> list[Path]:
    return sorted(p for p in RECIPES_DIR.iterdir() if p.is_dir())


class RecipesCatalogDriftTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()

    def test_shipped_catalog_has_no_internal_test_recipes(self):
        leaked = [p.name for p in RECIPES_DIR.iterdir() if p.is_dir() and p.name.startswith("test-")]
        self.assertEqual(leaked, [], f"internal fixtures must not ship in catalog: {leaked}")

    def test_at_a_glance_table_has_installs_mcp_column(self):
        self.assertIn("| Installs MCP |", self.catalog)

    def test_mcp_recipes_documented_in_catalog(self):
        for recipe_id, mcp_id in MCP_RECIPES.items():
            with self.subTest(recipe=recipe_id, mcp=mcp_id):
                self.assertIn(f"| `{mcp_id}` |", self.catalog)
                recipe_toml = RECIPES_DIR / recipe_id / "recipe.toml"
                data = tomllib.loads(recipe_toml.read_text())
                provides_mcp = (data.get("provides") or {}).get("mcp") or []
                if isinstance(provides_mcp, dict):
                    provides_mcp = [provides_mcp]
                mcp_ids = {entry["id"] for entry in provides_mcp}
                self.assertIn(mcp_id, mcp_ids)

    def test_documented_config_keys_exist_in_recipe_toml(self):
        for recipe_id, keys in CONFIG_KEYS_IN_CATALOG.items():
            recipe_toml = RECIPES_DIR / recipe_id / "recipe.toml"
            text = recipe_toml.read_text()
            section = self._recipe_section(recipe_id)
            for key in keys:
                with self.subTest(recipe=recipe_id, key=key):
                    self.assertIn(key, text)
                    self.assertIn(key, section)

    def test_worktree_gate_runtime_hook_documented(self):
        section = self._recipe_section("worktree-flow")
        self.assertIn("worktree-gate", section)
        self.assertIn("runtime-hooks.md", section)

    def test_vault_recipe_no_longer_claims_mcp_is_external_only(self):
        section = self._recipe_section("vault-canonical-store")
        self.assertNotIn("does **not** declare the MCP server", section)
        self.assertIn("vault-canonical", section)

    def _recipe_section(self, recipe_id: str) -> str:
        pattern = rf"## {re.escape(recipe_id)}\n(.*?)(?=\n## |\Z)"
        match = re.search(pattern, self.catalog, re.DOTALL)
        self.assertIsNotNone(match, f"missing ## {recipe_id} section")
        return match.group(1)


CAPABILITIES_DOC = ROOT / "docs" / "capabilities.md"


class GitlabMrFlowDocsContractTests(unittest.TestCase):
    """Phase 4: docs contract tests for gitlab-mr-flow."""

    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()
        cls.capabilities = CAPABILITIES_DOC.read_text()
        cls.readme_path = RECIPES_DIR / "gitlab-mr-flow" / "README.md"
        cls.readme_text = cls.readme_path.read_text() if cls.readme_path.is_file() else ""

    # --- README exists and has real content ---

    def test_readme_exists(self):
        """catalog/recipes/gitlab-mr-flow/README.md exists."""
        self.assertTrue(self.readme_path.is_file(), "gitlab-mr-flow README.md must exist")

    def test_readme_is_not_placeholder(self):
        """README has real content, not a placeholder."""
        self.assertNotIn("Placeholder", self.readme_text,
                         "README must not contain placeholder text")

    def test_readme_has_overview(self):
        """README describes what gitlab-mr-flow provides."""
        self.assertIn("GitLab", self.readme_text)
        self.assertIn("merge request", self.readme_text.lower())

    def test_readme_has_prerequisites(self):
        """README documents glab CLI as a prerequisite."""
        self.assertIn("glab", self.readme_text)

    def test_readme_has_config_table(self):
        """README documents base_branch config only (no provider key)."""
        self.assertIn("base_branch", self.readme_text)
        self.assertNotIn("| `provider`", self.readme_text)

    def test_readme_documents_vcs_pr_flow_capability(self):
        """README declares the vcs-pr-flow capability."""
        self.assertIn("vcs-pr-flow", self.readme_text)

    def test_readme_has_safety_note(self):
        """README includes a safety note about explicit push/merge."""
        self.assertIn("Safety", self.readme_text)
        self.assertIn("explicit", self.readme_text.lower())

    def test_readme_references_sibling_git_pr_flow(self):
        """README cross-links to git-pr-flow for GitHub users."""
        self.assertIn("git-pr-flow", self.readme_text)

    # --- recipes-catalog.md section ---

    def test_catalog_has_gitlab_mr_flow_section(self):
        """recipes-catalog.md has a ## gitlab-mr-flow section."""
        pattern = r"## gitlab-mr-flow\n"
        self.assertRegex(self.catalog, pattern,
                         "recipes-catalog.md must have a ## gitlab-mr-flow section")

    def test_catalog_at_a_glance_includes_gitlab_mr_flow(self):
        """The 'At a glance' table has a row for gitlab-mr-flow."""
        self.assertIn("gitlab-mr-flow", self.catalog)
        # Check it appears in the At a glance table area (between ## At a glance and ---)
        glance_start = self.catalog.find("## At a glance")
        self.assertGreater(glance_start, 0, "At a glance section must exist")
        glance_end = self.catalog.find("\n---\n", glance_start)
        table_section = self.catalog[glance_start:glance_end] if glance_end > 0 else self.catalog[glance_start:]
        self.assertIn("gitlab-mr-flow", table_section,
                       "gitlab-mr-flow must appear in the At a glance table")

    def test_catalog_section_has_config_table(self):
        """The catalog section documents base_branch only."""
        section = self._recipe_section("gitlab-mr-flow")
        self.assertIn("base_branch", section)
        self.assertNotIn("| `provider`", section)

    def test_catalog_section_links_readme(self):
        """The catalog section links to the full README."""
        section = self._recipe_section("gitlab-mr-flow")
        self.assertIn("README.md", section)

    def test_catalog_section_mentions_no_mcp(self):
        """The catalog section notes that gitlab-mr-flow installs no MCP server."""
        section = self._recipe_section("gitlab-mr-flow")
        self.assertIn("Installs no MCP server", section)

    def test_catalog_section_cross_links_git_pr_flow(self):
        """The catalog section cross-links to git-pr-flow for GitHub users."""
        section = self._recipe_section("gitlab-mr-flow")
        self.assertIn("git-pr-flow", section)

    # --- capabilities.md ---

    def test_capabilities_mentions_gitlab_mr_flow_as_provider(self):
        """capabilities.md lists gitlab-mr-flow as a vcs-pr-flow provider."""
        self.assertIn("gitlab-mr-flow", self.capabilities)

    def _recipe_section(self, recipe_id: str) -> str:
        pattern = rf"## {re.escape(recipe_id)}\n(.*?)(?=\n## |\Z)"
        match = re.search(pattern, self.catalog, re.DOTALL)
        self.assertIsNotNone(match, f"missing ## {recipe_id} section")
        return match.group(1)


class BitbucketPrFlowDocsContractTests(unittest.TestCase):
    """Docs contract tests for bitbucket-pr-flow."""

    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()
        cls.capabilities = CAPABILITIES_DOC.read_text()
        cls.readme_path = RECIPES_DIR / "bitbucket-pr-flow" / "README.md"
        cls.readme_text = cls.readme_path.read_text() if cls.readme_path.is_file() else ""

    def test_readme_exists(self):
        self.assertTrue(self.readme_path.is_file())

    def test_readme_has_config_table_without_provider(self):
        self.assertIn("base_branch", self.readme_text)
        self.assertNotIn("| `provider`", self.readme_text)

    def test_catalog_has_bitbucket_section(self):
        self.assertRegex(self.catalog, r"## bitbucket-pr-flow\n")

    def test_catalog_section_has_base_branch_only(self):
        section = self._recipe_section("bitbucket-pr-flow")
        self.assertIn("base_branch", section)
        self.assertNotIn("| `provider`", section)

    def test_capabilities_mentions_bitbucket_pr_flow(self):
        self.assertIn("bitbucket-pr-flow", self.capabilities)

    def _recipe_section(self, recipe_id: str) -> str:
        pattern = rf"## {re.escape(recipe_id)}\n(.*?)(?=\n## |\Z)"
        match = re.search(pattern, self.catalog, re.DOTALL)
        self.assertIsNotNone(match, f"missing ## {recipe_id} section")
        return match.group(1)


class GitPrFlowDocsContractTests(unittest.TestCase):
    """Docs contract tests for git-pr-flow — mirrors GitLab/Bitbucket symmetry."""

    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()
        cls.capabilities = CAPABILITIES_DOC.read_text()
        cls.readme_path = RECIPES_DIR / "git-pr-flow" / "README.md"
        cls.readme_text = cls.readme_path.read_text() if cls.readme_path.is_file() else ""

    def test_readme_exists(self):
        """catalog/recipes/git-pr-flow/README.md exists."""
        self.assertTrue(self.readme_path.is_file(), "git-pr-flow README.md must exist")

    def test_readme_has_config_table_without_provider(self):
        """README documents base_branch config only (no provider key)."""
        self.assertIn("base_branch", self.readme_text)
        self.assertNotIn("| `provider`", self.readme_text)

    def test_catalog_has_git_pr_flow_section(self):
        """recipes-catalog.md has a ## git-pr-flow section."""
        self.assertRegex(self.catalog, r"## git-pr-flow\n")

    def test_catalog_section_has_base_branch_only(self):
        """The catalog section documents base_branch only (no provider row)."""
        section = self._recipe_section("git-pr-flow")
        self.assertIn("base_branch", section)
        self.assertNotIn("| `provider`", section)

    def test_capabilities_mentions_git_pr_flow(self):
        """capabilities.md lists git-pr-flow as a vcs-pr-flow provider."""
        self.assertIn("git-pr-flow", self.capabilities)

    def _recipe_section(self, recipe_id: str) -> str:
        pattern = rf"## {re.escape(recipe_id)}\n(.*?)(?=\n## |\Z)"
        match = re.search(pattern, self.catalog, re.DOTALL)
        self.assertIsNotNone(match, f"missing ## {recipe_id} section")
        return match.group(1)


class VcsRecipesCatalogTierTests(unittest.TestCase):
    """VCS sibling recipes are documented as Specific tier."""

    @classmethod
    def setUpClass(cls):
        cls.catalog = CATALOG_DOC.read_text()

    def test_at_a_glance_marks_vcs_recipes_as_specific(self):
        glance_start = self.catalog.find("## At a glance")
        glance_end = self.catalog.find("\n---\n", glance_start)
        table = self.catalog[glance_start:glance_end]
        for recipe_id in ("git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow"):
            with self.subTest(recipe=recipe_id):
                line = next((ln for ln in table.splitlines() if recipe_id in ln), "")
                self.assertIn("Specific", line, f"{recipe_id} must be Specific tier in catalog")


if __name__ == "__main__":
    unittest.main()

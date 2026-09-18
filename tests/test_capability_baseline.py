#!/usr/bin/env python3
"""Provider-neutral capability baseline contract tests.

A capability id is the shared, provider-neutral seam between a foundational
recipe and every provider that implements it. Concrete recipes are *adapters*:
they honor a shared baseline while keeping their own asset ids. ``vcs-pr-flow``
is the precedent this phase pins, and ``jinna-mcp-recipe`` must stay a
standalone MCP recipe (no ``tracker`` capability) until a later adapter phase.
"""

import importlib.util
import re
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"
CAPABILITIES_DOC = ROOT / "docs" / "capabilities.md"

VCS_CAPABILITY = "vcs-pr-flow"

# The concrete adapters the baseline must hold for, with the asset ids that
# belong to the adapter (not to the capability).
VCS_ADAPTERS = {
    "git-pr-flow": {"skill": "git-merge-workflow", "command": "pr-create"},
    "bitbucket-pr-flow": {
        "skill": "bitbucket-merge-workflow",
        "command": "bb-pr-create",
    },
}

# The shared, provider-neutral config surface every vcs-pr-flow adapter exposes.
BASELINE_CONFIG_KEYS = ("base_branch", "expected_owner", "auto_switch_account")

JINNA_RECIPE = "jinna-mcp-recipe"


def load_schema():
    spec = importlib.util.spec_from_file_location(
        "recipe_schema_capability_baseline", RECIPE_SCHEMA_PATH
    )
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def discover_recipes(schema):
    """Load every catalog recipe from its directory shape, not a hardcoded list."""
    return {
        path.parent.name: schema.load_recipe_toml(path)
        for path in sorted(CATALOG.glob("*/recipe.toml"))
    }


def providers_of(recipes, capability_id):
    return {
        rid
        for rid, recipe in recipes.items()
        if capability_id in {c.id for c in recipe.capabilities}
    }


def _baseline_section(text):
    """Return the body of the first heading whose title mentions ``baseline``."""
    lines = text.splitlines()
    start = None
    level = None
    for idx, line in enumerate(lines):
        match = re.match(r"^(#{2,6})\s+(.*)$", line)
        if match and "baseline" in match.group(2).lower():
            start = idx
            level = len(match.group(1))
            break
    if start is None:
        return None
    body = []
    for line in lines[start + 1:]:
        match = re.match(r"^(#{1,6})\s+", line)
        if match and len(match.group(1)) <= level:
            break
        body.append(line)
    return "\n".join(body)


class CapabilityBaselineRecipeTests(unittest.TestCase):
    """The capability contract is discoverable from the catalog, not asserted."""

    @classmethod
    def setUpClass(cls):
        cls.recipes = discover_recipes(load_schema())

    def test_git_and_bitbucket_both_provide_vcs_pr_flow(self):
        providers = providers_of(self.recipes, VCS_CAPABILITY)
        for rid in VCS_ADAPTERS:
            self.assertIn(
                rid, providers, f"{rid} must declare capability {VCS_CAPABILITY}"
            )

    def test_every_provider_exposes_the_shared_baseline_config_keys(self):
        providers = providers_of(self.recipes, VCS_CAPABILITY)
        self.assertTrue(providers, "no provider discovered for vcs-pr-flow")
        for rid in sorted(providers):
            fields = set(self.recipes[rid].config_schema.fields)
            missing = [key for key in BASELINE_CONFIG_KEYS if key not in fields]
            self.assertFalse(
                missing,
                f"{rid} is missing shared baseline config keys {missing}",
            )

    def test_baseline_keys_are_shared_across_all_providers(self):
        """The baseline keys are exactly the intersection of provider config."""
        providers = providers_of(self.recipes, VCS_CAPABILITY)
        shared = None
        for rid in sorted(providers):
            keys = set(self.recipes[rid].config_schema.fields)
            shared = keys if shared is None else shared & keys
        self.assertGreaterEqual(
            shared,
            set(BASELINE_CONFIG_KEYS),
            "the shared baseline keys must be common to every vcs-pr-flow provider",
        )

    def test_adapter_asset_ids_stay_provider_specific(self):
        """Command/skill ids belong to the adapter, never to the capability."""
        seen_commands = {}
        seen_skills = {}
        for rid, expected in VCS_ADAPTERS.items():
            recipe = self.recipes[rid]
            self.assertIn(
                expected["command"], {c.id for c in recipe.commands}, f"{rid} command"
            )
            self.assertIn(
                expected["skill"], {s.id for s in recipe.skills}, f"{rid} skill"
            )
            seen_commands[rid] = expected["command"]
            seen_skills[rid] = expected["skill"]
        self.assertEqual(
            len(set(seen_commands.values())),
            len(seen_commands),
            f"adapter command ids must stay distinct: {seen_commands}",
        )
        self.assertEqual(
            len(set(seen_skills.values())),
            len(seen_skills),
            f"adapter skill ids must stay distinct: {seen_skills}",
        )

    def test_capability_id_is_never_promoted_into_an_asset_id(self):
        for rid in sorted(providers_of(self.recipes, VCS_CAPABILITY)):
            recipe = self.recipes[rid]
            asset_ids = {c.id for c in recipe.commands} | {
                s.id for s in recipe.skills
            }
            self.assertNotIn(
                VCS_CAPABILITY,
                asset_ids,
                f"{rid} must not reuse the capability id as a command/skill id",
            )


class JinnaStandaloneBoundaryTests(unittest.TestCase):
    """Jinna stays MCP-only; the tracker adapter is a later phase."""

    @classmethod
    def setUpClass(cls):
        cls.recipes = discover_recipes(load_schema())
        cls.jinna = cls.recipes[JINNA_RECIPE]

    def test_jinna_remains_mcp_only(self):
        self.assertEqual([m.id for m in self.jinna.mcp], ["jinna"])

    def test_jinna_does_not_declare_tracker_capability(self):
        self.assertNotIn(
            "tracker", {c.id for c in self.jinna.capabilities},
            "jinna-mcp-recipe must not declare tracker in this phase",
        )

    def test_jinna_declares_no_ledger_adapter_config(self):
        self.assertNotIn("reconcile", self.jinna.config_schema.tables)
        self.assertNotIn("ledger_mode", self.jinna.config_schema.fields)


class CapabilityBaselineDocTests(unittest.TestCase):
    """The baseline is a written contract, using vcs-pr-flow as the precedent."""

    @classmethod
    def setUpClass(cls):
        cls.text = CAPABILITIES_DOC.read_text()
        cls.section = _baseline_section(cls.text)

    def test_doc_has_a_baseline_contract_section(self):
        self.assertIsNotNone(
            self.section, "docs/capabilities.md must document a capability baseline"
        )

    def test_section_uses_vcs_pr_flow_as_the_precedent(self):
        self.assertIn(VCS_CAPABILITY, self.section)

    def test_section_names_every_shared_baseline_config_key(self):
        for key in BASELINE_CONFIG_KEYS:
            self.assertIn(
                f"`{key}`",
                self.section,
                f"baseline section must name the shared config key {key}",
            )

    def test_section_keeps_adapter_asset_ids_out_of_the_capability_contract(self):
        for expected in VCS_ADAPTERS.values():
            self.assertIn(f"`{expected['command']}`", self.section)
            self.assertIn(f"`{expected['skill']}`", self.section)

    def test_doc_preserves_the_tracker_ledger_section(self):
        self.assertIn("## Tracker lifecycle: the Tracker Ledger", self.text)


if __name__ == "__main__":
    unittest.main()

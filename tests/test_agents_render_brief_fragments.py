"""Tests for recipe brief_fragments support in agents-render.py.

Tests cover:
  - substitute_config: {config.KEY} resolution, missing key verbatim, bare key verbatim,
    {{ }} escape, mixed escape+sub, lone unbalanced brace
  - collect_recipe_brief_fragments: ordering, key-dedup, exact-string dedup,
    no fragments, disabled recipe, empty brief_fragments
  - Section merge: APPEND default, REPLACE opt-in, REPLACE isolation,
    manifest prose never substituted, empty manifest [brief] end-to-end
  - _validate_brief_modes: unknown mode value → error
  - mcp_descriptions override-fills-gap: project wins, recipe fills gap,
    no descriptions → no crash, multi-recipe non-overlapping
"""
from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
AGENTS_RENDER_PATH = ROOT / "lib" / "_internal" / "agents-render.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class SubstituteConfigTests(unittest.TestCase):
    """Tests for substitute_config(text, cfg_ns) -> str."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_substitute")

    def test_known_key_resolves(self):
        cfg = {"config.integration_branch": "development"}
        result = self.mod.substitute_config(
            "Do not push to `{config.integration_branch}` without a PR.", cfg
        )
        self.assertEqual(result, "Do not push to `development` without a PR.")

    def test_missing_key_verbatim(self):
        cfg = {}
        result = self.mod.substitute_config("Run {config.test_command} first.", cfg)
        self.assertEqual(result, "Run {config.test_command} first.")

    def test_missing_key_no_crash(self):
        cfg = {}
        # Must not raise
        result = self.mod.substitute_config("{config.missing_key}", cfg)
        self.assertEqual(result, "{config.missing_key}")

    def test_bare_key_verbatim(self):
        cfg = {"config.integration_branch": "main"}
        result = self.mod.substitute_config("See {integration_branch}.", cfg)
        self.assertEqual(result, "See {integration_branch}.")

    def test_double_brace_escape(self):
        result = self.mod.substitute_config("Use {{config.KEY}} to reference.", {})
        self.assertEqual(result, "Use {config.KEY} to reference.")

    def test_mixed_escape_and_substitution(self):
        cfg = {"config.test_command": "./run.sh"}
        result = self.mod.substitute_config(
            "Run `{config.test_command}` (not {{skip}}).", cfg
        )
        self.assertEqual(result, "Run `./run.sh` (not {skip}).")

    def test_lone_unbalanced_brace_no_crash(self):
        result = self.mod.substitute_config("Some prose { with brace.", {})
        # Must not crash; returns text untouched
        self.assertIsInstance(result, str)

    def test_empty_string(self):
        result = self.mod.substitute_config("", {"config.x": "y"})
        self.assertEqual(result, "")


# ---------------------------------------------------------------------------

class CollectRecipeBriefFragmentsTests(unittest.TestCase):
    """Tests for collect_recipe_brief_fragments(resolved, section) -> list[dict]."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_collect")

    def _resolved(self, enabled, recipes):
        return {"enabled": enabled, "recipes": recipes}

    def test_single_recipe_fragment_returned(self):
        resolved = self._resolved(
            ["recipe-a"],
            {"recipe-a": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "Rule A."}]}}},
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["text"], "Rule A.")

    def test_enabled_order_preserved(self):
        resolved = self._resolved(
            ["wf", "tdd"],
            {
                "wf": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "WF rule."}]}},
                "tdd": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "TDD rule."}]}},
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual([f["text"] for f in result], ["WF rule.", "TDD rule."])

    def test_reversed_enabled_order(self):
        resolved = self._resolved(
            ["tdd", "wf"],
            {
                "wf": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "WF rule."}]}},
                "tdd": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "TDD rule."}]}},
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual([f["text"] for f in result], ["TDD rule.", "WF rule."])

    def test_key_dedup_first_wins(self):
        resolved = self._resolved(
            ["recipe-a", "recipe-b"],
            {
                "recipe-a": {"brief_fragments": {"context_sources": [{"key": "trello-sot", "text": "Trello is the source of truth."}]}},
                "recipe-b": {"brief_fragments": {"context_sources": [{"key": "trello-sot", "text": "Trello: source of truth — updated wording."}]}},
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "context_sources")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["text"], "Trello is the source of truth.")

    def test_exact_string_dedup_across_recipes(self):
        resolved = self._resolved(
            ["recipe-a", "recipe-b"],
            {
                "recipe-a": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "Run tests before committing."}]}},
                "recipe-b": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "Run tests before committing."}]}},
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["text"], "Run tests before committing.")

    def test_recipe_without_brief_fragments_key(self):
        resolved = self._resolved(
            ["recipe-a"],
            {"recipe-a": {}},
        )
        # Must not raise
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(result, [])

    def test_recipe_with_empty_brief_fragments(self):
        resolved = self._resolved(
            ["recipe-a"],
            {"recipe-a": {"brief_fragments": {}}},
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(result, [])

    def test_disabled_recipe_not_in_enabled(self):
        # recipe-b is in recipes but NOT in enabled
        resolved = self._resolved(
            ["recipe-a"],
            {
                "recipe-a": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "A rule."}]}},
                "recipe-b": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "B rule."}]}},
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["text"], "A rule.")

    def test_recipe_not_in_recipes_dict(self):
        # enabled references a recipe not in recipes dict — should not crash
        resolved = self._resolved(["missing-recipe"], {})
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(result, [])

    def test_substitution_applied(self):
        resolved = self._resolved(
            ["wf"],
            {
                "wf": {
                    "integration_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Do not push to `{config.integration_branch}` without a PR."}
                        ]
                    },
                }
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(result[0]["text"], "Do not push to `main` without a PR.")

    def test_empty_enabled_list(self):
        resolved = self._resolved([], {"recipe-a": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "X"}]}}})
        result = self.mod.collect_recipe_brief_fragments(resolved, "workflow_rules")
        self.assertEqual(result, [])

    def test_section_not_present_in_fragments(self):
        resolved = self._resolved(
            ["recipe-a"],
            {"recipe-a": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "WF."}]}}},
        )
        # context_sources not declared — should return []
        result = self.mod.collect_recipe_brief_fragments(resolved, "context_sources")
        self.assertEqual(result, [])


# ---------------------------------------------------------------------------

class SectionMergeTests(unittest.TestCase):
    """Tests for _section_* functions after resolved threading + merge logic."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_section")

    def _resolved_with_wf(self, frags, extra_cfg=None):
        cfg = {"brief_fragments": {"workflow_rules": [{"key": None, "text": f} for f in frags]}}
        if extra_cfg:
            cfg.update(extra_cfg)
        return {
            "enabled": ["wf"],
            "recipes": {"wf": cfg},
            "bindings": {},
        }

    def test_append_default_recipe_before_manifest(self):
        brief = {"workflow_rules": ["Manifest rule."]}
        resolved = self._resolved_with_wf(["Recipe rule."])
        lines = self.mod._section_workflow_rules(brief, resolved)
        # Find bullet positions
        recipe_pos = next(i for i, l in enumerate(lines) if "Recipe rule." in l)
        manifest_pos = next(i for i, l in enumerate(lines) if "Manifest rule." in l)
        self.assertLess(recipe_pos, manifest_pos)

    def test_replace_mode_suppresses_recipe_fragments(self):
        brief = {"workflow_rules_mode": "replace", "workflow_rules": ["Only this rule."]}
        resolved = self._resolved_with_wf(["Recipe rule."])
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Only this rule.", content)
        self.assertNotIn("Recipe rule.", content)

    def test_replace_mode_isolates_other_sections(self):
        # workflow_rules REPLACE, but runtime_flow should still get recipe fragments
        brief = {"workflow_rules_mode": "replace", "workflow_rules": ["WF only."]}
        resolved = {
            "enabled": ["wf"],
            "recipes": {
                "wf": {
                    "brief_fragments": {
                        "workflow_rules": [{"key": None, "text": "WF recipe."}],
                        "runtime_flow": [{"key": None, "text": "RF recipe."}],
                    }
                }
            },
            "bindings": {},
        }
        wf_lines = self.mod._section_workflow_rules(brief, resolved)
        rf_lines = self.mod._section_runtime_flow(brief, resolved)
        self.assertNotIn("- WF recipe.", wf_lines)
        self.assertIn("- RF recipe.", rf_lines)

    def test_manifest_prose_never_substituted(self):
        brief = {"workflow_rules": ["Check {config.test_command}"]}
        resolved = self._resolved_with_wf([])
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Check {config.test_command}", content)

    def test_empty_manifest_brief_populated_by_recipe_fragments(self):
        brief = {}
        resolved = self._resolved_with_wf(["Create a worktree.", "Do not merge directly."])
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Create a worktree.", content)
        self.assertIn("Do not merge directly.", content)

    def test_recipe_without_fragments_unchanged_output(self):
        brief = {"workflow_rules": ["Static rule."]}
        resolved_with = self._resolved_with_wf(["Recipe frag."])
        resolved_without = {
            "enabled": ["wf"],
            "recipes": {"wf": {}},
            "bindings": {},
        }
        lines_with = self.mod._section_workflow_rules(brief, resolved_with)
        lines_without = self.mod._section_workflow_rules(brief, resolved_without)
        # Without fragments, should still emit the manifest rule
        content_without = "\n".join(lines_without)
        self.assertIn("Static rule.", content_without)

    def test_idempotent_collection(self):
        brief = {"workflow_rules": ["Manifest rule."]}
        resolved = self._resolved_with_wf(["Recipe rule."])
        lines1 = self.mod._section_workflow_rules(brief, resolved)
        lines2 = self.mod._section_workflow_rules(brief, resolved)
        self.assertEqual(lines1, lines2)

    def test_exact_string_dedup_recipe_vs_manifest(self):
        # Same text in recipe and manifest → appears once
        brief = {"workflow_rules": ["Create a worktree."]}
        resolved = self._resolved_with_wf(["Create a worktree."])
        lines = self.mod._section_workflow_rules(brief, resolved)
        count = sum(1 for l in lines if "Create a worktree." in l)
        self.assertEqual(count, 1)

    def test_context_sources_append(self):
        brief = {"context_sources": ["Manifest ctx."]}
        resolved = {
            "enabled": ["r"],
            "recipes": {"r": {"brief_fragments": {"context_sources": [{"key": None, "text": "Recipe ctx."}]}}},
            "bindings": {},
        }
        lines = self.mod._section_context_sources(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Recipe ctx.", content)
        self.assertIn("Manifest ctx.", content)

    def test_conflict_policy_append(self):
        brief = {"conflict_policy": ["Manifest policy."]}
        resolved = {
            "enabled": ["r"],
            "recipes": {"r": {"brief_fragments": {"conflict_policy": [{"key": None, "text": "Recipe policy."}]}}},
            "bindings": {},
        }
        lines = self.mod._section_conflict_policy(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Recipe policy.", content)
        self.assertIn("Manifest policy.", content)

    def test_useful_commands_append(self):
        brief = {"useful_commands": ["Manifest cmd."]}
        resolved = {
            "enabled": ["r"],
            "recipes": {"r": {"brief_fragments": {"useful_commands": [{"key": None, "text": "Recipe cmd."}]}}},
            "bindings": {"test-runner": ""},
        }
        lines = self.mod._section_useful_commands(brief, resolved)
        content = "\n".join(lines)
        self.assertIn("Recipe cmd.", content)
        self.assertIn("Manifest cmd.", content)

    def test_no_section_header_when_no_bullets(self):
        # Both recipe and manifest have no workflow_rules → section not emitted
        brief = {}
        resolved = {"enabled": [], "recipes": {}, "bindings": {}}
        lines = self.mod._section_workflow_rules(brief, resolved)
        self.assertEqual(lines, [])


# ---------------------------------------------------------------------------

class ValidateBriefModesTests(unittest.TestCase):
    """Tests for _validate_brief_modes(brief)."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_validate")

    def test_valid_append_mode_no_error(self):
        brief = {"workflow_rules_mode": "append"}
        # Must not raise
        self.mod._validate_brief_modes(brief)

    def test_valid_replace_mode_no_error(self):
        brief = {"workflow_rules_mode": "replace"}
        self.mod._validate_brief_modes(brief)

    def test_unknown_mode_raises(self):
        brief = {"workflow_rules_mode": "merge"}
        with self.assertRaises((ValueError, SystemExit)) as ctx:
            self.mod._validate_brief_modes(brief)
        # error message must mention the key and list valid values
        if isinstance(ctx.exception, ValueError):
            msg = str(ctx.exception)
            self.assertIn("workflow_rules_mode", msg)

    def test_unknown_mode_error_mentions_valid_values(self):
        brief = {"context_sources_mode": "upsert"}
        with self.assertRaises((ValueError, SystemExit)) as ctx:
            self.mod._validate_brief_modes(brief)
        if isinstance(ctx.exception, ValueError):
            msg = str(ctx.exception)
            self.assertTrue("append" in msg or "replace" in msg)

    def test_no_mode_keys_no_error(self):
        brief = {"workflow_rules": ["rule."]}
        self.mod._validate_brief_modes(brief)

    def test_empty_brief_no_error(self):
        self.mod._validate_brief_modes({})


# ---------------------------------------------------------------------------

class McpDescriptionsOverrideFillsGapTests(unittest.TestCase):
    """Tests for mcp_descriptions override-fills-gap in _render_lines / _section_mcp."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_mcp")

    def _make_manifest(self, mcp_desc=None, mcp_servers=None):
        manifest = {
            "project": {"name": "fixture"},
            "brief": {},
        }
        if mcp_desc is not None:
            manifest["brief"]["mcp_descriptions"] = mcp_desc
        if mcp_servers is not None:
            manifest["mcp"] = mcp_servers
        return manifest

    def _make_resolved(self, recipe_mcp_frags=None, enabled=None):
        recipes = {}
        if recipe_mcp_frags:
            for rid, frags in recipe_mcp_frags.items():
                recipes[rid] = {"brief_fragments": {"mcp_descriptions": frags}}
        return {
            "enabled": enabled or list(recipes.keys()),
            "recipes": recipes,
            "bindings": {},
        }

    def test_project_override_wins(self):
        manifest = self._make_manifest(
            mcp_desc={"trello": "Project override."},
            mcp_servers={"trello": {}},
        )
        resolved = self._make_resolved(
            recipe_mcp_frags={"recipe-a": [{"key": "trello", "text": "Recipe default."}]}
        )
        lines = self.mod._render_lines(manifest, resolved)
        content = "\n".join(lines)
        self.assertIn("Project override.", content)
        self.assertNotIn("Recipe default.", content)

    def test_recipe_fills_gap(self):
        manifest = self._make_manifest(
            mcp_servers={"trello": {}},
        )
        resolved = self._make_resolved(
            recipe_mcp_frags={"recipe-a": [{"key": "trello", "text": "Recipe default."}]}
        )
        lines = self.mod._render_lines(manifest, resolved)
        content = "\n".join(lines)
        self.assertIn("Recipe default.", content)

    def test_no_mcp_descriptions_no_crash(self):
        manifest = self._make_manifest(mcp_servers={"vault": {}})
        resolved = self._make_resolved()
        # Must not crash
        lines = self.mod._render_lines(manifest, resolved)
        content = "\n".join(lines)
        self.assertIn("vault", content)

    def test_multi_recipe_non_overlapping_keys(self):
        manifest = self._make_manifest(
            mcp_servers={"trello": {}, "engram": {}},
        )
        resolved = self._make_resolved(
            recipe_mcp_frags={
                "recipe-a": [{"key": "trello", "text": "Trello desc."}],
                "recipe-b": [{"key": "engram", "text": "Engram desc."}],
            }
        )
        lines = self.mod._render_lines(manifest, resolved)
        content = "\n".join(lines)
        self.assertIn("Trello desc.", content)
        self.assertIn("Engram desc.", content)

    def test_manifest_override_does_not_affect_other_servers(self):
        manifest = self._make_manifest(
            mcp_desc={"trello": "Project trello."},
            mcp_servers={"trello": {}, "engram": {}},
        )
        resolved = self._make_resolved(
            recipe_mcp_frags={
                "recipe-a": [
                    {"key": "trello", "text": "Recipe trello."},
                    {"key": "engram", "text": "Recipe engram."},
                ]
            }
        )
        lines = self.mod._render_lines(manifest, resolved)
        content = "\n".join(lines)
        self.assertIn("Project trello.", content)
        self.assertNotIn("Recipe trello.", content)
        self.assertIn("Recipe engram.", content)


# ---------------------------------------------------------------------------

class EndToEndRenderTests(unittest.TestCase):
    """End-to-end tests via render() function using temp files."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_brief_fragments_e2e")

    def _run_render(self, toml_content: str, resolved_data: dict) -> str:
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            toml_path = tmp / "ai-specs.toml"
            output_path = tmp / "AGENTS.md"
            resolved_path = tmp / "resolved-config.json"
            toml_path.write_text(toml_content)
            resolved_path.write_text(json.dumps(resolved_data))
            self.mod.render(
                toml_path,
                output_path,
                preserve_if_marker=False,
                resolved_config_path=resolved_path,
            )
            return output_path.read_text()

    def test_runtime_brief_marker_suppresses_regeneration(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            toml_path = tmp / "ai-specs.toml"
            output_path = tmp / "AGENTS.md"
            resolved_path = tmp / "resolved-config.json"
            toml_path.write_text("[project]\nname = 'test'\n")
            resolved_path.write_text(json.dumps({
                "enabled": ["wf"],
                "recipes": {"wf": {"brief_fragments": {"workflow_rules": [{"key": None, "text": "New fragment."}]}}},
                "bindings": {},
            }))
            # Pre-existing AGENTS.md with marker
            existing = "# Existing\n<!-- ai-specs:runtime-brief -->\nHand-written content.\n"
            output_path.write_text(existing)
            self.mod.render(
                toml_path,
                output_path,
                preserve_if_marker=True,
                resolved_config_path=resolved_path,
            )
            self.assertEqual(output_path.read_text(), existing)

    def test_idempotent_render_with_fragments(self):
        toml = "[project]\nname = 'test'\n\n[brief]\n"
        resolved = {
            "enabled": ["wf"],
            "recipes": {
                "wf": {
                    "integration_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Do not push to `{config.integration_branch}` without a PR."}
                        ]
                    },
                }
            },
            "bindings": {},
        }
        out1 = self._run_render(toml, resolved)
        out2 = self._run_render(toml, resolved)
        self.assertEqual(out1, out2)

    def test_empty_brief_populated_by_recipe_fragments(self):
        toml = "[project]\nname = 'test'\n\n[brief]\nintro = 'Test project.'\npurpose = 'For testing.'\n"
        resolved = {
            "enabled": ["wf"],
            "recipes": {
                "wf": {
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Create a worktree."},
                            {"key": None, "text": "Do not merge directly."},
                        ]
                    }
                }
            },
            "bindings": {},
        }
        content = self._run_render(toml, resolved)
        self.assertIn("Create a worktree.", content)
        self.assertIn("Do not merge directly.", content)

    def test_no_fragments_backward_compat(self):
        toml = "[project]\nname = 'test'\n\n[brief]\nworkflow_rules = ['Static rule.']\n"
        resolved = {
            "enabled": ["wf"],
            "recipes": {"wf": {}},
            "bindings": {},
        }
        content = self._run_render(toml, resolved)
        self.assertIn("Static rule.", content)

    def test_replace_mode_in_full_render(self):
        toml = (
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules_mode = 'replace'\n"
            "workflow_rules = ['Only this rule.']\n"
        )
        resolved = {
            "enabled": ["wf"],
            "recipes": {
                "wf": {
                    "brief_fragments": {
                        "workflow_rules": [{"key": None, "text": "Recipe rule — should not appear."}]
                    }
                }
            },
            "bindings": {},
        }
        content = self._run_render(toml, resolved)
        self.assertIn("Only this rule.", content)
        self.assertNotIn("Recipe rule — should not appear.", content)

    def test_validate_brief_modes_called_from_render(self):
        toml = "[project]\nname = 'test'\n\n[brief]\nworkflow_rules_mode = 'invalid_mode'\n"
        resolved = {"enabled": [], "recipes": {}, "bindings": {}}
        with self.assertRaises((ValueError, SystemExit)):
            self._run_render(toml, resolved)


# ---------------------------------------------------------------------------
# Batch 6 — Regression & Idempotency
# ---------------------------------------------------------------------------

class B6RegressionTests(unittest.TestCase):
    """Batch 6 regression tests: marker suppression, idempotency, minimal manifest,
    recipe-without-fragments compatibility. These exercise the full render() pipeline."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_b6_regression")

    def _run_render(self, toml_content: str, resolved_data: dict) -> str:
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            toml_path = tmp / "ai-specs.toml"
            output_path = tmp / "AGENTS.md"
            resolved_path = tmp / "resolved-config.json"
            toml_path.write_text(toml_content)
            resolved_path.write_text(json.dumps(resolved_data))
            self.mod.render(
                toml_path,
                output_path,
                preserve_if_marker=False,
                resolved_config_path=resolved_path,
            )
            return output_path.read_text()

    # 6.1 / 6.2 — marker suppression intact after resolved threading
    def test_marker_suppresses_regeneration_with_recipe_fragments(self):
        """AGENTS.md with <!-- ai-specs:runtime-brief --> must NOT be modified even when
        recipes now contribute [provides.brief] fragments (B6 regression for 6.1/6.2)."""
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            toml_path = tmp / "ai-specs.toml"
            output_path = tmp / "AGENTS.md"
            resolved_path = tmp / "resolved-config.json"
            toml_path.write_text(
                "[project]\nname = 'test'\n\n[brief]\nintro = 'Intro.'\npurpose = 'Purpose.'\n"
            )
            resolved_path.write_text(json.dumps({
                "enabled": ["worktree-flow", "tdd-flow"],
                "recipes": {
                    "worktree-flow": {
                        "integration_branch": "main",
                        "brief_fragments": {
                            "workflow_rules": [
                                {"key": None, "text": "Create worktree for every change."},
                                {"key": None, "text": "Do not push to `{config.integration_branch}` without a PR."},
                            ]
                        },
                    },
                    "tdd-flow": {
                        "test_command": "./tests/run.sh",
                        "brief_fragments": {
                            "workflow_rules": [
                                {"key": None, "text": "Write failing tests first."},
                            ],
                            "useful_commands": [
                                {"key": None, "text": "Run tests: `{config.test_command}`"},
                            ],
                        },
                    },
                },
                "bindings": {},
            }))
            # Pre-existing AGENTS.md with the runtime-brief marker (hand-managed)
            hand_managed = (
                "# Hand-Managed Brief\n"
                "<!-- ai-specs:runtime-brief -->\n"
                "This content is hand-written and MUST NOT be replaced.\n"
            )
            output_path.write_text(hand_managed)
            self.mod.render(
                toml_path,
                output_path,
                preserve_if_marker=True,
                resolved_config_path=resolved_path,
            )
            # Must be byte-identical to the original hand-managed content
            self.assertEqual(output_path.read_text(), hand_managed)

    # 6.3 / 6.4 — idempotency with config substitution
    def test_idempotency_with_config_substitution(self):
        """Two consecutive renders with config substitution must produce byte-identical output."""
        toml = (
            "[project]\nname = 'test'\n\n"
            "[brief]\nintro = 'Test project.'\npurpose = 'Testing.'\n"
        )
        resolved = {
            "enabled": ["worktree-flow", "git-pr-flow"],
            "recipes": {
                "worktree-flow": {
                    "integration_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Create worktree. Branch: `{config.integration_branch}`."},
                            {"key": None, "text": "Preserve unrelated changes."},
                        ]
                    },
                },
                "git-pr-flow": {
                    "base_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use GitHub PRs to merge into `{config.base_branch}`."},
                        ]
                    },
                },
            },
            "bindings": {},
        }
        out1 = self._run_render(toml, resolved)
        out2 = self._run_render(toml, resolved)
        # Must be byte-identical — no ordering drift or duplicate bullets
        self.assertEqual(out1, out2, "Render output must be idempotent (byte-identical on two runs)")

    # 6.8 — minimal manifest: only intro+purpose in [brief], populated by recipe fragments
    def test_minimal_brief_with_config_key_substitution(self):
        """Minimal [brief] (only intro+purpose) + recipe with {config.KEY} → rendered output
        contains substituted values, not raw placeholders (B6 scenario 6.8)."""
        toml = (
            "[project]\nname = 'test'\n\n"
            "[brief]\n"
            "intro = 'Test intro.'\n"
            "purpose = 'Test purpose.'\n"
        )
        resolved = {
            "enabled": ["tdd-flow"],
            "recipes": {
                "tdd-flow": {
                    "test_command": "./tests/run.sh",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Write failing tests first."},
                            {"key": None, "text": "Run the suite before committing."},
                        ],
                        "useful_commands": [
                            {"key": None, "text": "Run tests: `{config.test_command}`"},
                        ],
                    },
                }
            },
            "bindings": {},
        }
        content = self._run_render(toml, resolved)
        # Substituted value must appear (not the placeholder)
        self.assertIn("./tests/run.sh", content)
        self.assertNotIn("{config.test_command}", content)
        # Section populated entirely from recipe fragments
        self.assertIn("Write failing tests first.", content)
        self.assertIn("Run the suite before committing.", content)
        self.assertIn("Run tests: `./tests/run.sh`", content)

    # 6.5 — recipe without [provides.brief] must not break rendering
    def test_recipe_without_provides_brief_does_not_break_render(self):
        """Enabled recipe with no brief_fragments key → render succeeds, other sections intact."""
        toml = (
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules = ['Static manifest rule.']\n"
        )
        resolved = {
            "enabled": ["no-brief-recipe", "with-brief-recipe"],
            "recipes": {
                # no brief_fragments at all
                "no-brief-recipe": {"some_config": "value"},
                # has brief_fragments
                "with-brief-recipe": {
                    "brief_fragments": {
                        "workflow_rules": [{"key": None, "text": "Recipe rule."}]
                    }
                },
            },
            "bindings": {},
        }
        content = self._run_render(toml, resolved)
        # Recipe rule still appears
        self.assertIn("Recipe rule.", content)
        # Manifest rule appears (deduped, not duplicated)
        self.assertIn("Static manifest rule.", content)
        self.assertEqual(content.count("Static manifest rule."), 1)

    # 6.3 extended — exact-string dedup prevents duplicates on repeated fragments
    def test_exact_string_dedup_idempotency(self):
        """Same fragment text from two recipes → appears exactly once; render is idempotent."""
        toml = "[project]\nname = 'test'\n\n[brief]\n"
        resolved = {
            "enabled": ["recipe-a", "recipe-b"],
            "recipes": {
                "recipe-a": {
                    "brief_fragments": {
                        "workflow_rules": [{"key": None, "text": "Shared rule."}]
                    }
                },
                "recipe-b": {
                    "brief_fragments": {
                        "workflow_rules": [{"key": None, "text": "Shared rule."}]
                    }
                },
            },
            "bindings": {},
        }
        out1 = self._run_render(toml, resolved)
        out2 = self._run_render(toml, resolved)
        self.assertEqual(out1, out2, "Must be idempotent")
        # "Shared rule." must appear exactly once
        self.assertEqual(out1.count("Shared rule."), 1)


# ---------------------------------------------------------------------------

class VcsFragmentIsolationTests(unittest.TestCase):
    """VCS workflow_rules fragments stay isolated to the bound recipe.

    When multiple VCS sibling recipes are enabled but only one is bound to
    vcs-pr-flow, only the bound recipe contributes workflow_rules fragments.
    When no binding exists, no VCS sibling fragments are emitted.
    Non-VCS recipes always contribute regardless of VCS binding state.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_vcs_fragment_isolation")

    def _resolved_with_vcs_siblings(self, bound_vcs_id: str | None):
        """Build resolved config with 3 VCS siblings + 1 non-VCS recipe."""
        bindings = {}
        if bound_vcs_id:
            bindings["vcs-pr-flow"] = bound_vcs_id
        return {
            "enabled": ["git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow", "worktree-flow"],
            "recipes": {
                "git-pr-flow": {
                    "base_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use GitHub PRs to merge."},
                        ],
                    },
                },
                "gitlab-mr-flow": {
                    "base_branch": "development",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use GitLab MRs to merge."},
                        ],
                    },
                },
                "bitbucket-pr-flow": {
                    "base_branch": "develop",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use Bitbucket PRs to merge."},
                        ],
                    },
                },
                "worktree-flow": {
                    "integration_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Create a worktree for every change."},
                        ],
                    },
                },
            },
            "bindings": bindings,
        }

    def test_bound_gitlab_only_gitlab_fragments_in_workflow_rules(self):
        """3 VCS recipes enabled, bound to gitlab-mr-flow → only GitLab fragments."""
        resolved = self._resolved_with_vcs_siblings("gitlab-mr-flow")
        brief = {}
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        # GitLab fragments MUST appear
        self.assertIn("Use GitLab MRs to merge.", content)
        # GitHub and Bitbucket fragments MUST NOT appear
        self.assertNotIn("Use GitHub PRs to merge.", content)
        self.assertNotIn("Use Bitbucket PRs to merge.", content)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree for every change.", content)

    def test_no_vcs_binding_no_vcs_fragments(self):
        """VCS siblings enabled but no vcs-pr-flow binding → no VCS fragments."""
        resolved = self._resolved_with_vcs_siblings(None)
        brief = {}
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        # No VCS fragments should appear when unbound
        self.assertNotIn("Use GitHub PRs to merge.", content)
        self.assertNotIn("Use GitLab MRs to merge.", content)
        self.assertNotIn("Use Bitbucket PRs to merge.", content)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree for every change.", content)

    def test_bound_custom_recipe_contributes_own_fragments(self):
        """Custom recipe bound to vcs-pr-flow → its own fragments still appear."""
        resolved = {
            "enabled": ["my-custom-vcs", "git-pr-flow", "worktree-flow"],
            "recipes": {
                "my-custom-vcs": {
                    "base_branch": "trunk",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use custom VCS flow."},
                        ],
                    },
                },
                "git-pr-flow": {
                    "base_branch": "main",
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Use GitHub PRs to merge."},
                        ],
                    },
                },
                "worktree-flow": {
                    "brief_fragments": {
                        "workflow_rules": [
                            {"key": None, "text": "Create a worktree."},
                        ],
                    },
                },
            },
            "bindings": {"vcs-pr-flow": "my-custom-vcs"},
        }
        brief = {}
        lines = self.mod._section_workflow_rules(brief, resolved)
        content = "\n".join(lines)
        # Custom recipe fragments MUST appear (it's the bound recipe)
        self.assertIn("Use custom VCS flow.", content)
        # Known VCS sibling fragments MUST NOT appear (not the bound recipe)
        self.assertNotIn("Use GitHub PRs to merge.", content)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree.", content)



class RepoTopologyBriefTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            ROOT / "lib" / "_internal" / "agents-render.py",
            "agents_render_topology",
        )

    def test_repo_topology_line_in_project_section(self):
        import tempfile
        from pathlib import Path as P
        import sys
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(P(tmp.name))
        resolved = {
            "bindings": {"worktree-isolation": "worktree-flow"},
            "enabled": ["worktree-flow"],
            "recipes": {
                "worktree-flow": {
                    "integration_branch": "main",
                    "repo_topology": "auto",
                }
            },
            "project_root": str(super_repo),
        }
        manifest = {"project": {"name": "topo"}, "agents": {"enabled": ["claude"]}}
        lines = self.mod._section_project(manifest, resolved)
        text = "\n".join(lines)
        self.assertIn("- **Repo topology**: `monorepo-submodules` (via auto)", text)


    def test_repo_topology_omitted_when_worktree_flow_disabled(self):
        """Config dict alone must not surface Repo topology when recipe disabled."""
        import tempfile
        from pathlib import Path as P
        import sys
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(P(tmp.name))
        resolved = {
            "bindings": {},
            "enabled": [],  # worktree-flow NOT enabled
            "recipes": {
                "worktree-flow": {
                    "integration_branch": "main",
                    "repo_topology": "auto",
                }
            },
            "project_root": str(super_repo),
        }
        manifest = {"project": {"name": "topo"}, "agents": {"enabled": ["claude"]}}
        lines = self.mod._section_project(manifest, resolved)
        text = "\n".join(lines)
        self.assertNotIn("Repo topology", text)


if __name__ == "__main__":
    unittest.main()

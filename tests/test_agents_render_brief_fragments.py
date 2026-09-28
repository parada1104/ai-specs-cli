"""Black-box tests for recipe brief_fragments support in the rendered AGENTS.md.

Every test drives ``bin/ai-specs sync`` through its process boundary: recipe
brief fragments are staged into an isolated catalog (real recipe dirs, never
written through symlinks into the repository catalog), the manifest is written
to the temp project, and assertions observe AGENTS.md content, sync exit
codes, and sync stderr. No test may import ``lib/_internal`` modules.

Coverage preserved from the original white-box suite:
  - {config.KEY} substitution semantics in recipe fragments (resolved value,
    missing key verbatim, bare key verbatim, {{ }} escape, mixed escape+sub,
    lone unbalanced brace, empty text)
  - fragment collection ordering, key-dedup, exact-string dedup, disabled
    recipes, empty/absent [provides.brief]
  - section merge: APPEND default (recipe before manifest), REPLACE opt-in,
    REPLACE isolation per section, manifest prose never substituted,
    empty manifest [brief] populated by recipe fragments
  - brief mode validation: unknown <section>_mode value fails sync with the
    key and valid values named
  - mcp_descriptions override-fills-gap: project wins, recipe fills gap,
    no descriptions → no crash, multi-recipe non-overlapping
  - end-to-end marker suppression, idempotency, backward compatibility
  - VCS fragment isolation to the bound vcs-pr-flow recipe
  - repo topology line in ## Project
  - worktree gate_mode brief rendering (real catalog recipe source)
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, temp_project  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


# ---------------------------------------------------------------------------
# Staging helpers
# ---------------------------------------------------------------------------

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


def _toml_str(text: str) -> str:
    """Escape a Python string as a TOML basic string (JSON-compatible for ASCII/UTF-8)."""
    return json.dumps(text, ensure_ascii=False)


def recipe_toml(recipe_id: str, *, fragments: dict | None = None,
                capabilities: tuple[str, ...] = ()) -> str:
    """Build a minimal catalog recipe.toml.

    fragments maps a [provides.brief] section name to a list of items; an item
    is a plain string (key=None) or a (key, text) tuple (inline-table form).
    """
    parts = [
        f'[recipe]\nid = "{recipe_id}"\nname = "{recipe_id}"\n'
        f'description = "test recipe {recipe_id}"\nversion = "1.0.0"\n'
    ]
    for cap in capabilities:
        parts.append(f'\n[[capabilities]]\nid = "{cap}"\n')
    if fragments:
        parts.append("\n[provides.brief]\n")
        for section, items in fragments.items():
            entries = ", ".join(
                f'{{ key = {_toml_str(key)}, text = {_toml_str(text)} }}'
                if key is not None else _toml_str(text)
                for key, text in ((i if isinstance(i, tuple) else (None, i)) for i in items)
            )
            parts.append(f"{section} = [{entries}]\n")
    return "".join(parts)


def catalog_recipe(cli_home: Path, recipe_id: str, toml: str) -> Path:
    """Seed or override one catalog recipe with a REAL (non-symlinked) dir.

    The isolated home's catalog starts as a symlink to the repository catalog;
    it is materialized once into a real dir of per-recipe symlinks, and every
    test recipe is then written as a real directory, so no test ever writes
    through a symlink into the repository's own catalog.
    """
    catalog = cli_home / "catalog"
    if catalog.is_symlink():
        target = catalog.resolve()
        catalog.unlink()
        catalog.mkdir()
        for entry in target.iterdir():
            if entry.name == "recipes":
                (catalog / "recipes").mkdir()
                for recipe in entry.iterdir():
                    (catalog / "recipes" / recipe.name).symlink_to(recipe)
            else:
                (catalog / entry.name).symlink_to(entry)
    rdir = catalog / "recipes" / recipe_id
    if rdir.is_symlink():
        rdir.unlink()
    rdir.mkdir(parents=True, exist_ok=True)
    (rdir / "recipe.toml").write_text(toml)
    return rdir


class BriefFragmentCLITest(unittest.TestCase):
    """Shared staging: one temp project + one isolated install root per test."""

    def setUp(self):
        self._td, self.project = temp_project(
            name=self.__class__.__name__.lower(), agents=("claude",)
        )
        # The install root lives in its own temp dir: isolated_home builds
        # cli-home inside `base`, mirroring test_sync_pipeline staging.
        self._home_base = Path(tempfile.mkdtemp(prefix="ai-specs-homebase-"))
        self.home = _make_home(self._home_base)

    def tearDown(self):
        self._td.cleanup()
        shutil.rmtree(self._home_base, ignore_errors=True)

    def tearDown(self):
        self._td.cleanup()

    def write_manifest(self, body: str) -> None:
        (self.project / "ai-specs" / "ai-specs.toml").write_text(body)

    def sync(self):
        return invoke(self.project, "sync", cli_home=self.home)

    def agents_md(self) -> str:
        path = self.project / "AGENTS.md"
        return path.read_text() if path.exists() else ""

    def seed_recipe(self, recipe_id: str, toml: str) -> None:
        catalog_recipe(self.home, recipe_id, toml)

    def section_text(self, title: str) -> str:
        """Return the rendered body of one '## <title>' section, or ''."""
        text = self.agents_md()
        marker = f"## {title}\n"
        start = text.find(marker)
        if start < 0:
            return ""
        tail = text[start + len(marker):]
        end = tail.find("\n## ")
        return tail[:end] if end >= 0 else tail


# ---------------------------------------------------------------------------
# {config.KEY} substitution in recipe fragments
# ---------------------------------------------------------------------------

class SubstituteConfigTests(BriefFragmentCLITest):
    """{config.KEY} substitution semantics, observed through rendered fragments.

    Each test stages one catalog recipe whose workflow_rules fragment carries
    the prose under test plus (optionally) a manifest config value, then
    asserts on the rendered Workflow Rules section of AGENTS.md.
    """

    def _sync_fragment(self, text: str, cfg: dict | None = None) -> str:
        self.seed_recipe("sub-recipe", recipe_toml(
            "sub-recipe", fragments={"workflow_rules": [text]}
        ))
        lines = [
            "[project]\nname = 'sub'\n\n[agents]\nenabled = ['claude']\n\n",
            "[recipes.sub-recipe]\nenabled = true\n",
        ]
        if cfg:
            lines.append("[recipes.sub-recipe.config]\n")
            for key, value in cfg.items():
                lines.append(f"{key} = {_toml_str(value)}\n")
        self.write_manifest("".join(lines))
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return self.section_text("Workflow Rules")
    def test_known_key_resolves(self):
        section = self._sync_fragment(
            "Do not push to `{config.integration_branch}` without a PR.",
            cfg={"integration_branch": "development"},
        )
        self.assertIn("Do not push to `development` without a PR.", section)

    def test_artifact_store_enum_value_resolves(self):
        section = self._sync_fragment(
            "Default artifact store: `{config.artifact_store_default}`.",
            cfg={"artifact_store_default": "both"},
        )
        self.assertIn("Default artifact store: `both`.", section)
        self.assertNotIn("{config.artifact_store_default}", section)

    def test_missing_key_verbatim(self):
        section = self._sync_fragment("Run {config.test_command} first.")
        self.assertIn("Run {config.test_command} first.", section)

    def test_missing_key_no_crash(self):
        # Must not raise: sync exits 0 and the placeholder is re-emitted verbatim
        section = self._sync_fragment("{config.missing_key}")
        self.assertIn("{config.missing_key}", section)

    def test_bare_key_verbatim(self):
        section = self._sync_fragment(
            "See {integration_branch}.", cfg={"integration_branch": "main"}
        )
        self.assertIn("See {integration_branch}.", section)

    def test_double_brace_escape(self):
        section = self._sync_fragment("Use {{config.KEY}} to reference.")
        self.assertIn("Use {config.KEY} to reference.", section)

    def test_mixed_escape_and_substitution(self):
        section = self._sync_fragment(
            "Run `{config.test_command}` (not {{skip}}).",
            cfg={"test_command": "./run.sh"},
        )
        self.assertIn("Run `./run.sh` (not {skip}).", section)

    def test_lone_unbalanced_brace_no_crash(self):
        # Must not crash: sync exits 0 and the prose is rendered untouched
        section = self._sync_fragment("Some prose { with brace.")
        self.assertIn("Some prose { with brace.", section)

    def test_empty_string(self):
        # Must not crash: an empty fragment text keeps sync green and renders
        # the section pipeline without raising
        self._sync_fragment("")
        self.assertIn("## Workflow Rules", self.agents_md())


# ---------------------------------------------------------------------------

class CollectRecipeBriefFragmentsTests(BriefFragmentCLITest):
    """Fragment collection ordering/dedup/enabling, observed via AGENTS.md."""

    def _wf_section(self) -> str:
        return self.section_text("Workflow Rules")

    def _ctx_section(self) -> str:
        return self.section_text("Context Sources")

    def test_single_recipe_fragment_returned(self):
        self.seed_recipe("recipe-a", recipe_toml(
            "recipe-a", fragments={"workflow_rules": ["Rule A."]}
        ))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        section = self._wf_section()
        self.assertIn("- Rule A.", section)

    def _sync_two_recipe_order(self, enabled_first: str, enabled_second: str):
        for rid in ("wf", "tdd"):
            self.seed_recipe(rid, recipe_toml(rid, fragments={
                "workflow_rules": [f"{rid.upper()} rule."]
            }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{enabled_first}]\nenabled = true\n\n"
            f"[recipes.{enabled_second}]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return self._wf_section()

    def test_enabled_order_preserved(self):
        section = self._sync_two_recipe_order("wf", "tdd")
        self.assertLess(section.index("WF rule."), section.index("TDD rule."))

    def test_reversed_enabled_order(self):
        section = self._sync_two_recipe_order("tdd", "wf")
        self.assertLess(section.index("TDD rule."), section.index("WF rule."))

    def test_key_dedup_first_wins(self):
        self.seed_recipe("recipe-a", recipe_toml("recipe-a", fragments={
            "context_sources": [("trello-sot", "Trello is the source of truth.")]
        }))
        self.seed_recipe("recipe-b", recipe_toml("recipe-b", fragments={
            "context_sources": [("trello-sot", "Trello: source of truth — updated wording.")]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[recipes.recipe-b]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        section = self._ctx_section()
        self.assertIn("Trello is the source of truth.", section)
        self.assertNotIn("Trello: source of truth — updated wording.", section)

    def test_exact_string_dedup_across_recipes(self):
        for rid in ("recipe-a", "recipe-b"):
            self.seed_recipe(rid, recipe_toml(rid, fragments={
                "workflow_rules": ["Run tests before committing."]
            }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[recipes.recipe-b]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self._wf_section().count("Run tests before committing."), 1)

    def test_recipe_without_brief_fragments_key(self):
        # Must not raise: a recipe with no [provides.brief] keeps sync green
        self.seed_recipe("recipe-a", recipe_toml("recipe-a"))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self._wf_section(), "")

    def test_recipe_with_empty_brief_fragments(self):
        self.seed_recipe("recipe-a", (
            '[recipe]\nid = "recipe-a"\nname = "recipe-a"\n'
            'description = "test recipe recipe-a"\nversion = "1.0.0"\n\n'
            "[provides.brief]\n"
        ))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self._wf_section(), "")

    def test_disabled_recipe_not_in_enabled(self):
        # recipe-b is declared but NOT enabled
        self.seed_recipe("recipe-a", recipe_toml("recipe-a", fragments={
            "workflow_rules": ["A rule."]
        }))
        self.seed_recipe("recipe-b", recipe_toml("recipe-b", fragments={
            "workflow_rules": ["B rule."]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[recipes.recipe-b]\nenabled = false\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        section = self._wf_section()
        self.assertIn("A rule.", section)
        self.assertNotIn("B rule.", section)

    def test_recipe_not_in_recipes_dict(self):
        # TRIAGE: ai-specs sync — the CLI derives the enabled list from the
        # [recipes.*] manifest entries, so an enabled id absent from the recipes
        # map is unreachable black-box; this pins the no-crash contract at the
        # renderer process boundary using the isolated home's own lib copy.
        tmp = self.project / "ai-specs"
        toml_path = tmp / "ai-specs.toml"
        output_path = self.project / "AGENTS.md"
        resolved_path = tmp / "resolved-config.json"
        toml_path.write_text("[project]\nname = 'p'\n")
        resolved_path.write_text(json.dumps({
            "enabled": ["missing-recipe"], "recipes": {}, "bindings": {},
        }))
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.project.parent / "home"),
            "TMPDIR": str(self.project.parent),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        (self.project.parent / "home").mkdir(parents=True, exist_ok=True)
        proc = subprocess.run(
            [sys.executable, str(self.home / "lib" / "_internal" / "agents-render.py"),
             str(toml_path), str(output_path), "--resolved-config", str(resolved_path)],
            cwd=str(ROOT), env=env, text=True, capture_output=True, check=False, input="",
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        # Must not crash: rendering an enabled id without recipe config succeeds
        self.assertTrue(output_path.exists())

    def test_substitution_applied(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": [
                "Do not push to `{config.integration_branch}` without a PR."
            ]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.wf]\nenabled = true\n"
            "[recipes.wf.config]\nintegration_branch = 'main'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(
            "Do not push to `main` without a PR.", self._wf_section()
        )

    def test_empty_enabled_list(self):
        self.seed_recipe("recipe-a", recipe_toml("recipe-a", fragments={
            "workflow_rules": ["X"]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = false\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self._wf_section(), "")

    def test_section_not_present_in_fragments(self):
        # context_sources not declared by the recipe — no Context Sources section
        self.seed_recipe("recipe-a", recipe_toml("recipe-a", fragments={
            "workflow_rules": ["WF."]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.recipe-a]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self._ctx_section(), "")


# ---------------------------------------------------------------------------

class SectionMergeTests(BriefFragmentCLITest):
    """Section merge behavior (APPEND default / REPLACE opt-in) via AGENTS.md."""

    def _wf_section(self) -> str:
        return self.section_text("Workflow Rules")

    def _sync_recipe_and_manifest(self, recipe_frags: dict, manifest_brief: str,
                                  recipe_id: str = "wf") -> str:
        self.seed_recipe(recipe_id, recipe_toml(recipe_id, fragments=recipe_frags))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            + manifest_brief +
            f"\n[recipes.{recipe_id}]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return self.agents_md()

    def test_append_default_recipe_before_manifest(self):
        agents = self._sync_recipe_and_manifest(
            {"workflow_rules": ["Recipe rule."]},
            "[brief]\nworkflow_rules = ['Manifest rule.']\n",
        )
        section = self.section_text("Workflow Rules")
        self.assertLess(section.index("Recipe rule."), section.index("Manifest rule."))

    def test_replace_mode_suppresses_recipe_fragments(self):
        agents = self._sync_recipe_and_manifest(
            {"workflow_rules": ["Recipe rule."]},
            "[brief]\nworkflow_rules_mode = 'replace'\nworkflow_rules = ['Only this rule.']\n",
        )
        self.assertIn("Only this rule.", agents)
        self.assertNotIn("Recipe rule.", agents)

    def test_replace_mode_isolates_other_sections(self):
        # workflow_rules REPLACE, but runtime_flow should still get recipe fragments
        agents = self._sync_recipe_and_manifest(
            {
                "workflow_rules": ["WF recipe."],
                "runtime_flow": ["RF recipe."],
            },
            "[brief]\nworkflow_rules_mode = 'replace'\nworkflow_rules = ['WF only.']\n",
        )
        self.assertNotIn("- WF recipe.", self.section_text("Workflow Rules"))
        self.assertIn("- RF recipe.", self.section_text("Runtime Flow"))

    def test_manifest_prose_never_substituted(self):
        agents = self._sync_recipe_and_manifest(
            {"workflow_rules": ["Recipe filler."]},
            "[brief]\nworkflow_rules = ['Check {config.test_command}']\n",
        )
        self.assertIn("Check {config.test_command}", agents)

    def test_empty_manifest_brief_populated_by_recipe_fragments(self):
        agents = self._sync_recipe_and_manifest(
            {"workflow_rules": ["Create a worktree.", "Do not merge directly."]},
            "",  # no [brief] section at all
        )
        self.assertIn("Create a worktree.", agents)
        self.assertIn("Do not merge directly.", agents)

    def test_recipe_without_fragments_unchanged_output(self):
        agents = self._sync_recipe_and_manifest(
            None,
            "[brief]\nworkflow_rules = ['Static rule.']\n",
        )
        # Without fragments, the manifest rule is still emitted
        self.assertIn("Static rule.", agents)

    def test_idempotent_collection(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": ["Recipe rule."]
        }))
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            "[brief]\nworkflow_rules = ['Manifest rule.']\n\n"
            "[recipes.wf]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        first = (self.project / "AGENTS.md").read_bytes()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        second = (self.project / "AGENTS.md").read_bytes()
        self.assertEqual(first, second)

    def test_exact_string_dedup_recipe_vs_manifest(self):
        # Same text in recipe and manifest → appears once
        agents = self._sync_recipe_and_manifest(
            {"workflow_rules": ["Create a worktree."]},
            "[brief]\nworkflow_rules = ['Create a worktree.']\n",
        )
        self.assertEqual(
            self.section_text("Workflow Rules").count("Create a worktree."), 1
        )

    def test_context_sources_append(self):
        agents = self._sync_recipe_and_manifest(
            {"context_sources": ["Recipe ctx."]},
            "[brief]\ncontext_sources = ['Manifest ctx.']\n",
        )
        section = self.section_text("Context Sources")
        self.assertIn("Recipe ctx.", section)
        self.assertIn("Manifest ctx.", section)

    def test_conflict_policy_append(self):
        agents = self._sync_recipe_and_manifest(
            {"conflict_policy": ["Recipe policy."]},
            "[brief]\nconflict_policy = ['Manifest policy.']\n",
        )
        section = self.section_text("Conflict Policy")
        self.assertIn("Recipe policy.", section)
        self.assertIn("Manifest policy.", section)

    def test_useful_commands_append(self):
        agents = self._sync_recipe_and_manifest(
            {"useful_commands": ["Recipe cmd."]},
            "[brief]\nuseful_commands = ['Manifest cmd.']\n",
        )
        section = self.section_text("Useful Commands")
        self.assertIn("Recipe cmd.", section)
        self.assertIn("Manifest cmd.", section)

    def test_no_section_header_when_no_bullets(self):
        # Both recipe and manifest have no workflow_rules → section not emitted
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("## Workflow Rules", self.agents_md())


# ---------------------------------------------------------------------------

class ValidateBriefModesTests(BriefFragmentCLITest):
    """[brief].<section>_mode validation, observed through sync exit codes."""

    def _sync_brief(self, brief_body: str):
        self.write_manifest(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n\n"
            + brief_body
        )
        return self.sync()

    def test_valid_append_mode_no_error(self):
        result = self._sync_brief("[brief]\nworkflow_rules_mode = 'append'\n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_valid_replace_mode_no_error(self):
        result = self._sync_brief("[brief]\nworkflow_rules_mode = 'replace'\n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_unknown_mode_raises(self):
        result = self._sync_brief("[brief]\nworkflow_rules_mode = 'merge'\n")
        self.assertNotEqual(result.returncode, 0)
        # error message must mention the key
        self.assertIn("workflow_rules_mode", result.stderr)

    def test_unknown_mode_error_mentions_valid_values(self):
        result = self._sync_brief("[brief]\ncontext_sources_mode = 'upsert'\n")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue("append" in result.stderr or "replace" in result.stderr)

    def test_no_mode_keys_no_error(self):
        result = self._sync_brief("[brief]\nworkflow_rules = ['rule.']\n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_empty_brief_no_error(self):
        result = self._sync_brief("")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


# ---------------------------------------------------------------------------

class McpDescriptionsOverrideFillsGapTests(BriefFragmentCLITest):
    """mcp_descriptions override-fills-gap, observed in ## Runtime MCPs."""

    def _sync_mcp(self, recipe_descriptions: dict[str, list[tuple[str, str]]] | None,
                  manifest_mcp_desc: dict | None, mcp_servers: dict) -> str:
        if recipe_descriptions:
            for rid, frags in recipe_descriptions.items():
                self.seed_recipe(rid, recipe_toml(rid, fragments={
                    "mcp_descriptions": frags
                }))
        lines = ["[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n"]
        if manifest_mcp_desc:
            lines.append("[brief.mcp_descriptions]\n")
            for server, desc in manifest_mcp_desc.items():
                lines.append(f"{server} = {_toml_str(desc)}\n")
        for server, body in mcp_servers.items():
            lines.append(f"\n[mcp.{server}]\n{body}")
        for rid in (recipe_descriptions or {}):
            lines.append(f"\n[recipes.{rid}]\nenabled = true\n")
        self.write_manifest("".join(lines))
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return self.section_text("Runtime MCPs")

    def test_project_override_wins(self):
        section = self._sync_mcp(
            recipe_descriptions={"recipe-a": [("trello", "Recipe default.")]},
            manifest_mcp_desc={"trello": "Project override."},
            mcp_servers={"trello": "command = 'npx'\nargs = ['-y', '@t/m']\n"},
        )
        self.assertIn("Project override.", section)
        self.assertNotIn("Recipe default.", section)

    def test_recipe_fills_gap(self):
        section = self._sync_mcp(
            recipe_descriptions={"recipe-a": [("trello", "Recipe default.")]},
            manifest_mcp_desc=None,
            mcp_servers={"trello": "command = 'npx'\nargs = ['-y', '@t/m']\n"},
        )
        self.assertIn("Recipe default.", section)

    def test_no_mcp_descriptions_no_crash(self):
        # Must not crash: an MCP server without any description renders fine
        section = self._sync_mcp(
            recipe_descriptions=None,
            manifest_mcp_desc=None,
            mcp_servers={"vault": "command = 'npx'\nargs = ['-y', '@v']\n"},
        )
        self.assertIn("vault", section)

    def test_multi_recipe_non_overlapping_keys(self):
        section = self._sync_mcp(
            recipe_descriptions={
                "recipe-a": [("trello", "Trello desc.")],
                "recipe-b": [("engram", "Engram desc.")],
            },
            manifest_mcp_desc=None,
            mcp_servers={
                "trello": "command = 'npx'\nargs = ['-y', '@t']\n",
                "engram": "command = 'npx'\nargs = ['-y', '@e']\n",
            },
        )
        self.assertIn("Trello desc.", section)
        self.assertIn("Engram desc.", section)

    def test_manifest_override_does_not_affect_other_servers(self):
        section = self._sync_mcp(
            recipe_descriptions={
                "recipe-a": [("trello", "Recipe trello."), ("engram", "Recipe engram.")],
            },
            manifest_mcp_desc={"trello": "Project trello."},
            mcp_servers={
                "trello": "command = 'npx'\nargs = ['-y', '@t']\n",
                "engram": "command = 'npx'\nargs = ['-y', '@e']\n",
            },
        )
        self.assertIn("Project trello.", section)
        self.assertNotIn("Recipe trello.", section)
        self.assertIn("Recipe engram.", section)


# ---------------------------------------------------------------------------

class EndToEndRenderTests(BriefFragmentCLITest):
    """End-to-end render contracts through full sync runs."""

    def test_runtime_brief_marker_suppresses_regeneration(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": ["New fragment."]
        }))
        self.write_manifest("[project]\nname = 'test'\n\n[recipes.wf]\nenabled = true\n")
        # Pre-existing AGENTS.md with marker
        existing = "# Existing\n<!-- ai-specs:runtime-brief -->\nHand-written content.\n"
        (self.project / "AGENTS.md").write_text(existing)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.project / "AGENTS.md").read_text(), existing)

    def test_idempotent_render_with_fragments(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": [
                "Do not push to `{config.integration_branch}` without a PR."
            ]
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n[brief]\n\n"
            "[recipes.wf]\nenabled = true\n"
            "[recipes.wf.config]\nintegration_branch = 'main'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        first = (self.project / "AGENTS.md").read_bytes()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        second = (self.project / "AGENTS.md").read_bytes()
        self.assertEqual(first, second)

    def test_empty_brief_populated_by_recipe_fragments(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": ["Create a worktree.", "Do not merge directly."]
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nintro = 'Test project.'\npurpose = 'For testing.'\n\n"
            "[recipes.wf]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = self.agents_md()
        self.assertIn("Create a worktree.", agents)
        self.assertIn("Do not merge directly.", agents)

    def test_no_fragments_backward_compat(self):
        self.seed_recipe("wf", recipe_toml("wf"))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules = ['Static rule.']\n\n"
            "[recipes.wf]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Static rule.", self.agents_md())

    def test_replace_mode_in_full_render(self):
        self.seed_recipe("wf", recipe_toml("wf", fragments={
            "workflow_rules": ["Recipe rule — should not appear."]
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules_mode = 'replace'\n"
            "workflow_rules = ['Only this rule.']\n\n"
            "[recipes.wf]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = self.agents_md()
        self.assertIn("Only this rule.", agents)
        self.assertNotIn("Recipe rule — should not appear.", agents)

    def test_validate_brief_modes_called_from_render(self):
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules_mode = 'invalid_mode'\n"
        )
        result = self.sync()
        self.assertNotEqual(result.returncode, 0)


# ---------------------------------------------------------------------------
# Batch 6 — Regression & Idempotency
# ---------------------------------------------------------------------------

class B6RegressionTests(BriefFragmentCLITest):
    """Batch 6 regression contracts via full sync runs: marker suppression,
    idempotency, minimal manifest, recipe-without-fragments compatibility."""

    def test_marker_suppresses_regeneration_with_recipe_fragments(self):
        """AGENTS.md with <!-- ai-specs:runtime-brief --> must NOT be modified even when
        recipes now contribute [provides.brief] fragments (B6 regression for 6.1/6.2)."""
        self.seed_recipe("worktree-flow", recipe_toml("worktree-flow", fragments={
            "workflow_rules": [
                "Create worktree for every change.",
                "Do not push to `{config.integration_branch}` without a PR.",
            ]
        }))
        self.seed_recipe("tdd-flow", recipe_toml("tdd-flow", fragments={
            "workflow_rules": ["Write failing tests first."],
            "useful_commands": ["Run tests: `{config.test_command}`"],
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nintro = 'Intro.'\npurpose = 'Purpose.'\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
            "[recipes.worktree-flow.config]\nintegration_branch = 'main'\n\n"
            "[recipes.tdd-flow]\nenabled = true\n"
            "[recipes.tdd-flow.config]\ntest_command = './tests/run.sh'\n"
        )
        # Pre-existing AGENTS.md with the runtime-brief marker (hand-managed)
        hand_managed = (
            "# Hand-Managed Brief\n"
            "<!-- ai-specs:runtime-brief -->\n"
            "This content is hand-written and MUST NOT be replaced.\n"
        )
        (self.project / "AGENTS.md").write_text(hand_managed)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        # Must be byte-identical to the original hand-managed content
        self.assertEqual((self.project / "AGENTS.md").read_text(), hand_managed)

    def test_idempotency_with_config_substitution(self):
        """Two consecutive syncs with config substitution must produce byte-identical output."""
        self.seed_recipe("worktree-flow", recipe_toml("worktree-flow", fragments={
            "workflow_rules": [
                "Create worktree. Branch: `{config.integration_branch}`.",
                "Preserve unrelated changes.",
            ]
        }))
        self.seed_recipe("git-pr-flow", recipe_toml("git-pr-flow", fragments={
            "workflow_rules": [
                "Use GitHub PRs to merge into `{config.base_branch}`.",
            ]
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nintro = 'Test project.'\npurpose = 'Testing.'\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
            "[recipes.worktree-flow.config]\nintegration_branch = 'main'\n\n"
            "[recipes.git-pr-flow]\nenabled = true\n"
            "[recipes.git-pr-flow.config]\nbase_branch = 'main'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        first = (self.project / "AGENTS.md").read_bytes()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        second = (self.project / "AGENTS.md").read_bytes()
        # Must be byte-identical — no ordering drift or duplicate bullets
        self.assertEqual(first, second, "Render output must be idempotent (byte-identical on two runs)")

    def test_minimal_brief_with_config_key_substitution(self):
        """Minimal [brief] (only intro+purpose) + recipe with {config.KEY} → rendered output
        contains substituted values, not raw placeholders (B6 scenario 6.8)."""
        self.seed_recipe("tdd-flow", recipe_toml("tdd-flow", fragments={
            "workflow_rules": [
                "Write failing tests first.",
                "Run the suite before committing.",
            ],
            "useful_commands": ["Run tests: `{config.test_command}`"],
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nintro = 'Test intro.'\npurpose = 'Test purpose.'\n\n"
            "[recipes.tdd-flow]\nenabled = true\n"
            "[recipes.tdd-flow.config]\ntest_command = './tests/run.sh'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = self.agents_md()
        # Substituted value must appear (not the placeholder)
        self.assertIn("./tests/run.sh", agents)
        self.assertNotIn("{config.test_command}", agents)
        # Section populated entirely from recipe fragments
        self.assertIn("Write failing tests first.", agents)
        self.assertIn("Run the suite before committing.", agents)
        self.assertIn("Run tests: `./tests/run.sh`", agents)

    def test_recipe_without_provides_brief_does_not_break_render(self):
        """Enabled recipe with no brief_fragments key → sync succeeds, other sections intact."""
        self.seed_recipe("no-brief-recipe", recipe_toml("no-brief-recipe"))
        self.seed_recipe("with-brief-recipe", recipe_toml("with-brief-recipe", fragments={
            "workflow_rules": ["Recipe rule."]
        }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n"
            "[brief]\nworkflow_rules = ['Static manifest rule.']\n\n"
            "[recipes.no-brief-recipe]\nenabled = true\n\n"
            "[recipes.with-brief-recipe]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = self.agents_md()
        # Recipe rule still appears
        self.assertIn("Recipe rule.", agents)
        # Manifest rule appears (deduped, not duplicated)
        self.assertIn("Static manifest rule.", agents)
        self.assertEqual(agents.count("Static manifest rule."), 1)

    def test_exact_string_dedup_idempotency(self):
        """Same fragment text from two recipes → appears exactly once; sync is idempotent."""
        for rid in ("recipe-a", "recipe-b"):
            self.seed_recipe(rid, recipe_toml(rid, fragments={
                "workflow_rules": ["Shared rule."]
            }))
        self.write_manifest(
            "[project]\nname = 'test'\n\n[brief]\n\n"
            "[recipes.recipe-a]\nenabled = true\n\n"
            "[recipes.recipe-b]\nenabled = true\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        first = (self.project / "AGENTS.md").read_text()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        second = (self.project / "AGENTS.md").read_text()
        self.assertEqual(first, second, "Must be idempotent")
        # "Shared rule." must appear exactly once
        self.assertEqual(first.count("Shared rule."), 1)


# ---------------------------------------------------------------------------

class VcsFragmentIsolationTests(BriefFragmentCLITest):
    """VCS workflow_rules fragments stay isolated to the bound recipe.

    When multiple VCS sibling recipes are enabled but only one is bound to
    vcs-pr-flow, only the bound recipe contributes workflow_rules fragments.
    When no binding exists, no VCS sibling fragments are emitted.
    Non-VCS recipes always contribute regardless of VCS binding state.
    """

    def _seed_vcs_siblings(self) -> None:
        """3 VCS siblings + 1 non-VCS recipe in the isolated catalog."""
        for rid, branch, rule in (
            ("git-pr-flow", "main", "Use GitHub PRs to merge."),
            ("gitlab-mr-flow", "development", "Use GitLab MRs to merge."),
            ("bitbucket-pr-flow", "develop", "Use Bitbucket PRs to merge."),
        ):
            self.seed_recipe(rid, recipe_toml(
                rid,
                capabilities=("vcs-pr-flow",),
                fragments={"workflow_rules": [rule]},
            ))
        self.seed_recipe("worktree-flow", recipe_toml("worktree-flow", fragments={
            "workflow_rules": ["Create a worktree for every change."]
        }))

    def _enable(self, *recipe_ids: str, binding: str | None = None) -> None:
        blocks = ["[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n"]
        for rid in recipe_ids:
            blocks.append(f"\n[recipes.{rid}]\nenabled = true\n")
        if binding:
            blocks.append(
                "\n[[bindings]]\ncapability = 'vcs-pr-flow'\n"
                f"recipe = '{binding}'\n"
            )
        self.write_manifest("".join(blocks))
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_bound_gitlab_only_gitlab_fragments_in_workflow_rules(self):
        """3 VCS recipes enabled, bound to gitlab-mr-flow → only GitLab fragments."""
        self._seed_vcs_siblings()
        self._enable("git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow",
                     "worktree-flow", binding="gitlab-mr-flow")
        section = self.section_text("Workflow Rules")
        # GitLab fragments MUST appear
        self.assertIn("Use GitLab MRs to merge.", section)
        # GitHub and Bitbucket fragments MUST NOT appear
        self.assertNotIn("Use GitHub PRs to merge.", section)
        self.assertNotIn("Use Bitbucket PRs to merge.", section)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree for every change.", section)

    def test_no_vcs_binding_no_vcs_fragments(self):
        """VCS siblings enabled but no vcs-pr-flow binding → no VCS fragments."""
        self._seed_vcs_siblings()
        self._enable("git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow",
                     "worktree-flow")
        section = self.section_text("Workflow Rules")
        # No VCS fragments should appear when unbound
        self.assertNotIn("Use GitHub PRs to merge.", section)
        self.assertNotIn("Use GitLab MRs to merge.", section)
        self.assertNotIn("Use Bitbucket PRs to merge.", section)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree for every change.", section)

    def test_bound_custom_recipe_contributes_own_fragments(self):
        """Custom recipe bound to vcs-pr-flow → its own fragments still appear."""
        self.seed_recipe("my-custom-vcs", recipe_toml(
            "my-custom-vcs",
            capabilities=("vcs-pr-flow",),
            fragments={"workflow_rules": ["Use custom VCS flow."]},
        ))
        self.seed_recipe("git-pr-flow", recipe_toml(
            "git-pr-flow",
            capabilities=("vcs-pr-flow",),
            fragments={"workflow_rules": ["Use GitHub PRs to merge."]},
        ))
        self.seed_recipe("worktree-flow", recipe_toml("worktree-flow", fragments={
            "workflow_rules": ["Create a worktree."]
        }))
        self._enable("my-custom-vcs", "git-pr-flow", "worktree-flow",
                     binding="my-custom-vcs")
        section = self.section_text("Workflow Rules")
        # Custom recipe fragments MUST appear (it's the bound recipe)
        self.assertIn("Use custom VCS flow.", section)
        # Known VCS sibling fragments MUST NOT appear (not the bound recipe)
        self.assertNotIn("Use GitHub PRs to merge.", section)
        # Non-VCS fragments MUST still appear
        self.assertIn("Create a worktree.", section)


class RepoTopologyBriefTests(BriefFragmentCLITest):
    """Repo topology line in ## Project, observed through full sync runs."""

    @classmethod
    def setUpClass(cls):
        sys.path.insert(0, str(ROOT / "tests"))
        from test_repo_topology import make_super_with_submodule  # noqa: F401
        cls._make_super = staticmethod(make_super_with_submodule)

    def _super_project(self) -> Path:
        """Build a super repo with one submodule and stage ai-specs inside it."""
        super_repo = self._make_super(self.project)
        (super_repo / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
        (super_repo / "ai-specs" / "commands").mkdir(exist_ok=True)
        (super_repo / "ai-specs" / "ai-specs.toml").write_text("")
        return super_repo

    def _sync_super(self, manifest_body: str) -> str:
        super_repo = self._super_project()
        (super_repo / "ai-specs" / "ai-specs.toml").write_text(manifest_body)
        result = invoke(super_repo, "sync", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return (super_repo / "AGENTS.md").read_text()

    def test_repo_topology_line_in_project_section(self):
        text = self._sync_super(
            "[project]\nname = 'topo'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
        )
        self.assertIn("- **Repo topology**: `monorepo-submodules` (via auto)", text)


    def test_repo_topology_omitted_when_worktree_flow_disabled(self):
        """Config dict alone must not surface Repo topology when recipe disabled."""
        text = self._sync_super(
            "[project]\nname = 'topo'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = false\n"
        )
        self.assertNotIn("Repo topology", text)

    def test_repo_topology_from_project_field_even_without_worktree_flow(self):
        """[project].repo_topology is CLI-owned, not gated on the recipe."""
        self.write_manifest(
            "[project]\nname = 'topo'\nrepo_topology = 'standalone'\n\n"
            "[agents]\nenabled = ['claude']\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        text = self.agents_md()
        self.assertIn("- **Repo topology**: `standalone` (via config)", text)

    def test_project_field_wins_over_legacy_recipe_alias_in_brief(self):
        text = self._sync_super(
            "[project]\nname = 'topo'\nrepo_topology = 'standalone'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
            "[recipes.worktree-flow.config]\nrepo_topology = 'monorepo-apps'\n"
        )
        self.assertIn("- **Repo topology**: `standalone` (via config)", text)
        self.assertNotIn("monorepo-apps", text)


class WorktreeGateModeBriefRenderTests(BriefFragmentCLITest):
    """Rendered brief behavior for the worktree-flow config-aware gate fragment.

    Uses the real catalog recipe fragments so the recipe source is exercised
    end-to-end through the CLI for each gate_mode value.
    """

    def _sync_gate_mode(self, gate_mode: str) -> str:
        self.write_manifest(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n"
            "[recipes.worktree-flow.config]\n"
            f"gate_mode = '{gate_mode}'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return self.agents_md()

    def test_ask_mode_brief_uses_user_mediated_destinations(self):
        text = self._sync_gate_mode("ask")
        self.assertIn("`gate_mode = ask`", text)
        self.assertIn("ask the user to choose a destination", text)
        self.assertIn("feature branch in the current checkout", text)
        self.assertIn("explicit protected-branch override", text)
        self.assertNotIn("WORKTREE_GATE_MODE=off", text)
        self.assertNotIn(
            "Create a dedicated worktree for changes that write artifacts or modify code.",
            text,
        )

    def test_always_mode_brief_requires_dedicated_worktree(self):
        text = self._sync_gate_mode("always")
        self.assertIn("`gate_mode = always`", text)
        self.assertIn("`always` requires a dedicated worktree", text)

    def test_off_mode_brief_defers_to_user_direction(self):
        text = self._sync_gate_mode("off")
        self.assertIn("`gate_mode = off`", text)
        self.assertIn("where the user directs", text)


if __name__ == "__main__":
    unittest.main()

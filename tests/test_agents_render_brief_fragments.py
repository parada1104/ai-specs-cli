"""Black-box CLI tests for recipe brief_fragments support.

Converted from the coupled agents-render.py unit tests: every test drives
`bin/ai-specs sync` as a subprocess via `_blackbox.invoke` against a hermetic
project and an isolated CLI home whose recipe catalog is replaced per test
class. Each scenario therefore renders through the real sync pipeline —
recipe materialization, config resolution, fragment collection, and the
brief renderer — instead of calling `lib/_internal/agents-render.py` directly.

Tests cover:
  - {config.KEY} resolution: known key, missing key verbatim, bare key verbatim,
    {{ }} escape, mixed escape+substitution, lone unbalanced brace, empty text
  - fragment collection: ordering, key-dedup, exact-string dedup, no fragments,
    disabled recipe, empty brief_fragments
  - section merge: APPEND default, REPLACE opt-in, REPLACE isolation,
    manifest prose never substituted, empty manifest [brief] end-to-end
  - brief mode validation: unknown mode value → sync fails loudly
  - mcp_descriptions override-fills-gap: project wins, recipe fills gap,
    no descriptions → no crash, multi-recipe non-overlapping
  - VCS sibling isolation: only the bound vcs-pr-flow recipe contributes
    workflow_rules fragments
  - repo_topology: line rendered only when worktree-flow is enabled
"""
from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
MARKER = "<!-- ai-specs:runtime-brief -->"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _mini_recipe(
    rid: str,
    *,
    name: str | None = None,
    config: dict[str, str] | None = None,
    fragments: list[str] | None = None,
    context_sources: list[str] | None = None,
    context_sources_keyed: list[tuple[str, str]] | None = None,
    workflow_rules: list[str] | None = None,
    runtime_flow: list[str] | None = None,
    conflict_policy: list[str] | None = None,
    useful_commands: list[str] | None = None,
    mcp_descriptions: dict[str, str] | None = None,
    capabilities: list[str] | None = None,
) -> str:
    """Build a minimal recipe.toml with a [provides.brief] section (brief-optional)."""
    lines = [
        "[recipe]",
        f'id = "{rid}"',
        f'name = "{name or rid}"',
        'description = "Test recipe."',
        'version = "1.0.0"',
        'author = "tests"',
    ]
    if capabilities:
        for cap in capabilities:
            lines.append("")
            lines.append("[[capabilities]]")
            lines.append(f'id = "{cap}"')
    if config:
        for key, value in config.items():
            lines.append("")
            lines.append(f"[config.{key}]")
            lines.append("required = false")
            lines.append('type = "string"')
            lines.append(f'default = "{value}"')
    provides: list[str] = []
    if fragments:
        provides.append(f"workflow_rules = [{', '.join(repr(f) for f in fragments)}]")
    if context_sources:
        provides.append(
            f"context_sources = [{', '.join(repr(s) for s in context_sources)}]"
        )
    for key, values in (
        ("workflow_rules", workflow_rules),
        ("runtime_flow", runtime_flow),
        ("conflict_policy", conflict_policy),
        ("useful_commands", useful_commands),
    ):
        if values:
            provides.append(f"{key} = [{', '.join(repr(s) for s in values)}]")
    if mcp_descriptions:
        entries = ", ".join(
            f'{{ key = "{key}", text = "{text}" }}' for key, text in mcp_descriptions.items()
        )
        provides.append(f"mcp_descriptions = [{entries}]")
    if provides:
        lines.append("")
        lines.append("[provides.brief]")
        lines.extend(provides)
    for key, text in context_sources_keyed or ():
        lines.append("")
        lines.append("[[provides.brief.context_sources]]")
        lines.append(f'key = "{key}"')
        lines.append(f'text = "{text}"')
    return "\n".join(lines) + "\n"


def _recipes_block(ids: list[str], configs: dict[str, dict[str, str]] | None = None) -> str:
    """Render the `[recipes.<id>]` enablement block plus optional config overrides."""
    lines: list[str] = []
    for rid in ids:
        lines.append(f"[recipes.{rid}]")
        lines.append("enabled = true")
    for rid, values in (configs or {}).items():
        lines.append("")
        lines.append(f"[recipes.{rid}.config]")
        for key, value in values.items():
            lines.append(f'{key} = "{value}"')
    return "\n".join(lines) + "\n"


def _brief_block(
    *,
    intro: str | None = None,
    purpose: str | None = None,
    workflow_rules: list[str] | None = None,
    workflow_rules_mode: str | None = None,
    context_sources: list[str] | None = None,
    context_sources_mode: str | None = None,
    conflict_policy: list[str] | None = None,
    useful_commands: list[str] | None = None,
    mcp_descriptions: dict[str, str] | None = None,
) -> str:
    """Render the manifest `[brief]` block."""
    lines = ["[brief]"]
    if intro:
        lines.append(f'intro = "{intro}"')
    if purpose:
        lines.append(f'purpose = "{purpose}"')
    if workflow_rules_mode:
        lines.append(f'workflow_rules_mode = "{workflow_rules_mode}"')
    if context_sources_mode:
        lines.append(f'context_sources_mode = "{context_sources_mode}"')
    for key, values in (
        ("workflow_rules", workflow_rules),
        ("context_sources", context_sources),
        ("conflict_policy", conflict_policy),
        ("useful_commands", useful_commands),
    ):
        if values is not None:
            lines.append(f"{key} = [{', '.join(repr(v) for v in values)}]")

    if mcp_descriptions is not None:
        lines.append("[brief.mcp_descriptions]")
        for key, value in mcp_descriptions.items():
            lines.append(f'{key} = "{value}"')
    return "\n".join(lines) + "\n"


def _bindings_block(pairs: dict[str, str]) -> str:
    """Render the manifest `[[bindings]]` block (array-of-tables form)."""
    lines: list[str] = []
    for capability, recipe in pairs.items():
        lines.append("[[bindings]]")
        lines.append(f'capability = "{capability}"')
        lines.append(f'recipe = "{recipe}"')
    return "\n".join(lines) + "\n"


def _mcp_block(ids: list[str]) -> str:
    """Render the manifest `[mcp]` block."""
    lines = ["[mcp]"]
    for mcp_id in ids:
        lines.append(f'"{mcp_id}" = {{ command = "echo" }}')
    return "\n".join(lines) + "\n"


def _section(text: str, heading: str) -> str:
    """Return the body of a `## heading` section in rendered AGENTS.md."""
    marker = f"## {heading}\n"
    start = text.index(marker) + len(marker)
    end = text.index("\n## ", start) if "\n## " in text[start:] else len(text)
    return text[start:end]


def _git(repo: Path, *args: str) -> subprocess.CompletedProcess:
    env = os.environ.copy()
    env.update(
        {
            "GIT_AUTHOR_NAME": "t",
            "GIT_AUTHOR_EMAIL": "t@t",
            "GIT_COMMITTER_NAME": "t",
            "GIT_COMMITTER_EMAIL": "t@t",
        }
    )
    return subprocess.run(
        ["git", "-C", str(repo), *args], env=env, text=True, capture_output=True, check=False
    )


class CliBriefTestBase(unittest.TestCase):
    """Hermetic fixture: isolated CLI home whose catalog vendors this class's recipes."""

    RECIPES: dict[str, str] = {}
    REAL_RECIPES: tuple[str, ...] = ()

    def _home(self) -> Path:
        if not hasattr(self, "_cli_home"):
            td = tempfile.TemporaryDirectory(prefix="bb-brief-home-")
            self.addCleanup(td.cleanup)
            home = isolated_home(Path(td.name))
            catalog = home / "catalog"
            catalog.unlink()
            (catalog / "recipes").mkdir(parents=True)
            for rid, toml in self.RECIPES.items():
                dest = catalog / "recipes" / rid
                dest.mkdir()
                (dest / "recipe.toml").write_text(toml)
            for rid in self.REAL_RECIPES:
                (catalog / "recipes" / rid).symlink_to(CATALOG / rid)
            self._cli_home = home
        return self._cli_home

    def _cli(self, project: Path, verb: str, *args: str):
        """Single shared wrapper: every test in this class invokes through here."""
        return invoke(project, verb, *args, cli_home=self._home())

    def _project(self, manifest: str) -> Path:
        td = tempfile.TemporaryDirectory(prefix="bb-brief-project-")
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(manifest)
        return root


# ---------------------------------------------------------------------------
# SubstituteConfigTests — {config.KEY} resolution through the real renderer
# ---------------------------------------------------------------------------

class SubstituteConfigTests(CliBriefTestBase):
    RECIPES = {
        "known": _mini_recipe(
            "known",
            config={"integration_branch": "development"},
            fragments=["Do not push to `{config.integration_branch}` without a PR."],
        ),
        "enum": _mini_recipe(
            "enum",
            config={"artifact_store_default": "both"},
            fragments=["Default artifact store: `{config.artifact_store_default}`."],
        ),
        "missing": _mini_recipe(
            "missing",
            fragments=[
                "Run {config.test_command} first.",
                "{config.missing_key}",
                "Some prose { with brace.",
                "",
            ],
        ),
        "bare": _mini_recipe(
            "bare",
            config={"integration_branch": "main"},
            fragments=["See {integration_branch}."],
        ),
        "escapes": _mini_recipe(
            "escapes",
            config={"test_command": "./run.sh"},
            fragments=[
                "Use {{config.KEY}} to reference.",
                "Run `{config.test_command}` (not {{skip}}).",
            ],
        ),
    }

    def _sync(self, recipe: str, configs: dict[str, dict[str, str]] | None = None):
        manifest = "[project]\nname = 'demo'\n\n" + _recipes_block([recipe], configs)
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_known_key_resolves(self):
        result, text = self._sync("known", {"known": {"integration_branch": "development"}})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Do not push to `development` without a PR.", text)
        self.assertNotIn("{config.integration_branch}", text)

    def test_artifact_store_enum_value_resolves(self):
        result, text = self._sync("enum", {"enum": {"artifact_store_default": "both"}})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Default artifact store: `both`.", text)
        self.assertNotIn("{config.artifact_store_default}", text)

    def test_missing_key_verbatim(self):
        result, text = self._sync("missing")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Run {config.test_command} first.", text)

    def test_missing_key_no_crash(self):
        result, text = self._sync("missing")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("{config.missing_key}", text)

    def test_bare_key_verbatim(self):
        result, text = self._sync("bare", {"bare": {"integration_branch": "main"}})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("See {integration_branch}.", text)

    def test_double_brace_escape(self):
        result, text = self._sync("escapes")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Use {config.KEY} to reference.", text)

    def test_mixed_escape_and_substitution(self):
        result, text = self._sync("escapes", {"escapes": {"test_command": "./run.sh"}})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Run `./run.sh` (not {skip}).", text)
        self.assertNotIn("{config.test_command}", text)

    def test_lone_unbalanced_brace_no_crash(self):
        result, text = self._sync("missing")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Some prose { with brace.", text)

    def test_empty_string(self):
        result, _ = self._sync("missing")
        self.assertEqual(result.returncode, 0, result.stderr)


# ---------------------------------------------------------------------------
# CollectRecipeBriefFragmentsTests
# ---------------------------------------------------------------------------

class CollectRecipeBriefFragmentsTests(CliBriefTestBase):
    RECIPES = {
        "a": _mini_recipe("a", fragments=["A rule."]),
        "b": _mini_recipe("b", fragments=["B rule."]),
        "wf": _mini_recipe(
            "wf",
            config={"integration_branch": "main"},
            fragments=[
                "WF rule.",
                "Do not push to `{config.integration_branch}` without a PR.",
            ],
        ),
        "tdd": _mini_recipe("tdd", fragments=["TDD rule."]),
        "ctx-a": _mini_recipe(
            "ctx-a",
            context_sources_keyed=[("trello-sot", "Trello is the source of truth.")],
        ),
        "ctx-b": _mini_recipe(
            "ctx-b",
            context_sources_keyed=[
                ("trello-sot", "Trello: source of truth — updated wording.")
            ],
        ),
        "ra": _mini_recipe("ra", fragments=["Run tests before committing."]),
        "rb": _mini_recipe("rb", fragments=["Run tests before committing."]),
        "nobrief": _mini_recipe("nobrief"),
        "emptybrief": "[recipe]\nid = \"emptybrief\"\nname = \"emptybrief\"\n"
        "description = \"Test recipe.\"\nversion = \"1.0.0\"\nauthor = \"tests\"\n\n"
        "[provides.brief]\n",
        "x": _mini_recipe("x", fragments=["X"]),
    }

    def _sync(self, ids: list[str], brief: str = ""):
        manifest = "[project]\nname = 'demo'\n\n" + _recipes_block(ids)
        if brief:
            manifest += "\n" + brief
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_single_recipe_fragment_returned(self):
        result, text = self._sync(["a"])
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertEqual(section.count("A rule."), 1)
        self.assertIn("A rule.", section)

    def test_enabled_order_preserved(self):
        result, text = self._sync(["wf", "tdd"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(text.index("WF rule."), text.index("TDD rule."))

    def test_reversed_enabled_order(self):
        result, text = self._sync(["tdd", "wf"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(text.index("TDD rule."), text.index("WF rule."))

    def test_key_dedup_first_wins(self):
        result, text = self._sync(["ctx-a", "ctx-b"])
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Context Sources")
        self.assertEqual(section.count("Trello is the source of truth."), 1)
        self.assertIn("Trello is the source of truth.", section)
        self.assertNotIn("updated wording", section)

    def test_exact_string_dedup_across_recipes(self):
        result, text = self._sync(["ra", "rb"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(text.count("Run tests before committing."), 1)

    def test_recipe_without_brief_fragments_key(self):
        result, text = self._sync(
            ["nobrief"], _brief_block(workflow_rules=["Static rule."])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Static rule.", section)
        self.assertEqual(section.count("Static rule."), 1)

    def test_recipe_with_empty_brief_fragments(self):
        result, text = self._sync(
            ["emptybrief"], _brief_block(workflow_rules=["Static rule."])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Static rule.", section)
        self.assertEqual(section.count("Static rule."), 1)

    def test_disabled_recipe_not_in_enabled(self):
        result, text = self._sync(["a"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("A rule.", text)
        self.assertNotIn("B rule.", text)

    def test_recipe_not_in_recipes_dict(self):
        # TRIAGE: coupled to lib/_internal/agents-render.py.
        # (1) Specific assertion: collect_recipe_brief_fragments(resolved,
        #     "workflow_rules") returns [] when an enabled id is absent from
        #     the resolved recipes dict.
        # (2) Exact command run: `bin/ai-specs sync <project>` with
        #     `[recipes] "missing-recipe" = { enabled = true }` exits 1 with
        #     "recipe directory not found: .../catalog/recipes/missing-recipe"
        #     — the CLI rejects unknown recipe ids during materialization,
        #     before collect_recipe_brief_fragments ever runs.
        # (3) What it did not expose: collect-level tolerance for an enabled id
        #     absent from the recipes dict. No CLI surface (sync, sync-agent,
        #     init) can reach the collector in that state, so the behavior has
        #     no black-box equivalent.
        spec = importlib.util.spec_from_file_location(
            "agents_render_brief_collect_triage", ROOT / "lib" / "_internal" / "agents-render.py"
        )
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
        result = module.collect_recipe_brief_fragments(
            {"enabled": ["missing-recipe"], "recipes": {}}, "workflow_rules"
        )
        self.assertEqual(result, [])

    def test_substitution_applied(self):
        result, text = self._sync(["wf"])
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Do not push to `main` without a PR.", section)
        self.assertNotIn("{config.integration_branch}", section)

    def test_empty_enabled_list(self):
        manifest = (
            "[project]\nname = 'demo'\n\n"
            "[recipes.x]\nenabled = false\n"
        )
        project = self._project(manifest)
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("\n- X\n", (project / "AGENTS.md").read_text())

    def test_section_not_present_in_fragments(self):
        result, text = self._sync(["a"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("## Context Sources", text)


# ---------------------------------------------------------------------------
# SectionMergeTests
# ---------------------------------------------------------------------------

class SectionMergeTests(CliBriefTestBase):
    RECIPES = {
        "wf": _mini_recipe(
            "wf",
            workflow_rules=["Recipe rule.", "WF recipe.", "Create a worktree.", "Do not merge directly."],
            runtime_flow=["RF recipe."],
            context_sources=["Recipe ctx."],
            conflict_policy=["Recipe policy."],
            useful_commands=["Recipe cmd."],
        ),
        "nobrief": _mini_recipe("nobrief"),
    }

    def _sync(self, brief: str, recipes: list[str] | None = None):
        manifest = "[project]\nname = 'demo'\n\n" + _recipes_block(recipes or ["wf"])
        if brief:
            manifest += "\n" + brief
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_append_default_recipe_before_manifest(self):
        result, text = self._sync(_brief_block(workflow_rules=["Manifest rule."]))
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Recipe rule.", section)
        self.assertLess(
            section.index("Recipe rule."), section.index("Manifest rule.")
        )

    def test_replace_mode_suppresses_recipe_fragments(self):
        result, text = self._sync(
            _brief_block(workflow_rules_mode="replace", workflow_rules=["Only this rule."])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Only this rule.", section)
        self.assertNotIn("Recipe rule.", section)

    def test_replace_mode_isolates_other_sections(self):
        result, text = self._sync(
            _brief_block(workflow_rules_mode="replace", workflow_rules=["WF only."])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("WF recipe.", _section(text, "Workflow Rules"))
        self.assertIn("RF recipe.", _section(text, "Runtime Flow"))

    def test_manifest_prose_never_substituted(self):
        result, text = self._sync(
            _brief_block(workflow_rules=["Check {config.test_command}"])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Check {config.test_command}", text)

    def test_empty_manifest_brief_populated_by_recipe_fragments(self):
        result, text = self._sync("")
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Create a worktree.", section)
        self.assertIn("Do not merge directly.", section)

    def test_recipe_without_fragments_unchanged_output(self):
        result, text = self._sync(
            _brief_block(workflow_rules=["Static rule."]), ["nobrief"]
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Static rule.", section)
        self.assertEqual(section.count("Static rule."), 1)

    def test_idempotent_collection(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf"])
            + "\n" + _brief_block(workflow_rules=["Manifest rule."])
        )
        first = self._cli(project, "sync")
        self.assertEqual(first.returncode, 0, first.stderr)
        first_text = (project / "AGENTS.md").read_text()
        second = self._cli(project, "sync")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first_text, (project / "AGENTS.md").read_text())

    def test_exact_string_dedup_recipe_vs_manifest(self):
        result, text = self._sync(_brief_block(workflow_rules=["Create a worktree."]))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            _section(text, "Workflow Rules").count("Create a worktree."), 1
        )

    def test_context_sources_append(self):
        result, text = self._sync(_brief_block(context_sources=["Manifest ctx."]))
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Context Sources")
        self.assertIn("Recipe ctx.", section)
        self.assertIn("Manifest ctx.", section)

    def test_conflict_policy_append(self):
        result, text = self._sync(_brief_block(conflict_policy=["Manifest policy."]))
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Conflict Policy")
        self.assertIn("Recipe policy.", section)
        self.assertIn("Manifest policy.", section)

    def test_useful_commands_append(self):
        result, text = self._sync(_brief_block(useful_commands=["Manifest cmd."]))
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Useful Commands")
        self.assertIn("Recipe cmd.", section)
        self.assertIn("Manifest cmd.", section)

    def test_no_section_header_when_no_bullets(self):
        project = self._project("[project]\nname = 'demo'\n")
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("## Workflow Rules", (project / "AGENTS.md").read_text())


# ---------------------------------------------------------------------------
# ValidateBriefModesTests
# ---------------------------------------------------------------------------

class ValidateBriefModesTests(CliBriefTestBase):
    def _sync(self, brief: str):
        manifest = "[project]\nname = 'demo'\n\n" + brief
        return self._cli(self._project(manifest), "sync")

    def test_valid_append_mode_no_error(self):
        result = self._sync(_brief_block(workflow_rules_mode="append"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_valid_replace_mode_no_error(self):
        result = self._sync(_brief_block(workflow_rules_mode="replace"))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_unknown_mode_raises(self):
        result = self._sync(_brief_block(workflow_rules_mode="merge"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("workflow_rules_mode", result.stderr)
        self.assertIn("append", result.stderr)

    def test_unknown_mode_error_mentions_valid_values(self):
        result = self._sync(_brief_block(context_sources_mode="upsert"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("context_sources_mode", result.stderr)
        self.assertTrue("append" in result.stderr or "replace" in result.stderr)

    def test_no_mode_keys_no_error(self):
        result = self._sync(_brief_block(workflow_rules=["rule."]))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_empty_brief_no_error(self):
        result = self._sync("[brief]\n")
        self.assertEqual(result.returncode, 0, result.stderr)


# ---------------------------------------------------------------------------
# McpDescriptionsOverrideFillsGapTests
# ---------------------------------------------------------------------------

class McpDescriptionsOverrideFillsGapTests(CliBriefTestBase):
    RECIPES = {
        "recipe-a": _mini_recipe(
            "recipe-a",
            mcp_descriptions={"trello": "Trello desc."},
        ),
        "recipe-b": _mini_recipe("recipe-b", mcp_descriptions={"engram": "Engram desc."}),
        "recipe-c": _mini_recipe(
            "recipe-c",
            mcp_descriptions={"trello": "Recipe trello.", "engram": "Recipe engram."},
        ),
    }

    def _sync(self, recipes: list[str], mcp: list[str], brief: str = ""):
        manifest = (
            "[project]\nname = 'demo'\n\n"
            + _recipes_block(recipes)
            + "\n"
            + _mcp_block(mcp)
        )
        if brief:
            manifest += "\n" + brief
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_project_override_wins(self):
        result, text = self._sync(
            ["recipe-a"], ["trello"],
            _brief_block(mcp_descriptions={"trello": "Project override."}),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Project override.", text)
        self.assertNotIn("Trello desc.", text)

    def test_recipe_fills_gap(self):
        result, text = self._sync(["recipe-a"], ["trello"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Trello desc.", text)

    def test_no_mcp_descriptions_no_crash(self):
        result, text = self._sync([], ["vault"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("vault", text)

    def test_multi_recipe_non_overlapping_keys(self):
        result, text = self._sync(["recipe-a", "recipe-b"], ["trello", "engram"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Trello desc.", text)
        self.assertIn("Engram desc.", text)

    def test_manifest_override_does_not_affect_other_servers(self):
        result, text = self._sync(
            ["recipe-c"], ["trello", "engram"],
            _brief_block(mcp_descriptions={"trello": "Project trello."}),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Project trello.", text)
        self.assertNotIn("Recipe trello.", text)
        self.assertIn("Recipe engram.", text)


# ---------------------------------------------------------------------------
# EndToEndRenderTests
# ---------------------------------------------------------------------------

class EndToEndRenderTests(CliBriefTestBase):
    RECIPES = {
        "wf": _mini_recipe(
            "wf",
            fragments=[
                "Create a worktree.",
                "Do not merge directly.",
                "Recipe rule — should not appear.",
            ],
        ),
        "nobrief": _mini_recipe("nobrief"),
    }

    def _sync(self, brief: str = "", recipes: list[str] | None = None):
        manifest = "[project]\nname = 'demo'\n\n" + _recipes_block(recipes or ["wf"])
        if brief:
            manifest += "\n" + brief
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_runtime_brief_marker_suppresses_regeneration(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf"])
        )
        existing = "# Existing\n<!-- ai-specs:runtime-brief -->\nHand-written content.\n"
        (project / "AGENTS.md").write_text(existing)
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((project / "AGENTS.md").read_text(), existing)

    def test_idempotent_render_with_fragments(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf"])
            + "\n" + _brief_block(intro="Test project.", purpose="For testing.")
        )
        first = self._cli(project, "sync")
        self.assertEqual(first.returncode, 0, first.stderr)
        first_text = (project / "AGENTS.md").read_text()
        second = self._cli(project, "sync")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first_text, (project / "AGENTS.md").read_text())

    def test_empty_brief_populated_by_recipe_fragments(self):
        result, text = self._sync(_brief_block(intro="Test project.", purpose="For testing."))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Create a worktree.", text)
        self.assertIn("Do not merge directly.", text)

    def test_no_fragments_backward_compat(self):
        result, text = self._sync(
            _brief_block(workflow_rules=["Static rule."]), ["nobrief"]
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Static rule.", text)

    def test_replace_mode_in_full_render(self):
        result, text = self._sync(
            _brief_block(workflow_rules_mode="replace", workflow_rules=["Only this rule."])
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Only this rule.", text)
        self.assertNotIn("Recipe rule — should not appear.", text)

    def test_validate_brief_modes_called_from_render(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf"])
            + "\n" + _brief_block(workflow_rules_mode="invalid_mode")
        )
        result = self._cli(project, "sync")
        self.assertNotEqual(result.returncode, 0)


# ---------------------------------------------------------------------------
# B6RegressionTests
# ---------------------------------------------------------------------------

class B6RegressionTests(CliBriefTestBase):
    RECIPES = {
        "wf-x": _mini_recipe(
            "wf-x",
            config={"integration_branch": "main"},
            fragments=[
                "Create worktree. Branch: `{config.integration_branch}`.",
                "Preserve unrelated changes.",
            ],
        ),
        "pr-x": _mini_recipe(
            "pr-x",
            config={"base_branch": "main"},
            fragments=["Use GitHub PRs to merge into `{config.base_branch}`."],
        ),
        "tdd-flow": _mini_recipe(
            "tdd-flow",
            config={"test_command": "./tests/run.sh"},
            workflow_rules=["Write failing tests first.", "Run the suite before committing."],
            useful_commands=["Run tests: `{config.test_command}`"],
        ),
        "no-brief-recipe": _mini_recipe("no-brief-recipe", config={"some_config": "value"}),
        "with-brief-recipe": _mini_recipe("with-brief-recipe", fragments=["Recipe rule."]),
        "recipe-a": _mini_recipe("recipe-a", fragments=["Shared rule."]),
        "recipe-b": _mini_recipe("recipe-b", fragments=["Shared rule."]),
    }

    def test_marker_suppresses_regeneration_with_recipe_fragments(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf-x", "tdd-flow"])
            + "\n" + _brief_block(intro="Intro.", purpose="Purpose.")
        )
        hand_managed = (
            "# Hand-Managed Brief\n"
            "<!-- ai-specs:runtime-brief -->\n"
            "This content is hand-written and MUST NOT be replaced.\n"
        )
        (project / "AGENTS.md").write_text(hand_managed)
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((project / "AGENTS.md").read_text(), hand_managed)

    def test_idempotency_with_config_substitution(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["wf-x", "pr-x"])
            + "\n" + _brief_block(intro="Test project.", purpose="Testing.")
        )
        first = self._cli(project, "sync")
        self.assertEqual(first.returncode, 0, first.stderr)
        first_text = (project / "AGENTS.md").read_text()
        second = self._cli(project, "sync")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first_text, (project / "AGENTS.md").read_text())

    def test_minimal_brief_with_config_key_substitution(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["tdd-flow"])
            + "\n" + _brief_block(intro="Test intro.", purpose="Test purpose.")
        )
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        text = (project / "AGENTS.md").read_text()
        self.assertIn("./tests/run.sh", text)
        self.assertNotIn("{config.test_command}", text)
        self.assertIn("Write failing tests first.", text)
        self.assertIn("Run the suite before committing.", text)
        self.assertIn("Run tests: `./tests/run.sh`", text)

    def test_recipe_without_provides_brief_does_not_break_render(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n"
            + _recipes_block(["no-brief-recipe", "with-brief-recipe"])
            + "\n" + _brief_block(workflow_rules=["Static manifest rule."])
        )
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        text = (project / "AGENTS.md").read_text()
        self.assertIn("Recipe rule.", text)
        self.assertIn("Static manifest rule.", text)
        self.assertEqual(text.count("Static manifest rule."), 1)

    def test_exact_string_dedup_idempotency(self):
        project = self._project(
            "[project]\nname = 'demo'\n\n" + _recipes_block(["recipe-a", "recipe-b"])
            + "\n" + _brief_block(intro="Intro.", purpose="Purpose.")
        )
        first = self._cli(project, "sync")
        self.assertEqual(first.returncode, 0, first.stderr)
        first_text = (project / "AGENTS.md").read_text()
        second = self._cli(project, "sync")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first_text, (project / "AGENTS.md").read_text())
        self.assertEqual(first_text.count("Shared rule."), 1)


# ---------------------------------------------------------------------------
# VcsFragmentIsolationTests
# ---------------------------------------------------------------------------

class VcsFragmentIsolationTests(CliBriefTestBase):
    RECIPES = {
        "my-custom-vcs": _mini_recipe(
            "my-custom-vcs",
            capabilities=["vcs-pr-flow"],
            config={"base_branch": "trunk"},
            fragments=["Use custom VCS flow."],
        ),
    }
    REAL_RECIPES = (
        "git-pr-flow",
        "gitlab-mr-flow",
        "bitbucket-pr-flow",
        "worktree-flow",
    )

    def _sync(self, recipes: list[str], bindings: dict[str, str] | None = None):
        manifest = "[project]\nname = 'demo'\n\n" + _recipes_block(recipes)
        if bindings:
            manifest += "\n" + _bindings_block(bindings)
        project = self._project(manifest)
        result = self._cli(project, "sync")
        return result, (project / "AGENTS.md").read_text()

    def test_bound_gitlab_only_gitlab_fragments_in_workflow_rules(self):
        result, text = self._sync(
            ["git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow", "worktree-flow"],
            {"vcs-pr-flow": "gitlab-mr-flow"},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Use glab for all MR operations.", section)
        self.assertNotIn("Use gh for all PR operations.", section)
        self.assertNotIn("Use bb for all PR operations.", section)
        self.assertIn("Create a dedicated worktree", section)

    def test_no_vcs_binding_no_vcs_fragments(self):
        result, text = self._sync(
            ["git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow", "worktree-flow"]
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertNotIn("Use gh for all PR operations.", section)
        self.assertNotIn("Use glab for all MR operations.", section)
        self.assertNotIn("Use bb for all PR operations.", section)
        self.assertIn("Create a dedicated worktree", section)

    def test_bound_custom_recipe_contributes_own_fragments(self):
        result, text = self._sync(
            ["my-custom-vcs", "git-pr-flow", "worktree-flow"],
            {"vcs-pr-flow": "my-custom-vcs"},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Workflow Rules")
        self.assertIn("Use custom VCS flow.", section)
        self.assertNotIn("Use gh for all PR operations.", section)
        self.assertIn("Create a dedicated worktree", section)


# ---------------------------------------------------------------------------
# RepoTopologyBriefTests
# ---------------------------------------------------------------------------

class RepoTopologyBriefTests(CliBriefTestBase):
    REAL_RECIPES = ("worktree-flow",)

    def _topology_project(self, *, enabled: bool) -> Path:
        td = tempfile.TemporaryDirectory(prefix="bb-topo-")
        self.addCleanup(td.cleanup)
        base = Path(td.name)
        origin = base / "sub-origin.git"
        origin.mkdir()
        _git(origin, "init", "--bare", "-q")
        super_dir = base / "super"
        super_dir.mkdir()
        _git(super_dir, "init", "-q", "-b", "main")
        sub_dir = super_dir / "sub"
        sub_dir.mkdir()
        _git(sub_dir, "init", "-q", "-b", "main")
        (sub_dir / "README.md").write_text("sub\n")
        _git(sub_dir, "add", "-A")
        _git(sub_dir, "commit", "-qm", "init sub")
        _git(
            super_dir,
            "-c", "protocol.file.allow=always",
            "submodule", "add", "-q", str(origin), "sub",
        )
        _git(super_dir, "commit", "-qm", "add submodule")
        (super_dir / "ai-specs").mkdir(parents=True)
        state = "true" if enabled else "false"
        (super_dir / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'topo'\n\n"
            "[recipes.worktree-flow]\nenabled = " + state + "\n"
        )
        return super_dir

    def test_repo_topology_line_in_project_section(self):
        project = self._topology_project(enabled=True)
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        text = (project / "AGENTS.md").read_text()
        section = _section(text, "Project")
        self.assertIn("- **Repo topology**: `monorepo-submodules` (via auto)", section)

    def test_repo_topology_omitted_when_worktree_flow_disabled(self):
        project = self._topology_project(enabled=False)
        result = self._cli(project, "sync")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("Repo topology", (project / "AGENTS.md").read_text())


if __name__ == "__main__":
    unittest.main()

"""Validation + materialization tests for the plan-build-flow catalog recipe.

Converted (see openspec/changes/blackbox-test-conversion) to drive the CLI
through `bin/ai-specs sync` and to assert recipe content through the emitted
file tree (materialized skill/doc/AGENTS.md) and the raw recipe.toml text —
never by importing lib/_internal modules.
"""
from __future__ import annotations

import re
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "plan-build-flow"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home  # noqa: E402
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir  # noqa: E402
from _fixture_catalog import populate_catalog  # noqa: E402

FORBIDDEN_TERMS = ("sdd", "spec-driven")
FORBIDDEN_SLASH = ("/plan", "/build")
STORE_ENUM = ["openspec", "engram", "both"]


def _without_store_config_table(raw: str) -> str:
    """Remove exactly the store table, leaving all other recipe prose guarded."""
    pattern = r"(?ms)^\[config\.artifact_store_default\]\n.*?(?=^\[|\Z)"
    stripped, count = re.subn(pattern, "", raw, count=1)
    assert count == 1, "store config table must be present exactly once"
    return stripped


def _without_delivery_contracts_section(raw: str) -> str:
    """Remove exactly one README section through the next same-level heading."""
    pattern = r"(?ms)^## Delivery contracts\n.*?(?=^## |\Z)"
    stripped, count = re.subn(pattern, "", raw, count=1)
    assert count == 1, "README delivery contracts section must be present exactly once"
    return stripped


def _recipe_surface_text(recipe_dir: Path) -> str:
    recipe = _without_store_config_table((recipe_dir / "recipe.toml").read_text())
    readme = _without_delivery_contracts_section((recipe_dir / "README.md").read_text())
    skill = (recipe_dir / "skills" / RECIPE_ID / "SKILL.md").read_text()
    return "\n".join((recipe, readme, skill)).lower()


def _recipe_toml() -> str:
    return (CATALOG / RECIPE_ID / "recipe.toml").read_text()


def _workflow_rules_raw() -> list[str]:
    """The [provides.brief] workflow_rules as an ordered list (raw file text)."""
    raw = _recipe_toml()
    block = raw.split("workflow_rules = [", 1)[1].split("\n]", 1)[0]
    rules = []
    for line in block.splitlines():
        stripped = line.strip()
        if stripped.startswith('"'):
            rules.append(stripped.rstrip(",").strip('"'))
    return rules


def _version_of(recipe_id: str) -> str:
    text = (CATALOG / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, f"could not find version in {recipe_id}/recipe.toml"
    return match.group(1)


def _recipe_version() -> str:
    return _version_of(RECIPE_ID)


class PlanBuildFlowRecipeTests(unittest.TestCase):

    def _cli_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        catalog = home / "catalog"
        catalog.unlink()
        recipes_dir = catalog / "recipes"
        recipes_dir.mkdir(parents=True)
        populate_catalog(recipes_dir, include_fixtures=False)
        return home

    def _make_project(self, extra: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{RECIPE_ID}]\nenabled = true\n"
            + extra
        )
        return root

    def _sync(self, root: Path, home: Path):
        return invoke(root, "sync", cli_home=home)

    def _sync_agents(self, root: Path, home: Path) -> str:
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        return (root / "AGENTS.md").read_text()

    # --- Materialization + schema content through the CLI -----------------

    def test_recipe_materializes_skill_only(self):
        raw = _recipe_toml()
        self.assertIn('id = "plan-build-flow"', raw)
        self.assertIn('skills = [{ id = "plan-build-flow", source = "bundled" }]', raw)
        self.assertNotIn("commands", raw)
        home = self._cli_home()
        root = self._make_project()
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill = recipe_root(root, RECIPE_ID, cli_home=home) / "skills" / RECIPE_ID / "SKILL.md"
        self.assertTrue(skill.is_file())
        for forbidden in ("plan.md", "build.md", "archive.md"):
            self.assertFalse(
                (root / "ai-specs" / "commands" / forbidden).exists(),
                f"unexpected command {forbidden}",
            )

    def test_recipe_declares_exact_store_schema_and_hook_pair(self):
        raw = _recipe_toml()
        self.assertEqual(raw.count("[config.artifact_store_default]"), 1)
        self.assertIn("required = false", raw)
        self.assertIn('type = "string"', raw)
        self.assertIn('default = "openspec"', raw)
        self.assertIn('enum = ["openspec", "engram", "both"]', raw)
        self.assertIn("persistence preference", raw)
        self.assertIn("readiness", raw)
        self.assertIn('event = "on-sync"', raw)
        self.assertIn('action = "validate-config"', raw)
    def test_recipe_brief_rules_preserve_store_and_add_phase_guidance(self):
        rules = _workflow_rules_raw()
        self.assertEqual(len(rules), 11)
        self.assertTrue(all(not rule.startswith("{") and "=" not in rule[:12] for rule in rules))
        self.assertEqual(
            rules[:5],
            [
                "Classify each substantial change (full planning chain, spec+tasks, or tasks-only) before writing production code; compute the signal depth, compare any explicit requested depth, ask on conflicts, and annotate requested/signal/decided depth in tasks.md before authorization.",
                "Direct implementation requests without a change folder still require planning at the classified depth; approval verbs do not skip the plan step.",
                "Do not open a PR until the change folder on the branch contains the tier minimum planning files (Light: proposal.md + tasks.md; Standard: proposal.md + tasks.md + specs/**/*.md; Full: tasks.md plus proposal.md or design.md plus specs/**/*.md), committed.",
                "After authorization, implement and validate in the change worktree when isolated worktrees are enabled.",
                "Before merge, run verify evidence before archive-tail (Standard/Full block without a conforming verify-report.md; Light is advisory), archive the change folder on the review branch at openspec/changes/archive/YYYY-MM-DD-<slug>/ using a valid ISO calendar date, and run the pre-merge guardian again; exact undated archive/<slug>/ is legacy fallback only, ambiguity and malformed or near-match candidates block, and archive is never deferred until after merge.",
            ],
        )
        self.assertIn("{config.artifact_store_default}", rules[5])
        self.assertEqual(rules[5].count("{config.artifact_store_default}"), 1)
        self.assertIn("topology", rules[6])
        self.assertIn("superproject", rules[6])
        self.assertIn("Full planning", rules[7])
        self.assertIn("inline", rules[7])
        self.assertIn("malformed", rules[8])
        self.assertIn("Standard and Light", rules[8])
        self.assertIn("session-level preflight", rules[9])
        self.assertIn("artifact-derived", rules[10])

    def test_brief_describes_adversarial_depth_conflicts(self):
        rules = _workflow_rules_raw()
        combined = "\n".join(rules).lower()
        self.assertIn("compare any explicit requested depth", combined)
        self.assertIn("ask on conflicts", combined)
        self.assertIn("annotate requested/signal/decided depth", combined)
        self.assertIn("{config.artifact_store_default}", rules[5])
        self.assertEqual(rules[5].count("{config.artifact_store_default}"), 1)

    def test_brief_and_readme_vocabulary_clean(self):
        rules = _workflow_rules_raw()
        self.assertGreater(len(rules), 0)
        fragments = "\n".join(rules).lower()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, fragments)
        for slash in FORBIDDEN_SLASH:
            self.assertNotIn(slash, fragments)
        home = self._cli_home()
        root = self._make_project()
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        readme = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(readme.is_file())
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, _without_delivery_contracts_section(readme.read_text()).lower())

    def test_store_defaults_override_and_enum_rejection(self):
        raw = _recipe_toml()
        self.assertIn('default = "openspec"', raw)
        home = self._cli_home()
        root = self._make_project(
            "\n[recipes.plan-build-flow.config]\nartifact_store_default = 'both'\n"
        )
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        agents = (root / "AGENTS.md").read_text()
        self.assertIn("`both`", agents)
        root2 = self._make_project(
            "\n[recipes.plan-build-flow.config]\nartifact_store_default = 'vault'\n"
        )
        result2 = self._sync(root2, home)
        self.assertNotEqual(result2.returncode, 0)
        self.assertIn("artifact_store_default", result2.stderr)
        self.assertIn("openspec | engram | both", result2.stderr)

    def test_recipe_surface_excludes_session_controls_and_removed_contract(self):
        raw = _recipe_toml()
        config_block = raw.split("[config.artifact_store_default]", 1)[1].split("[provides]", 1)[0]
        self.assertNotIn("chained_" + "pr_default", config_block)
        self.assertNotIn("mode", config_block.lower())
        surface = _recipe_surface_text(CATALOG / RECIPE_ID)
        removed_root = "bud" + "get"
        removed_key = "review_" + removed_root
        self.assertNotIn(removed_key, surface)
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text()
        self.assertNotRegex(skill, r"(?im)^#{1,6}\s*7\.5\b")
        self.assertNotRegex(skill, r"(?im)^#{1,6}\s*Review workload budget\b")
        self.assertNotRegex(skill, r"(?im)^\s*WARN:\s*review budget\b")
        gate = (CATALOG / RECIPE_ID / "hooks" / "plan-build-gate.sh").read_text().lower()
        self.assertNotIn(removed_root, gate)
        self.assertNotIn("forecast", gate)
        external_terms = ("gentle-" + "ai", "gentle-" + "pi")
        catalog_section = (ROOT / "docs" / "recipes-catalog.md").read_text().lower()
        for term in external_terms:
            self.assertNotIn(term, surface)
            self.assertNotIn(term, catalog_section)

    def test_materialization_renders_manifest_store_override_into_agents(self):
        home = self._cli_home()
        root = self._make_project(
            "\n[recipes.plan-build-flow.config]\nartifact_store_default = 'both'\n"
        )
        content = self._sync_agents(root, home)
        self.assertIn("Default artifact store", content)
        self.assertIn("`both`", content)
        self.assertNotIn("{config.artifact_store_default}", content)
        self.assertLess(content.index("Classify each substantial change"), content.index("Default artifact store"))

    def test_materialization_renders_default_store_into_agents(self):
        home = self._cli_home()
        root = self._make_project()
        content = self._sync_agents(root, home)
        self.assertIn("`openspec`", content)
        self.assertNotIn("{config.artifact_store_default}", content)

    def test_validate_config_hook_accepts_each_store_enum(self):
        home = self._cli_home()
        for value in STORE_ENUM:
            with self.subTest(value=value):
                root = self._make_project(
                    f"\n[recipes.plan-build-flow.config]\nartifact_store_default = '{value}'\n"
                )
                result = self._sync(root, home)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_config_help_text_states_persistence_preference(self):
        raw = _recipe_toml()
        help_line = [ln for ln in raw.splitlines() if ln.strip().startswith("help_text")][0]
        self.assertIn("persistence preference", help_line)
        self.assertIn("readiness", help_line)

    def test_brief_rule_six_states_persistence_preference_and_readiness_invariant(self):
        rules = _workflow_rules_raw()
        rule6 = rules[5]
        self.assertEqual(rule6.count("{config.artifact_store_default}"), 1)
        self.assertIn("persistence preference", rule6)
        self.assertIn("readiness", rule6)
        self.assertIn("file-backed", rule6)
        self.assertIn("never", rule6)

    def test_cross_repo_artifact_scope_recipe_contract(self):
        readme = (CATALOG / RECIPE_ID / "README.md").read_text().lower()
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text().lower()
        surface = "\n".join((readme, skill, catalog))
        for text in (readme, skill, catalog):
            self.assertIn("topology", text)
            self.assertIn("central", text)
            self.assertIn("superproject", text)
        for text in (skill, catalog):
            self.assertIn("openspec/changes", text)
        self.assertIn("standalone", surface)
        self.assertIn("fail-safe", surface)
        self.assertIn("no duplication", surface)
        self.assertIn("no orchestration", surface)
        for forbidden in ("[sdd]", "decision matrix", "artifact_root", "per-subrepository"):
            self.assertNotIn(forbidden, surface)
        raw = _recipe_toml()
        self.assertIn('id = "plan-build-gate"', raw)
        self.assertIn('event = "pre-tool-use"', raw)
        self.assertIn('matcher = "Edit|Write|MultiEdit|NotebookEdit"', raw)
        self.assertIn("blocking = true", raw)
        self.assertIn('event = "on-sync"', raw)
        self.assertIn('action = "validate-config"', raw)

    def test_materialization_preserves_cross_repo_guidance(self):
        home = self._cli_home()
        root = self._make_project()
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        generated_readme = (
            root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        ).read_text().lower()
        generated_skill = (
            recipe_root(root, RECIPE_ID, cli_home=home) / "skills" / RECIPE_ID / "SKILL.md"
        ).read_text().lower()
        for text in (generated_readme, generated_skill):
            self.assertIn("topology", text)
            self.assertIn("central", text)
            self.assertIn("superproject", text)
            self.assertIn("standalone", text)
            self.assertIn("fail-safe", text)
        self.assertIn("openspec/changes", generated_skill)
        self.assertIn("no duplication", generated_skill)
        self.assertIn("orchestration", generated_skill)

    def test_implementation_brief_references_worktree_flow(self):
        rules = _workflow_rules_raw()
        combined = "\n".join(rules).lower()
        self.assertIn("worktree", combined)
        self.assertNotIn("/build", combined)
        raw = _recipe_toml()
        self.assertNotIn("conflicts_with", raw)

    def test_classic_sdd_commands_unchanged(self):
        home = self._cli_home()
        root = self._make_project()
        ai_specs = root / "ai-specs"
        commands = ai_specs / "commands"
        legacy = commands / "legacy-sdd-cmd.md"
        legacy.write_text("# Legacy\n")
        local_skill = ai_specs / "skills" / "legacy-sdd-skill"
        local_skill.mkdir()
        (local_skill / "SKILL.md").write_text("---\nname: legacy\n---\n")
        before = legacy.read_text()
        skill_before = (local_skill / "SKILL.md").read_text()
        result = self._sync(root, home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(legacy.read_text(), before)
        self.assertEqual((local_skill / "SKILL.md").read_text(), skill_before)

    def test_recipe_does_not_stage_premerge_guardian_into_project(self):
        raw = _recipe_toml()
        self.assertNotIn("provides.templates", raw)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", raw)
        self.assertTrue(
            (ROOT / "lib" / "_internal" / "premerge_guardian.py").is_file()
        )

    def test_brief_mentions_depth_and_pr_gate(self):
        rules = _workflow_rules_raw()
        combined = "\n".join(rules).lower()
        self.assertIn("classify", combined)
        self.assertIn("tasks-only", combined)
        self.assertIn("do not open a pr", combined)
        self.assertIn("before merge", combined)

    def test_phase_contract_stays_out_of_recipe_config_and_named_vocabulary(self):
        rules = _workflow_rules_raw()
        raw = _recipe_toml()
        self.assertEqual(raw.count("[config.artifact_store_default]"), 1)
        surface = _recipe_surface_text(CATALOG / RECIPE_ID)
        for term in ("gentle-ai", "gentle ai"):
            self.assertNotIn(term, _recipe_surface_text(CATALOG / RECIPE_ID).lower())
        for term in ("gentle-ai", "gentle ai"):
            self.assertNotIn(term, "\n".join(rules).lower())

    # --- Catalog content tests (read-only file assertions) ----------------

    def test_skill_documents_minima_explore_and_staged_verify_modes(self):
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        for marker in (
            "proposal.md", "explore.md", "multi-approach", "unknown surface",
            "advisory", "enforcement", "required", "verify-report.md",
            "pre-archive", "ready_for_archive: true", "grandfather",
        ):
            self.assertIn(marker, skill)

    def test_readme_and_catalog_pin_recipe_1_7_0(self):
        readme = (CATALOG / RECIPE_ID / "README.md").read_text()
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text()
        self.assertIn('version = "1.7.0"', readme)
        self.assertIn('version = "1.7.0"', catalog)
        changelog = (ROOT / "CHANGELOG.md").read_text()
        unreleased = changelog.split("## [0.21.0]", 1)[0]
        self.assertIn("1.6.0` → `1.7.0", unreleased)
        self.assertIn("1.5.0` → `1.6.0", unreleased)

    def test_skill_describes_adversarial_depth_conflicts(self):
        skill = CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md"
        text = skill.read_text().lower()
        for phrase in (
            "depth conflict",
            "requested depth",
            "signal depth",
            "decision source: user",
            "same-turn",
            "both directions",
            "illustrative",
            "exhaustive parser",
            "fixed token whitelist",
        ):
            self.assertIn(phrase, text)

    def test_skill_preserves_standalone_depth_annotation_contract(self):
        skill = CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md"
        text = skill.read_text()
        self.assertRegex(text, r"(?m)^Depth: full$")
        for label in (
            "Requested depth: full",
            "Signal depth: standard",
            "Decided depth: full",
            "Decision source: user",
        ):
            self.assertIn(label, text)
        self.assertNotRegex(text, r"(?m)^Depth: (?:light|standard|full) \(")

    def test_skill_has_ambient_auto_invoke(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text()
        self.assertIn("auto_invoke:", text)
        self.assertIn("substantial", text.lower())
        self.assertNotIn("/plan", text.split("auto_invoke")[0])

    def test_skill_separates_persistence_from_readiness(self):
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        for phrase in (
            "persistence preference",
            "readiness",
            "file-backed",
            "mirror",
            "memory-only",
            "never replaces",
            "tier minimum",
        ):
            self.assertIn(phrase, skill)

    def test_readme_delivery_contracts_state_file_backed_readiness(self):
        readme = (CATALOG / RECIPE_ID / "README.md").read_text().lower()
        for phrase in (
            "persistence preference",
            "file-backed",
            "openspec/changes/<slug>/",
            "memory-only",
            "mirror",
        ):
            self.assertIn(phrase, readme)

    def test_catalog_documents_store_preference_and_readiness_invariant(self):
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text().lower()
        for phrase in (
            "persistence preference",
            "file-backed",
            "openspec/changes/<slug>/",
        ):
            self.assertIn(phrase, catalog)

    def test_version_and_catalog_documentation_use_current_contract(self):
        self.assertEqual(_recipe_version(), "1.7.0")
        readme = (CATALOG / RECIPE_ID / "README.md").read_text()
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text()
        for text in (readme, catalog):
            self.assertIn("artifact_store_default", text)
            self.assertIn("openspec", text)
            self.assertIn("engram", text)
            self.assertIn("both", text)
            self.assertIn("1.7.0", text)

    def test_success_criteria_source_selection_contract_is_documented(self):
        recipe_dir = CATALOG / RECIPE_ID
        readme = (recipe_dir / "README.md").read_text()
        skill = (recipe_dir / "skills" / RECIPE_ID / "SKILL.md").read_text()
        self.assertIn("authoritative source", readme)
        self.assertIn("when present, otherwise `design.md`", readme)
        self.assertIn("does not fall back to `design.md`", skill)
        self.assertIn("Duplicate `## Success Criteria` headings", skill)

    def test_skill_has_change_depth_classifier(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text().lower()
        self.assertIn("change depth classifier", text)
        for tier in ("full", "standard", "light"):
            self.assertIn(tier, text)

    def test_skill_has_pr_and_archive_gates(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        raw = skill.read_text()
        text = raw.lower()
        self.assertIn("pr creation gate", text)
        self.assertIn("pre-merge archive gate", text)
        self.assertIn("pre-merge merge guardian", text)
        self.assertIn("premerge_guardian", text)
        self.assertIn("AI_SPECS_HOME", raw)
        self.assertIn("lib/_internal/premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)
        self.assertIn("gh pr create", text)
        self.assertIn("before merge", text)
        self.assertIn("--stage pre-archive", text)
        self.assertIn("before moving", text)

    def test_full_phase_contract_maps_dependencies_and_fallbacks(self):
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        normalized = " ".join(skill.split())
        for phrase in (
            "explore -> proposal -> spec/design -> tasks",
            "`explore.md`",
            "`proposal.md`",
            "`specs/**/*.md`",
            "`design.md`",
            "`tasks.md`",
            "spec and design may run in parallel only after proposal",
            "tasks waits for both outputs",
            "host-advertised executor",
            "provider-neutral",
            "current phase inline",
            "inline fallback",
            "malformed",
            "partial",
            "blocked",
            "stop and preserve",
            "incomplete artifact",
            "do not skip phases",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, normalized)

    def test_preflight_and_presentation_contracts_are_composed(self):
        recipe_dir = CATALOG / RECIPE_ID
        skill = (recipe_dir / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        readme = (recipe_dir / "README.md").read_text().lower()
        combined = " ".join("\n".join((skill, readme)).split())
        for phrase in (
            "one session-level authority",
            "execution mode",
            "artifact store",
            "review budget",
            "delivery strategy",
            "chain strategy",
            "must never recollect",
            "must never recollect or override them",
            "intent",
            "scope",
            "key decisions",
            "affected areas",
            "risks",
            "open questions",
            "recommendations",
            "accept",
            "adjust",
            "stop",
            "labeled assumptions",
            "unresolved product decisions block",
        ):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, combined)

    def test_standard_and_light_remain_collapsed(self):
        skill = (CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md").read_text().lower()
        skill = " ".join(skill.split())
        self.assertIn("standard", skill)
        self.assertIn("light", skill)
        self.assertIn("remain collapsed", skill)
        self.assertNotIn("standard planning runs explore", skill)
        self.assertNotIn("light planning runs explore", skill)


if __name__ == "__main__":
    unittest.main()
"""Tests for runtime-brief-baseline change.

Covers:
  - Unit: template default enables session-context (TemplateDefaultTests)
  - E2E: fresh init produces behavioral brief (InitBriefE2ETests)
  - E2E: render failure → placeholder fallback, init exits 0
  - E2E: init→sync byte-stability
  - E2E: --preserve-if-runtime-brief marker preserved under --force
  - E2E: no this-repo tokens in baseline AGENTS.md
  - Unit: W1 — dedupe with session-context + second recipe sharing a key (SessionContextDedupTests)
  - E2E: W2 — sync-side marker preservation after user adds marker post-init
  - E2E: optional — no unrendered {config.} or {{ placeholders in baseline brief

All offline: catalog read from AI_SPECS_HOME; session-context skills are bundled.
"""
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
AGENTS_RENDER_PATH = ROOT / "lib" / "_internal" / "agents-render.py"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
TEMPLATE_PATH = ROOT / "templates" / "ai-specs.toml.tmpl"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TemplateDefaultTests(unittest.TestCase):
    """Unit tests: the default TOML template pre-enables session-context."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_baseline_unit")

    def _make_project_from_template(self) -> Path:
        """Render ai-specs.toml.tmpl into a fresh temp project directory."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()

        # Mimic what init.sh does: sed replace {{PROJECT_NAME}} and write toml
        template_text = TEMPLATE_PATH.read_text()
        toml_text = template_text.replace("{{PROJECT_NAME}}", "test-proj")
        (ai_specs / "ai-specs.toml").write_text(toml_text)

        return root

    def test_template_default_enables_session_context(self):
        """build_resolved_config on the default template yields session-context in enabled."""
        root = self._make_project_from_template()
        result = self.mod.build_resolved_config(root)
        self.assertIn(
            "session-context",
            result["enabled"],
            f"Expected 'session-context' in enabled list. Got: {result['enabled']!r}",
        )

    def test_template_default_no_project_specific_tokens(self):
        """Resolved config from the default template must not contain this-repo tokens."""
        root = self._make_project_from_template()
        result = self.mod.build_resolved_config(root)
        serialized = json.dumps(result)

        # These are tokens from the ai-specs-cli dogfood project; they must not
        # appear in a generic project's baseline config.
        forbidden_tokens = [
            "69ec097f13e2d38ecd89a557",   # board id
            "nnodes/proyectos",             # vault scope
            "ai-specs-cli",                 # project name
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                serialized,
                f"Found project-specific token {token!r} in resolved config output.",
            )


# ---------------------------------------------------------------------------
# E2E: fresh init produces behavioral brief
# ---------------------------------------------------------------------------

class InitBriefE2ETests(unittest.TestCase):
    """E2E tests for the init → AGENTS.md rendering pipeline."""

    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_fresh_init_produces_behavioral_brief(self):
        """After init, AGENTS.md must contain the session-context behavioral sections."""
        target = self._make_target()
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")

        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md was not created")
        content = agents_md.read_text()

        # Must contain Workflow Rules section
        self.assertIn(
            "## Workflow Rules",
            content,
            "AGENTS.md must contain '## Workflow Rules' section",
        )
        # Must have at least one session-context workflow_rules bullet
        self.assertIn(
            "A session works on one explicit user request",
            content,
            "AGENTS.md must contain session-context workflow_rules fragment",
        )
        # Must contain Conflict Policy section
        self.assertIn(
            "## Conflict Policy",
            content,
            "AGENTS.md must contain '## Conflict Policy' section",
        )
        # Must have at least two conflict_policy bullets
        conflict_count = content.count("- ")
        # Count bullets specifically in the Conflict Policy section
        cp_start = content.find("## Conflict Policy")
        self.assertGreater(cp_start, -1, "## Conflict Policy section must exist")
        # Find the next ## heading after Conflict Policy
        tail = content[cp_start:]
        next_heading = tail.find("\n## ", 4)  # skip past the ## Conflict Policy line itself
        if next_heading > 0:
            cp_section = tail[:next_heading]
        else:
            cp_section = tail
        bullet_count = cp_section.count("\n- ")
        self.assertGreaterEqual(
            bullet_count, 2,
            f"## Conflict Policy must have at least 2 bullets, found {bullet_count}:\n{cp_section}",
        )

    def test_init_render_failure_falls_back_to_placeholder(self):
        """When the render scripts fail, init must still create AGENTS.md and exit 0.

        Uses a fake python3 that delegates to the real python3 for all scripts
        EXCEPT recipe-materialize.py and agents-render.py, which it makes exit 1.
        This simulates a render-pipeline failure without breaking the rest of init
        (gitignore-render.py, refresh-bundled.py, etc. still run via real python3).
        """
        target = self._make_target()

        # Find the real python3
        import shutil as _shutil
        real_python3 = _shutil.which("python3")
        if not real_python3:
            self.skipTest("python3 not found on PATH")

        # Create a selective fake python3 that fails only for render scripts
        fake_bin = Path(target.parent) / "fake-bin"
        fake_bin.mkdir()
        fake_python = fake_bin / "python3"
        fake_python.write_text(
            "#!/bin/sh\n"
            "# Fail only for the render pipeline scripts; pass through for others.\n"
            "case \"$*\" in\n"
            "  *recipe-materialize*|*agents-render*) exit 1 ;;\n"
            f"  *) exec \"{real_python3}\" \"$@\" ;;\n"
            "esac\n"
        )
        fake_python.chmod(0o755)

        # Build a PATH that puts fake-bin FIRST
        original_path = os.environ.get("PATH", "")
        patched_path = f"{fake_bin}:{original_path}"

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            env={**os.environ, "PATH": patched_path},
            text=True,
            capture_output=True,
            check=False,
        )

        # init MUST exit 0 even if the render pipeline fails
        self.assertEqual(
            result.returncode, 0,
            f"init must exit 0 on render failure; got {result.returncode}\nstderr: {result.stderr}",
        )

        # AGENTS.md must still exist (fallback placeholder)
        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md must still be created on render failure")
        content = agents_md.read_text()
        self.assertTrue(len(content) > 0, "AGENTS.md must be non-empty (placeholder)")

        # stderr must mention the skip/fallback
        self.assertIn(
            "render skipped",
            result.stderr,
            f"stderr must mention render skip; got:\n{result.stderr}",
        )

    def test_init_then_sync_is_byte_stable(self):
        """Running sync after init must produce byte-identical AGENTS.md."""
        target = self._make_target()

        subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            check=True,
        )

        agents_md = target / "AGENTS.md"
        after_init = agents_md.read_bytes()

        subprocess.run(
            [str(CLI), "sync", str(target)],
            text=True,
            check=True,
        )

        after_sync = agents_md.read_bytes()
        self.assertEqual(
            after_init,
            after_sync,
            "AGENTS.md must be byte-identical after init and after sync",
        )

    def test_force_init_preserves_runtime_brief_marker(self):
        """If AGENTS.md contains <!-- ai-specs:runtime-brief -->, --force must not overwrite it."""
        target = self._make_target()

        # First init to bootstrap the directory
        subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            check=True,
        )

        # Write the user-managed marker into AGENTS.md
        agents_md = target / "AGENTS.md"
        original_content = "# Manual Brief\n<!-- ai-specs:runtime-brief -->\n\nCustom content.\n"
        agents_md.write_text(original_content)

        # --force init: must preserve the file because the marker is present
        result = subprocess.run(
            [str(CLI), "init", str(target), "--force"],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(
            result.returncode, 0,
            f"--force init must exit 0; stderr:\n{result.stderr}",
        )

        final_content = agents_md.read_text()
        self.assertIn(
            "<!-- ai-specs:runtime-brief -->",
            final_content,
            "The runtime-brief marker must be preserved after --force init",
        )
        # The file must not have been overwritten (custom content preserved)
        self.assertIn(
            "Custom content.",
            final_content,
            "User custom content must be preserved when marker is present",
        )

    def test_no_project_specific_tokens_in_baseline_agents_md(self):
        """A fresh default init must not leak any this-repo tokens into AGENTS.md."""
        target = self._make_target()

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")

        agents_md = target / "AGENTS.md"
        content = agents_md.read_text()

        forbidden_tokens = [
            "69ec097f13e2d38ecd89a557",   # dogfood board id
            "nnodes/proyectos",             # dogfood vault scope
            "ai-specs-cli",                 # dogfood project name
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                content,
                f"Found project-specific token {token!r} in baseline AGENTS.md",
            )


# ---------------------------------------------------------------------------
# W1 — Fragment dedupe with session-context + second concrete recipe
# ---------------------------------------------------------------------------

class SessionContextDedupTests(unittest.TestCase):
    """W1: Dedupe when session-context and a second recipe share the same key.

    Uses collect_recipe_brief_fragments directly (same harness as
    CollectRecipeBriefFragmentsTests in test_agents_render_brief_fragments.py)
    with a session-context-shaped resolved config alongside a second recipe
    that also contributes key='conflict-policy-source-authority'.

    Asserts:
    - The keyed bullet appears exactly once (first-wins).
    - session-context wins over the second recipe (ordering preserved).
    - The session-context workflow_rules fragment is present and also deduplicated.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(AGENTS_RENDER_PATH, "agents_render_session_context_dedup")

    def _session_context_conflict_policy_frags(self):
        """Fragment list matching catalog/recipes/session-context/recipe.toml [provides.brief]."""
        return [
            {
                "key": "conflict-policy-source-authority",
                "text": (
                    "Current explicit human instruction controls the immediate scope "
                    "unless it conflicts with safety, secrets, or a higher-authority project rule."
                ),
            },
            {
                "key": "conflict-policy-source-hierarchy",
                "text": (
                    "Tracker controls work state; vault controls canonical decisions and handoffs; "
                    "repo docs and manifests control versioned project contracts. "
                    "Agent plans are lowest authority until accepted and recorded."
                ),
            },
        ]

    def _resolved(self, enabled, recipes):
        return {"enabled": enabled, "recipes": recipes}

    def test_session_context_key_wins_over_second_recipe(self):
        """W1 core: session-context and a second recipe both provide
        key='conflict-policy-source-authority'. collect_recipe_brief_fragments must
        return the bullet exactly once with session-context's wording (first-wins)."""
        resolved = self._resolved(
            ["session-context", "recipe-extra"],
            {
                "session-context": {
                    "brief_fragments": {
                        "conflict_policy": self._session_context_conflict_policy_frags()
                    }
                },
                "recipe-extra": {
                    "brief_fragments": {
                        "conflict_policy": [
                            {
                                "key": "conflict-policy-source-authority",
                                "text": "Extra recipe override — MUST NOT appear.",
                            }
                        ]
                    }
                },
            },
        )
        result = self.mod.collect_recipe_brief_fragments(resolved, "conflict_policy")

        # Must have exactly 2 entries: the two session-context keys (not a third from recipe-extra)
        self.assertEqual(
            len(result),
            2,
            f"Expected 2 unique keyed bullets, got {len(result)}: {[r['text'] for r in result]}",
        )

        texts = [r["text"] for r in result]
        # session-context wording wins — check as substring of any text entry
        self.assertTrue(
            any("Current explicit human instruction controls the immediate scope" in t for t in texts),
            f"session-context source-authority bullet must be present in texts: {texts}",
        )
        # second recipe duplicate must be suppressed
        for t in texts:
            self.assertNotIn(
                "MUST NOT appear",
                t,
                "recipe-extra override must be suppressed by first-wins key dedup",
            )

    def test_session_context_key_dedup_appears_exactly_once_in_full_render(self):
        """W1 end-to-end: full render() with session-context + second recipe sharing key.
        The conflict_policy bullet must appear exactly once in the rendered AGENTS.md."""
        toml = (
            "[project]\nname = 'dedup-fixture'\n\n"
            "[brief]\n"
            "intro = 'Dedup fixture project.'\n"
            "purpose = 'Testing fragment dedup with session-context.'\n"
        )
        session_context_bullet = (
            "Current explicit human instruction controls the immediate scope "
            "unless it conflicts with safety, secrets, or a higher-authority project rule."
        )
        resolved = {
            "enabled": ["session-context", "recipe-extra"],
            "recipes": {
                "session-context": {
                    "brief_fragments": {
                        "conflict_policy": self._session_context_conflict_policy_frags(),
                        "workflow_rules": [
                            {
                                "key": None,
                                "text": (
                                    "A session works on one explicit user request or tracker card; "
                                    "resolve focus from memory and tracker before starting."
                                ),
                            }
                        ],
                    }
                },
                "recipe-extra": {
                    "brief_fragments": {
                        "conflict_policy": [
                            {
                                "key": "conflict-policy-source-authority",
                                "text": "Extra recipe override — MUST NOT appear.",
                            }
                        ],
                        "workflow_rules": [
                            {
                                "key": None,
                                "text": (
                                    "A session works on one explicit user request or tracker card; "
                                    "resolve focus from memory and tracker before starting."
                                ),
                            }
                        ],
                    }
                },
            },
            "bindings": {},
        }

        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            resolved_path = tmp_path / "resolved-config.json"
            toml_path.write_text(toml)
            resolved_path.write_text(json.dumps(resolved))
            self.mod.render(
                toml_path,
                output_path,
                preserve_if_marker=False,
                resolved_config_path=resolved_path,
            )
            content = output_path.read_text()

        # Key-dedup: session-context wording appears exactly once
        self.assertEqual(
            content.count(session_context_bullet),
            1,
            f"session-context source-authority bullet must appear exactly once.\nContent:\n{content}",
        )
        # Override from second recipe must not appear at all
        self.assertNotIn(
            "MUST NOT appear",
            content,
            "recipe-extra duplicate bullet must be suppressed by key dedup",
        )
        # Exact-string dedup: shared workflow_rules text appears exactly once
        session_wf = (
            "A session works on one explicit user request or tracker card; "
            "resolve focus from memory and tracker before starting."
        )
        self.assertEqual(
            content.count(session_wf),
            1,
            f"Shared workflow_rules bullet must appear exactly once (exact-string dedup).\nContent:\n{content}",
        )


# ---------------------------------------------------------------------------
# W2 — Sync-side marker preservation
# ---------------------------------------------------------------------------

class SyncMarkerPreservationTests(unittest.TestCase):
    """W2: Sync honors --preserve-if-runtime-brief on the sync path.

    Scenario:
    1. init a fresh project (AGENTS.md written without user marker).
    2. User edits AGENTS.md to add <!-- ai-specs:runtime-brief --> and custom content.
    3. Run sync.
    4. Assert AGENTS.md is left untouched (byte-identical to the user-edited version).

    This closes the gap identified in the verify report: the init --force path was
    already tested in test_force_init_preserves_runtime_brief_marker, but the
    sync path had only manual verification.
    """

    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_sync_preserves_user_edited_agents_md_with_runtime_brief_marker(self):
        """W2: After init (no marker), user adds the marker + custom content.
        Subsequent sync must leave the file byte-identical (marker honored)."""
        target = self._make_target()

        # Step 1: fresh init (AGENTS.md rendered from session-context, no marker)
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")
        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md must exist after init")

        # Step 2: user replaces AGENTS.md with hand-managed content + marker
        hand_managed = (
            "# Hand-Managed Runtime Brief\n"
            "<!-- ai-specs:runtime-brief -->\n\n"
            "This brief is manually maintained. Sync MUST NOT overwrite it.\n"
        )
        agents_md.write_text(hand_managed)

        # Step 3: run sync
        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")

        # Step 4: assert byte-identical
        final = agents_md.read_text()
        self.assertEqual(
            final,
            hand_managed,
            "sync must not modify AGENTS.md when <!-- ai-specs:runtime-brief --> marker is present",
        )
        self.assertIn(
            "<!-- ai-specs:runtime-brief -->",
            final,
            "Marker must be preserved after sync",
        )
        self.assertIn(
            "This brief is manually maintained.",
            final,
            "User custom content must be preserved after sync",
        )

    def test_sync_preserves_a_truly_untracked_agents_md(self):
        """The real first-sight migration path: a brief with NO lock baseline.

        This is the repository-predates-ai-specs case the change exists for, and
        it was not covered end to end: every sibling test ran `init` first, which
        records a baseline and therefore drives `user_modified` instead.
        """
        target = self._make_target()
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")
        agents_md = target / "AGENTS.md"

        # Drop the recorded baseline so the brief is genuinely untracked, the
        # state every existing project is in before its first sync on this code.
        lock_path = target / "ai-specs/.ai-specs.lock"
        lock_text = lock_path.read_text()
        lock_path.write_text(
            "\n".join(
                line for line in lock_text.splitlines()
                if "AGENTS.md" not in line
            ).replace("[managed.]\n", "")
            + "\n"
        )

        handwritten = "# Hand-written brief that predates ai-specs\n"
        agents_md.write_text(handwritten)

        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")
        self.assertEqual(
            agents_md.read_text(), handwritten,
            "a brief with no baseline must survive sync untouched",
        )
        combined = result.stdout + result.stderr
        self.assertIn("untracked", combined, "the reported state must name untracked")
        self.assertIn("--adopt-brief", combined)
        self.assertIn("ai-specs:runtime-brief", combined)

    def test_sync_without_marker_preserves_user_modified_agents_md(self):
        """An edited generated brief is preserved even without the marker.

        NOTE ON THE NAME: this drives `user_modified`, not `untracked`. `init`
        records a baseline before the overwrite, so a baseline exists by the
        time sync runs. An earlier name claimed `untracked` and the assertions
        still passed, because the remedy text is state-agnostic — the test could
        not have caught a regression specific to the first-sight path. The true
        no-baseline path is covered by the sibling test below.
        """
        target = self._make_target()

        # Fresh init
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")
        agents_md = target / "AGENTS.md"

        # Overwrite with stale content — NO marker
        stale = "# Stale content — no marker — sync must regenerate this.\n"
        agents_md.write_text(stale)

        # Run sync
        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"sync failed:\n{result.stderr}")

        final = agents_md.read_text()
        # Divergent bytes must survive migration without a recorded baseline.
        self.assertEqual(
            final,
            stale,
            "sync must preserve a divergent untracked AGENTS.md",
        )
        combined = result.stdout + result.stderr
        self.assertIn("--adopt-brief", combined)
        self.assertIn("ai-specs:runtime-brief", combined)


# ---------------------------------------------------------------------------
# Optional hardening — no unrendered placeholders in baseline brief
# ---------------------------------------------------------------------------

class BaselineBriefNoPlaceholderTests(unittest.TestCase):
    """Optional: tighten no-leakage test with regex for unrendered placeholders.

    A fresh default init must produce an AGENTS.md with no unrendered
    {config.KEY} or {{ escape sequences — every placeholder either resolved
    or absent from the output.
    """

    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_no_unrendered_config_placeholders_in_baseline_agents_md(self):
        """A fresh init must produce no {config.KEY} or {{ patterns in AGENTS.md.

        {config.KEY} → indicates a placeholder that should have been substituted
        but the config key was not present in the resolved recipe config.
        {{ → indicates an escaped brace that was not collapsed back to {.
        """
        target = self._make_target()
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")

        agents_md = target / "AGENTS.md"
        content = agents_md.read_text()

        # No {config.KEY} patterns — all substitutions resolved or absent
        config_placeholder_re = re.compile(r"\{config\.[A-Za-z_][A-Za-z0-9_]*\}")
        matches = config_placeholder_re.findall(content)
        self.assertEqual(
            matches,
            [],
            f"Unrendered {{config.KEY}} placeholders found in AGENTS.md: {matches}",
        )

        # No {{ patterns remaining (these should be collapsed to { by substitute_config)
        self.assertNotIn(
            "{{",
            content,
            "Unrendered {{ escape sequences found in AGENTS.md",
        )


if __name__ == "__main__":
    unittest.main()

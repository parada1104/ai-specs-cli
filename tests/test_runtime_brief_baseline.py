"""Tests for runtime-brief-baseline change.

Covers:
  - Init: default template enables session-context (TemplateDefaultTests)
  - E2E: fresh init produces behavioral brief (InitBriefE2ETests)
  - E2E: render failure → placeholder fallback, init exits 0
  - E2E: init→sync byte-stability
  - E2E: --preserve-if-runtime-brief marker preserved under --force
  - E2E: no this-repo tokens in baseline AGENTS.md
  - Black-box: W1 — dedupe via sync with session-context + a second recipe sharing a key (SessionContextDedupTests)
  - E2E: W2 — sync-side marker preservation after user adds marker post-init
  - E2E: optional — no unrendered {config.} or {{ placeholders in baseline brief

All offline: catalog read from AI_SPECS_HOME; session-context skills are bundled.
"""
import os
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
CATALOG = ROOT / "catalog" / "recipes"


def _section(text: str, heading: str) -> str:
    """Return the body of a `## heading` section in rendered AGENTS.md."""
    marker = f"## {heading}\n"
    start = text.index(marker) + len(marker)
    end = text.index("\n## ", start) if "\n## " in text[start:] else len(text)
    return text[start:end]


def _keyed_override_recipe() -> str:
    """A second recipe that re-declares session-context's conflict-policy key.

    session-context already contributes key='conflict-policy-source-authority';
    this recipe claims the same key with different wording, and repeats
    session-context's workflow_rules fragment verbatim so the sync dedupe must
    collapse both to a single bullet (first-wins for the keyed fragment,
    exact-string for the shared workflow rule).
    """
    return (
        '[recipe]\n'
        'id = "recipe-extra"\n'
        'name = "Recipe Extra"\n'
        'description = "Test recipe."\n'
        'version = "1.0.0"\n'
        'author = "tests"\n'
        '\n'
        '[provides.brief]\n'
        'workflow_rules = [\n'
        '  "A session works on one explicit user request or tracker card; '
        'resolve focus from memory and tracker before starting.",\n'
        ']\n'
        '\n'
        '[[provides.brief.conflict_policy]]\n'
        'key = "conflict-policy-source-authority"\n'
        'text = "Extra recipe override \u2014 MUST NOT appear."\n'
    )


class _HermeticCliTests(unittest.TestCase):
    """Shared hermetic fixture: one isolated CLI home whose catalog provides the
    recipes a class needs, plus a single wrapper for invoking the CLI."""

    RECIPES: dict[str, str] = {}        # custom recipe id -> inline recipe.toml
    REAL_RECIPES: tuple[str, ...] = ()  # real catalog recipe ids to symlink in

    def _home(self) -> Path:
        if not hasattr(self, "_cli_home"):
            td = tempfile.TemporaryDirectory(prefix="bb-runtime-brief-home-")
            self.addCleanup(td.cleanup)
            home = isolated_home(Path(td.name))
            catalog = home / "catalog"
            catalog.unlink()
            (catalog / "recipes").mkdir(parents=True)
            for rid, body in self.RECIPES.items():
                dest = catalog / "recipes" / rid
                dest.mkdir()
                (dest / "recipe.toml").write_text(body)
            for rid in self.REAL_RECIPES:
                (catalog / "recipes" / rid).symlink_to(CATALOG / rid)
            self._cli_home = home
        return self._cli_home

    def _cli(self, project: Path, verb: str, *args: str):
        """Single shared wrapper: every test invokes the CLI through here."""
        return invoke(project, verb, *args, cli_home=self._home())


class TemplateDefaultTests(unittest.TestCase):
    """Init on a bare dir must produce the default template manifest.

    The generated ai-specs.toml pre-enables the session-context recipe, and the
    rendered brief plus the installed-recipe list show the recipe was
    materialized. This is the observable equivalent of the old
    `build_resolved_config` unit probe: the manifest init writes and the
    surfaces it drives are the resolved-config input and output.
    """

    @classmethod
    def setUpClass(cls):
        cls._td = tempfile.TemporaryDirectory(prefix="bb-default-tmpl-home-")
        cls.home = isolated_home(Path(cls._td.name))

    @classmethod
    def tearDownClass(cls):
        cls._td.cleanup()

    def _make_target(self) -> Path:
        return Path(tempfile.mkdtemp(prefix="default-proj-", dir=self._td.name))

    def _invoke(self, root: Path, verb: str, *args: str):
        """Single shared wrapper: every test invokes the CLI through here."""
        return invoke(root, verb, *args, cli_home=self.home)

    def test_template_default_enables_session_context(self):
        """Init writes a manifest that pre-enables session-context, and the
        rendered brief proves the recipe was materialized into the runtime."""
        target = self._make_target()
        result = self._invoke(target, "init")
        self.assertEqual(result.returncode, 0, result.stderr)

        toml = (target / "ai-specs" / "ai-specs.toml").read_text()
        self.assertIn("[recipes.session-context]", toml)
        self.assertIn(
            "enabled = true",
            toml.split("[recipes.session-context]", 1)[1],
            "Default manifest must pre-enable session-context.",
        )

        listed = self._invoke(target, "recipe", "list")
        self.assertEqual(listed.returncode, 0, listed.stderr)
        self.assertIn(
            "session-context",
            listed.stdout,
            "recipe list must report session-context installed from the default manifest.",
        )
        self.assertIn("installed", listed.stdout)

        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md must be rendered on init")
        self.assertIn(
            "A session works on one explicit user request",
            agents_md.read_text(),
            "The session-context workflow_rules fragment must render into AGENTS.md.",
        )

    def test_template_default_no_project_specific_tokens(self):
        """The runtime surfaces of a fresh default init must not leak this-repo tokens."""
        target = self._make_target()
        result = self._invoke(target, "init")
        self.assertEqual(result.returncode, 0, result.stderr)

        agents_md = (target / "AGENTS.md").read_text()
        listed = self._invoke(target, "recipe", "list")

        # Tokens from the ai-specs-cli dogfood project must not appear in a
        # generic project's rendered runtime surfaces. The generated
        # ai-specs.toml legitimately names the CLI repo (…/ai-specs-cli) in a
        # comment, so the runtime surfaces — not the raw template text — are the
        # analogue of the comment-stripped resolved config the unit test parsed.
        forbidden_tokens = [
            "69ec097f13e2d38ecd89a557",   # board id
            "nnodes/proyectos",             # vault scope
            "ai-specs-cli",                 # dogfood project name
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                agents_md,
                f"Found project-specific token {token!r} in rendered AGENTS.md",
            )
            self.assertNotIn(
                token,
                listed.stdout,
                f"Found project-specific token {token!r} in recipe list output",
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

class SessionContextDedupTests(_HermeticCliTests):
    """W1: Dedupe when session-context and a second recipe share the same key.

    Drives `ai-specs sync` through the real pipeline with the real
    session-context recipe enabled alongside a second recipe that re-declares
    key='conflict-policy-source-authority' and repeats the workflow_rules
    fragment verbatim. The rendered AGENTS.md is the observable surface.

    Asserts:
    - The keyed bullet appears exactly once (first-wins).
    - session-context wins over the second recipe (ordering preserved).
    - The session-context workflow_rules fragment is present and also deduplicated.
    """

    REAL_RECIPES = ("session-context",)
    RECIPES = {
        "recipe-extra": _keyed_override_recipe(),
    }

    def _sync(self):
        td = tempfile.TemporaryDirectory(prefix="bb-runtime-dedup-")
        self.addCleanup(td.cleanup)
        root = Path(td.name)
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'dedup'\n\n"
            "[recipes.session-context]\nenabled = true\n\n"
            "[recipes.recipe-extra]\nenabled = true\n"
        )
        result = self._cli(root, "sync")
        return result, (root / "AGENTS.md").read_text()

    def test_session_context_key_wins_over_second_recipe(self):
        """session-context and the second recipe both provide
        key='conflict-policy-source-authority'. The Conflict Policy section must
        keep exactly the two session-context bullets, and the second recipe's
        override is suppressed (first-wins)."""
        result, text = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        section = _section(text, "Conflict Policy")
        self.assertEqual(
            section.count("\n- "),
            2,
            f"Conflict Policy must hold exactly the two session-context keys.\n{section}",
        )
        self.assertIn(
            "Current explicit human instruction controls the immediate scope",
            section,
            "session-context source-authority wording must win.",
        )
        self.assertIn(
            "Tracker controls work state",
            section,
            "session-context source-hierarchy bullet must render.",
        )
        self.assertNotIn(
            "MUST NOT appear",
            section,
            "recipe-extra override must be suppressed by first-wins key dedup.",
        )

    def test_session_context_key_dedup_appears_exactly_once_in_full_render(self):
        """W1 end-to-end: full sync renders the conflict_policy bullet exactly once."""
        result, text = self._sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        # Key-dedup: session-context wording appears exactly once
        self.assertEqual(
            text.count(
                "Current explicit human instruction controls the immediate scope "
                "unless it conflicts with safety, secrets, or a higher-authority project rule."
            ),
            1,
            f"session-context source-authority bullet must appear exactly once.\n{text}",
        )
        # Override from second recipe must not appear at all
        self.assertNotIn(
            "MUST NOT appear",
            text,
            "recipe-extra duplicate bullet must be suppressed by key dedup",
        )
        # Exact-string dedup: shared workflow_rules text appears exactly once
        self.assertEqual(
            text.count(
                "A session works on one explicit user request or tracker card; "
                "resolve focus from memory and tracker before starting."
            ),
            1,
            f"Shared workflow_rules bullet must appear exactly once.\n{text}",
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

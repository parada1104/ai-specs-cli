"""Black-box tests for the runtime-brief-baseline behavior.

Every test drives the native CLI (``init``/``sync``) at its process boundary.
No test may import or exec ``lib/_internal`` modules.

Covers:
  - E2E: fresh init produces behavioral brief (InitBriefE2ETests)
  - E2E: render failure → placeholder fallback, init exits 0
  - E2E: init→sync byte-stability
  - E2E: --preserve-if-runtime-brief marker preserved under --force
  - E2E: no this-repo tokens in baseline AGENTS.md
  - W2 — sync-side marker preservation after user adds marker post-init
  - Optional — no unrendered {config.} or {{ placeholders in baseline brief

The template default (session-context pre-enabled, no dogfood tokens) is pinned
natively in ``internal/cli`` and the recipe-fragment dedupe rules (key
first-wins, then exact text) in ``internal/sync``.

All offline: catalog read from the isolated AI_SPECS_HOME; session-context
skills are bundled.
"""
from __future__ import annotations

import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cli_path, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CLI = cli_path()


def _make_home(base: Path) -> Path:
    """Isolated CLI install root (the native binary resolves its own assets)."""
    return isolated_home(base)


_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    """One shared isolated install root per command sequence."""
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = _make_home(base)
    return _HOMES[key]


def _run_cli_env(project, *args, home: Path, tmpdir: Path, extra_env=None,
                 append_root: bool = True):
    """Raw CLI run for tests needing env vars invoke() cannot pass.

    stdin is closed (input='') so the CLI can never block on a prompt.
    Mirrors invoke()'s hermetic environment plus any extra_env overrides.
    """
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(tmpdir / "home"),
        "TMPDIR": str(tmpdir),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
    }
    if extra_env:
        env.update(extra_env)
    (tmpdir / "home").mkdir(parents=True, exist_ok=True)
    argv = [str(CLI), *args]
    if append_root:
        argv.append(str(project))
    return subprocess.run(argv, cwd=ROOT, env=env, text=True,
                          capture_output=True, check=False, input="")


# ---------------------------------------------------------------------------
# E2E: fresh init produces behavioral brief
# ---------------------------------------------------------------------------

class InitBriefE2ETests(unittest.TestCase):
    """E2E tests for the init → AGENTS.md rendering pipeline."""

    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-initbrief-")
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_fresh_init_produces_behavioral_brief(self):
        """After init, AGENTS.md must contain the session-context behavioral sections."""
        target = self._make_target()
        result = invoke(target, "init", cli_home=_home_for(target))
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
        """When the baseline render cannot start, init must still create AGENTS.md and exit 0.

        The native render chain (internal/cli/init.go renderInitBrief) stages its
        resolved config with os.CreateTemp, which honours TMPDIR. Pointing TMPDIR
        at a directory that does not exist makes that staging fail, so the native
        binary takes its real fallback (briefFallback: placeholder + "render skipped").
        """
        target = self._make_target()
        home = _home_for(target)
        base = target.parent

        result = _run_cli_env(
            target, "init", home=home, tmpdir=base,
            extra_env={"TMPDIR": str(base / "missing-tmpdir")},
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
        home = _home_for(target)

        result = invoke(target, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        agents_md = target / "AGENTS.md"
        after_init = agents_md.read_bytes()

        result = invoke(target, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        after_sync = agents_md.read_bytes()
        self.assertEqual(
            after_init,
            after_sync,
            "AGENTS.md must be byte-identical after init and after sync",
        )

    def test_force_init_preserves_runtime_brief_marker(self):
        """If AGENTS.md contains <!-- ai-specs:runtime-brief -->, --force must not overwrite it."""
        target = self._make_target()
        home = _home_for(target)

        # First init to bootstrap the directory
        result = invoke(target, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        # Write the user-managed marker into AGENTS.md
        agents_md = target / "AGENTS.md"
        original_content = "# Manual Brief\n<!-- ai-specs:runtime-brief -->\n\nCustom content.\n"
        agents_md.write_text(original_content)

        # --force init: must preserve the file because the marker is present
        result = invoke(target, "init", "--force", cli_home=home)
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

        result = invoke(target, "init", cli_home=_home_for(target))
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
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-syncmarker-")
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_sync_preserves_user_edited_agents_md_with_runtime_brief_marker(self):
        """W2: After init (no marker), user adds the marker + custom content.
        Subsequent sync must leave the file byte-identical (marker honored)."""
        target = self._make_target()
        home = _home_for(target)

        # Step 1: fresh init (AGENTS.md rendered from session-context, no marker)
        result = invoke(target, "init", cli_home=home)
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
        result = invoke(target, "sync", cli_home=home)
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

    @staticmethod
    def _drop_managed_baseline(lock_path: Path, name: str = "AGENTS.md") -> None:
        """Remove one [managed."name"] entry from a lock file, line-based.

        Mirrors the lock writer's format: the entry header line plus its
        following key = "value" lines, up to the next table header.
        """
        lines = lock_path.read_text().splitlines(keepends=True)
        out: list[str] = []
        skipping = False
        for line in lines:
            if line.strip() == f'[managed."{name}"]':
                skipping = True
                continue
            if skipping:
                if line.startswith("["):
                    skipping = False
                else:
                    continue
            out.append(line)
        lock_path.write_text("".join(out))

    def test_sync_preserves_a_truly_untracked_agents_md(self):
        """The real first-sight migration path: a brief with NO lock baseline.

        This is the repository-predates-ai-specs case the change exists for, and
        it was not covered end to end: every sibling test ran `init` first, which
        records a baseline and therefore drives `user_modified` instead.
        """
        target = self._make_target()
        home = _home_for(target)
        result = invoke(target, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")
        agents_md = target / "AGENTS.md"

        # Drop the recorded baseline so the brief is genuinely untracked, the
        # state every existing project is in before its first sync on this code.
        lock_path = target / "ai-specs/.ai-specs.lock"
        self._drop_managed_baseline(lock_path)

        handwritten = "# Hand-written brief that predates ai-specs\n"
        agents_md.write_text(handwritten)

        result = invoke(target, "sync", cli_home=home)
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
        no-baseline path is covered by the sibling test above.
        """
        target = self._make_target()
        home = _home_for(target)

        # Fresh init
        result = invoke(target, "init", cli_home=home)
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")
        agents_md = target / "AGENTS.md"

        # Overwrite with stale content — NO marker
        stale = "# Stale content — no marker — sync must regenerate this.\n"
        agents_md.write_text(stale)

        # Run sync
        result = invoke(target, "sync", cli_home=home)
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
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-noplaceholder-")
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
        result = invoke(target, "init", cli_home=_home_for(target))
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

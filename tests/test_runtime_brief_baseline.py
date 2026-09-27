"""Black-box tests for the runtime-brief-baseline behavior.

Every test drives ``bin/ai-specs`` (init/sync) or an internal render script at
its process boundary. No test may import ``lib/_internal`` modules; the two
process-boundary render/materialize invocations are marked with distinct
``# TRIAGE:`` comments.

Covers:
  - Default template pre-enables session-context in the resolved config
    (TemplateDefaultTests, materializer process boundary)
  - E2E: fresh init produces behavioral brief (InitBriefE2ETests)
  - E2E: render failure → placeholder fallback, init exits 0
  - E2E: init→sync byte-stability
  - E2E: --preserve-if-runtime-brief marker preserved under --force
  - E2E: no this-repo tokens in baseline AGENTS.md
  - W1 — dedupe with session-context + second recipe sharing a key
    (SessionContextDedupTests, renderer process boundary)
  - W2 — sync-side marker preservation after user adds marker post-init
  - Optional — no unrendered {config.} or {{ placeholders in baseline brief

All offline: catalog read from the isolated AI_SPECS_HOME; session-context
skills are bundled.
"""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
TEMPLATE_PATH = ROOT / "templates" / "ai-specs.toml.tmpl"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy (see test_sync_pipeline)."""
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


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


def _run_internal(home: Path, script: str, *args: str, base: Path):
    """Run an internal render/materialize script at the process boundary."""
    return subprocess.run(
        ["python3", str(home / "lib" / "_internal" / script), *args],
        text=True, capture_output=True, check=False, input="",
        env={
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "home"),
            "TMPDIR": str(base),
            "AI_SPECS_HOME": str(home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        },
    )


class TemplateDefaultTests(unittest.TestCase):
    """The default TOML template pre-enables session-context (resolved config)."""

    def _make_project_from_template(self) -> tuple[Path, Path, Path]:
        """Render ai-specs.toml.tmpl into a fresh temp project directory.

        Returns (base, project, home) — base is the temp dir backing home.
        """
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-tmpl-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        root = base / "project"
        ai_specs = root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()

        # Mimic what init.sh does: replace {{PROJECT_NAME}} and write toml
        template_text = TEMPLATE_PATH.read_text()
        toml_text = template_text.replace("{{PROJECT_NAME}}", "test-proj")
        (ai_specs / "ai-specs.toml").write_text(toml_text)

        return base, root, _make_home(base)

    def _resolved_from_template(self):
        base, root, home = self._make_project_from_template()
        out = base / "resolved.json"
        # TRIAGE: ai-specs sync — the resolved-config JSON (enabled list, per-
        # recipe configs) is written to a temp file the CLI deletes after each
        # run; the enabled-list contract is only observable at the
        # materializer's --resolved-config-only process boundary.
        proc = _run_internal(
            home, "recipe-materialize.py", str(root), str(home),
            "--resolved-config-only", "--resolved-config-out", str(out),
            base=base,
        )
        self.assertEqual(
            proc.returncode, 0,
            f"materialize failed:\n{proc.stderr}\n{proc.stdout}",
        )
        return json.loads(out.read_text())

    def test_template_default_enables_session_context(self):
        """The default template's resolved config yields session-context in enabled."""
        # TRIAGE: ai-specs sync — the resolved-config temp file is deleted after
        # every verb run, so the enabled-list JSON is only observable at the
        # materializer's --resolved-config-only process boundary.
        result = self._resolved_from_template()
        self.assertIn(
            "session-context",
            result["enabled"],
            f"Expected 'session-context' in enabled list. Got: {result['enabled']!r}",
        )

    def test_template_default_no_project_specific_tokens(self):
        """Resolved config from the default template must not contain this-repo tokens."""
        # TRIAGE: ai-specs sync — the resolved-config JSON blob is not exposed
        # by any verb (temp file deleted post-run); token leakage is checked at
        # the materializer's --resolved-config-only process boundary.
        result = self._resolved_from_template()
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
        """When the render scripts fail, init must still create AGENTS.md and exit 0.

        Uses a fake python3 that delegates to the real python3 for all scripts
        EXCEPT recipe-materialize.py and agents-render.py, which it makes exit 1.
        This simulates a render-pipeline failure without breaking the rest of init
        (gitignore-render.py, refresh-bundled.py, etc. still run via real python3).
        """
        target = self._make_target()
        home = _home_for(target)
        base = target.parent

        # Create a selective fake python3 that fails only for render scripts
        fake_bin = base / "fake-bin"
        fake_bin.mkdir()
        fake_python = fake_bin / "python3"
        fake_python.write_text(
            "#!/bin/sh\n"
            "# Fail only for the render pipeline scripts; pass through for others.\n"
            "case \"$*\" in\n"
            "  *recipe-materialize*|*agents-render*) exit 1 ;;\n"
            f"  *) exec \"{sys.executable}\" \"$@\" ;;\n"
            "esac\n"
        )
        fake_python.chmod(0o755)

        # PATH with fake-bin FIRST
        patched_path = f"{fake_bin}:{os.environ.get('PATH', '')}"

        result = _run_cli_env(
            target, "init", home=home, tmpdir=base,
            extra_env={"PATH": patched_path},
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

class SessionContextDedupTests(unittest.TestCase):
    """W1: Dedupe when session-context and a second recipe share the same key.

    Both tests hand-craft a resolved-config JSON (two recipes contributing the
    same keyed fragment) and drive the renderer at its process boundary.

    Asserts:
    - The keyed bullet appears exactly once (first-wins).
    - session-context wins over the second recipe (ordering preserved).
    - The session-context workflow_rules fragment is present and also deduplicated.
    """

    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory(prefix="ai-specs-dedup-")
        cls.addClassCleanup(cls._tmp.cleanup)
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)

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

    def _render_resolved(self, resolved: dict) -> str:
        """Render AGENTS.md at the renderer's process boundary from a crafted config."""
        toml_path = self.base / f"{self.id().rsplit('.', 1)[-1]}.toml"
        output_path = self.base / f"{self.id().rsplit('.', 1)[-1]}.out.md"
        resolved_path = self.base / f"{self.id().rsplit('.', 1)[-1]}.json"
        toml_path.write_text(
            "[project]\nname = 'dedup-fixture'\n\n"
            "[brief]\n"
            "intro = 'Dedup fixture project.'\n"
            "purpose = 'Testing fragment dedup with session-context.'\n"
        )
        resolved_path.write_text(json.dumps(resolved))
        proc = _run_internal(
            self.home, "agents-render.py", str(toml_path), str(output_path),
            "--resolved-config", str(resolved_path),
            base=self.base,
        )
        self.assertEqual(
            proc.returncode, 0,
            f"agents-render failed:\n{proc.stderr}\n{proc.stdout}",
        )
        return output_path.read_text()

    def test_session_context_key_wins_over_second_recipe(self):
        """W1 core: session-context and a second recipe both provide
        key='conflict-policy-source-authority'. The renderer must emit each
        session-context keyed bullet exactly once with session-context's
        wording (first-wins)."""
        # TRIAGE: ai-specs sync — the CLI derives the resolved config from the
        # project manifest, so two recipes sharing one keyed fragment can only
        # be exercised at the renderer's --resolved-config process boundary.
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
        content = self._render_resolved(resolved)

        # Each session-context keyed bullet appears exactly once (the two unique
        # keys survive; no third duplicate from recipe-extra)
        session_bullet = (
            "Current explicit human instruction controls the immediate scope "
            "unless it conflicts with safety, secrets, or a higher-authority project rule."
        )
        hierarchy_bullet = (
            "Tracker controls work state; vault controls canonical decisions and handoffs; "
            "repo docs and manifests control versioned project contracts. "
            "Agent plans are lowest authority until accepted and recorded."
        )
        self.assertEqual(
            content.count(session_bullet), 1,
            f"session-context source-authority bullet must appear exactly once.\nContent:\n{content}",
        )
        self.assertEqual(
            content.count(hierarchy_bullet), 1,
            f"session-context source-hierarchy bullet must appear exactly once.\nContent:\n{content}",
        )
        # second recipe duplicate must be suppressed
        self.assertNotIn(
            "MUST NOT appear",
            content,
            "recipe-extra override must be suppressed by first-wins key dedup",
        )

    def test_session_context_key_dedup_appears_exactly_once_in_full_render(self):
        """W1 end-to-end: full render with session-context + second recipe sharing key.
        The conflict_policy bullet must appear exactly once in the rendered AGENTS.md."""
        # TRIAGE: ai-specs sync — a hand-crafted resolved-config JSON cannot be
        # produced by any verb (sync always derives it from the manifest), so
        # this full-render dedup contract stays at the renderer's process
        # boundary.
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

        content = self._render_resolved(resolved)

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

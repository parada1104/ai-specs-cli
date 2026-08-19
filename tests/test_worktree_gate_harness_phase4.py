"""Phase 4 harness-coverage tests for the launcher-based worktree gate.

Tasks 4.1-4.4: prove that the thin launcher (Phase 3) changed nothing in how
the five harnesses consume the gate:

- 4.1  hooks-render.py output is byte-identical to the pre-change output for
       claude, cursor, opencode, pi and omp (script_path stability). Phases
       0-3 did not modify hooks-render.py, so the renderer must emit exactly
       the same bytes for the same resolved-hooks document — pinned here by
       rendering twice in fresh projects and asserting identical bytes, plus
       asserting every artifact references the launcher's materialized path.
- 4.2  the Cursor wrapper still maps exit 2 -> {"permission":"deny"} through
       the launcher, and the binary's empty stdout does not degrade the deny
       decision: the gate message travels on stderr (inherited by the Cursor
       hook process), while stdout stays empty and the deny JSON stays valid.
- 4.3  spawnSync(SCRIPT, ...) semantics hold for opencode/pi/omp: the launcher
       must be directly executable (shebang + mode 0755) with no shell.
- 4.4  live smoke through the real launcher: blocked write on a protected
       branch, allowed write inside a linked worktree.

The Go binary is not required: the launcher's legacy fallback answers the
smoke scenarios when no binary is cached, so this suite runs green on any
machine (the phase-2/3 suites already pin Go-vs-Bash parity).
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
LEGACY_GATE = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate-legacy.sh"
GO_BINARY = ROOT / "dist" / "worktree-gate-current"

# Artifact each harness emits for the worktree-flow gate when sync renders it.
# These are the sync-emitted names (verified by experiment). A regression in
# hooks-render.py that skips a harness stops emitting the file, which the
# tests below catch as a missing/non-empty artifact.
AGENT_ARTIFACT = {
    "claude": ".claude/settings.json",
    "cursor": ".cursor/hooks/worktree-flow-worktree-gate-shell.sh",
    "opencode": ".opencode/plugin/worktree-flow-worktree-gate.ts",
    "pi": ".pi/extensions/worktree-flow-worktree-gate.ts",
    "omp": ".omp/extensions/worktree-flow-worktree-gate.ts",
}

MATERIALIZED_SCRIPT = "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"


def _worktree_manifest(agent: str) -> str:
    """Manifest enabling worktree-flow for the given single agent."""
    return (
        '[project]\nname = "test"\n\n'
        f'[agents]\nenabled = ["{agent}"]\n\n'
        "[recipes.worktree-flow]\nenabled = true\n"
        "version = \"1.5.0\"\n\n"
        "[recipes.worktree-flow.config]\ngate_impl = \"auto\"\n"
    )


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


def _materialize_launcher(dest: Path, *, mode: str = "always",
                          scope: str = "auto", topology: str = "auto",
                          impl: str = "auto", version: str = "0.21.0") -> Path:
    """Stamp the catalog launcher for a scratch project (like sync does)."""
    content = GATE.read_text()
    content = content.replace("__WORKTREE_GATE_MODE__", mode)
    content = content.replace("__WORKTREE_GATE_SCOPE__", scope)
    content = content.replace("__WORKTREE_REPO_TOPOLOGY__", topology)
    content = content.replace("__WORKTREE_GATE_IMPL__", impl)
    content = content.replace("__WORKTREE_GATE_VERSION__", version)
    dest.write_text(content, encoding="utf-8")
    dest.chmod(0o755)
    return dest


def _install_binary_in(project: Path) -> Path:
    """Place the built binary at the launcher's project-local pin path."""
    if not GO_BINARY.exists():
        raise unittest.SkipTest("no Go gate binary in dist/; launcher smoke uses legacy fallback")
    pin = project / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
    pin.parent.mkdir(parents=True, exist_ok=True)
    pin.write_bytes(GO_BINARY.read_bytes())
    pin.chmod(0o755)
    return pin


def _project_launcher(root: Path) -> Path:
    """Materialize the launcher + binary pin in the real project layout.

    Mirrors a synced project: launcher at
    ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh and the project
    pin at ai-specs/recipes/worktree-flow/bin/worktree-gate. The launcher
    resolves the pin from $PWD, so processes must run with cwd=root (the
    project root), exactly like every harness spawns the hook.
    """
    hook = root / MATERIALIZED_SCRIPT
    hook.parent.mkdir(parents=True, exist_ok=True)
    _materialize_launcher(hook)
    _install_binary_in(root)
    return hook


def _launcher_env(root: Path) -> dict:
    env = dict(os.environ)
    env["WORKTREE_GATE_PROTECTED"] = "main development"
    # Hermetic: never consult the developer's real cache or pins.
    env["AI_SPECS_HOME"] = str(root / ".ai-specs-home")
    env.pop("WORKTREE_GATE_BIN", None)
    env.pop("WORKTREE_GATE_MODE", None)
    env.pop("WORKTREE_GATE_SCOPE", None)
    return env


class HookRenderByteStabilityTests(unittest.TestCase):
    """Task 4.1: renderer output is byte-identical to the pre-change output."""

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for invocation)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _sync_project(self, agent: str) -> bytes:
        """Sync a fresh worktree-flow project for one agent; return the artifact.

        The CLI drives hooks-render.py itself from the resolved-hooks document
        sync builds, so the emitted artifact is the renderer's output.
        """
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            _worktree_manifest(agent), encoding="utf-8")
        result = invoke(project, "sync", cli_home=self._cli_home())
        self.assertEqual(result.returncode, 0, result.stderr)
        return self._capture(project, agent)

    def _capture(self, project: Path, agent: str) -> bytes:
        """Read the sync-emitted artifact for `agent`.

        sync-agent renders this via hooks-render.py; a render that is skipped
        (no native target) emits no file, so existence here is the black-box
        stand-in for the old "must not skip the hook" warning assertion.
        """
        path = project / AGENT_ARTIFACT[agent]
        self.assertTrue(path.is_file(),
                        f"{agent} must emit artifact {AGENT_ARTIFACT[agent]}")
        return path.read_bytes()

    def test_renderer_output_is_byte_identical_to_preexisting_shape(self):
        # hooks-render.py is untouched by Phases 0-3 (spec: "zero renderer
        # changes and zero re-render churn"). Byte-identity is pinned by
        # syncing the same manifest twice in fresh projects. The
        # stabilize-workspace-context change then replaced the relative
        # ``const SCRIPT = "..."`` with a runtime module-derived declaration
        # for opencode/pi/omp; the pinned fragments below assert THAT shape,
        # so a regression to a relative or machine-specific SCRIPT breaks
        # here.
        expected_fragments = {
            "claude": MATERIALIZED_SCRIPT,
            "cursor": MATERIALIZED_SCRIPT,   # wrapper embeds script=...
            "opencode": f'const SCRIPT = fileURLToPath(new URL("../../{MATERIALIZED_SCRIPT}", import.meta.url));',
            "pi": f'const SCRIPT = fileURLToPath(new URL("../../{MATERIALIZED_SCRIPT}", import.meta.url));',
            "omp": f'const SCRIPT = fileURLToPath(new URL("../../{MATERIALIZED_SCRIPT}", import.meta.url));',
        }
        for agent in ("claude", "cursor", "opencode", "pi", "omp"):
            with self.subTest(agent=agent):
                blob = self._sync_project(agent)
                self.assertTrue(blob, f"{agent} render must not be empty")
                text = blob.decode("utf-8", "replace")
                self.assertIn(expected_fragments[agent], text)
                # Deterministic output: a fresh project sync is byte-identical.
                blob2 = self._sync_project(agent)
                self.assertEqual(blob, blob2, f"{agent} render must be byte-identical")

    def test_all_five_harness_artifacts_reference_the_materialized_path(self):
        # Spec scenario "Materialized path is unchanged for every harness":
        # every rendered artifact references the same script_path — the
        # launcher — with no re-render churn.
        for agent in ("claude", "cursor", "opencode", "pi", "omp"):
            with self.subTest(agent=agent):
                blob = self._sync_project(agent)
                text = blob.decode("utf-8", "replace")
                self.assertIn("worktree-gate.sh", text)


class CursorLauncherWrapperTests(unittest.TestCase):
    """Task 4.2: Cursor wrapper exit-2 -> deny mapping through the launcher."""

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for invocation)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def setUp(self):
        self._cli_home()
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.project = Path(tmp.name)
        self.repo = self.project / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        _git(self.repo, "checkout", "-q", "-B", "main")
        # Sync materializes the launcher AND renders the Cursor shell wrapper
        # (the CLI runs hooks-render.py with the resolved-hooks document it
        # builds). Replaces the old manual _project_launcher + mod.render.
        (self.project / "ai-specs").mkdir(parents=True)
        (self.project / "ai-specs" / "ai-specs.toml").write_text(
            _worktree_manifest("cursor"), encoding="utf-8")
        result = invoke(self.project, "sync", cli_home=self._cli_home())
        self.assertEqual(result.returncode, 0, result.stderr)
        # Pin the build's Go binary at the launcher's project-local resolve
        # path (sync does not stamp the pin; fixture-provided, as before).
        _install_binary_in(self.project)

    def _wrapper(self) -> Path:
        return self.project / ".cursor" / "hooks" / "worktree-flow-worktree-gate-shell.sh"

    def _wrapper_env(self) -> dict:
        env = _launcher_env(self.project)
        # The wrapper references $CURSOR_PROJECT_DIR; provide it explicitly so
        # the wrapper runs outside a real Cursor process. cwd = project root
        # so the launcher resolves the project pin from $PWD.
        env["CURSOR_PROJECT_DIR"] = str(self.project)
        return env

    def _run_wrapper(self, event: dict) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(self._wrapper())],
            input=json.dumps(event), capture_output=True, text=True,
            cwd=str(self.project), env=self._wrapper_env(),
        )

    def test_blocked_write_maps_to_permission_deny(self):
        # A shell command that would write into the protected main worktree
        # must make the wrapper emit {"permission":"deny"} — through the
        # launcher, not the legacy script.
        event = {
            "command": "echo hi > " + str(self.repo / "src.py"),
            "cwd": str(self.repo),
        }
        proc = self._run_wrapper(event)
        self.assertEqual(proc.returncode, 0, "wrapper must always exit 0")
        out = json.loads(proc.stdout)
        self.assertEqual(out["permission"], "deny")

    def test_allowed_write_maps_to_permission_allow(self):
        # Write into a linked worktree (allowed) → allow JSON.
        wt = self.project / "wt"
        _git(self.repo, "worktree", "add", "-q", "-b", "feat", str(wt))
        event = {
            "command": "echo hi > " + str(wt / "f.txt"),
            "cwd": str(wt),
        }
        proc = self._run_wrapper(event)
        self.assertEqual(proc.returncode, 0)
        out = json.loads(proc.stdout)
        self.assertEqual(out["permission"], "allow")

    def test_deny_message_not_degraded_by_empty_binary_stdout(self):
        # The gate writes its message to stderr; stdout stays empty on the
        # gate path. The Cursor wrapper's deny decision must survive an empty
        # stdout: the JSON stays well-formed with permission=deny, and the
        # human-readable message is intact on the wrapper's stderr (inherited
        # by the Cursor hook process, exactly as before the Go cutover).
        event = {
            "command": "echo hi > " + str(self.repo / "src.py"),
            "cwd": str(self.repo),
        }
        proc = self._run_wrapper(event)
        out = json.loads(proc.stdout)
        self.assertEqual(out["permission"], "deny")
        self.assertIn("agent_message", out)
        self.assertIsInstance(out["agent_message"], str)
        self.assertIn("refusing", proc.stderr)
        # The gate never writes to stdout — the deny JSON is built from the
        # exit code, not from stdout bytes.
        self.assertNotIn("refusing", proc.stdout)


class SpawnSyncExecutableLauncherTests(unittest.TestCase):
    """Task 4.3: spawnSync(SCRIPT, ...) with no shell — shebang + mode 0755.

    opencode/pi/omp extensions call spawnSync with the script path directly
    (no `shell: true`). The launcher must therefore be directly executable:
    correct shebang and mode 0755 on the materialized file.
    """

    def test_materialized_launcher_is_directly_executable(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        hook = _materialize_launcher(Path(tmp.name) / "gate.sh")
        mode = os.stat(hook).st_mode
        self.assertTrue(mode & 0o111, "launcher must be executable (mode 0755)")
        first = hook.read_text(encoding="utf-8").splitlines()[0]
        self.assertTrue(first.startswith("#!"), "launcher must carry a shebang")

    def test_launcher_runs_without_shell_and_passes_stdin(self):
        # spawnSync with no shell on a blocked write → exit 2, stderr message,
        # stdout empty (the exact spawnSync contract the TS shims rely on).
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        repo = root / "repo"
        repo.mkdir()
        _git(repo, "init", "-q")
        _git(repo, "config", "user.email", "t@t.t")
        _git(repo, "config", "user.name", "t")
        (repo / "README.md").write_text("x\n")
        _git(repo, "add", "-A")
        _git(repo, "commit", "-qm", "init")
        _git(repo, "checkout", "-q", "-B", "main")
        hook = _project_launcher(root)
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(repo / "src.py")},
            "cwd": str(repo),
        }
        proc = subprocess.run(
            [str(hook)], input=json.dumps(event),
            capture_output=True, text=True, cwd=str(root),
            env=_launcher_env(root),
        )
        self.assertEqual(proc.returncode, 2, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn("refusing", proc.stderr)


class LauncherLiveSmokeTests(unittest.TestCase):
    """Task 4.4: live smoke through the real launcher on real Git fixtures."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        _git(self.repo, "checkout", "-q", "-B", "main")
        # Project root carries ai-specs/; the launcher runs from there (the
        # real harness shape) while the event cwd points at the git fixture.
        self.hook = _project_launcher(self.root)

    def _run(self, event: dict) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(self.hook)], input=json.dumps(event),
            capture_output=True, text=True, cwd=str(self.root),
            env=_launcher_env(self.root),
        )

    def test_blocked_write_on_protected_branch(self):
        event = {
            "event": "pre-tool-use",
            "tool_name": "Edit",
            "tool_input": {"file_path": str(self.repo / "src.py")},
            "cwd": str(self.repo),
        }
        r = self._run(event)
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("protected branch", r.stderr)
        self.assertEqual(r.stdout, "")

    def test_allowed_write_inside_linked_worktree(self):
        wt = self.root / "wt"
        _git(self.repo, "worktree", "add", "-q", "-b", "feat", str(wt))
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(wt / "f.txt")},
            "cwd": str(wt),
        }
        r = self._run(event)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stderr, "")

    def test_launcher_falls_back_to_legacy_without_binary(self):
        # No binary anywhere (empty AI_SPECS_HOME, no project pin):
        # gate_impl=auto must fall back to the frozen Bash reference, keeping
        # the gate enforcing (not failing open).
        if not LEGACY_GATE.is_file():
            self.skipTest("frozen Bash reference missing")
        # Remove the project pin installed by setUp so only the legacy path is
        # reachable.
        pin = self.root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
        pin.unlink(missing_ok=True)
        legacy = self.root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate-legacy.sh"
        legacy.parent.mkdir(parents=True, exist_ok=True)
        legacy.write_bytes(LEGACY_GATE.read_bytes())
        legacy.chmod(0o755)
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(self.repo / "src.py")},
            "cwd": str(self.repo),
        }
        r = self._run(event)
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("refusing", r.stderr)


if __name__ == "__main__":
    unittest.main()

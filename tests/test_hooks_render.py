"""Black-box tests for per-harness runtime-hook wiring.

Every test drives ``bin/ai-specs sync`` through its process boundary:
hook-declaring recipes are staged into an isolated catalog (fresh recipe ids,
real non-symlinked recipe dirs, never touching the repository catalog), the
project manifest enables the agents under test, and the assertions observe
the per-agent hook artifacts ``sync`` writes (.claude/settings.json, .cursor
wrappers + hooks.json, .opencode plugins, .pi/.omp extensions) plus sync
stderr warnings. No test may import ``lib/_internal`` modules.

Contract reference: openspec/changes/recipe-runtime-hooks/specs/
runtime-hook-distribution/spec.md.

WorkspaceContextProcessBoundaryTests relocate the sync-generated adapter into
a temporary materialized installation and execute it through the Node harness
with a recording ``spawnSync`` double, observing the actual process boundary
(executable path, event cwd, options cwd, fail-open behavior) — never
generated source text.
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

NODE_HARNESS = (
    ROOT / "tests" / "fixtures" / "workspace-context" / "opencode_process_boundary.mjs"
)
NODE = shutil.which("node")

# Fresh catalog recipe ids (never colliding with repository catalog recipes).
RECIPE_SHELL = "hookbb-shell"
RECIPE_FILEWRITE = "hookbb-filewrite"
RECIPE_STOP = "hookbb-stop"
HOOK_SCRIPT_REL = "hooks/gate.sh"

HOOK_SCRIPT_BODY = "#!/usr/bin/env bash\n# launcher marker\nexit 0\n"


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


def _hook_recipe_toml(recipe_id: str, hook_id: str, event: str, matcher: str) -> str:
    return (
        f'[recipe]\nid = "{recipe_id}"\nname = "{recipe_id}"\n'
        f'description = "test recipe {recipe_id}"\nversion = "1.0.0"\n\n'
        "[[provides.hooks]]\n"
        f'id = "{hook_id}"\n'
        f'event = "{event}"\n'
        f'script = "{HOOK_SCRIPT_REL}"\n'
        f'matcher = "{matcher}"\n'
        "blocking = true\n"
        f'description = "test hook {hook_id}"\n'
    )


class _HookFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-hooksbb-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        (self.root / "ai-specs" / "skills").mkdir(parents=True)
        (self.root / "ai-specs" / "commands").mkdir()
        self._recipe_id = ""

    def seed_recipe(self, recipe_id: str, hook_id: str, *,
                    event: str = "pre-tool-use", matcher: str = "Bash") -> None:
        """Stage one hook-declaring catalog recipe into the isolated home."""
        rdir = populate_catalog(
            self.home, recipe_id, _hook_recipe_toml(recipe_id, hook_id, event, matcher)
        )
        script = rdir / HOOK_SCRIPT_REL
        script.parent.mkdir(parents=True, exist_ok=True)
        script.write_text(HOOK_SCRIPT_BODY)
        self._recipe_id = recipe_id

    @property
    def script_path(self) -> str:
        """Project-relative materialized launcher path for the seeded recipe."""
        return f"ai-specs/recipes/{self._recipe_id}/{HOOK_SCRIPT_REL}"

    @property
    def shim_base(self) -> str:
        return f"{self._recipe_id}-{self._hook_id}"

    def write_manifest(self, *agents: str) -> None:
        enabled = ", ".join(f"'{a}'" for a in agents)
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            f"[project]\nname = 'hookbb'\n\n[agents]\nenabled = [{enabled}]\n\n"
            f"[recipes.{self._recipe_id}]\nenabled = true\n"
        )

    def sync(self):
        result = invoke(self.root, "sync", cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"sync failed: {result.stdout}{result.stderr}",
        )
        return result

    def seed_and_sync(self, recipe_id: str, hook_id: str, *agents: str, **kw) -> object:
        self._hook_id = hook_id
        self.seed_recipe(recipe_id, hook_id, **kw)
        self.write_manifest(*agents)
        return self.sync()


class HooksRenderTests(_HookFixtureMixin, unittest.TestCase):
    def test_claude_pretooluse_managed_block(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        pre = settings["hooks"]["PreToolUse"]
        # Find the managed entry for this hook
        entry = next(e for e in pre if e.get("matcher") == "Bash")
        cmd = entry["hooks"][0]["command"]
        self.assertIn(self.script_path, cmd)
        self.assertEqual(entry["hooks"][0]["type"], "command")

    def test_cursor_hooks_json(self):
        # Use a Bash-matcher hook so Cursor has a real target (beforeShellExecution)
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "cursor")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertTrue(wrapper.is_file(), "cursor wrapper should be generated")
        wtext = wrapper.read_text()
        self.assertIn("GENERATED by ai-specs", wtext)
        self.assertIn('"permission":"deny"', wtext.replace(" ", ""))
        hooks_json = json.loads((self.root / ".cursor" / "hooks.json").read_text())
        self.assertIn("beforeShellExecution", json.dumps(hooks_json))

    def test_opencode_plugin_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "opencode")
        plugin = self.root / ".opencode" / "plugin" / f"{self.shim_base}.ts"
        self.assertTrue(plugin.is_file(), "opencode plugin should be generated")
        text = plugin.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("tool.execute.before", text)
        self.assertIn("throw", text)
        self.assertIn(self.script_path, text)
        # OpenCode tool ids may be lowercase while recipe matchers use
        # Claude-style names — same case-insensitive contract as pi/omp.
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)

    def test_pi_extension_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "pi")
        ext = self.root / ".pi" / "extensions" / f"{self.shim_base}.ts"
        self.assertTrue(ext.is_file(), "pi extension should be generated")
        text = ext.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("@earendil-works/pi-coding-agent", text)
        self.assertIn("ExtensionAPI", text)
        self.assertIn('pi.on("tool_call"', text)
        self.assertIn("block: true", text)
        # Pi exposes the tool name as camelCase `toolName` (verified June 2026);
        # reading only snake_case would make the matcher never fire. Lock it in.
        self.assertIn("call?.toolName", text)
        # pi/omp tool names are lowercase (write, edit) while matchers use
        # Claude-style names (Write, Edit) — the regex must be case-insensitive
        # (live-verified June 2026: case-sensitive matcher never fired in omp).
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)
        # pi/omp tool input uses `path`, not `file_path`; the event must
        # normalize so hook scripts reading file_path keep working.
        self.assertIn("rawInput.file_path ?? rawInput.path ?? rawInput.notebook_path", text)

    def test_omp_extension_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "omp")
        ext = self.root / ".omp" / "extensions" / f"{self.shim_base}.ts"
        self.assertTrue(ext.is_file(), "omp extension should be generated")
        text = ext.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("@oh-my-pi/pi-coding-agent", text)
        self.assertIn("ExtensionAPI", text)
        self.assertIn('pi.on("tool_call"', text)
        self.assertIn("block: true", text)
        # omp exposes the tool name as camelCase `toolName` (same as pi, verified June 2026).
        self.assertIn("call?.toolName", text)
        # Live-verified June 2026 in omp: tool names arrive lowercase (write,
        # bash, read) and tool input uses `path`. Case-insensitive matcher +
        # input normalization are required for the gate to ever fire.
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)
        self.assertIn("rawInput.file_path ?? rawInput.path ?? rawInput.notebook_path", text)

    def test_unsupported_event_warns_and_skips(self):
        # `stop` has no opencode mapping -> warn + skip for opencode, present for claude
        result = self.seed_and_sync(RECIPE_STOP, "stopper", "opencode", "claude",
                                    event="stop", matcher="")
        # No plugin file emitted for opencode
        self.assertFalse((self.root / ".opencode" / "plugin" / f"{self.shim_base}.ts").exists())
        joined = result.stderr
        self.assertIn(RECIPE_STOP, joined)
        self.assertIn("stopper", joined)
        self.assertIn("stop", joined)
        self.assertIn("opencode", joined)
        # claude supports stop
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        self.assertIn("Stop", settings["hooks"])

    def test_cursor_no_prefilewrite_target_warns_and_skips(self):
        result = self.seed_and_sync(RECIPE_FILEWRITE, "worktree-gate", "cursor",
                                    matcher="Edit|Write|MultiEdit|NotebookEdit")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertFalse(wrapper.exists(), "cursor must skip file-write gates")
        joined = result.stderr.lower()
        self.assertIn("cursor", joined)
        self.assertIn("worktree-gate", result.stderr)

    def test_render_idempotent(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings_path = self.root / ".claude" / "settings.json"
        first = settings_path.read_bytes()
        self.sync()
        second = settings_path.read_bytes()
        self.assertEqual(first, second, "second render must be byte-identical")

    def test_user_hook_preserved(self):
        self._hook_id = "shell-gate"
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        claude_dir = self.root / ".claude"
        claude_dir.mkdir(parents=True)
        (claude_dir / "settings.json").write_text(json.dumps({
            "model": "opus",
            "hooks": {
                "PreToolUse": [
                    {"matcher": "UserOwned", "hooks": [
                        {"type": "command", "command": "/usr/bin/true"}
                    ]}
                ]
            }
        }, indent=2))
        self.write_manifest("claude")
        self.sync()
        settings = json.loads((claude_dir / "settings.json").read_text())
        # Sibling user key preserved
        self.assertEqual(settings["model"], "opus")
        matchers = [e.get("matcher") for e in settings["hooks"]["PreToolUse"]]
        self.assertIn("UserOwned", matchers)  # user hook preserved
        self.assertIn("Bash", matchers)       # managed hook added


class WorkspaceContextProcessBoundaryTests(_HookFixtureMixin, unittest.TestCase):
    """Deterministic Node process-boundary tests for generated adapters.

    Render the generated adapter for a real hook through ``bin/ai-specs sync``,
    relocate it into a temporary installation (as sync would materialize it),
    and execute it through the Node harness with a recording ``spawnSync``
    double. The assertions observe the actual process boundary: executable
    path, event cwd, options cwd, and fail-open behavior — never generated
    source text.

    Covers OpenCode directory normalization and child cwd (1.2), Pi/OMP
    module-location launcher paths with process-cwd events (1.3), and the
    preserved Claude/Cursor project-directory wiring (1.4).
    """

    RUNTIME_DIRS = {
        "opencode": Path(".opencode") / "plugin",
        "pi": Path(".pi") / "extensions",
        "omp": Path(".omp") / "extensions",
    }

    def setUp(self):
        super().setUp()
        self.assertTrue(NODE, "node is required for the process-boundary harness")
        self.assertTrue(NODE_HARNESS.is_file(),
                        f"missing harness fixture: {NODE_HARNESS.relative_to(ROOT)}")
        self.unrelated = self.base / "unrelated-cwd"
        self.unrelated.mkdir()
        # Node's chdir resolves symlinks (macOS /var -> /private/var), so the
        # process-cwd value the adapter sees is the real path.
        self.unrelated_real = str(self.unrelated.resolve())
        self.valid_dir = self.base / "valid-dir"
        self.valid_dir.mkdir()

    # --- helpers -----------------------------------------------------------

    def _render(self, agent: str) -> Path:
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", agent)
        return self.root / self.RUNTIME_DIRS[agent] / f"{self.shim_base}.ts"

    def _install(self) -> tuple[Path, Path]:
        """Temporary materialized installation: launcher + relocated adapter."""
        install = self.base / "install"
        launcher = install / self.script_path
        launcher.parent.mkdir(parents=True, exist_ok=True)
        launcher.write_text(HOOK_SCRIPT_BODY)
        launcher.chmod(0o755)
        return install, launcher

    def _relocate(self, agent: str, generated: Path, install: Path) -> Path:
        target = install / self.RUNTIME_DIRS[agent] / generated.name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(generated, target)
        return target

    def _run_harness(self, kind: str, plugin: Path, *,
                     directory=None, directory_json=None, tool=None,
                     input_json=None, args_json=None, status=None,
                     stderr=None, error=None, throw=None) -> dict:
        cmd = [NODE, str(NODE_HARNESS), "--kind", kind,
               "--plugin", str(plugin), "--cwd", str(self.unrelated)]
        if directory is not None:
            cmd += ["--directory", str(directory)]
        if directory_json is not None:
            cmd += ["--directory-json", json.dumps(directory_json)]
        if tool is not None:
            cmd += ["--tool", tool]
        if input_json is not None:
            cmd += ["--input-json", json.dumps(input_json)]
        if args_json is not None:
            cmd += ["--args-json", json.dumps(args_json)]
        if status is not None:
            cmd += ["--status", str(status)]
        if stderr is not None:
            cmd += ["--stderr", stderr]
        if error is not None:
            cmd += ["--error", error]
        if throw is not None:
            cmd += ["--throw", throw]
        proc = subprocess.run(cmd, capture_output=True, text=True, check=False,
                              input="")
        self.assertEqual(proc.returncode, 0,
                         f"harness failed: {proc.stderr}\nstdout: {proc.stdout}")
        return json.loads(proc.stdout)

    def _assert_single_record(self, res: dict) -> dict:
        self.assertEqual(len(res["records"]), 1, res)
        return res["records"][0]

    # --- OpenCode directory normalization and child cwd (1.2) --------------

    def test_opencode_valid_directory_trimmed_for_event_and_child_cwd(self):
        generated = self._render("opencode")
        install, launcher = self._install()
        relocated = self._relocate("opencode", generated, install)
        # Directory carries outer whitespace; the normalized value must drive
        # both the event cwd and the spawnSync child cwd.
        res = self._run_harness(
            "opencode", relocated,
            directory=f"  {self.valid_dir}  ",
            args_json={"file_path": "x.py"},
        )
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()),
                         "launcher path must be module-derived and absolute")
        self.assertEqual(rec["input"]["cwd"], str(self.valid_dir),
                         "event cwd must be the outer-trimmed directory")
        self.assertEqual(rec["optionsCwd"], str(self.valid_dir),
                         "child spawnSync cwd must equal the trimmed directory")
        self.assertFalse(res["outcome"]["threw"], res)

    def test_opencode_unrelated_process_cwd_still_finds_launcher(self):
        # Harness runs from unrelated-cwd; the valid directory is the event
        # target. The launcher must come from module location, not $PWD.
        generated = self._render("opencode")
        install, launcher = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated,
            directory=str(self.valid_dir),
            args_json={"file_path": "x.py"},
        )
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()))
        self.assertEqual(rec["optionsCwd"], str(self.valid_dir))

    def test_opencode_absent_directory_falls_back_to_process_cwd(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness("opencode", relocated, args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_non_string_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory_json=123,
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_whitespace_only_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory="   ",
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_relative_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory="relative/path",
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_nonexistent_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated,
            directory=str(self.base / "does-not-exist"),
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_non_directory_value_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        plain_file = self.base / "plain-file.txt"
        plain_file.write_text("x")
        res = self._run_harness(
            "opencode", relocated, directory=str(plain_file),
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_status_two_is_the_only_block(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            status=2, stderr="blocked by fixture",
            args_json={"file_path": "x.py"})
        self.assertTrue(res["outcome"]["threw"], res)
        self.assertIn("blocked by fixture", res["outcome"]["error"])

    def test_opencode_child_error_fails_open(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            error="spawn failed", args_json={"file_path": "x.py"})
        self.assertFalse(res["outcome"]["threw"],
                         "a child spawn error must fail open, not throw")

    def test_opencode_child_throw_fails_open(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            throw="child threw", args_json={"file_path": "x.py"})
        self.assertFalse(res["outcome"]["threw"],
                         "a thrown child-process exception must fail open")

    # --- Pi/OMP module-location launcher paths (1.3) -----------------------

    def _assert_process_cwd_event(self, res: dict, launcher: Path) -> None:
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()),
                         "pi/omp launcher must be module-derived and absolute")
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real,
                         "pi/omp event cwd must stay process.cwd()")
        self.assertEqual(rec["optionsCwd"], None,
                         "pi/omp child process must keep inheriting process cwd")
        self.assertFalse(res["outcome"]["block"], res)

    def test_pi_relocated_extension_finds_launcher_from_module_location(self):
        generated = self._render("pi")
        install, launcher = self._install()
        relocated = self._relocate("pi", generated, install)
        res = self._run_harness("pi", relocated, tool="bash",
                                input_json={"path": "x.py"})
        self._assert_process_cwd_event(res, launcher)
        self.assertEqual(res["outcome"]["block"], False)
        # No workspace root may be claimed: the event cwd must not be the
        # installation root.
        self.assertNotEqual(res["records"][0]["input"]["cwd"], str(install))

    def test_omp_relocated_extension_finds_launcher_from_module_location(self):
        generated = self._render("omp")
        install, launcher = self._install()
        relocated = self._relocate("omp", generated, install)
        res = self._run_harness("omp", relocated, tool="bash",
                                input_json={"path": "x.py"})
        self._assert_process_cwd_event(res, launcher)
        self.assertNotEqual(res["records"][0]["input"]["cwd"], str(install))

    # --- preserved Claude/Cursor wiring (1.4) ------------------------------

    def test_claude_script_keeps_project_dir_variable(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        entry = next(e for e in settings["hooks"]["PreToolUse"]
                     if e.get("matcher") == "Bash")
        cmd = entry["hooks"][0]["command"]
        self.assertIn("$CLAUDE_PROJECT_DIR/", cmd)
        self.assertIn(self.script_path, cmd)

    def test_cursor_wrapper_keeps_project_dir_variable(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "cursor")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertIn("$CURSOR_PROJECT_DIR/", wrapper.read_text())

    def test_cursor_filewrite_limitation_preserved(self):
        # The adapter change must not replace Cursor's missing pre-file-write
        # hook: a file-write matcher still warns and skips for cursor.
        result = self.seed_and_sync(RECIPE_FILEWRITE, "worktree-gate", "cursor",
                                    matcher="Edit|Write|MultiEdit|NotebookEdit")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertFalse(wrapper.exists())
        self.assertIn("no pre-file-write hook", result.stderr)


if __name__ == "__main__":
    unittest.main()

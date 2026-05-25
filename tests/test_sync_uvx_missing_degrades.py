"""Group 7 — `sync.sh` degrades to stdio with WARN when uvx is absent.

Spec: openspec/changes/mcp-compartido-por-proyecto/specs/mcp-shared-daemon/spec.md
      § "Comportamiento cuando uvx no está en PATH".

When at least one MCP has `mode = "shared"` post-merge (i.e. the
`proxy.named-config.json` is present after recipe-materialize) but `uvx` is
NOT on PATH, `ai-specs sync` SHALL:

  - emit a `WARN` mentioning `uvx`
  - skip the `ensure_daemon` step (no `proxy.pid` created)
  - rewrite the per-render recipe-mcp temp so `mcp-render` treats shared MCPs
    as stdio for THIS render only (no manifest mutation)
  - complete the pipeline with exit code 0

Tests pre-stage `proxy.named-config.json` to simulate the post-materialize
state (recipe-materialize has nothing to do because no recipes are enabled),
then drive sync with a minimal `PATH` that excludes `uvx`.
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


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"


def _ai_specs_init(workspace: Path) -> None:
    subprocess.run(
        [str(CLI), "init", str(workspace)],
        check=True, text=True, capture_output=True,
    )


def _write_manifest_with_shared_mcp(workspace: Path) -> None:
    """Manifest with claude enabled and a single shared MCP.

    `mode = "shared"` is preserved across syncs; the test asserts this
    survives the uvx-missing degrade path.
    """
    toml = workspace / "ai-specs" / "ai-specs.toml"
    toml.write_text(
        "[project]\n"
        "name = 'fixture-uvx-degrade'\n\n"
        "[agents]\n"
        "enabled = ['claude']\n\n"
        "[mcp.demo]\n"
        "command = 'echo'\n"
        "args = ['hi']\n"
        "env = { TOKEN = '$TOKEN' }\n"
        "mode = 'shared'\n"
        "enabled = true\n"
    )


def _stage_named_config(workspace: Path) -> Path:
    """Pre-create the post-materialize named-config to trigger the conditional."""
    run_dir = workspace / ".ai-specs" / "run"
    run_dir.mkdir(parents=True, exist_ok=True)
    named = run_dir / "proxy.named-config.json"
    named.write_text(json.dumps({
        "mcpServers": {"demo": {"command": "echo", "args": ["hi"]}}
    }))
    return named


def _path_without_uvx() -> str:
    """A PATH guaranteed not to contain uvx but with the system utilities
    sync.sh needs (bash, python3, git, mktemp, rm, mkdir, realpath).

    Build from the running interpreter's bindir plus directories on the
    current PATH that DO NOT contain `uvx`. On macOS the bare `/usr/bin`
    routes to Apple's 3.9 stub which lacks `tomllib`, so the interpreter's
    bindir must come first.
    """
    seen: set[str] = set()
    keep: list[str] = []
    py_bin = str(Path(sys.executable).resolve().parent)
    for d in [py_bin, *os.environ.get("PATH", "").split(os.pathsep), "/usr/bin", "/bin"]:
        if not d or d in seen:
            continue
        seen.add(d)
        if not Path(d).is_dir():
            continue
        if (Path(d) / "uvx").exists() or (Path(d) / "uvx").is_symlink():
            continue
        keep.append(d)
    return os.pathsep.join(keep)


def _run_sync(workspace: Path) -> subprocess.CompletedProcess[str]:
    env = {
        "PATH": _path_without_uvx(),
        "HOME": os.environ.get("HOME", str(workspace)),
        # Deliberately do NOT set AI_SPECS_MCP_DAEMON_FAKE — if the degrade
        # branch erroneously falls through to ensure_daemon, it must fail
        # loudly trying to spawn the real (absent) uvx, not silently succeed.
        "AI_SPECS_HOME": str(ROOT),
    }
    # Confirm uvx really is missing from the synthesized PATH.
    assert shutil.which("uvx", path=env["PATH"]) is None, (
        "test setup error: uvx unexpectedly present on PATH=" + env["PATH"]
    )
    return subprocess.run(
        [str(CLI), "sync", str(workspace)],
        env=env,
        capture_output=True,
        text=True,
        timeout=120,
    )


class SyncUvxMissingDegradesTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmpdir = tempfile.TemporaryDirectory(prefix="ai-specs-uvx-degrade-")
        self.addCleanup(self.tmpdir.cleanup)
        self.workspace = (Path(self.tmpdir.name) / "workspace").resolve()
        self.workspace.mkdir()
        _ai_specs_init(self.workspace)
        _write_manifest_with_shared_mcp(self.workspace)
        _stage_named_config(self.workspace)

    # -- bullet a -----------------------------------------------------------
    def test_sync_exits_zero_and_warns_and_renders_stdio(self) -> None:
        proc = _run_sync(self.workspace)

        combined = proc.stdout + proc.stderr
        self.assertEqual(
            proc.returncode, 0,
            msg=f"sync must exit 0 in degrade mode\nSTDOUT:\n{proc.stdout}\nSTDERR:\n{proc.stderr}",
        )
        self.assertIn("WARN", combined)
        self.assertIn("uvx", combined)

        # Claude's per-project .mcp.json must render the shared MCP as stdio
        # (command/args present, url absent).
        mcp_json = self.workspace / ".mcp.json"
        self.assertTrue(
            mcp_json.is_file(),
            f".mcp.json not generated; sync output:\n{combined}",
        )
        data = json.loads(mcp_json.read_text())
        self.assertIn("demo", data.get("mcpServers", {}))
        demo = data["mcpServers"]["demo"]
        self.assertEqual(demo.get("command"), "echo")
        self.assertEqual(demo.get("args"), ["hi"])
        self.assertNotIn(
            "url", demo,
            msg=f"shared MCP must degrade to stdio, not emit a url: {demo}",
        )
        # Internal `mode` key must NOT leak into the rendered agent config.
        self.assertNotIn("mode", demo)

    # -- bullet b -----------------------------------------------------------
    def test_daemon_not_invoked_when_uvx_absent(self) -> None:
        proc = _run_sync(self.workspace)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)

        run_dir = self.workspace / ".ai-specs" / "run"
        self.assertFalse(
            (run_dir / "proxy.pid").exists(),
            "proxy.pid must NOT be created when uvx is absent",
        )
        # The WARN branch must also clear the named-config so the next sync
        # (with uvx restored) re-triggers ensure_daemon afresh and the
        # daemon-running doctor check does not confuse a degraded sync
        # with a live daemon.
        self.assertFalse(
            (run_dir / "proxy.named-config.json").exists(),
            "named-config must be cleared on degrade",
        )
        combined = proc.stdout + proc.stderr
        self.assertNotIn(
            "ensure mcp-proxy daemon", combined,
            "ensure_daemon banner must NOT appear when uvx is missing",
        )

    # -- bullet c -----------------------------------------------------------
    def test_manifest_unchanged_after_degraded_sync(self) -> None:
        toml = self.workspace / "ai-specs" / "ai-specs.toml"
        before = toml.read_text()
        self.assertIn("mode = 'shared'", before)  # sanity

        proc = _run_sync(self.workspace)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)

        after = toml.read_text()
        self.assertEqual(
            before, after,
            "manifest must NOT be mutated by the degrade path "
            "(degradation is local to this render)",
        )


if __name__ == "__main__":
    unittest.main()

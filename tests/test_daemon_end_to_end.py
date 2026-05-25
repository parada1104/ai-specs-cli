"""Group 9.1-9.3 + 9.7 — daemon end-to-end happy path against real `uvx mcp-proxy`.

Covers:
  - bullet 1: sync starts the daemon; proxy.pid exists and the PID is alive.
  - bullet 2: GET http://localhost:{port}/status returns HTTP 200.
  - bullet 3: per-agent claude .mcp.json contains the URL pointing at the daemon.
  - bullet 7: `ai-specs daemon stop` SIGTERMs the proxy and removes state files.

Skipped at class level when `uvx` is not in PATH so CI environments without
`uv` installed still pass the suite.
"""
from __future__ import annotations

import json
import shutil
import tempfile
import unittest
from pathlib import Path

from _daemon_fixtures import (
    INNER_MCP_TIME,
    init_workspace,
    pid_alive,
    read_pid,
    read_port,
    reap_proxy,
    run_daemon_stop,
    run_sync,
    stage_named_config,
    uvx_available,
    wait_for_status,
    write_manifest_toml,
)


MANIFEST_BODY = """\
[project]
name = "g9-end-to-end"

[agents]
enabled = ["claude"]

[mcp.time]
command = "uvx"
args = ["mcp-server-time"]
mode = "shared"
"""


@unittest.skipUnless(uvx_available(), "uvx not in PATH — G9 integration suite requires uv")
class DaemonEndToEndTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-g9-e2e-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = init_workspace(Path(self.tmp.name))
        # Reap MUST be registered before sync so a failure mid-sync still
        # terminates any spawned daemon — otherwise it leaks across tests.
        self.addCleanup(reap_proxy, self.workspace)

        write_manifest_toml(self.workspace, MANIFEST_BODY)
        # Manifest-only shared MCPs do NOT get a named-config from
        # recipe-materialize (which only writes when a recipe is enabled),
        # so we pre-stage it ourselves with the real inner stdio MCP.
        stage_named_config(self.workspace, {"time": INNER_MCP_TIME})

    # ---- bullets 1 + 2 ----------------------------------------------------

    def test_sync_spawns_daemon_and_status_endpoint_returns_200(self) -> None:
        proc = run_sync(self.workspace)
        self.assertEqual(
            proc.returncode, 0,
            msg=f"sync exited {proc.returncode}\nstdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
        )

        pid_file = self.workspace / ".ai-specs" / "run" / "proxy.pid"
        port_file = self.workspace / ".ai-specs" / "run" / "proxy.port"
        self.assertTrue(pid_file.is_file(), f"missing {pid_file}; stdout:\n{proc.stdout}")
        self.assertTrue(port_file.is_file(), f"missing {port_file}")

        pid = read_pid(self.workspace)
        self.assertIsNotNone(pid)
        self.assertTrue(pid_alive(pid), f"PID {pid} not alive after sync")

        port = read_port(self.workspace)
        self.assertIsNotNone(port)

        resp = wait_for_status(port)
        self.assertEqual(resp.status, 200)
        # The proxy's /status payload includes a `server_instances` map keyed
        # by configured MCP id — a sanity check that our pre-staged config
        # actually loaded into the running proxy.
        body = json.loads(resp.body)
        self.assertIn("server_instances", body, msg=f"unexpected /status body: {body!r}")
        self.assertIn("time", body["server_instances"])

    # ---- bullet 3 ---------------------------------------------------------

    def test_claude_mcp_config_contains_proxy_url_for_shared_mcp(self) -> None:
        proc = run_sync(self.workspace)
        self.assertEqual(
            proc.returncode, 0,
            msg=f"sync failed\nstdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
        )
        port = read_port(self.workspace)
        self.assertIsNotNone(port, "port file missing after sync")

        mcp_json = self.workspace / ".mcp.json"
        self.assertTrue(mcp_json.is_file(), f"missing {mcp_json}")
        data = json.loads(mcp_json.read_text())
        # Claude/Cursor schema: top-level "mcpServers".
        self.assertIn("mcpServers", data)
        self.assertIn("time", data["mcpServers"])
        entry = data["mcpServers"]["time"]
        # Shared MCP MUST render as a bare URL entry — command/args dropped.
        self.assertEqual(
            entry.get("url"),
            f"http://localhost:{port}/servers/time/mcp",
            msg=f"unexpected claude render: {entry!r}",
        )
        self.assertNotIn("command", entry)
        self.assertNotIn("args", entry)

    # ---- bullet 7 ---------------------------------------------------------

    def test_daemon_stop_terminates_proxy_and_cleans_state_files(self) -> None:
        proc = run_sync(self.workspace)
        self.assertEqual(
            proc.returncode, 0, msg=proc.stdout + proc.stderr,
        )
        pid = read_pid(self.workspace)
        self.assertIsNotNone(pid)
        self.assertTrue(pid_alive(pid))

        stop = run_daemon_stop(self.workspace)
        self.assertEqual(
            stop.returncode, 0,
            msg=f"daemon stop exit {stop.returncode}\nstdout:\n{stop.stdout}\nstderr:\n{stop.stderr}",
        )

        # The PID must be reaped (process gone) and state files removed.
        # `_is_pid_alive` may briefly observe lingering kernel state on macOS;
        # poll a short window before asserting.
        import time as _t
        deadline = _t.monotonic() + 3.0
        while _t.monotonic() < deadline and pid_alive(pid):
            _t.sleep(0.05)
        self.assertFalse(pid_alive(pid), f"PID {pid} still alive after daemon stop")

        run_dir = self.workspace / ".ai-specs" / "run"
        self.assertFalse((run_dir / "proxy.pid").exists())
        self.assertFalse((run_dir / "proxy.port").exists())
        # Per spec: stop also removes proxy.named-config.json.
        self.assertFalse((run_dir / "proxy.named-config.json").exists())


if __name__ == "__main__":
    unittest.main()

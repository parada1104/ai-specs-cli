"""Group 9.4 — daemon idempotency: a second sync reuses the running daemon.

Per spec `mcp-shared-daemon` ("Segunda sync reutiliza daemon sin recrear"):
  - WHEN ai-specs sync runs twice with an unchanged manifest
  - AND the first sync's daemon is still alive and healthy
  - THEN the proxy.pid stays the same AND only one mcp-proxy process exists.

The "only one process" assertion is scoped to the PID file under this fixture's
state dir — not a hostwide `pgrep` — so the test can run alongside the user's
own daemons without false positives.
"""
from __future__ import annotations

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
    run_sync,
    stage_named_config,
    uvx_available,
    wait_for_status,
    write_manifest_toml,
)


MANIFEST_BODY = """\
[project]
name = "g9-idempotency"

[agents]
enabled = ["claude"]

[mcp.time]
command = "uvx"
args = ["mcp-server-time"]
mode = "shared"
"""


@unittest.skipUnless(uvx_available(), "uvx not in PATH — G9 integration suite requires uv")
class DaemonIdempotencyTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-g9-idem-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = init_workspace(Path(self.tmp.name))
        self.addCleanup(reap_proxy, self.workspace)
        write_manifest_toml(self.workspace, MANIFEST_BODY)
        stage_named_config(self.workspace, {"time": INNER_MCP_TIME})

    def test_second_sync_reuses_same_daemon_pid(self) -> None:
        # First sync — spawn fresh daemon.
        proc1 = run_sync(self.workspace)
        self.assertEqual(proc1.returncode, 0, msg=proc1.stdout + proc1.stderr)
        pid1 = read_pid(self.workspace)
        port1 = read_port(self.workspace)
        self.assertIsNotNone(pid1)
        self.assertIsNotNone(port1)
        self.assertTrue(pid_alive(pid1))
        # Make sure /status answers before issuing the second sync — otherwise
        # ensure_daemon may treat the daemon as unhealthy and respawn.
        wait_for_status(port1)

        # Second sync — identical manifest, identical named-config.
        proc2 = run_sync(self.workspace)
        self.assertEqual(proc2.returncode, 0, msg=proc2.stdout + proc2.stderr)
        pid2 = read_pid(self.workspace)
        port2 = read_port(self.workspace)

        self.assertEqual(
            pid1, pid2,
            msg=f"daemon respawned: pid1={pid1} pid2={pid2}\nstdout:\n{proc2.stdout}",
        )
        self.assertEqual(port1, port2, "port changed across syncs")
        self.assertTrue(pid_alive(pid2), f"PID {pid2} not alive after second sync")


if __name__ == "__main__":
    unittest.main()

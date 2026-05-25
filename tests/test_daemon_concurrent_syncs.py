"""Group 9.5 — concurrent syncs serialize via the per-state-dir file lock.

Two `ai-specs sync` processes start in parallel against the same workspace
(same `.ai-specs/run/proxy.lock`). The `_acquire_lock` `fcntl.flock` exclusion
inside `mcp-daemon.ensure_daemon` MUST serialize them so that exactly one
mcp-proxy process is spawned and both syncs observe the same proxy.pid.

NOTE on scope: the spec scenario "Worktrees comparten daemon" calls for a
shared identity across `git worktree`s of the same repo. The current
implementation keys the daemon by the ai-specs root path (= worktree path),
so cross-worktree sharing is a known gap to be closed in a follow-up. This
test covers the equivalent contract enforced by the existing implementation:
concurrent ensure_daemon calls against the SAME state dir collapse to one
process — which is what the file lock is for.
"""
from __future__ import annotations

import concurrent.futures
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
    write_manifest_toml,
)


MANIFEST_BODY = """\
[project]
name = "g9-concurrent"

[agents]
enabled = ["claude"]

[mcp.time]
command = "uvx"
args = ["mcp-server-time"]
mode = "shared"
"""


@unittest.skipUnless(uvx_available(), "uvx not in PATH — G9 integration suite requires uv")
class ConcurrentSyncsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-g9-concur-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = init_workspace(Path(self.tmp.name))
        self.addCleanup(reap_proxy, self.workspace)
        write_manifest_toml(self.workspace, MANIFEST_BODY)
        stage_named_config(self.workspace, {"time": INNER_MCP_TIME})

    def test_two_parallel_syncs_share_one_daemon(self) -> None:
        # Run both syncs from the same workspace in parallel threads — each
        # one shells out to bin/ai-specs sync, which acquires the per-state-dir
        # flock inside ensure_daemon. Only one ought to spawn a proxy.
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            futures = [pool.submit(run_sync, self.workspace) for _ in range(2)]
            results = [f.result(timeout=180) for f in futures]

        for i, proc in enumerate(results):
            self.assertEqual(
                proc.returncode, 0,
                msg=f"concurrent sync #{i} failed (exit {proc.returncode})\n"
                    f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
            )

        pid = read_pid(self.workspace)
        port = read_port(self.workspace)
        self.assertIsNotNone(pid)
        self.assertIsNotNone(port)
        self.assertTrue(pid_alive(pid), f"PID {pid} not alive after concurrent syncs")

        # Sanity: there is exactly ONE mcp-proxy process belonging to this
        # workspace's state dir. We scope the check to our pid (and its
        # process group) — pgrep across the whole host would catch the
        # operator's unrelated daemons.
        import os
        import subprocess
        try:
            pgid = os.getpgid(pid)
        except ProcessLookupError:
            self.fail(f"PID {pid} disappeared before pgrep check")
        # `ps -g <pgid> -o pid=` lists every PID under our session leader.
        # `uvx mcp-proxy` forks twice (the `uvx` launcher + the python child
        # it execs into), so raw `mcp-proxy`-name matches would count to 2
        # for a single logical daemon. We scope by the `--port <port>` flag
        # instead — every spawn passes its assigned port on the command line,
        # so distinct ports ⇔ distinct logical daemons. A second-spawn
        # racing past the lock would surface as a SECOND `--port` value.
        ps = subprocess.run(
            ["ps", "-g", str(pgid), "-o", "pid=,command="],
            capture_output=True, text=True, check=False,
        )
        ports_seen: set[str] = set()
        for line in ps.stdout.splitlines():
            if "mcp-proxy" not in line or "--port" not in line:
                continue
            tokens = line.split()
            for i, tok in enumerate(tokens):
                if tok == "--port" and i + 1 < len(tokens):
                    ports_seen.add(tokens[i + 1])
                    break
        self.assertEqual(
            len(ports_seen), 1,
            msg=f"expected 1 unique --port in pgid {pgid}, got {sorted(ports_seen)}\n"
                f"ps output:\n{ps.stdout}",
        )
        self.assertIn(
            str(port), ports_seen,
            msg=f"recorded proxy.port={port} not present among live procs: {ports_seen}",
        )


if __name__ == "__main__":
    unittest.main()

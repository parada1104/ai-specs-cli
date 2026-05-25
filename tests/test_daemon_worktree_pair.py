"""Group 9.6 — daemon identity by canonical git root: worktrees share one daemon.

Per spec `mcp-shared-daemon` Requirement "Identidad del daemon por raíz git",
Scenario "Worktrees comparten daemon":

    Two worktrees of the same git repository running `ai-specs sync` with
    shared MCPs SHALL use the same `mcp-proxy` process. The number of active
    daemons for that repository SHALL be 1.

Mechanically this requires the four daemon-identity sites
(`recipe-materialize.py`, `mcp-render.py`, `doctor.py`, `daemon.sh`) and
`sync.sh`'s ROOT_PATH-derived `PROXY_NAMED_CONFIG` to key off
`git rev-parse --git-common-dir` (whose parent is the canonical repo root
shared by all worktrees) — not `--show-toplevel`, which returns the worktree
path and would yield a daemon per worktree.

This is the regression pin for that fix. It is uvx-gated (real `uvx mcp-proxy`
spawn) and reaps the daemon in tearDown.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

from _daemon_fixtures import (
    CLI,
    INNER_MCP_TIME,
    ROOT,
    pid_alive,
    reap_proxy,
    uvx_available,
    wait_for_status,
)


MANIFEST_BODY = """\
[project]
name = "g9-worktree-pair"

[agents]
enabled = ["claude"]

[mcp.time]
command = "uvx"
args = ["mcp-server-time"]
mode = "shared"
"""


def _run_sync(workspace: Path, *, timeout: int = 180) -> subprocess.CompletedProcess:
    """Invoke ``ai-specs sync <workspace>`` with the canonical environment."""
    env = dict(os.environ)
    env["AI_SPECS_HOME"] = str(ROOT)
    env.pop("AI_SPECS_MCP_DAEMON_FAKE", None)
    return subprocess.run(
        [str(CLI), "sync", str(workspace)],
        env=env, capture_output=True, text=True, timeout=timeout,
    )


@unittest.skipUnless(uvx_available(), "uvx not in PATH — G9 integration suite requires uv")
class WorktreePairSharesDaemonTests(unittest.TestCase):
    """Two worktrees of the same repo MUST share one mcp-proxy daemon."""

    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-g9-wtpair-")
        self.addCleanup(self.tmp.cleanup)
        tmp_root = Path(self.tmp.name).resolve()

        # Primary worktree IS the canonical repo: `git init` here and the
        # git_common_dir's parent will resolve to this path from any linked
        # worktree.
        self.primary = (tmp_root / "primary").resolve()
        self.primary.mkdir()
        subprocess.run([str(CLI), "init", str(self.primary)],
                       check=True, text=True, capture_output=True)
        subprocess.run(["git", "init", "-q", "-b", "main", str(self.primary)],
                       check=True, capture_output=True)
        for k, v in (("user.email", "g9@test"), ("user.name", "g9-test")):
            subprocess.run(["git", "-C", str(self.primary), "config", k, v],
                           check=True, capture_output=True)

        # Overwrite the manifest with our shared-MCP fixture before committing
        # so the secondary worktree inherits it.
        (self.primary / "ai-specs" / "ai-specs.toml").write_text(MANIFEST_BODY)

        subprocess.run(["git", "-C", str(self.primary), "add", "-A"],
                       check=True, capture_output=True)
        subprocess.run(
            ["git", "-C", str(self.primary), "commit", "-q", "-m", "init"],
            check=True, capture_output=True,
        )

        # Linked worktree at <tmp>/secondary checking out a new branch.
        self.secondary = (tmp_root / "secondary").resolve()
        subprocess.run(
            ["git", "-C", str(self.primary), "worktree", "add",
             "-b", "wt-secondary", str(self.secondary)],
            check=True, capture_output=True,
        )

        # Canonical git root resolves from both worktrees to the same path
        # (the primary). Sanity-check the prerequisite up-front so a regression
        # in the test environment surfaces clearly.
        def _git_root(cwd: Path) -> Path:
            common = subprocess.run(
                ["git", "-C", str(cwd), "rev-parse",
                 "--path-format=absolute", "--git-common-dir"],
                check=True, capture_output=True, text=True,
            ).stdout.strip()
            return Path(common).parent.resolve()

        self.git_root = _git_root(self.primary)
        self.assertEqual(self.git_root, self.primary,
                         "primary IS the canonical repo in this fixture")
        self.assertEqual(_git_root(self.secondary), self.primary,
                         "secondary worktree must resolve to the same canonical root")

        # Pre-stage the named-config at the CANONICAL location so the first
        # sync triggers ensure_daemon without needing a recipe with shared MCPs.
        # The named-config writer in recipe-materialize would otherwise place
        # this file at the same path once the fix is in place.
        run_dir = self.git_root / ".ai-specs" / "run"
        run_dir.mkdir(parents=True, exist_ok=True)
        named = run_dir / "proxy.named-config.json"
        import json as _json
        named.write_text(_json.dumps({"mcpServers": {"time": INNER_MCP_TIME}}, indent=2) + "\n")
        os.chmod(named, 0o600)

        # Reap whatever daemon the syncs spawn, scoped to the canonical run dir.
        self.addCleanup(reap_proxy, self.git_root)

    def test_sync_from_two_worktrees_reuses_one_daemon(self) -> None:
        # First sync from the primary worktree. Spawns the daemon.
        proc_primary = _run_sync(self.primary)
        self.assertEqual(
            proc_primary.returncode, 0,
            msg=f"primary sync failed\nstdout:\n{proc_primary.stdout}"
                f"\nstderr:\n{proc_primary.stderr}",
        )

        pid_path = self.git_root / ".ai-specs" / "run" / "proxy.pid"
        port_path = self.git_root / ".ai-specs" / "run" / "proxy.port"
        self.assertTrue(pid_path.is_file(),
                        f"proxy.pid must live at canonical repo, not in a worktree: {pid_path}")
        self.assertTrue(port_path.is_file(), f"proxy.port missing at {port_path}")

        # The PID/port file MUST NOT exist under either worktree's own
        # `.ai-specs/run/` — that would prove the daemon was keyed by
        # show-toplevel and not by git_common_dir.
        for label, wt in (("primary", self.primary), ("secondary", self.secondary)):
            if wt == self.git_root:
                continue  # primary IS the canonical root in this fixture
            worktree_pid = wt / ".ai-specs" / "run" / "proxy.pid"
            self.assertFalse(
                worktree_pid.is_file(),
                f"{label} worktree wrote its own proxy.pid at {worktree_pid} — "
                f"daemon is still keyed by --show-toplevel, not --git-common-dir",
            )

        pid_after_primary = int(pid_path.read_text().strip())
        port_after_primary = int(port_path.read_text().strip())
        self.assertTrue(pid_alive(pid_after_primary),
                        f"daemon PID {pid_after_primary} not alive after primary sync")

        # Wait for /status so the second sync's healthcheck sees a ready daemon.
        wait_for_status(port_after_primary)

        # Second sync from the secondary worktree. Must reuse the same daemon.
        proc_secondary = _run_sync(self.secondary)
        self.assertEqual(
            proc_secondary.returncode, 0,
            msg=f"secondary sync failed\nstdout:\n{proc_secondary.stdout}"
                f"\nstderr:\n{proc_secondary.stderr}",
        )

        # Still exactly one PID file, at the canonical location.
        self.assertTrue(pid_path.is_file())
        pid_after_secondary = int(pid_path.read_text().strip())
        port_after_secondary = int(port_path.read_text().strip())

        self.assertEqual(
            pid_after_primary, pid_after_secondary,
            msg=f"daemon respawned across worktrees: pid1={pid_after_primary} "
                f"pid2={pid_after_secondary}\nsecondary stdout:\n{proc_secondary.stdout}",
        )
        self.assertEqual(port_after_primary, port_after_secondary,
                         "port changed across worktree syncs")
        self.assertTrue(pid_alive(pid_after_secondary),
                        f"PID {pid_after_secondary} not alive after secondary sync")

        # Secondary MUST NOT have planted a competing pid file.
        self.assertFalse(
            (self.secondary / ".ai-specs" / "run" / "proxy.pid").is_file(),
            "secondary worktree wrote a competing proxy.pid — daemon identity is still per-worktree",
        )


if __name__ == "__main__":
    unittest.main()

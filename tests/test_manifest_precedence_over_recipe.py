"""Group 9.6 — manifest `mode = "stdio"` overrides recipe `mode = "shared"`.

The bundled `trello-mcp-workflow` recipe declares `[[provides.mcp]] id = "trello"
mode = "shared"`. When the project's `[mcp.trello]` table sets `mode = "stdio"`,
the recipe-materialize shallow-merge rule (manifest keys win on conflicts)
demotes trello back to stdio. The pipeline-level consequence:

  * `recipe-materialize` does NOT write `.ai-specs/run/proxy.named-config.json`
    (because no shared MCP survives the merge).
  * `sync.sh` skips the ensure-daemon step (no `proxy.pid` is created).
  * Per-agent `.mcp.json` (claude) renders trello as a stdio entry — bare
    `command`/`args`/`env`, NOT a `url`.

This guards the precedence rule end-to-end so a future change to
`split_mcps_by_mode`, `load_mcp`, or `sync.sh`'s detection of the named-config
file cannot silently invert the manifest's intent.
"""
from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

from _daemon_fixtures import (
    init_workspace,
    reap_proxy,
    run_sync,
    write_manifest_toml,
)


# Manifest pins trello to stdio + supplies its own command. The trello
# recipe's `mode = "shared"` field MUST lose this contest.
MANIFEST_BODY = """\
[project]
name = "g9-precedence"

[agents]
enabled = ["claude"]

[recipes.trello-mcp-workflow]
enabled = true
version = "1.0.0"
config = { board_id = "507f1f77bcf86cd799439011" }

[mcp.trello]
command = "echo"
args = ["stdio-only"]
mode = "stdio"
"""


class ManifestPrecedenceOverRecipeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-g9-precedence-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = init_workspace(Path(self.tmp.name))
        # Safety net: even though we expect NO daemon spawn, reap-on-cleanup
        # makes the test resilient to regressions that would leak a process.
        self.addCleanup(reap_proxy, self.workspace)
        write_manifest_toml(self.workspace, MANIFEST_BODY)

    def test_manifest_stdio_overrides_recipe_shared_end_to_end(self) -> None:
        proc = run_sync(self.workspace)
        self.assertEqual(
            proc.returncode, 0,
            msg=f"sync failed (exit {proc.returncode})\n"
                f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
        )

        run_dir = self.workspace / ".ai-specs" / "run"
        named = run_dir / "proxy.named-config.json"
        self.assertFalse(
            named.exists(),
            msg=f"named-config must NOT be written when no shared MCP survives merge; "
                f"contents: {named.read_text() if named.exists() else '<missing>'}",
        )

        pid_file = run_dir / "proxy.pid"
        self.assertFalse(pid_file.exists(), "daemon must not be started when no shared MCP")
        # And of course no ensure-daemon banner in the sync output.
        self.assertNotIn("ensure mcp-proxy daemon", proc.stdout)

        # Per-agent render: trello must be stdio shape, not a URL entry.
        mcp_json = self.workspace / ".mcp.json"
        self.assertTrue(mcp_json.is_file(), f"missing {mcp_json}")
        data = json.loads(mcp_json.read_text())
        self.assertIn("mcpServers", data)
        self.assertIn("trello", data["mcpServers"])
        entry = data["mcpServers"]["trello"]
        # Manifest's command/args win; recipe's npx+@delorenj is overridden.
        self.assertEqual(entry.get("command"), "echo")
        self.assertEqual(entry.get("args"), ["stdio-only"])
        self.assertNotIn("url", entry, f"trello rendered as URL despite stdio override: {entry!r}")
        # `mode` is stripped before serialization (it is an ai-specs-internal key).
        self.assertNotIn("mode", entry)


if __name__ == "__main__":
    unittest.main()

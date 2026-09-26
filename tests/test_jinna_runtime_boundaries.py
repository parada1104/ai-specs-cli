#!/usr/bin/env python3
"""Runtime-boundary contract for the Jinna recipe docs.

The Jinna recipe is a local provider installation and MCP materialization
path, not a remote OpenProject client. Its provider-facing docs must keep the
local/live boundary explicit:

* ``ai-specs sync`` and ``ai-specs doctor`` are local materialization checks
  and never contact OpenProject;
* ``jinna health`` and ``jinna whoami`` are explicit opt-in live diagnostics;
* the recipe's 30-second MCP timeout bounds local server startup/transport,
  not the remote OpenProject API SLA;
* slow or unavailable self-hosted service behavior stays visible and must not
  be hidden by automatic retries, proxy fallback, endpoint switching, or write
  replay.

The no-fallback wording already shipped in both docs is pinned here so a later
edit cannot quietly drop it.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE = ROOT / "catalog" / "recipes" / "jinna-mcp-recipe"
README = RECIPE / "README.md"
SKILL = RECIPE / "skills" / "jinna-mcp-recipe" / "SKILL.md"

# Boundary statements every provider-facing doc must carry (matched case- and
# whitespace-insensitively so markdown wrapping cannot hide a regression).
BOUNDARY_PHRASES = (
    "ai-specs sync",
    "ai-specs doctor",
    "do not contact openproject",
    "jinna health",
    "jinna whoami",
    "opt-in",
    "30-second mcp timeout",
    "not the remote openproject api sla",
    "slow",
    "unavailable",
    "automatic retries",
    "proxy",
    "endpoint switching",
    "write replay",
)

# The exact no-fallback sentences already shipped, preserved verbatim.
NO_FALLBACK_WORDING = {
    README: "never proxies, automatically switches, or replays failed writes",
    SKILL: "do not automatically proxy, switch, or replay writes",
}


def _normalized(path: Path) -> str:
    text = path.read_text(encoding="utf-8").lower()
    return re.sub(r"\s+", " ", text)


class JinnaRuntimeBoundaryDocTests(unittest.TestCase):
    def test_docs_keep_the_local_live_runtime_boundary(self):
        for path in (README, SKILL):
            with self.subTest(doc=path.name):
                text = _normalized(path)
                for phrase in BOUNDARY_PHRASES:
                    self.assertIn(
                        phrase, text, f"{path.name} must state the boundary: {phrase!r}"
                    )

    def test_docs_preserve_the_no_fallback_wording(self):
        for path, wording in NO_FALLBACK_WORDING.items():
            with self.subTest(doc=path.name):
                self.assertIn(wording, _normalized(path))


if __name__ == "__main__":
    unittest.main()

"""Group 4: render shared MCPs as url for HTTP agents, stdio fallback for codex/gemini.

Covers tasks 4.1–4.9 in openspec/changes/mcp-compartido-por-proyecto/tasks.md.
"""

import importlib.util
import json
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

MODULE_PATH = (
    Path(__file__).resolve().parents[1] / "lib" / "_internal" / "mcp-render.py"
)


def _load_module():
    spec = importlib.util.spec_from_file_location("mcp_render_under_test", MODULE_PATH)
    assert spec is not None and spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _git_init(path: Path) -> None:
    subprocess.run(
        ["git", "init", "-q", str(path)],
        check=True,
        capture_output=True,
    )


def _write_toml(path: Path, body: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(body)


STDIO_TRELLO_SHARED = {
    "trello": {
        "mode": "shared",
        "command": "npx",
        "args": ["-y", "@trello/mcp"],
        "env": {"TOKEN": "$TOKEN"},
    }
}

STDIO_GITHUB_PLAIN = {
    "github": {
        "command": "npx",
        "args": ["-y", "@github/mcp"],
        "env": {"TOKEN": "$TOKEN"},
    }
}


# --- 4.1 -------------------------------------------------------------------


class ResolveProxyPortTests(unittest.TestCase):
    """Task 4.1 — _resolve_proxy_port + _render_url_entry."""

    def setUp(self) -> None:
        self.mod = _load_module()
        self.tmp = Path(tempfile.mkdtemp(prefix="mcp-render-resolve-"))
        _git_init(self.tmp)
        self.run_dir = self.tmp / ".ai-specs" / "run"
        self.run_dir.mkdir(parents=True)

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp, ignore_errors=True)

    def test_reads_int_when_file_present(self) -> None:
        (self.run_dir / "proxy.port").write_text("54321\n")
        port = self.mod._resolve_proxy_port(self.tmp)
        self.assertEqual(port, 54321)

    def test_raises_explicit_when_port_file_missing(self) -> None:
        with self.assertRaises(Exception) as ctx:
            self.mod._resolve_proxy_port(self.tmp)
        msg = str(ctx.exception)
        self.assertIn("proxy.port", msg)
        # explicit hint that the daemon is not started
        self.assertTrue(
            "daemon" in msg.lower() or "not been started" in msg.lower(),
            f"expected daemon hint, got: {msg!r}",
        )

    def test_render_url_entry_shape(self) -> None:
        entry = self.mod._render_url_entry("trello", 54321)
        self.assertEqual(
            entry, {"url": "http://localhost:54321/servers/trello/mcp"}
        )


# --- 4.2 — 4.6 (translate_servers per agent) -------------------------------


class TranslateSharedPerAgentTests(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = _load_module()

    # 4.2
    def test_claude_shared_emits_url_only(self) -> None:
        out = self.mod.translate_servers("claude", STDIO_TRELLO_SHARED, port=54321)
        entry = out["trello"]
        self.assertEqual(
            entry, {"url": "http://localhost:54321/servers/trello/mcp"}
        )
        for forbidden in ("command", "args", "env", "mode"):
            self.assertNotIn(forbidden, entry)

    # 4.3
    def test_cursor_shared_emits_url_only(self) -> None:
        out = self.mod.translate_servers("cursor", STDIO_TRELLO_SHARED, port=54321)
        entry = out["trello"]
        self.assertEqual(
            entry["url"], "http://localhost:54321/servers/trello/mcp"
        )
        for forbidden in ("command", "args", "env", "mode"):
            self.assertNotIn(forbidden, entry)

    # 4.4
    def test_opencode_shared_emits_remote_url(self) -> None:
        out = self.mod.translate_servers(
            "opencode", STDIO_TRELLO_SHARED, port=54321
        )
        entry = out["trello"]
        self.assertEqual(
            entry,
            {"type": "remote", "url": "http://localhost:54321/servers/trello/mcp"},
        )
        for forbidden in ("command", "environment", "mode"):
            self.assertNotIn(forbidden, entry)

    # 4.5
    def test_codex_shared_falls_back_to_stdio(self) -> None:
        out = self.mod.translate_servers("codex", STDIO_TRELLO_SHARED, port=54321)
        entry = out["trello"]
        self.assertEqual(entry["command"], "npx")
        self.assertEqual(entry["args"], ["-y", "@trello/mcp"])
        self.assertIn("env", entry)
        self.assertNotIn("url", entry)
        self.assertNotIn("mode", entry)

    # 4.6
    def test_gemini_shared_falls_back_to_stdio(self) -> None:
        out = self.mod.translate_servers("gemini", STDIO_TRELLO_SHARED, port=54321)
        entry = out["trello"]
        self.assertEqual(entry["command"], "npx")
        self.assertEqual(entry["args"], ["-y", "@trello/mcp"])
        self.assertNotIn("url", entry)
        self.assertNotIn("mode", entry)


# --- 4.7 (byte-identical for plain stdio MCP across agents) ----------------


class StdioUntouchedTests(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = _load_module()

    def test_claude_stdio_matches_pre_change_shape(self) -> None:
        out = self.mod.translate_servers("claude", STDIO_GITHUB_PLAIN, port=None)
        self.assertEqual(
            out,
            {
                "github": {
                    "command": "npx",
                    "args": ["-y", "@github/mcp"],
                    "env": {"TOKEN": "${TOKEN}"},
                }
            },
        )

    def test_opencode_stdio_matches_pre_change_shape(self) -> None:
        out = self.mod.translate_servers(
            "opencode", STDIO_GITHUB_PLAIN, port=None
        )
        self.assertEqual(
            out,
            {
                "github": {
                    "type": "local",
                    "command": ["npx", "-y", "@github/mcp"],
                    "environment": {"TOKEN": "{env:TOKEN}"},
                }
            },
        )

    def test_codex_stdio_unchanged(self) -> None:
        out = self.mod.translate_servers("codex", STDIO_GITHUB_PLAIN, port=None)
        self.assertEqual(out["github"]["command"], "npx")
        self.assertNotIn("url", out["github"])
        self.assertNotIn("mode", out["github"])


# --- 4.8 (port resolved exactly once / skipped when zero shared) ----------


class MainPortResolutionTests(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = _load_module()
        self.tmp = Path(tempfile.mkdtemp(prefix="mcp-render-main-"))
        _git_init(self.tmp)
        run_dir = self.tmp / ".ai-specs" / "run"
        run_dir.mkdir(parents=True)
        self.port_file = run_dir / "proxy.port"
        self.port_file.write_text("54321\n")
        self.toml = self.tmp / "ai-specs" / "ai-specs.toml"

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp, ignore_errors=True)

    def _invoke(self, agent: str, target_rel: str, mcp_key: str) -> int:
        target = self.tmp / target_rel
        with mock.patch.object(
            sys,
            "argv",
            ["mcp-render.py", str(self.toml), agent, str(target), mcp_key],
        ):
            return self.mod.main()

    def _spy_on_resolve(self) -> list:
        calls: list = []
        orig = self.mod._resolve_proxy_port

        def spy(project_root):
            calls.append(project_root)
            return orig(project_root)

        self.mod._resolve_proxy_port = spy
        self.addCleanup(setattr, self.mod, "_resolve_proxy_port", orig)
        return calls

    def test_port_resolved_exactly_once_when_shared_present(self) -> None:
        _write_toml(
            self.toml,
            '[mcp.trello]\n'
            'mode = "shared"\n'
            'command = "npx"\n'
            'args = ["-y", "@trello/mcp"]\n'
            '\n'
            '[mcp.github]\n'
            'mode = "stdio"\n'
            'command = "npx"\n'
            'args = ["-y", "@github/mcp"]\n',
        )
        calls = self._spy_on_resolve()
        rc = self._invoke("claude", ".mcp.json", "mcpServers")
        self.assertEqual(rc, 0)
        self.assertEqual(
            len(calls), 1, f"expected exactly one port read, got {len(calls)}"
        )

    def test_port_not_read_when_zero_shared(self) -> None:
        _write_toml(
            self.toml,
            '[mcp.github]\n'
            'command = "npx"\n'
            'args = ["-y", "@github/mcp"]\n',
        )
        # Make sure a stray read would fail loudly.
        self.port_file.unlink()
        calls = self._spy_on_resolve()
        rc = self._invoke("claude", ".mcp.json", "mcpServers")
        self.assertEqual(rc, 0)
        self.assertEqual(
            calls, [], "must not resolve proxy.port when no shared MCPs"
        )

    def test_main_fails_when_shared_present_but_port_missing(self) -> None:
        _write_toml(
            self.toml,
            '[mcp.trello]\n'
            'mode = "shared"\n'
            'command = "npx"\n'
            'args = ["-y", "@trello/mcp"]\n',
        )
        self.port_file.unlink()
        rc = self._invoke("claude", ".mcp.json", "mcpServers")
        self.assertNotEqual(rc, 0)


# --- 4.9 (final agent config files never contain `mode`) ------------------


class FinalConfigStripsModeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = _load_module()
        self.tmp = Path(tempfile.mkdtemp(prefix="mcp-render-strip-"))
        _git_init(self.tmp)
        run_dir = self.tmp / ".ai-specs" / "run"
        run_dir.mkdir(parents=True)
        (run_dir / "proxy.port").write_text("54321\n")
        self.toml = self.tmp / "ai-specs" / "ai-specs.toml"
        _write_toml(
            self.toml,
            '[mcp.trello]\n'
            'mode = "shared"\n'
            'command = "npx"\n'
            'args = ["-y", "@trello/mcp"]\n'
            'env = { TOKEN = "$TOKEN" }\n'
            '\n'
            '[mcp.github]\n'
            'mode = "stdio"\n'
            'command = "npx"\n'
            'args = ["-y", "@github/mcp"]\n'
            'env = { TOKEN = "$TOKEN" }\n',
        )

    def tearDown(self) -> None:
        shutil.rmtree(self.tmp, ignore_errors=True)

    def _render(self, agent: str, target_rel: str, mcp_key: str) -> Path:
        target = self.tmp / target_rel
        with mock.patch.object(
            sys,
            "argv",
            ["mcp-render.py", str(self.toml), agent, str(target), mcp_key],
        ):
            rc = self.mod.main()
        self.assertEqual(rc, 0)
        self.assertTrue(target.is_file())
        return target

    def _assert_no_mode_in_json(self, target: Path, key: str) -> None:
        data = json.loads(target.read_text())
        servers = data.get(key, {})
        for name, entry in servers.items():
            self.assertIsInstance(entry, dict)
            self.assertNotIn(
                "mode", entry, f"{target.name}:{name} retained 'mode' key"
            )

    def test_claude_output_has_no_mode(self) -> None:
        t = self._render("claude", ".mcp.json", "mcpServers")
        self._assert_no_mode_in_json(t, "mcpServers")

    def test_cursor_output_has_no_mode(self) -> None:
        t = self._render("cursor", ".cursor/mcp.json", "mcpServers")
        self._assert_no_mode_in_json(t, "mcpServers")

    def test_opencode_output_has_no_mode(self) -> None:
        t = self._render("opencode", "opencode.json", "mcp")
        self._assert_no_mode_in_json(t, "mcp")

    def test_codex_output_has_no_mode(self) -> None:
        t = self._render("codex", ".codex/config.toml", "mcp_servers")
        text = t.read_text()
        self.assertIsNone(
            re.search(r"^\s*mode\s*=", text, re.MULTILINE),
            f"codex toml leaked 'mode' field:\n{text}",
        )

    def test_gemini_output_has_no_mode(self) -> None:
        t = self._render("gemini", ".gemini/settings.json", "mcpServers")
        self._assert_no_mode_in_json(t, "mcpServers")


if __name__ == "__main__":
    unittest.main()

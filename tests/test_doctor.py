import importlib.util
import sys
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
DOCTOR_SH = ROOT / "lib" / "doctor.sh"
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    # Pre-register so dataclasses (Python 3.12+) can resolve cls.__module__ during exec.
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def toml_value(v):
    """Serialize a Python value to a TOML literal."""
    if v is None:
        return ""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, str):
        return f'"{v}"'
    if isinstance(v, list):
        items = ", ".join(toml_value(x) for x in v)
        return f"[{items}]"
    if isinstance(v, dict):
        pairs = ", ".join(f"{toml_value(kk)} = {toml_value(vv)}" for kk, vv in v.items())
        return f"{{{pairs}}}"
    return str(v)


def update_toml_field(path: Path, section: str, key: str, value) -> None:
    """Surgical edits for tests — preserves the rest of ai-specs.toml from init."""
    import re

    text = path.read_text()
    if section == "agents" and key == "enabled":
        rep = f"enabled = {toml_value(value)}"
        new, n = re.subn(
            r"(?m)^enabled\s*=\s*\[.*?\]\s*$",
            rep,
            text,
            count=1,
        )
        if n != 1:
            raise AssertionError("could not patch [agents].enabled in test manifest")
        path.write_text(new)
        return
    if section == "mcp":
        block = f"\n[mcp.{key}]\n"
        for kk, vv in (value or {}).items():
            block += f"{kk} = {toml_value(vv)}\n"
        path.write_text(text.rstrip() + block + "\n")
        return
    raise ValueError(f"unsupported test update: {section}.{key}")


def ai_specs_init(path: Path, agents: list[str] | None = None) -> None:
    subprocess.run([str(CLI), "init", str(path)], check=True, text=True)
    toml_path = path / "ai-specs" / "ai-specs.toml"
    if agents is not None:
        update_toml_field(toml_path, "agents", "enabled", agents)
    else:
        # Tests assume no enabled agents until sync; template may default to a trio.
        update_toml_field(toml_path, "agents", "enabled", [])


class DoctorCommandAvailabilityTests(unittest.TestCase):
    def test_help_lists_doctor(self):
        result = subprocess.run(
            [str(CLI), "help"], capture_output=True, text=True, check=False
        )
        self.assertIn("doctor", result.stdout)
        self.assertIn("diagnose", result.stdout.lower())

    def test_doctor_accepts_target_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn(target.name, result.stdout)

    def test_doctor_is_read_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            before = set(_find_files(target))
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            after = set(_find_files(target))
            self.assertEqual(before, after)


class CoreProjectStructureTests(unittest.TestCase):
    def test_manifest_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("manifest", result.stdout)

    def test_manifest_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("manifest", result.stdout.lower())

    def test_agents_md_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("AGENTS", result.stdout)

    def test_agents_md_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            (target / "ai-specs").mkdir()
            (target / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "orphan"\n'
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("AGENTS", result.stdout)
            self.assertIn("sync", result.stdout.lower())


class AgentDiagnosticsTests(unittest.TestCase):
    def test_no_enabled_agents_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", []
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("enabled", result.stdout.lower())

    def test_unknown_enabled_agent_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", ["fakerobot"]
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_enabled_agent_output_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_enabled_agent_output_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            (target / "AGENTS.md").unlink()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)


class BundledAssetDiagnosticsTests(unittest.TestCase):
    def test_bundled_skills_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("skill-creator", result.stdout)

    def test_bundled_skill_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            shutil.rmtree(target / "ai-specs" / "skills" / "skill-sync")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("skill-sync", result.stdout)

    def test_bundled_commands_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("commands", result.stdout)

    def test_bundled_commands_missing_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            shutil.rmtree(target / "ai-specs" / "commands")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)


class SymlinkDiagnosticsTests(unittest.TestCase):
    def test_instruction_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_instruction_symlink_invalid_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            claude_md = target / "CLAUDE.md"
            if claude_md.is_symlink() or claude_md.exists():
                claude_md.unlink()
            claude_md.write_text("stale content")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_skill_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("skills", result.stdout)

    def test_copied_skill_directory_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["opencode"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)


class MCPDiagnosticsTests(unittest.TestCase):
    def test_no_mcp_servers_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True, text=True
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            mcp_file = target / ".mcp.json"
            if mcp_file.exists():
                mcp_file.unlink()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("mcp", result.stdout.lower())


class ReportAndExitCodeTests(unittest.TestCase):
    def test_healthy_project_exits_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_project_with_errors_exits_nonzero(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_severity_labels_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            found = False
            for label in ("OK", "WARN", "ERROR"):
                if label in result.stdout:
                    found = True
                    break
            self.assertTrue(found)
            self.assertIn("Summary", result.stdout)

    def test_non_ok_includes_actionable_guidance(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            if "ERROR" in result.stdout:
                words = result.stdout.lower()
                self.assertTrue(
                    "init" in words or "sync" in words or "missing" in words
                )


class DaemonUvxCheckTests(unittest.TestCase):
    def _make_uvx_stub(self, dir_: Path) -> None:
        stub = dir_ / "uvx"
        stub.write_text("#!/bin/sh\nexit 0\n")
        stub.chmod(0o755)

    def _run_doctor(self, target: Path, path: str):
        import os as _os
        env = {**_os.environ, "PATH": path}
        return subprocess.run(
            [sys.executable, str(DOCTOR_PY), str(target)],
            capture_output=True, text=True, env=env, check=False,
        )

    def test_daemon_uvx_shared_with_uvx_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "trello",
                {"command": "npx", "mode": "shared"},
            )
            empty = Path(tmp) / "empty-path"
            empty.mkdir()
            path = f"{empty}:/usr/bin:/bin"
            result = self._run_doctor(target, path)
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            uvx_lines = [l for l in result.stdout.splitlines() if "daemon-uvx" in l]
            self.assertTrue(uvx_lines, result.stdout)
            self.assertTrue(any("ERROR" in l for l in uvx_lines), result.stdout)
            self.assertIn("uv", result.stdout)

    def test_daemon_uvx_shared_with_uvx_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "trello",
                {"command": "npx", "mode": "shared"},
            )
            stub_dir = Path(tmp) / "fakebin"
            stub_dir.mkdir()
            self._make_uvx_stub(stub_dir)
            path = f"{stub_dir}:/usr/bin:/bin"
            result = self._run_doctor(target, path)
            uvx_lines = [l for l in result.stdout.splitlines() if "daemon-uvx" in l]
            self.assertTrue(uvx_lines, result.stdout)
            self.assertTrue(any("OK" in l for l in uvx_lines), result.stdout)

    def test_daemon_uvx_no_shared_check_absent(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"},
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False,
            )
            self.assertNotIn("daemon-uvx", result.stdout)


class DaemonRunningCheckTests(unittest.TestCase):
    def _git_init(self, path: Path) -> None:
        subprocess.run(
            ["git", "-c", "init.defaultBranch=main", "init", "-q", str(path)],
            check=True,
        )

    def test_daemon_running_no_state_files_check_absent(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._git_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False,
            )
            self.assertNotIn("daemon-running", result.stdout)

    def test_daemon_running_state_present_unhealthy_reports_warn(self):
        import socket as _socket
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._git_init(target)
            run_dir = target / ".ai-specs" / "run"
            run_dir.mkdir(parents=True)
            with _socket.socket(_socket.AF_INET, _socket.SOCK_STREAM) as s:
                s.bind(("", 0))
                free_port = s.getsockname()[1]
            (run_dir / "proxy.port").write_text(str(free_port))
            (run_dir / "proxy.pid").write_text("999999")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False,
            )
            running_lines = [l for l in result.stdout.splitlines() if "daemon-running" in l]
            self.assertTrue(running_lines, result.stdout)
            self.assertTrue(any("WARN" in l for l in running_lines), result.stdout)
            self.assertIn("ai-specs sync", result.stdout)

    def test_daemon_running_state_present_healthy_reports_ok(self):
        import threading as _threading
        from http.server import BaseHTTPRequestHandler, HTTPServer

        class _Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path == "/status":
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(b"{}")
                else:
                    self.send_response(404)
                    self.end_headers()

            def log_message(self, *a, **k):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._git_init(target)
            server = HTTPServer(("127.0.0.1", 0), _Handler)
            port = server.server_address[1]
            t = _threading.Thread(target=server.serve_forever, daemon=True)
            t.start()
            try:
                run_dir = target / ".ai-specs" / "run"
                run_dir.mkdir(parents=True)
                (run_dir / "proxy.port").write_text(str(port))
                import os as _os
                (run_dir / "proxy.pid").write_text(str(_os.getpid()))
                result = subprocess.run(
                    [str(CLI), "doctor", str(target)],
                    capture_output=True, text=True, check=False,
                )
                running_lines = [l for l in result.stdout.splitlines() if "daemon-running" in l]
                self.assertTrue(running_lines, result.stdout)
                self.assertTrue(any("OK" in l for l in running_lines), result.stdout)
            finally:
                server.shutdown()
                server.server_close()


def _find_files(root: Path):
    for p in root.rglob("*"):
        if p.is_file():
            yield p


if __name__ == "__main__":
    unittest.main()
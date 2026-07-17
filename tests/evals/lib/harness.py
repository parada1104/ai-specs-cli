"""Scenario runner: materialize fixture, invoke headless runtimes, assert outcomes."""

from __future__ import annotations

import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[3]
CATALOG = ROOT / "catalog"
RECIPE_MATERIALIZE = ROOT / "lib" / "_internal" / "recipe-materialize.py"

SUPPORTED_RUNTIMES = ("claude", "opencode", "pi", "omp")

# opencode/pi/omp default through the local "API for Cursor" provider (cursorapi).
# Override any runtime with EVALS_MODEL=<provider/model>.
DEFAULT_MODELS = {
    "claude": "opus",
    "opencode": "cursorapi/composer-2.5",
    "pi": "cursorapi/composer-2.5",
    "omp": "cursorapi/composer-2.5",
}

META_PROMPT_RE = re.compile(
    r"(?i)(/plan\b|/build\b|haz un plan|run the /plan|produce planning artifacts only)"
)


def _load_materialize():
    spec = importlib.util.spec_from_file_location("recipe_materialize_eval", RECIPE_MATERIALIZE)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


@dataclass
class Scenario:
    id: str
    recipe_id: str
    prompt_path: Path
    meta: dict[str, Any]
    mode: str  # plan | build


def load_scenario(path: Path) -> Scenario:
    meta_path = path / "scenario.toml"
    data = tomllib.loads(meta_path.read_text())
    return Scenario(
        id=data.get("id", path.name),
        recipe_id=data["recipe_id"],
        prompt_path=path / data.get("prompt_file", "prompt.txt"),
        meta=data,
        mode=str(data.get("mode", "plan")).lower(),
    )


def assert_natural_prompt(prompt: str) -> None:
    if META_PROMPT_RE.search(prompt):
        raise AssertionError(
            "scenario prompt must be a natural user request; "
            "must not coach /plan, /build, or 'haz un plan'"
        )


def materialize_project(project_root: Path, recipe_id: str, version: str, extra: str = "") -> None:
    from tests.evals.lib.project_fixture import write_manifest

    write_manifest(project_root, recipe_id=recipe_id, version=version, extra_recipes=extra)
    mod = _load_materialize()
    mod.materialize_recipes(project_root, ROOT)


def live_enabled() -> bool:
    return os.environ.get("EVALS_LIVE", "").lower() in {"1", "true", "yes"}


def detect_runtime() -> str | None:
    forced = os.environ.get("EVALS_RUNTIME", "").strip().lower()
    if forced:
        return forced if forced in SUPPORTED_RUNTIMES else None
    for name in SUPPORTED_RUNTIMES:
        if shutil.which(name):
            return name
    return None


def runtime_available(runtime: str | None = None) -> bool:
    name = runtime or detect_runtime()
    return bool(name and shutil.which(name))


def claude_available() -> bool:
    return shutil.which("claude") is not None


def default_model(runtime: str) -> str:
    override = os.environ.get("EVALS_MODEL", "").strip()
    if override:
        return override
    return DEFAULT_MODELS.get(runtime, DEFAULT_MODELS["claude"])


def api_key_present(runtime: str | None = None) -> bool:
    """Runtime-aware credential gate. OpenCode/Pi/OMP often use local auth."""
    name = runtime or detect_runtime() or "claude"
    if name == "claude":
        return bool(os.environ.get("ANTHROPIC_API_KEY") or os.environ.get("CLAUDE_API_KEY"))
    # Local CLIs typically have their own auth stores; do not require env keys.
    return True


def _timeout() -> int:
    return int(os.environ.get("EVALS_TIMEOUT_SEC", "600"))


def _max_turns() -> str:
    return os.environ.get("EVALS_MAX_TURNS", "12")


def _run(cmd: list[str], cwd: Path) -> dict[str, Any]:
    """Run a command; on timeout kill and return partial stdout/stderr."""
    proc = subprocess.Popen(
        cmd,
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        stdout, stderr = proc.communicate(timeout=_timeout())
        return {
            "returncode": proc.returncode,
            "stdout": stdout or "",
            "stderr": stderr or "",
            "cmd": cmd,
            "timed_out": False,
        }
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout, stderr = proc.communicate()
        return {
            "returncode": 124,
            "stdout": stdout or "",
            "stderr": (stderr or "") + f"\ntimed out after {_timeout()}s",
            "cmd": cmd,
            "timed_out": True,
        }


def _parse_claude_json(stdout: str) -> dict[str, Any]:
    if not stdout.strip():
        return {}
    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        return {}


def _parse_opencode_ndjson(stdout: str) -> dict[str, Any]:
    """OpenCode --format json emits NDJSON events."""
    texts: list[str] = []
    cost = None
    for line in stdout.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        etype = event.get("type")
        part = event.get("part") or {}
        if etype == "text" and isinstance(part.get("text"), str):
            texts.append(part["text"])
        if etype == "step_finish" and "cost" in part:
            cost = part.get("cost")
    return {"result_text": "".join(texts), "cost": cost}


def _parse_pi_ndjson(stdout: str) -> dict[str, Any]:
    texts: list[str] = []
    cost = None
    for line in stdout.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "message_end" and event.get("message", {}).get("role") == "assistant":
            content = event["message"].get("content")
            if isinstance(content, str):
                texts.append(content)
            elif isinstance(content, list):
                for block in content:
                    if isinstance(block, dict) and block.get("type") == "text":
                        texts.append(str(block.get("text", "")))
            usage = event.get("message", {}).get("usage") or event.get("usage") or {}
            if isinstance(usage, dict):
                cost = (usage.get("cost") or {}).get("total", cost)
    return {"result_text": "".join(texts), "cost": cost}


def run_claude_prompt(project_root: Path, prompt: str, *, mode: str = "plan") -> dict[str, Any]:
    model = default_model("claude")
    # Claude's "plan" permission mode is read-only and cannot write OpenSpec
    # artifacts. Ambient plan-build needs writes; FS assertions gate production edits.
    permission = "acceptEdits"
    _ = mode
    cmd = [
        "claude",
        "-p",
        prompt,
        "--model",
        model,
        "--permission-mode",
        permission,
        "--max-turns",
        _max_turns(),
        "--output-format",
        "json",
    ]
    result = _run(cmd, project_root)
    result["runtime"] = "claude"
    result["model"] = model
    result["mode"] = mode
    result["json"] = _parse_claude_json(result["stdout"])
    result["result_text"] = (
        result["json"].get("result")
        if isinstance(result["json"], dict)
        else None
    ) or result["stdout"]
    return result


def run_opencode_prompt(project_root: Path, prompt: str, *, mode: str = "plan") -> dict[str, Any]:
    model = default_model("opencode")
    cmd = [
        "opencode",
        "run",
        "--pure",
        "--dir",
        str(project_root),
        "--format",
        "json",
        "--model",
        model,
        "--auto",
        prompt,
    ]
    # OpenCode has no universal --mode plan; rely on ambient skill + assertions.
    _ = mode
    result = _run(cmd, project_root)
    result["runtime"] = "opencode"
    result["model"] = model
    result["mode"] = mode
    parsed = _parse_opencode_ndjson(result["stdout"])
    result.update(parsed)
    return result


def run_pi_family_prompt(
    binary: str,
    project_root: Path,
    prompt: str,
    *,
    mode: str = "plan",
) -> dict[str, Any]:
    model = default_model(binary if binary in DEFAULT_MODELS else "pi")
    cmd = [binary, "-p", "--mode", "json", "--model", model, "--no-session"]
    # omp accepts --cwd; pi relies on process cwd from _run.
    if binary == "omp":
        cmd.extend(["--cwd", str(project_root)])
        # Extensions (e.g. background job tool) can hang headless evals.
        cmd.append("--no-extensions")
        cmd.extend(["--approval-mode", "yolo"])
    cmd.append(prompt)
    # Pi/OMP: plan-mode flags vary by extension; skill + FS assertions are the gate.
    _ = mode
    result = _run(cmd, project_root)
    result["runtime"] = binary
    result["model"] = model
    result["mode"] = mode
    parsed = _parse_pi_ndjson(result["stdout"])
    result.update(parsed)
    if not result.get("result_text"):
        result["result_text"] = result["stdout"]
    return result


def run_prompt(
    project_root: Path,
    prompt: str,
    *,
    runtime: str | None = None,
    mode: str = "plan",
) -> dict[str, Any]:
    assert_natural_prompt(prompt)
    name = runtime or detect_runtime()
    if not name:
        raise RuntimeError("no supported runtime on PATH (claude|opencode|pi|omp)")
    if name == "claude":
        return run_claude_prompt(project_root, prompt, mode=mode)
    if name == "opencode":
        return run_opencode_prompt(project_root, prompt, mode=mode)
    if name in {"pi", "omp"}:
        return run_pi_family_prompt(name, project_root, prompt, mode=mode)
    raise RuntimeError(f"unsupported runtime: {name}")


def git_paths_changed(project_root: Path) -> list[str]:
    proc = subprocess.run(
        ["git", "status", "--porcelain"],
        cwd=project_root,
        capture_output=True,
        text=True,
        check=True,
    )
    return [line[3:].strip() for line in proc.stdout.splitlines() if line.strip()]


def init_git_repo(project_root: Path) -> None:
    subprocess.run(["git", "init", "-q"], cwd=project_root, check=True)
    subprocess.run(["git", "config", "user.email", "eval@ai-specs.local"], cwd=project_root, check=True)
    subprocess.run(["git", "config", "user.name", "eval"], cwd=project_root, check=True)

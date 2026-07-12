"""Scenario runner: materialize fixture, optionally invoke claude -p, assert outcomes."""

from __future__ import annotations

import importlib.util
import json
import os
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


def load_scenario(path: Path) -> Scenario:
    meta_path = path / "scenario.toml"
    data = tomllib.loads(meta_path.read_text())
    return Scenario(
        id=data.get("id", path.name),
        recipe_id=data["recipe_id"],
        prompt_path=path / data.get("prompt_file", "prompt.txt"),
        meta=data,
    )


def materialize_project(project_root: Path, recipe_id: str, version: str, extra: str = "") -> None:
    from tests.evals.lib.project_fixture import write_manifest

    write_manifest(project_root, recipe_id=recipe_id, version=version, extra_recipes=extra)
    mod = _load_materialize()
    mod.materialize_recipes(project_root, ROOT)


def live_enabled() -> bool:
    return os.environ.get("EVALS_LIVE", "").lower() in {"1", "true", "yes"}


def claude_available() -> bool:
    return shutil.which("claude") is not None


def api_key_present() -> bool:
    return bool(os.environ.get("ANTHROPIC_API_KEY") or os.environ.get("CLAUDE_API_KEY"))


def run_claude_prompt(project_root: Path, prompt: str) -> dict[str, Any]:
    max_turns = os.environ.get("EVALS_MAX_TURNS", "12")
    cmd = [
        "claude",
        "-p",
        prompt,
        "--permission-mode",
        "acceptEdits",
        "--max-turns",
        str(max_turns),
        "--output-format",
        "json",
    ]
    proc = subprocess.run(
        cmd,
        cwd=project_root,
        capture_output=True,
        text=True,
        timeout=int(os.environ.get("EVALS_TIMEOUT_SEC", "600")),
        check=False,
    )
    payload: dict[str, Any] = {
        "returncode": proc.returncode,
        "stdout": proc.stdout,
        "stderr": proc.stderr,
    }
    if proc.stdout.strip():
        try:
            payload["json"] = json.loads(proc.stdout)
        except json.JSONDecodeError:
            payload["json"] = None
    return payload


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

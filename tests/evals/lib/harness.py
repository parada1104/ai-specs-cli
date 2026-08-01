"""Scenario runner: materialize fixture, invoke headless runtimes, assert outcomes."""

from __future__ import annotations

import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[3]
CATALOG = ROOT / "catalog"
RECIPE_MATERIALIZE = ROOT / "lib" / "_internal" / "recipe-materialize.py"

SUPPORTED_RUNTIMES = ("claude", "cursor-agent", "opencode", "pi", "omp")

# Routing (this harness):
# - claude → Claude Code subscription via `claude` CLI (model id like `opus`)
# - cursor-agent → Cursor Agent CLI subscription (`cursor-agent` / `agent`;
#   model ids like `composer-2.5` — not OpenCode `cursorapi/*`)
# - opencode / pi / omp → OpenCode provider `cursorapi` ("API for Cursor") only
#   Never anthropic/* on those runtimes (no Anthropic API key path).
DEFAULT_MODELS = {
    "claude": "opus",
    "cursor-agent": "composer-2.5",
    "opencode": "cursorapi/composer-2.5",
    "pi": "cursorapi/composer-2.5",
    "omp": "cursorapi/composer-2.5",
}

_OPENCODE_FAMILY = frozenset({"opencode", "pi", "omp"})

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

    # vault-canonical-store vendors kepano deps; keep offline via fixture when set.
    os.environ.setdefault(
        "AI_SPECS_VENDOR_FIXTURE_ROOT",
        str(ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"),
    )
    write_manifest(project_root, recipe_id=recipe_id, version=version, extra_recipes=extra)
    mod = _load_materialize()
    mod.materialize_recipes(project_root, ROOT)


# Eval runtime id → platform agent id (platform.sh / hooks-render use `cursor`).
RUNTIME_TO_AGENT = {"cursor-agent": "cursor"}
# Runtimes whose native hook channel exposes no pre-file-write event; a
# file-write gate hook cannot be wired for them (see hooks-render.py).
NO_FILE_WRITE_HOOK_RUNTIMES = frozenset({"cursor-agent"})


def runtime_to_agent(runtime: str) -> str:
    """Map an eval runtime id to its platform agent id."""
    return RUNTIME_TO_AGENT.get(runtime, runtime)


def wire_runtime_hooks(project_root: Path, runtime: str) -> None:
    """Wire enabled recipes' `[[provides.hooks]]` into the runtime's native
    channel (e.g. `.claude/settings.json` PreToolUse), exactly as
    `sync-agent.sh` does, so live scenarios exercise runtime hooks and not only
    the advisory skill/brief layer.

    Safe by construction: `hooks-render.py` skips agents with no runtime-hook
    target and skips `Edit|Write|MultiEdit|NotebookEdit` matchers for cursor.
    """
    agent = runtime_to_agent(runtime)
    mod = _load_materialize()
    fd, tmp = tempfile.mkstemp(prefix="eval-resolved-hooks-", suffix=".json")
    os.close(fd)
    resolved = Path(tmp)
    try:
        mod.materialize_recipes(project_root, ROOT, resolved_hooks_out=resolved)
        hooks_render = ROOT / "lib" / "_internal" / "hooks-render.py"
        subprocess.run(
            [sys.executable, str(hooks_render), str(resolved), agent, str(project_root)],
            check=False,
            capture_output=True,
            text=True,
        )
    finally:
        resolved.unlink(missing_ok=True)


def live_enabled() -> bool:
    return os.environ.get("EVALS_LIVE", "").lower() in {"1", "true", "yes"}


def resolve_runtime_binary(runtime: str) -> str | None:
    """Resolve CLI path for a supported runtime.

    `cursor-agent` accepts either `cursor-agent` or `agent`. Never the `cursor`
    IDE shim (different binary; not headless-agent capable here).
    """
    if runtime == "cursor-agent":
        return shutil.which("cursor-agent") or shutil.which("agent")
    return shutil.which(runtime)


def detect_runtime() -> str | None:
    forced = os.environ.get("EVALS_RUNTIME", "").strip().lower()
    if forced:
        return forced if forced in SUPPORTED_RUNTIMES else None
    for name in SUPPORTED_RUNTIMES:
        if resolve_runtime_binary(name):
            return name
    return None


def runtime_available(runtime: str | None = None) -> bool:
    name = runtime or detect_runtime()
    return bool(name and resolve_runtime_binary(name))


def claude_available() -> bool:
    return shutil.which("claude") is not None


def _model_env_key(runtime: str) -> str:
    # Hyphenated runtime ids → EVALS_MODEL_CURSOR_AGENT
    return f"EVALS_MODEL_{runtime.upper().replace('-', '_')}"


def default_model(runtime: str) -> str:
    # Per-runtime override wins, then global EVALS_MODEL, then defaults.
    specific = os.environ.get(_model_env_key(runtime), "").strip()
    override = specific or os.environ.get("EVALS_MODEL", "").strip()
    if override:
        if runtime in _OPENCODE_FAMILY and not override.startswith("cursorapi/"):
            raise RuntimeError(
                f"{runtime} live evals must use cursorapi/* (API for Cursor); "
                f"got {override!r}. Example: EVALS_MODEL=cursorapi/composer-2.5 "
                f"(do not pass anthropic/* or bare Claude model ids here)."
            )
        if runtime == "cursor-agent" and override.startswith("cursorapi/"):
            raise RuntimeError(
                "cursor-agent live evals use Cursor Agent model ids "
                "(e.g. composer-2.5), not OpenCode cursorapi/* provider paths. "
                f"got {override!r}."
            )
        return override
    return DEFAULT_MODELS.get(runtime, DEFAULT_MODELS["claude"])


def api_key_present(runtime: str | None = None) -> bool:
    """Runtime-aware credential gate — local CLI/provider auth, not Anthropic keys for OpenCode-family."""
    name = runtime or detect_runtime() or "claude"
    if name == "claude":
        # Claude Code subscription / local auth store (headless `-p`).
        return shutil.which("claude") is not None
    if name == "cursor-agent":
        # Cursor Agent subscription / local login (`cursor-agent login`).
        return resolve_runtime_binary("cursor-agent") is not None
    if name in _OPENCODE_FAMILY:
        # cursorapi is configured in the OpenCode/Pi/OMP provider store.
        return shutil.which(name) is not None
    return True


def _timeout() -> int:
    return int(os.environ.get("EVALS_TIMEOUT_SEC", "600"))


def _max_turns() -> str:
    return os.environ.get("EVALS_MAX_TURNS", "12")


def _run(
    cmd: list[str],
    cwd: Path,
    *,
    env: dict[str, str] | None = None,
) -> dict[str, Any]:
    """Run a command; on timeout kill and return partial stdout/stderr."""
    proc = subprocess.Popen(
        cmd,
        cwd=cwd,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
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


def run_claude_prompt(
    project_root: Path,
    prompt: str,
    *,
    mode: str = "plan",
    env: dict[str, str] | None = None,
    mcp_config: Path | None = None,
    add_dirs: list[Path] | None = None,
) -> dict[str, Any]:
    model = default_model("claude")
    # Claude's "plan" permission mode is read-only and cannot write OpenSpec
    # artifacts. Ambient plan-build needs writes; FS assertions gate production edits.
    # Live MCP / headless writes: bypass permissions when caller opts in via
    # mcp_config OR EVALS_CLAUDE_BYPASS=1 (local-scope MCP registration path).
    bypass = mcp_config is not None or os.environ.get(
        "EVALS_CLAUDE_BYPASS", ""
    ).lower() in {"1", "true", "yes"}
    permission = "bypassPermissions" if bypass else "acceptEdits"
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
    if bypass:
        cmd.append("--dangerously-skip-permissions")
    if mcp_config is not None:
        cmd.extend(
            [
                "--mcp-config",
                str(mcp_config),
                "--strict-mcp-config",
            ]
        )
    # Modern filesystem MCP replaces argv dirs with client roots; --add-dir
    # puts the vault scope into Claude's root set so AllowedDirectories match.
    for extra in add_dirs or []:
        cmd.extend(["--add-dir", str(extra)])
    result = _run(cmd, project_root, env=env)
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


def run_cursor_agent_prompt(
    project_root: Path,
    prompt: str,
    *,
    mode: str = "plan",
    env: dict[str, str] | None = None,
    approve_mcps: bool = False,
) -> dict[str, Any]:
    """Headless Cursor Agent CLI (`cursor-agent` / `agent`)."""
    model = default_model("cursor-agent")
    binary = resolve_runtime_binary("cursor-agent")
    if not binary:
        raise RuntimeError("cursor-agent (or agent) not found on PATH")
    # Plan mode is read-only; eval writes (merge-plan / OpenSpec) need force.
    _ = mode
    cmd = [
        binary,
        "-p",
        prompt,
        "--model",
        model,
        "--force",
        "--trust",
        "--sandbox",
        "disabled",
        "--output-format",
        "text",
        "--workspace",
        str(project_root),
    ]
    if approve_mcps:
        cmd.append("--approve-mcps")
    result = _run(cmd, project_root, env=env)
    result["runtime"] = "cursor-agent"
    result["model"] = model
    result["mode"] = mode
    result["result_text"] = result["stdout"]
    return result


def run_opencode_prompt(
    project_root: Path,
    prompt: str,
    *,
    mode: str = "plan",
    env: dict[str, str] | None = None,
) -> dict[str, Any]:
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
    result = _run(cmd, project_root, env=env)
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
    env: dict[str, str] | None = None,
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
    result = _run(cmd, project_root, env=env)
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
    env: dict[str, str] | None = None,
    mcp_config: Path | None = None,
    approve_mcps: bool = False,
    add_dirs: list[Path] | None = None,
) -> dict[str, Any]:
    assert_natural_prompt(prompt)
    name = runtime or detect_runtime()
    if not name:
        raise RuntimeError(
            "no supported runtime on PATH (claude|cursor-agent|opencode|pi|omp)"
        )
    if name == "claude":
        return run_claude_prompt(
            project_root,
            prompt,
            mode=mode,
            env=env,
            mcp_config=mcp_config,
            add_dirs=add_dirs,
        )
    if name == "cursor-agent":
        return run_cursor_agent_prompt(
            project_root, prompt, mode=mode, env=env, approve_mcps=approve_mcps
        )
    if name == "opencode":
        return run_opencode_prompt(project_root, prompt, mode=mode, env=env)
    if name in {"pi", "omp"}:
        return run_pi_family_prompt(name, project_root, prompt, mode=mode, env=env)
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


# Markers that mean a forbidden phrase appears only as a counter-example /
# negative instruction (shared by notes-file prose checks).
FORBIDDEN_PHRASE_NEG_MARKERS = (
    "never",
    "do not",
    "don't",
    "dont",
    "sin ",
    "without",
    "forbid",
    "not ",
    "no uses",
    "no usar",
    "nunca",
    "❌",
    "✗",
    "🚫",
    "# bad",
    "# wrong",
    "# incorrect",
    "# avoid",
    "# no",
    "incorrect",
    "anti-pattern",
    "antipattern",
    "wrong:",
    "bad:",
)


def forbidden_phrase_violations(
    text: str,
    phrases: list[str],
    *,
    window: int = 3,
) -> list[str]:
    """Return forbidden phrases that appear in affirmative (non-negated) context.

    Notes-file prose often documents the anti-pattern with an explicit "do not /
    never / sin …" — those are allowed. Affirmative mentions (retry via
    ``python3 -c``, claim root ``git worktree list`` alone is enough, etc.) are
    violations. Window matches ``forbidden_command_line_violations`` in the VCS
    live eval module.
    """
    if not phrases:
        return []
    lines = text.splitlines()
    hits: list[str] = []
    seen: set[str] = set()
    for idx, raw in enumerate(lines):
        line = raw.strip()
        if not line or line.lstrip().startswith("#"):
            continue
        lower = line.lower().replace("`", "")
        # Placeholder / abbreviated negative examples
        if "..." in line or "…" in line:
            continue
        for phrase in phrases:
            needle = str(phrase).lower().replace("`", "")
            if not needle or needle not in lower:
                continue
            window_text = "\n".join(
                lines[max(0, idx - window) : idx + 1]
            ).lower()
            if any(marker in window_text for marker in FORBIDDEN_PHRASE_NEG_MARKERS):
                continue
            if any(marker in lower for marker in FORBIDDEN_PHRASE_NEG_MARKERS):
                continue
            key = f"{idx}:{needle}"
            if key in seen:
                continue
            seen.add(key)
            hits.append(raw if needle in raw.lower() else f"{phrase} @ line {idx + 1}")
    return hits

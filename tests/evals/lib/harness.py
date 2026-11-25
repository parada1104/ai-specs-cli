"""Scenario runner: materialize fixture, invoke headless runtimes, assert outcomes."""

from __future__ import annotations

import fnmatch
import json
import os
import re
import shutil
import subprocess
import tempfile
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[3]
CATALOG = ROOT / "catalog"

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
_INSTALL_ROOT: Path | None = None

META_PROMPT_RE = re.compile(
    r"(?i)(/plan\b|/build\b|haz un plan|run the /plan|produce planning artifacts only)"
)

# `write_manifest` writes exactly one [agents].enabled line; the helpers below
# edit that line in place, because `sync` renders (and therefore wires hooks for)
# exactly the agents it lists.
AGENTS_LINE_RE = re.compile(r"^enabled = \[(.*)\]$", re.MULTILINE)


def _cli() -> Path:
    """Native CLI path (built once per process, loud fallback when go is absent)."""
    from tests import _go_cli

    return _go_cli.cli()


def install_root() -> Path:
    """A throwaway install root for the fixture's rendered hook commands.

    `sync` pins the hook command to `<AI_SPECS_HOME>/dist/ai-specs`. Pointing
    AI_SPECS_HOME at the checkout left that path empty, so every rendered hook
    fell through to whatever `ai-specs` sat on PATH and the gate refused every
    write for the wrong reason. This mirrors a real install instead.
    """
    global _INSTALL_ROOT
    if _INSTALL_ROOT is None:
        root = Path(tempfile.mkdtemp(prefix="ai-specs-eval-install-"))
        dist = root / "dist"
        dist.mkdir()
        shutil.copy2(_cli(), dist / "ai-specs")
        (dist / "ai-specs").chmod(0o755)
        for name in ("templates", "catalog", "bundled-skills", "bundled-commands"):
            source = ROOT / name
            if source.is_dir():
                shutil.copytree(source, root / name)
        _INSTALL_ROOT = root
    return _INSTALL_ROOT


def _sync(project_root: Path) -> None:
    """Run the native `sync`: the one authority that materializes recipes, the
    lock, the brief and the gitignore/envrc, and fans out to enabled agents."""
    env = {**os.environ, "AI_SPECS_HOME": str(install_root())}
    proc = subprocess.run(
        [str(_cli()), "sync", str(project_root)],
        text=True,
        capture_output=True,
        env=env,
        check=False,
    )
    if proc.returncode != 0:
        # check=True threw away the cause; a failing fixture setup is unreadable
        # without sync's own stderr.
        raise AssertionError(
            f"sync failed ({proc.returncode}) on {project_root}\n"
            f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
        )


def _enabled_agents(project_root: Path) -> list[str]:
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    match = AGENTS_LINE_RE.search(manifest.read_text())
    if match is None:
        raise RuntimeError(f"no [agents].enabled line in {manifest}")
    return [a.strip().strip("'\"") for a in match.group(1).split(",") if a.strip()]


def _set_enabled_agents(project_root: Path, agents: list[str]) -> None:
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    literal = "[" + ", ".join(f"'{a}'" for a in agents) + "]"
    text = manifest.read_text()
    manifest.write_text(AGENTS_LINE_RE.sub(f"enabled = {literal}", text, count=1))


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
    # Materialize ONLY: the legacy materializer never fanned out to agents either,
    # and `sync` wires the runtime hooks of every agent it renders. So the spine
    # runs with no enabled agents (recipes, lock, brief, gitignore/envrc — all
    # agent-independent) and the manifest is restored verbatim; the scenario then
    # opts into one agent's channels through wire_runtime_hooks.
    agents = _enabled_agents(project_root)
    _set_enabled_agents(project_root, [])
    try:
        _sync(project_root)
    finally:
        _set_enabled_agents(project_root, agents)


# Eval runtime id → platform agent id (the native renderer names it `cursor`).
RUNTIME_TO_AGENT = {"cursor-agent": "cursor"}
# Runtimes whose native hook channel exposes no pre-file-write event; a
# file-write gate hook cannot be wired for them (internal/sync/hooksrender.go).
NO_FILE_WRITE_HOOK_RUNTIMES = frozenset({"cursor-agent"})


def runtime_to_agent(runtime: str) -> str:
    """Map an eval runtime id to its platform agent id."""
    return RUNTIME_TO_AGENT.get(runtime, runtime)


# The one command family the agent under test may run. Deliberately not a
# blanket wildcard: the skill only needs `ai-specs <verb>`.
CLAUDE_ALLOWED_COMMANDS = ("Bash(ai-specs:*)",)


def allow_cli_commands(project_root: Path) -> None:
    """Let the agent under test run `ai-specs` and nothing else.

    The skill tells the agent to run `changes start --none`, `specs promote` and
    `changes archive`. A headless session cannot approve them interactively, so
    without this the case measures the sandbox instead of the skill. This writes
    the allow-list into THIS sandbox's `.claude/settings.json` only -- never the
    repo's, never the user's -- and names exactly one command family.
    """
    settings = project_root / ".claude" / "settings.json"
    settings.parent.mkdir(parents=True, exist_ok=True)
    data = json.loads(settings.read_text()) if settings.is_file() else {}
    allow = data.setdefault("permissions", {}).setdefault("allow", [])
    for command in CLAUDE_ALLOWED_COMMANDS:
        if command not in allow:
            allow.append(command)
    settings.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def wire_runtime_hooks(project_root: Path, runtime: str) -> None:
    """Wire enabled recipes' `[[provides.hooks]]` into the runtime's native
    channel (e.g. `.claude/settings.json` PreToolUse) by re-syncing with ONLY
    that agent enabled, so live scenarios exercise runtime hooks and not only the
    advisory skill/brief layer.

    The native renderer is the single authority for the channel shape. Narrowing
    `[agents].enabled` to the agent under test keeps the wiring scoped to the
    runtime the scenario drives (the legacy `hooks-render.py <agent>` argument
    behaved the same way): no other agent's channel is touched. Safe by
    construction: the renderer skips agents with no runtime-hook target and skips
    `Edit|Write|MultiEdit|NotebookEdit` matchers for cursor, which has no
    pre-file-write event.
    """
    # `setup_runtime_skills` copies a recipe skill into a real `<agent>/skills`
    # directory, but `sync` owns that path and wants it to be a symlink into
    # ai-specs/skills — it refuses to replace a non-symlink, which used to abort
    # every live scenario's fixture setup.
    for skills in project_root.glob(".*/skills"):
        if skills.is_dir() and not skills.is_symlink():
            shutil.rmtree(skills)
    _set_enabled_agents(project_root, [runtime_to_agent(runtime)])
    _sync(project_root)


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


# --- Assertion primitives -------------------------------------------------
# Each one raises AssertionError with the scenario label, so the live module
# and the unit tests observe the same messages. Pure tree/git inspection: no
# agent, no network.

# Active change folder per artifact mode: `assert_active_change_gone` accepts
# True (today's openspec behavior) or the mode string.
ACTIVE_CHANGE_TREES = {"openspec": ("openspec", "changes"), "odd": ("odd", "changes")}


def glob_exists(root: Path, pattern: str) -> bool:
    return any(root.glob(pattern))


def path_matches_any(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatch(path, pat) for pat in patterns)


def current_branch(root: Path) -> str:
    proc = subprocess.run(
        ["git", "-C", str(root), "rev-parse", "--abbrev-ref", "HEAD"],
        capture_output=True,
        text=True,
        check=True,
    )
    return proc.stdout.strip()


def assert_forbidden_path_globs(changed: list[str], patterns: list[str], *, label: str = "scenario") -> None:
    for path in changed:
        if path_matches_any(path, patterns):
            raise AssertionError(f"{label}: forbidden path modified: {path}")


def assert_required_path_globs(
    root: Path, patterns: list[str], changed: list[str], *, label: str = "scenario"
) -> None:
    for required in patterns:
        if not glob_exists(root, required):
            raise AssertionError(f"{label}: missing {required}; changed={changed}")


def assert_required_any_path_globs(
    root: Path, groups: list[list[str]], changed: list[str], *, label: str = "scenario"
) -> None:
    for group in groups:
        if not any(glob_exists(root, pat) for pat in group):
            raise AssertionError(f"{label}: expected any of {group}; changed={changed}")


def assert_required_changed_globs(
    root: Path, changed: list[str], patterns: list[str], *, label: str = "scenario"
) -> None:
    for pattern in patterns:
        if not (
            any(path_matches_any(path, [pattern]) for path in changed)
            or glob_exists(root, pattern)
        ):
            raise AssertionError(
                f"{label}: expected change matching {pattern}; changed={changed}"
            )


def assert_absent_path_globs(root: Path, patterns: list[str], *, label: str = "scenario") -> None:
    """Negative half of `required_path_globs`: no matching path may exist."""
    for pattern in patterns:
        hits = sorted(str(p.relative_to(root)) for p in root.glob(pattern))
        if hits:
            raise AssertionError(
                f"{label}: expected no path matching {pattern}; found {hits}"
            )


def assert_required_content(root: Path, rules: list[dict[str, Any]], *, label: str = "scenario") -> None:
    for rule in rules:
        path = root / rule["path"]
        if not path.is_file():
            raise AssertionError(f"{label}: missing {rule['path']}")
        text = path.read_text(encoding="utf-8", errors="replace").lower()
        needles = [str(n).lower() for n in rule.get("contains_any", [])]
        if not any(n in text for n in needles):
            raise AssertionError(f"{label}: {rule['path']} missing any of {needles}")


def assert_forbidden_content(root: Path, rules: list[dict[str, Any]], *, label: str = "scenario") -> None:
    for rule in rules:
        path = root / rule["path"]
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8", errors="replace").lower()
        for needle in rule.get("contains_any", []):
            if str(needle).lower() in text:
                raise AssertionError(
                    f"{label}: {rule['path']} must not contain {needle!r}"
                )


def assert_transcript(
    transcript: str,
    *,
    all_of: list[str] | None = None,
    one_of: list[str] | None = None,
    any_of: list[str] | None = None,
    label: str = "scenario",
) -> None:
    """Transcript assertions are always enforced; a case that does not want
    them simply omits the key. They used to be skipped whenever
    `required_content` was present, which made "asked before merging"
    ungradable."""
    text = transcript.lower()
    for needle in [str(n).lower() for n in all_of or []]:
        if needle not in text:
            raise AssertionError(f"{label}: transcript missing required {needle!r}")
    needles = [str(n).lower() for n in one_of or []]
    if needles and not any(needle in text for needle in needles):
        raise AssertionError(f"{label}: transcript missing any of {needles!r}")
    for needle in [str(n).lower() for n in any_of or []]:
        if needle not in text:
            raise AssertionError(f"{label}: transcript missing required {needle!r}")


def _walk_tool_inputs(node: Any, out: list[str]) -> None:
    if isinstance(node, dict):
        for key in ("input", "arguments"):
            holder = node.get(key)
            if isinstance(holder, dict):
                command = holder.get("command") or holder.get("cmd")
                if isinstance(command, str) and command.strip():
                    out.append(command)
        for value in node.values():
            _walk_tool_inputs(value, out)
    elif isinstance(node, list):
        for value in node:
            _walk_tool_inputs(value, out)


def collect_executed_commands(result: dict[str, Any]) -> list[str] | None:
    """The command strings the runtime recorded as tool calls.

    Returns None when the runtime exposed no structured record, which is what
    makes the forbidden-command check fail closed. Reading the agent's final
    prose instead was the original defect: an agent could run a prohibited
    merge and then answer without quoting it, and the case still passed.
    """
    texts: list[str] = []
    payload = result.get("json")
    if isinstance(payload, dict):
        texts.append(json.dumps(payload))
    stdout = result.get("stdout") or ""
    texts.extend(line for line in stdout.splitlines() if line.strip().startswith("{"))

    found: list[str] = []
    saw_event = False
    for text in texts:
        try:
            event = json.loads(text)
        except json.JSONDecodeError:
            continue
        saw_event = True
        _walk_tool_inputs(event, found)
    return found if saw_event else None


def assert_no_forbidden_commands(
    executed: list[str] | None, commands: list[str], *, label: str = "scenario"
) -> None:
    """No forbidden command may have been executed by the agent.

    Graded against the runtime's tool-call records, never against the answer: a
    transcript match both misses a real merge and fires when the agent merely
    says it will not merge. When the runtime exposes no record the check fails
    closed rather than passing on absence of evidence.
    """
    if not commands:
        return
    if executed is None:
        raise AssertionError(
            f"{label}: cannot prove no forbidden command ran — this runtime "
            f"recorded no tool-call evidence (wanted to exclude {commands!r})"
        )
    for command in commands:
        needle = str(command).lower()
        for line in executed:
            if needle in line.lower():
                raise AssertionError(
                    f"{label}: agent executed forbidden command {command!r} ({line.strip()!r})"
                )


def git_config_get(root: Path, key: str) -> str | None:
    proc = subprocess.run(
        ["git", "-C", str(root), "config", "--get", key],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        return None
    return proc.stdout.strip()


def assert_git_config(
    root: Path,
    expected: dict[str, Any],
    *,
    branch: str | None = None,
    label: str = "scenario",
) -> None:
    """Read each key with `git config --get`; `<branch>` is substituted with
    the fixture's actual branch name. Missing key or different value fails."""
    if not expected:
        return
    name = branch or current_branch(root)
    for raw_key, want in expected.items():
        key = str(raw_key).replace("<branch>", name)
        got = git_config_get(root, key)
        if got != str(want):
            raise AssertionError(
                f"{label}: git config {key} expected {str(want)!r}, got {got!r}"
            )


TRAILER_ITEM_RANGE = (1, 6)


def assert_commit_trailer(root: Path, trailer: str, *, label: str = "scenario") -> None:
    """At least one commit body carries `<trailer> N (<reason>)`, N in 1..6.

    The range is every commit reachable from HEAD (the fixture records no base
    ref), read through `git log --format=%B`."""
    prefix = str(trailer).strip()
    lo, hi = TRAILER_ITEM_RANGE
    pattern = re.compile(
        rf"(?im)^[ \t]*{re.escape(prefix)}[ \t]+([{lo}-{hi}])[ \t]+\(.+\)[ \t]*$"
    )
    proc = subprocess.run(
        ["git", "-C", str(root), "log", "--format=%B"],
        capture_output=True,
        text=True,
        check=True,
    )
    if not pattern.search(proc.stdout):
        raise AssertionError(
            f"{label}: no commit carries {prefix!r} item {lo}-{hi} with a reason"
        )


def active_change_dir(root: Path, slug: str, mode: Any = True) -> Path:
    """`<mode tree>/changes/<slug>`: True keeps the openspec behavior."""
    name = "openspec" if mode is True else str(mode)
    tree = ACTIVE_CHANGE_TREES.get(name)
    if tree is None:
        raise ValueError(
            f"unknown assert_active_change_gone mode {mode!r}: expected true, 'openspec' or 'odd'"
        )
    return root.joinpath(*tree) / slug


def assert_active_change_gone(
    root: Path, slug: str, mode: Any = True, *, label: str = "scenario"
) -> None:
    active = active_change_dir(root, slug, mode)
    if active.exists():
        raise AssertionError(
            f"{label}: active change folder still present at {active}"
        )


def assert_scenario_outputs(
    root: Path,
    meta: dict[str, Any],
    result: dict[str, Any],
    changed: list[str],
    *,
    label: str = "scenario",
) -> None:
    """Run every `scenario.toml` assertion key in one place, so the live module
    and the primitive tests observe the same contract."""
    assert_forbidden_path_globs(changed, meta.get("forbidden_path_globs", []), label=label)
    assert_required_path_globs(root, meta.get("required_path_globs", []), changed, label=label)
    assert_required_any_path_globs(
        root, meta.get("required_any_path_globs", []), changed, label=label
    )
    assert_required_changed_globs(
        root, changed, meta.get("required_changed_globs", []), label=label
    )
    assert_absent_path_globs(root, meta.get("required_absent_path_globs", []), label=label)
    assert_required_content(root, meta.get("required_content", []), label=label)
    assert_forbidden_content(root, meta.get("forbidden_content", []), label=label)
    transcript = result.get("result_text") or result.get("stdout") or ""
    assert_transcript(
        transcript,
        all_of=meta.get("required_transcript_all", []),
        one_of=meta.get("required_transcript_one_of", []),
        any_of=meta.get("required_transcript_any", []),
        label=label,
    )
    assert_no_forbidden_commands(
        collect_executed_commands(result), meta.get("forbidden_commands", []), label=label
    )
    assert_git_config(root, meta.get("required_git_config", {}), label=label)
    trailer = meta.get("required_commit_trailer")
    if trailer:
        assert_commit_trailer(root, str(trailer), label=label)
    gone = meta.get("assert_active_change_gone")
    if gone:
        assert_active_change_gone(
            root, str(meta.get("slug", "signup-validation")), gone, label=label
        )


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
        lower = line.lower().replace("`", "").replace("*", "")
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

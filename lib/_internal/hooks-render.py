#!/usr/bin/env python3
"""Render recipe-declared runtime hooks into one harness's native format.

Usage:
    hooks-render.py <resolved_hooks_json> <agent> <project_root>

Reads a pre-resolved hooks blob (no catalog access) of the shape:

    { "enabled_agents": ["claude", ...],
      "hooks": [ { "recipe", "id", "event", "matcher", "blocking",
                   "script_path", "env": {KEY: VALUE} } ] }

and writes that agent's native wiring:

  - claude   → managed block in .claude/settings.json (script wired directly)
  - cursor   → generated shell wrapper + entry in .cursor/hooks.json
  - opencode → generated TS plugin in .opencode/plugin/<recipe>-<hook>.ts
  - pi       → generated TS extension in .pi/extensions/<recipe>-<hook>.ts

Unsupported (event, harness) pairs warn (to stderr) and skip — never emitting
broken wiring. The single materialized script is the source of truth; only the
decision channel differs per harness.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

# Abstract → native event map (v1). Single source of truth for every renderer.
# None means "no native mapping for this harness" → warn-and-skip.
# For cursor, the value is a dict keyed by the matcher *category* it applies to,
# because Cursor has no generic pre-tool event (no pre-file-write hook exists).
EVENT_MAP: dict[str, dict[str, Any]] = {
    "pre-tool-use": {
        "claude": "PreToolUse",
        "cursor": "beforeShellExecution",  # shell/MCP only; file writes have no Cursor target
        "opencode": "tool.execute.before",
        "pi": "tool_call",
    },
    "post-tool-use": {
        "claude": "PostToolUse",
        "cursor": "afterShellExecution",
        "opencode": "tool.execute.after",
        "pi": "tool_result",
    },
    "session-start": {
        "claude": "SessionStart",
        "cursor": "sessionStart",
        "opencode": None,  # observe-only via event hook; no blocking session hook
        "pi": "session_start",
    },
    "stop": {
        "claude": "Stop",
        "cursor": "stop",
        "opencode": None,  # no equivalent (session.idle is observe-only)
        "pi": "agent_end",
    },
}

# Tokens in a matcher that indicate the hook targets file writes. Cursor has no
# pre-file-write hook, so a pre-tool-use hook matching these has no Cursor target.
_FILE_WRITE_TOKENS = {"Edit", "Write", "MultiEdit", "NotebookEdit"}

MANAGED_KEY = "_ai_specs_managed"


def _managed_id(hook: dict[str, Any]) -> str:
    return f"ai-specs:hooks:{hook['recipe']}:{hook['id']}"


def _shim_basename(hook: dict[str, Any]) -> str:
    return f"{hook['recipe']}-{hook['id']}"


def _warn(msg: str) -> str:
    print(f"  ! {msg}", file=sys.stderr)
    return msg


def _matcher_targets_file_writes(matcher: str) -> bool:
    if not matcher:
        return False
    parts = {p.strip() for p in matcher.split("|")}
    return bool(parts & _FILE_WRITE_TOKENS)


def _load_json_file(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        data = json.loads(path.read_text())
        return data if isinstance(data, dict) else {}
    except (json.JSONDecodeError, OSError):
        return {}


def _write_json_file(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")


def _env_assignments(env: dict[str, Any]) -> str:
    """Render `KEY=value` assignments (shell-safe-ish) for inline env prefix."""
    out = []
    for k in sorted(env):
        v = str(env[k]).replace('"', '\\"')
        out.append(f'{k}="{v}"')
    return " ".join(out)


# --- Claude (direct, exit-code native) ---------------------------------------

def render_claude(hook: dict[str, Any], native_event: str, project_root: Path) -> None:
    settings_path = project_root / ".claude" / "settings.json"
    settings = _load_json_file(settings_path)
    hooks = settings.get("hooks")
    if not isinstance(hooks, dict):
        hooks = {}
    bucket = hooks.get(native_event)
    if not isinstance(bucket, list):
        bucket = []

    managed_id = _managed_id(hook)
    # Drop any prior managed entry for this hook (idempotent rewrite); keep user entries.
    bucket = [e for e in bucket if not (isinstance(e, dict) and e.get(MANAGED_KEY) == managed_id)]

    entry: dict[str, Any] = {MANAGED_KEY: managed_id}
    if hook.get("matcher"):
        entry["matcher"] = hook["matcher"]
    command_hook: dict[str, Any] = {
        "type": "command",
        "command": "$CLAUDE_PROJECT_DIR/" + hook["script_path"],
    }
    env = hook.get("env") or {}
    if env:
        command_hook["env"] = {k: str(v) for k, v in env.items()}
    entry["hooks"] = [command_hook]
    bucket.append(entry)

    hooks[native_event] = bucket
    settings["hooks"] = hooks
    _write_json_file(settings_path, settings)


# --- Cursor (shell wrapper, decision via stdout JSON) ------------------------

def render_cursor(hook: dict[str, Any], native_event: str, project_root: Path) -> list[str]:
    warnings: list[str] = []
    # Cursor has no pre-file-write hook: a file-write matcher has no Cursor target.
    if hook["event"] == "pre-tool-use" and _matcher_targets_file_writes(hook.get("matcher", "")):
        warnings.append(_warn(
            f"cursor: no pre-file-write hook exists; skipping hook "
            f"'{hook['recipe']}:{hook['id']}' (matcher '{hook.get('matcher','')}') for cursor"
        ))
        return warnings

    base = _shim_basename(hook)
    wrapper_path = project_root / ".cursor" / "hooks" / f"{base}.sh"
    script = "$CURSOR_PROJECT_DIR/" + hook["script_path"]
    env_prefix = _env_assignments(hook.get("env") or {})
    wrapper = (
        "#!/usr/bin/env bash\n"
        "# GENERATED by ai-specs — do not edit. Runs the recipe hook script and\n"
        "# maps exit 2 → Cursor deny (decision channel is stdout JSON, snake_case).\n"
        f'script="{script}"\n'
        "input=\"$(cat)\"\n"
        f'out="$(printf %s "$input" | {env_prefix + " " if env_prefix else ""}"$script")"; code=$?\n'
        'if [ "$code" = 2 ]; then\n'
        '  printf \'{"permission":"deny","agent_message":%s}\' "$(printf %s "$out" | python3 -c \'import json,sys;print(json.dumps(sys.stdin.read()))\')"\n'
        "else\n"
        '  printf \'{"permission":"allow"}\'\n'
        "fi\n"
        "exit 0\n"
    )
    wrapper_path.parent.mkdir(parents=True, exist_ok=True)
    wrapper_path.write_text(wrapper)
    import os
    os.chmod(wrapper_path, 0o755)

    # Reference the wrapper from .cursor/hooks.json under the native event.
    hooks_json_path = project_root / ".cursor" / "hooks.json"
    cfg = _load_json_file(hooks_json_path)
    hooks = cfg.get("hooks")
    if not isinstance(hooks, dict):
        hooks = {}
    bucket = hooks.get(native_event)
    if not isinstance(bucket, list):
        bucket = []
    managed_id = _managed_id(hook)
    bucket = [e for e in bucket if not (isinstance(e, dict) and e.get(MANAGED_KEY) == managed_id)]
    bucket.append({
        MANAGED_KEY: managed_id,
        "command": "./.cursor/hooks/" + base + ".sh",
    })
    hooks[native_event] = bucket
    cfg["hooks"] = hooks
    _write_json_file(hooks_json_path, cfg)
    return warnings


# --- OpenCode (TS plugin, decision via throw) --------------------------------

def render_opencode(hook: dict[str, Any], native_event: str, project_root: Path) -> None:
    base = _shim_basename(hook)
    path = project_root / ".opencode" / "plugin" / f"{base}.ts"
    env = hook.get("env") or {}
    env_lines = "".join(
        f"      {json.dumps(k)}: {json.dumps(str(v))},\n" for k, v in sorted(env.items())
    )
    content = (
        "// GENERATED by ai-specs — do not edit.\n"
        f"// Runtime hook {hook['recipe']}:{hook['id']} (event {hook['event']}).\n"
        "// Spawns the single materialized recipe script with the normalized event\n"
        "// on stdin; exit 2 -> throw (block). NOTE: tool.execute.before does NOT\n"
        "// fire for subagent (#5894) or MCP (#2319) tool calls.\n"
        "import { spawnSync } from \"node:child_process\";\n\n"
        f"const SCRIPT = \"{hook['script_path']}\";\n"
        f"const MATCHER = {json.dumps(hook.get('matcher') or '')};\n"
        "const ENV = {\n"
        f"{env_lines}"
        "};\n\n"
        "export const plugin = async ({ project, directory }) => {\n"
        "  return {\n"
        "    \"tool.execute.before\": async (input, output) => {\n"
        "      const toolName = input?.tool ?? output?.tool ?? \"\";\n"
        "      if (MATCHER) {\n"
        "        const re = new RegExp(`^(?:${MATCHER})$`);\n"
        "        if (!re.test(toolName)) return;\n"
        "      }\n"
        "      const event = {\n"
        "        event: \"pre-tool-use\",\n"
        "        tool_name: toolName,\n"
        "        tool_input: output?.args ?? {},\n"
        "        cwd: directory ?? process.cwd(),\n"
        "      };\n"
        "      const res = spawnSync(SCRIPT, {\n"
        "        input: JSON.stringify(event),\n"
        "        env: { ...process.env, ...ENV },\n"
        "        encoding: \"utf8\",\n"
        "      });\n"
        "      if (res.status === 2) {\n"
        "        throw new Error(res.stderr || \"blocked by ai-specs runtime hook\");\n"
        "      }\n"
        "      // any other exit code: fail-open (allow).\n"
        "    },\n"
        "  };\n"
        "};\n"
    )
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


# --- Pi (TS extension, decision via return {block:true}) ---------------------

def render_pi(hook: dict[str, Any], native_event: str, project_root: Path) -> None:
    base = _shim_basename(hook)
    path = project_root / ".pi" / "extensions" / f"{base}.ts"
    env = hook.get("env") or {}
    env_lines = "".join(
        f"  {json.dumps(k)}: {json.dumps(str(v))},\n" for k, v in sorted(env.items())
    )
    content = (
        "// GENERATED by ai-specs — do not edit.\n"
        f"// Runtime hook {hook['recipe']}:{hook['id']} (event {hook['event']}).\n"
        "import type { ExtensionAPI } from \"@earendil-works/pi-coding-agent\";\n"
        "import { spawnSync } from \"node:child_process\";\n\n"
        f"const SCRIPT = \"{hook['script_path']}\";\n"
        f"const MATCHER = {json.dumps(hook.get('matcher') or '')};\n"
        "const ENV: Record<string, string> = {\n"
        f"{env_lines}"
        "};\n\n"
        "export default function (pi: ExtensionAPI) {\n"
        "  pi.on(\"tool_call\", (call: any) => {\n"
        "    const toolName = call?.toolName ?? call?.tool_name ?? call?.name ?? \"\";\n"
        "    if (MATCHER) {\n"
        "      const re = new RegExp(`^(?:${MATCHER})$`);\n"
        "      if (!re.test(toolName)) return;\n"
        "    }\n"
        "    const event = {\n"
        "      event: \"pre-tool-use\",\n"
        "      tool_name: toolName,\n"
        "      tool_input: call?.input ?? call?.arguments ?? {},\n"
        "      cwd: process.cwd(),\n"
        "    };\n"
        "    const res = spawnSync(SCRIPT, {\n"
        "      input: JSON.stringify(event),\n"
        "      env: { ...process.env, ...ENV },\n"
        "      encoding: \"utf8\",\n"
        "    });\n"
        "    if (res.status === 2) {\n"
        "      return { block: true, reason: res.stderr || \"blocked by ai-specs runtime hook\" };\n"
        "    }\n"
        "    // any other exit code: fail-open (allow).\n"
        "  });\n"
        "}\n"
    )
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


# --- Dispatch ----------------------------------------------------------------

def render(resolved_hooks_path: Path, agent: str, project_root: Path) -> list[str]:
    """Render every applicable hook for one agent. Returns warning messages."""
    warnings: list[str] = []
    data = _load_json_file(Path(resolved_hooks_path))
    hooks = data.get("hooks") if isinstance(data, dict) else None
    if not isinstance(hooks, list):
        return warnings

    for hook in hooks:
        if not isinstance(hook, dict):
            continue
        event = hook.get("event", "")
        mapping = EVENT_MAP.get(event)
        if mapping is None:
            warnings.append(_warn(
                f"unknown abstract event '{event}' for hook "
                f"'{hook.get('recipe')}:{hook.get('id')}' — skipped"
            ))
            continue
        native_event = mapping.get(agent)
        if native_event is None:
            warnings.append(_warn(
                f"event '{event}' has no native mapping for harness '{agent}' "
                f"(recipe '{hook.get('recipe')}', hook '{hook.get('id')}') — skipped"
            ))
            continue

        if agent == "claude":
            render_claude(hook, native_event, project_root)
        elif agent == "cursor":
            warnings.extend(render_cursor(hook, native_event, project_root))
        elif agent == "opencode":
            render_opencode(hook, native_event, project_root)
        elif agent == "pi":
            render_pi(hook, native_event, project_root)
        else:
            warnings.append(_warn(
                f"harness '{agent}' has no runtime-hook renderer "
                f"(recipe '{hook.get('recipe')}', hook '{hook.get('id')}') — skipped"
            ))

    return warnings


def main() -> int:
    args = sys.argv[1:]
    if len(args) != 3:
        print(
            f"Usage: {sys.argv[0]} <resolved_hooks_json> <agent> <project_root>",
            file=sys.stderr,
        )
        return 2
    resolved_hooks_path = Path(args[0])
    agent = args[1]
    project_root = Path(args[2]).resolve()
    render(resolved_hooks_path, agent, project_root)
    return 0


if __name__ == "__main__":
    sys.exit(main())

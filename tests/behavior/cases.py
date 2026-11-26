"""One case per behavior the plan-build contract promises, in both modes.

A case receives a fresh `Sandbox`, drives the compiled CLI through the steps the
rule cares about, and returns the bodies it wants legible in the golden. The
golden then holds the three evidence layers: what the CLI said (exit/stdout/
stderr), what it left behind (tree digests) and, for the files a case names, the
normalized body itself.
"""
from __future__ import annotations

from collections.abc import Callable

from tests.behavior.harness import Sandbox

HookPayload = str


def pre_tool_use(path: str, tool: str = "Edit") -> HookPayload:
    """The stdin shape a pre-tool-use hook receives."""
    import json

    return json.dumps(
        {
            "hook_event_name": "PreToolUse",
            "tool_name": tool,
            "tool_input": {"file_path": path, "content": "x"},
            "cwd": ".",
        }
    )


def case_sync_plan_build_hook(sb: Sandbox) -> dict:
    """S2: which plan-build hook does `sync` materialize with the recipe enabled."""
    sb.manifest()
    sb.run("sync", ".")
    return {"files": sb.contents(".claude/**", ".pi/**", ".cursor/**", "ai-specs/recipes/**")}


CASES: list[tuple[str, Callable[[Sandbox], dict | None], str]] = [
    ("sync-plan-build-hook", case_sync_plan_build_hook, "openspec"),
]

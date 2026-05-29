#!/usr/bin/env python3
"""Generate or update AGENTS.md from the ai-specs manifest.

Usage:
    agents-render.py <toml_path> <output_path> [--preserve-if-runtime-brief]
                     [--resolved-config <path>]

Rules:
  - With --preserve-if-runtime-brief: if output_path exists and contains
    '<!-- ai-specs:runtime-brief -->', leave it untouched (user-managed brief).
  - Otherwise: write a generated AGENTS.md with project name, MCP summary,
    and [brief] prose sections (if present in manifest).
  - With --resolved-config <path>: load pre-computed JSON and emit structured
    fields (board_id, integration_branch, test_command, vault_scope, etc.).
  - Without --resolved-config: degrade gracefully — render prose + identity +
    MCP only; skip structured-field sections that need resolved config.
"""

from __future__ import annotations

import argparse
import json
import sys
import tomllib
from pathlib import Path
from typing import Any

RUNTIME_BRIEF_MARKER = "<!-- ai-specs:runtime-brief -->"


def _redact_env_value(value: str) -> str:
    """Return a safe representation of an MCP env value.

    $VAR or ${VAR}  →  ${VAR}
    literal secret  →  ***REDACTED***
    """
    stripped = str(value).strip()
    if stripped.startswith("$"):
        var = stripped[1:].strip("{}")
        return f"${{{var}}}"
    return "***REDACTED***"


# ---------------------------------------------------------------------------
# Per-section helpers
# ---------------------------------------------------------------------------

def _section_intro(brief: dict) -> list[str]:
    """Emit intro blockquote from [brief].intro (multi-line string → '> ' prefix)."""
    intro = brief.get("intro", "")
    if not intro or not str(intro).strip():
        return []
    lines: list[str] = []
    for raw_line in str(intro).splitlines():
        lines.append(f"> {raw_line}" if raw_line.strip() else ">")
    lines.append("")
    return lines


def _section_project(manifest: dict, resolved: dict) -> list[str]:
    """Emit ## Project section: name, manifest path, purpose, enabled runtimes, integration_branch."""
    project = manifest.get("project", {}) or {}
    agents = manifest.get("agents", {}) or {}
    brief = manifest.get("brief", {}) or {}
    name = project.get("name", "")

    lines: list[str] = ["## Project", ""]

    if name:
        lines.append(f"- **Project**: `{name}`")

    purpose = brief.get("purpose", "")
    if purpose:
        lines.append(f"- **Purpose**: {purpose}")

    enabled_runtimes = agents.get("enabled", []) or []
    if enabled_runtimes:
        runtimes_str = ", ".join(f"`{r}`" for r in enabled_runtimes)
        lines.append(f"- **Enabled runtimes**: {runtimes_str}")

    # integration_branch from resolved config via worktree-isolation capability binding
    integration_branch = ""
    recipes = resolved.get("recipes", {}) or {}
    bindings = resolved.get("bindings", {}) or {}
    worktree_recipe_id = bindings.get("worktree-isolation", "")
    if worktree_recipe_id:
        wf_cfg = recipes.get(worktree_recipe_id, {}) or {}
        integration_branch = wf_cfg.get("integration_branch", "")
    if not integration_branch:
        # Fallback: try well-known recipe ids directly
        wf_cfg = recipes.get("worktree-flow", {}) or {}
        integration_branch = wf_cfg.get("integration_branch", "")
    if not integration_branch:
        gp_cfg = recipes.get("git-pr-flow", {}) or {}
        integration_branch = gp_cfg.get("base_branch", "")

    if integration_branch:
        lines.append(f"- **Integration branch**: `{integration_branch}`")

    # vault_scope from canonical-store binding
    canonical_store_id = bindings.get("canonical-store", "")
    if canonical_store_id:
        cs_cfg = recipes.get(canonical_store_id, {}) or {}
        vault_scope = cs_cfg.get("vault_scope", "")
        if vault_scope:
            lines.append(f"- **Vault scope**: `{vault_scope}`")

    lines.append("")
    return lines


def _section_mcp(manifest: dict, brief: dict) -> list[str]:
    """Emit ## Runtime MCPs section: table + per-server description + secrets rule.

    Also renders description-only entries from [brief].mcp_descriptions that
    have no matching [mcp.*] block (e.g. global MCPs like engram).
    """
    mcp: dict = manifest.get("mcp", {}) or {}
    mcp_descriptions = brief.get("mcp_descriptions", {}) or {}

    if not mcp and not mcp_descriptions:
        return []

    lines: list[str] = ["## Runtime MCPs", ""]
    for server_name, cfg in mcp.items():
        cfg = cfg or {}
        lines.append(f"**{server_name}**")
        if "command" in cfg:
            lines.append(f"- command: {cfg['command']}")
        args = cfg.get("args", []) or []
        if args:
            lines.append(f"- args: {' '.join(str(a) for a in args)}")
        env = cfg.get("env") or cfg.get("environment") or {}
        if env:
            lines.append("- env:")
            if isinstance(env, list):
                for var in env:
                    lines.append(f"  - {var}: ${{{var}}}")
            else:
                for k, v in env.items():
                    lines.append(f"  - {k}: {_redact_env_value(str(v))}")
        desc = mcp_descriptions.get(server_name, "")
        if desc:
            lines.append(f"- description: {desc}")
        lines.append("")

    # Render description-only entries (present in mcp_descriptions but absent from [mcp.*])
    for server_name, desc in mcp_descriptions.items():
        if server_name in mcp:
            continue  # already rendered above
        if not desc:
            continue
        lines.append(f"**{server_name}** *(global)*")
        lines.append(f"- description: {desc}")
        lines.append("")

    lines.append("Never expose env-backed secrets from MCP config in generated docs or comments.")
    lines.append("")
    return lines


def _section_runtime_flow(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Runtime Flow bullets from [brief].runtime_flow + VCS provider bullet."""
    runtime_flow = brief.get("runtime_flow", []) or []
    bindings = resolved.get("bindings", {}) or {}
    recipes = resolved.get("recipes", {}) or {}

    # VCS provider bullet
    vcs_recipe_id = bindings.get("vcs-pr-flow", "")
    provider = ""
    base_branch = ""
    if vcs_recipe_id:
        vcs_cfg = recipes.get(vcs_recipe_id, {}) or {}
        provider = vcs_cfg.get("provider", "")
        base_branch = vcs_cfg.get("base_branch", "")

    if not runtime_flow and not provider:
        return []

    lines: list[str] = ["## Runtime Flow", ""]
    for item in runtime_flow:
        if item:
            lines.append(f"- {item}")
    if provider:
        vcs_note = f"VCS/PR provider: {provider}"
        if provider == "github":
            vcs_note += " (`gh` CLI)"
        if base_branch:
            vcs_note += f"; base branch: `{base_branch}`"
        lines.append(f"- {vcs_note}")
    lines.append("")
    return lines


def _section_trello(resolved: dict) -> list[str]:
    """Emit ## Trello Tracking from resolved bindings.tracker → board_id. Omit if absent."""
    bindings = resolved.get("bindings", {}) or {}
    recipes = resolved.get("recipes", {}) or {}

    tracker_recipe_id = bindings.get("tracker", "")
    if not tracker_recipe_id:
        return []

    tracker_cfg = recipes.get(tracker_recipe_id, {}) or {}
    board_id = tracker_cfg.get("board_id", "")
    if not board_id:
        return []

    lines: list[str] = [
        "## Trello Tracking",
        "",
        f"- **Board**: `{board_id}`",
        "",
    ]
    return lines


def _section_context_sources(brief: dict) -> list[str]:
    """Emit ## Context Sources bullets from [brief].context_sources. Omit if absent."""
    context_sources = brief.get("context_sources", []) or []
    if not context_sources:
        return []
    lines: list[str] = ["## Context Sources", ""]
    for item in context_sources:
        if item:
            lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_conflict_policy(brief: dict) -> list[str]:
    """Emit ## Conflict Policy bullets from [brief].conflict_policy. Omit if absent."""
    conflict_policy = brief.get("conflict_policy", []) or []
    if not conflict_policy:
        return []
    lines: list[str] = ["## Conflict Policy", ""]
    for item in conflict_policy:
        if item:
            lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_workflow_rules(brief: dict) -> list[str]:
    """Emit ## Workflow Rules bullets from [brief].workflow_rules. Omit if absent."""
    workflow_rules = brief.get("workflow_rules", []) or []
    if not workflow_rules:
        return []
    lines: list[str] = ["## Workflow Rules", ""]
    for item in workflow_rules:
        if item:
            lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_useful_commands(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Useful Commands from test-runner binding test_command + [brief].useful_commands.

    Only emits commands that are explicitly provided (from test-runner binding or
    [brief].useful_commands). Never fabricates commands via string replacement.
    """
    recipes = resolved.get("recipes", {}) or {}
    bindings = resolved.get("bindings", {}) or {}

    # Resolve test_command via test-runner capability binding (FIX 10)
    test_command = ""
    test_runner_id = bindings.get("test-runner", "")
    if test_runner_id:
        tdd_cfg = recipes.get(test_runner_id, {}) or {}
        test_command = tdd_cfg.get("test_command", "")
    if not test_command:
        # Fallback: try well-known recipe id directly
        tdd_cfg = recipes.get("tdd-flow", {}) or {}
        test_command = tdd_cfg.get("test_command", "")

    extra_commands = brief.get("useful_commands", []) or []

    if not test_command and not extra_commands:
        return []

    lines: list[str] = ["## Useful Commands", ""]
    if test_command:
        # Label heuristic: validate.sh = full suite; run.sh = focused/unit-only
        if "validate.sh" in test_command:
            lines.append(f"- Full validation: `{test_command}`")
        else:
            lines.append(f"- Focused tests: `{test_command}`")
    for cmd in extra_commands:
        if cmd:
            lines.append(f"- {cmd}")
    lines.append("")
    return lines


# ---------------------------------------------------------------------------
# Main render
# ---------------------------------------------------------------------------

def _render_lines(manifest: dict, resolved: dict) -> list[str]:
    """Compose AGENTS.md lines in fixed section order.

    Section order:
    1. H1 heading
    2. Intro blockquote (from [brief].intro)
    3. ## Project
    4. ## Runtime MCPs
    5. ## Runtime Flow
    6. ## Trello Tracking
    7. ## Context Sources
    8. ## Conflict Policy
    9. ## Workflow Rules
    10. ## Useful Commands
    """
    project = manifest.get("project", {}) or {}
    name = project.get("name", "")
    brief = manifest.get("brief", {}) or {}

    lines: list[str] = []

    # 1. H1 heading
    if name:
        lines += [f"# {name} Runtime Brief", ""]
    else:
        lines += ["# AGENTS.md - Runtime context", ""]

    # 2. Intro blockquote
    lines += _section_intro(brief)

    # 3. ## Project
    lines += _section_project(manifest, resolved)

    # 4. ## Runtime MCPs
    lines += _section_mcp(manifest, brief)

    # 5. ## Runtime Flow
    lines += _section_runtime_flow(brief, resolved)

    # 6. ## Trello Tracking
    lines += _section_trello(resolved)

    # 7. ## Context Sources
    lines += _section_context_sources(brief)

    # 8. ## Conflict Policy
    lines += _section_conflict_policy(brief)

    # 9. ## Workflow Rules
    lines += _section_workflow_rules(brief)

    # 10. ## Useful Commands
    lines += _section_useful_commands(brief, resolved)

    return lines


def render(toml_path: Path, output_path: Path, *, preserve_if_marker: bool, resolved_config_path: Path | None = None) -> None:
    if preserve_if_marker and output_path.exists():
        if RUNTIME_BRIEF_MARKER in output_path.read_text():
            return  # user-managed runtime brief — leave alone

    with open(toml_path, "rb") as fh:
        manifest = tomllib.load(fh)

    resolved: dict[str, Any] = {}
    if resolved_config_path is not None and resolved_config_path.is_file():
        try:
            with open(resolved_config_path) as fh:
                data = json.load(fh)
            if not isinstance(data, dict):
                data = {}
            # Coerce inner fields to their expected types so wrong-typed values
            # (e.g. {"bindings": ["x"]}) degrade instead of crashing downstream.
            if not isinstance(data.get("bindings"), dict):
                data["bindings"] = {}
            if not isinstance(data.get("recipes"), dict):
                data["recipes"] = {}
            if not isinstance(data.get("enabled"), list):
                data["enabled"] = []
            resolved = data
        except (json.JSONDecodeError, ValueError, OSError):
            resolved = {}  # degrade gracefully — render without structured fields

    content = "\n".join(_render_lines(manifest, resolved))
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(content)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("toml_path", help="Path to ai-specs.toml")
    parser.add_argument("output_path", help="Path to write AGENTS.md")
    parser.add_argument(
        "--preserve-if-runtime-brief",
        action="store_true",
        help="Leave output_path unchanged if it contains the runtime-brief marker",
    )
    parser.add_argument(
        "--resolved-config",
        metavar="PATH",
        help="Path to resolved-config JSON (bindings + per-recipe configs + enabled list)",
    )
    args = parser.parse_args()

    resolved_config_path = Path(args.resolved_config) if args.resolved_config else None

    render(
        Path(args.toml_path),
        Path(args.output_path),
        preserve_if_marker=args.preserve_if_runtime_brief,
        resolved_config_path=resolved_config_path,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())

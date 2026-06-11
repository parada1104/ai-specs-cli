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
import re
import sys
import tomllib
from pathlib import Path
from typing import Any

RUNTIME_BRIEF_MARKER = "<!-- ai-specs:runtime-brief -->"

_VALID_MODES = {"append", "replace"}

# Bound vcs-pr-flow recipe id → (display name, CLI slug) for Runtime Flow bullet.
_VCS_RECIPE_LABELS: dict[str, tuple[str, str]] = {
    "git-pr-flow": ("GitHub", "gh"),
    "gitlab-mr-flow": ("GitLab", "glab"),
    "bitbucket-pr-flow": ("Bitbucket", "bb"),
}


# ---------------------------------------------------------------------------
# Recipe brief fragment helpers
# ---------------------------------------------------------------------------

_CONFIG_PLACEHOLDER_RE = re.compile(r"\{\{|\}\}|\{config\.([A-Za-z0-9_]+)\}")


def substitute_config(text: str, cfg_ns: dict) -> str:
    """Apply {config.KEY} substitution to a recipe fragment text.

    - {config.KEY} with KEY present in cfg_ns → resolved to its value.
    - {config.KEY} with KEY absent from cfg_ns → re-emitted verbatim.
    - Bare {KEY} without the config. prefix → left verbatim (not substituted).
    - {{ → literal {, }} → literal } (escape sequences).
    - A lone unbalanced { in prose → text returned untouched (no crash).

    Uses regex scanning so that dotted keys like "config.integration_branch" are
    matched as a single token, avoiding str.format_map's attribute-access parsing.
    """
    # First, check for lone unbalanced braces that would confuse processing.
    # Strategy: use regex to handle only {{ }}, {config.KEY} patterns; pass everything else through.
    try:
        def _replace(m: re.Match) -> str:
            token = m.group(0)
            if token == "{{":
                return "{"
            if token == "}}":
                return "}"
            key_name = m.group(1)  # captured KEY from {config.KEY}
            cfg_key = f"config.{key_name}"
            if cfg_key in cfg_ns:
                return str(cfg_ns[cfg_key])
            # Unknown key → return verbatim placeholder
            return token

        return _CONFIG_PLACEHOLDER_RE.sub(_replace, text)
    except (ValueError, IndexError):
        return text  # safety net — should not normally be reached with this regex approach


def collect_recipe_brief_fragments(
    resolved: dict,
    section: str,
    *,
    recipe_ids: set[str] | None = None,
) -> list[dict]:
    """Collect and deduplicate brief fragments for *section* from enabled recipes.

    Iterates resolved["enabled"] in order, applies {config.KEY} substitution using
    each recipe's own config namespace, and deduplicates:
      1. Key-based: first occurrence of a non-None key wins; later duplicates discarded.
      2. Exact-string: duplicate text (after substitution) discarded.

    When *recipe_ids* is provided, only recipes in that set contribute fragments.
    When *recipe_ids* is ``None`` (default), all enabled recipes contribute.

    Returns a list of {"key": ..., "text": ...} dicts, substituted.
    """
    out: list[dict] = []
    seen_keys: set[str] = set()
    seen_text: set[str] = set()

    for rid in resolved.get("enabled", []):
        if recipe_ids is not None and rid not in recipe_ids:
            continue
        rcfg = resolved.get("recipes", {}).get(rid, {}) or {}
        # Build config namespace: all keys except brief_fragments itself, prefixed with "config."
        cfg_ns = {
            f"config.{k}": v
            for k, v in rcfg.items()
            if k != "brief_fragments"
        }
        for frag in (rcfg.get("brief_fragments") or {}).get(section, []):
            key = frag.get("key")
            raw = frag.get("text", "")
            # Key-based dedup — first occurrence wins
            if key is not None and key in seen_keys:
                continue
            text = substitute_config(raw, cfg_ns)
            # Exact-string dedup
            if text in seen_text:
                continue
            if key is not None:
                seen_keys.add(key)
            seen_text.add(text)
            out.append({"key": key, "text": text})

    return out


def _validate_brief_modes(brief: dict) -> None:
    """Validate all <section>_mode keys in *brief*.

    Raises ValueError for any value not in {"append", "replace"}.
    """
    for key, val in brief.items():
        if key.endswith("_mode"):
            if val not in _VALID_MODES:
                raise ValueError(
                    f"[brief].{key}: invalid mode {val!r}; valid values are "
                    f"{sorted(_VALID_MODES)}"
                )


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
    """Emit ## Runtime Flow bullets from recipe fragments + [brief].runtime_flow + VCS bullet."""
    mode = brief.get("runtime_flow_mode", "append")
    recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(resolved, "runtime_flow")
    manifest_items = brief.get("runtime_flow", []) or []

    bindings = resolved.get("bindings", {}) or {}
    recipes = resolved.get("recipes", {}) or {}

    # VCS provider bullet — identity from bound recipe id, not config.provider
    vcs_recipe_id = bindings.get("vcs-pr-flow", "")
    base_branch = ""
    vcs_label: tuple[str, str] | None = None
    _warned_vcs_ids: set[str] = set()  # local de-dupe for unknown VCS warnings
    if vcs_recipe_id:
        vcs_cfg = recipes.get(vcs_recipe_id, {}) or {}
        base_branch = vcs_cfg.get("base_branch", "")
        vcs_label = _VCS_RECIPE_LABELS.get(vcs_recipe_id)
        if vcs_label is None and vcs_recipe_id not in _warned_vcs_ids:
            print(
                f"\u26a0 ai-specs: VCS recipe '{vcs_recipe_id}' is not in the known "
                f"label set; using generic label 'VCS PR (custom)'",
                file=sys.stderr,
            )
            _warned_vcs_ids.add(vcs_recipe_id)
            vcs_label = ("VCS PR (custom)", "VCS PR (custom)")

    bullets: list[str] = [f["text"] for f in recipe_items]
    for m in manifest_items:
        if m and m not in bullets:
            bullets.append(m)

    if not bullets and not vcs_label and not base_branch:
        return []

    lines: list[str] = ["## Runtime Flow", ""]
    for item in bullets:
        lines.append(f"- {item}")
    if vcs_label:
        name, cli = vcs_label
        if cli == name:
            # Generic/custom label — no CLI slug
            vcs_note = f"VCS/PR provider: {name}"
        else:
            vcs_note = f"VCS/PR provider: {name} (`{cli}` CLI)"
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


def _section_context_sources(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Context Sources bullets from recipe fragments + [brief].context_sources."""
    mode = brief.get("context_sources_mode", "append")
    recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(resolved, "context_sources")
    manifest_items = brief.get("context_sources", []) or []

    bullets: list[str] = [f["text"] for f in recipe_items]
    for m in manifest_items:
        if m and m not in bullets:
            bullets.append(m)

    if not bullets:
        return []
    lines: list[str] = ["## Context Sources", ""]
    for item in bullets:
        lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_conflict_policy(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Conflict Policy bullets from recipe fragments + [brief].conflict_policy."""
    mode = brief.get("conflict_policy_mode", "append")
    recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(resolved, "conflict_policy")
    manifest_items = brief.get("conflict_policy", []) or []

    bullets: list[str] = [f["text"] for f in recipe_items]
    for m in manifest_items:
        if m and m not in bullets:
            bullets.append(m)

    if not bullets:
        return []
    lines: list[str] = ["## Conflict Policy", ""]
    for item in bullets:
        lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_workflow_rules(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Workflow Rules bullets from recipe fragments + [brief].workflow_rules."""
    mode = brief.get("workflow_rules_mode", "append")
    # Compute allowed recipe set for workflow_rules fragments.
    # VCS sibling fragments are filtered to the bound recipe only (or excluded
    # entirely when no vcs-pr-flow binding exists). Non-VCS recipes always contribute.
    recipe_ids: set[str] | None = None
    bindings = resolved.get("bindings", {}) or {}
    bound_vcs_id = bindings.get("vcs-pr-flow", "")
    enabled = set(resolved.get("enabled", []))
    vcs_siblings = set(_VCS_RECIPE_LABELS.keys())
    if bound_vcs_id:
        # Exclude VCS siblings except the bound one
        excluded = vcs_siblings - {bound_vcs_id}
        recipe_ids = enabled - excluded
    else:
        # No binding — exclude all VCS siblings from workflow_rules
        recipe_ids = enabled - vcs_siblings
    recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(
        resolved, "workflow_rules", recipe_ids=recipe_ids,
    )
    manifest_items = brief.get("workflow_rules", []) or []

    bullets: list[str] = [f["text"] for f in recipe_items]
    for m in manifest_items:
        if m and m not in bullets:
            bullets.append(m)

    if not bullets:
        return []
    lines: list[str] = ["## Workflow Rules", ""]
    for item in bullets:
        lines.append(f"- {item}")
    lines.append("")
    return lines


def _section_useful_commands(brief: dict, resolved: dict) -> list[str]:
    """Emit ## Useful Commands from test-runner binding, recipe fragments, and [brief].useful_commands.

    Only emits commands that are explicitly provided (from test-runner binding,
    recipe fragments, or [brief].useful_commands). Never fabricates commands via
    string replacement.
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

    mode = brief.get("useful_commands_mode", "append")
    recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(resolved, "useful_commands")
    extra_commands = brief.get("useful_commands", []) or []

    # Build bullets: recipe items first, then manifest items (exact-string dedup vs recipe)
    recipe_bullets: list[str] = [f["text"] for f in recipe_items]
    manifest_bullets: list[str] = []
    for cmd in extra_commands:
        if cmd and cmd not in recipe_bullets:
            manifest_bullets.append(cmd)

    if not test_command and not recipe_bullets and not manifest_bullets:
        return []

    lines: list[str] = ["## Useful Commands", ""]
    if test_command:
        # Label heuristic: validate.sh = full suite; run.sh = focused/unit-only
        if "validate.sh" in test_command:
            lines.append(f"- Full validation: `{test_command}`")
        else:
            lines.append(f"- Focused tests: `{test_command}`")
    for item in recipe_bullets:
        lines.append(f"- {item}")
    for cmd in manifest_bullets:
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

    # Validate _mode keys early — fail-fast with a clear message
    _validate_brief_modes(brief)

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

    # 4. ## Runtime MCPs — build effective mcp_descriptions (override-fills-gap)
    # Recipe-provided descriptions fill gaps; manifest [brief].mcp_descriptions overrides.
    eff_mcp: dict[str, str] = {
        f["key"]: f["text"]
        for f in collect_recipe_brief_fragments(resolved, "mcp_descriptions")
        if f.get("key")
    }
    eff_mcp.update(brief.get("mcp_descriptions", {}) or {})  # project wins
    # Build a local brief copy with effective mcp_descriptions (no mutation of caller's dict)
    brief_with_eff_mcp = {**brief, "mcp_descriptions": eff_mcp}
    lines += _section_mcp(manifest, brief_with_eff_mcp)

    # 5. ## Runtime Flow
    lines += _section_runtime_flow(brief, resolved)

    # 6. ## Trello Tracking
    lines += _section_trello(resolved)

    # 7. ## Context Sources
    lines += _section_context_sources(brief, resolved)

    # 8. ## Conflict Policy
    lines += _section_conflict_policy(brief, resolved)

    # 9. ## Workflow Rules
    lines += _section_workflow_rules(brief, resolved)

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

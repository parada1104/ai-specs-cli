#!/usr/bin/env python3
"""SDD / OpenSpec provider orchestration for ai-specs (manifest [sdd])."""
from __future__ import annotations

import argparse
import ast
import os
import re
import shutil
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import Any

AI_SPECS_HOME = Path(__file__).resolve().parents[2]
SDD_MARKER = "# ai-specs-sdd-merge:v1"
OPENSPEC_NPM_PACKAGE = "@fission-ai/openspec"
NODE_MIN = (20, 19)
# `openspec init --profile` accepts core | custom (see `openspec init --help`).
OPENSPEC_INIT_PROFILE = "custom"
ARTIFACT_STORES = frozenset({"filesystem", "hybrid", "memory"})
SDD_KNOWN_KEYS = frozenset({"enabled", "provider", "artifact_store"})
AGENT_TO_OPENSPEC_TOOL = {
    "claude": "claude",
    "cursor": "cursor",
    "opencode": "opencode",
    "codex": "codex",
    "copilot": "github-copilot",
    "gemini": "gemini",
}
RUN_OPENSPEC_UPDATE_AFTER_INIT = True

CEREMONY_LEVELS = frozenset({"trivial", "local_fix", "behavior_change", "domain_change"})


def _parse_yaml_scalar(val: str) -> Any:
    """Parse a minimal subset of YAML scalars."""
    val = val.strip()
    if val == "true":
        return True
    if val == "false":
        return False
    if val == "null" or val == "~":
        return None
    if val.startswith("[") and val.endswith("]"):
        safe = val.replace("true", "True").replace("false", "False").replace("null", "None")
        return ast.literal_eval(safe)
    if val.startswith('"') and val.endswith('"'):
        return val[1:-1]
    if val.startswith("'") and val.endswith("'"):
        return val[1:-1]
    return val


def _load_yaml_simple(text: str) -> dict[str, Any]:
    """Parse a minimal subset of YAML: nested mappings and basic scalars."""
    lines = text.splitlines()
    root: dict[str, Any] = {}
    # Stack of (indent, container_dict)
    stack: list[tuple[int, dict[str, Any]]] = [(-1, root)]

    for line in lines:
        if not line.strip() or line.strip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip())
        stripped = line.lstrip()
        if ":" not in stripped:
            continue
        key, _, rest = stripped.partition(":")
        key = key.strip()
        value = rest.strip()

        # Pop to correct parent level
        while len(stack) > 1 and stack[-1][0] >= indent:
            stack.pop()

        parent = stack[-1][1]
        if value:
            parent[key] = _parse_yaml_scalar(value)
        else:
            child: dict[str, Any] = {}
            parent[key] = child
            stack.append((indent, child))

    return root


def load_decision_matrix(config_path: Path) -> dict[str, Any] | None:
    """Read openspec/config.yaml and return the decision matrix dict if adaptive mode is set."""
    if not config_path.is_file():
        return None
    text = config_path.read_text(encoding="utf-8")
    data = _load_yaml_simple(text)
    sdd = data.get("sdd")
    if not isinstance(sdd, dict):
        return None
    mode = str(sdd.get("mode", ""))
    if mode != "adaptive":
        return None
    matrix = sdd.get("decision_matrix")
    if not isinstance(matrix, dict):
        return None
    return matrix


def validate_decision_matrix(matrix: dict[str, Any]) -> list[str]:
    """Validate a decision matrix dict. Returns a list of error strings (empty if valid)."""
    errs: list[str] = []
    missing = CEREMONY_LEVELS - set(matrix.keys())
    for level in sorted(missing):
        errs.append(f"missing level: {level}")
    extra = set(matrix.keys()) - CEREMONY_LEVELS
    for level in sorted(extra):
        errs.append(f"unknown level: {level}")
    for level in sorted(CEREMONY_LEVELS & set(matrix.keys())):
        entry = matrix[level]
        if not isinstance(entry, dict):
            errs.append(f"level {level}: expected mapping, got {type(entry).__name__}")
            continue
        artifacts = entry.get("artifacts")
        if not isinstance(artifacts, list):
            errs.append(f"level {level}: 'artifacts' must be a list, got {type(artifacts).__name__}")
        for flag in ("worktree_required", "proposal_required", "design_required"):
            val = entry.get(flag)
            if not isinstance(val, bool):
                errs.append(f"level {level}: '{flag}' must be a boolean, got {type(val).__name__}")
    return errs


def manifest_path(root: Path) -> Path:
    return root / "ai-specs" / "ai-specs.toml"


def load_manifest_dict(root: Path) -> dict[str, Any]:
    p = manifest_path(root)
    if not p.is_file():
        return {}
    with p.open("rb") as f:
        return tomllib.load(f)


def parse_node_version(text: str) -> tuple[int, int] | None:
    m = re.match(r"^v(\d+)\.(\d+)", text.strip())
    if not m:
        return None
    return int(m.group(1)), int(m.group(2))


def check_node_version() -> tuple[bool, str]:
    try:
        out = subprocess.run(
            ["node", "-v"],
            check=True,
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (FileNotFoundError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
        return False, f"node not runnable ({exc})"
    ver = parse_node_version(out.stdout)
    if ver is None:
        return False, f"unparseable node version: {out.stdout!r}"
    if ver < NODE_MIN:
        return (
            False,
            f"Node {ver[0]}.{ver[1]} < required {NODE_MIN[0]}.{NODE_MIN[1]} for {OPENSPEC_NPM_PACKAGE}",
        )
    return True, f"node {out.stdout.strip()}"


def which_openspec() -> str | None:
    return shutil.which("openspec")


def run(cmd: list[str], *, cwd: Path | None = None, env: dict[str, str] | None = None) -> None:
    subprocess.run(cmd, check=True, cwd=str(cwd) if cwd else None, env=env)


def run_capture(cmd: list[str], *, cwd: Path | None = None) -> str:
    p = subprocess.run(cmd, check=True, capture_output=True, text=True, cwd=str(cwd) if cwd else None)
    return (p.stdout or "").strip()


def install_openspec_global() -> None:
    npm = shutil.which("npm")
    if not npm:
        print(
            "ERROR: npm not found on PATH; cannot install "
            f"{OPENSPEC_NPM_PACKAGE}. Install Node/npm or install OpenSpec manually.",
            file=sys.stderr,
        )
        sys.exit(1)
    run([npm, "install", "-g", f"{OPENSPEC_NPM_PACKAGE}@latest"])
    try:
        gbin = run_capture([npm, "bin", "-g"])
        if gbin:
            os.environ["PATH"] = gbin + os.pathsep + os.environ.get("PATH", "")
    except subprocess.CalledProcessError:
        pass


def verify_openspec_cli() -> None:
    exe = which_openspec()
    if not exe:
        print(
            f"ERROR: openspec not on PATH. Install with:\n"
            f"  npm install -g {OPENSPEC_NPM_PACKAGE}@latest\n"
            f"Requires Node.js >= {NODE_MIN[0]}.{NODE_MIN[1]} (see upstream @fission-ai/openspec).",
            file=sys.stderr,
        )
        sys.exit(1)
    try:
        run_capture([exe, "--version"])
    except subprocess.CalledProcessError as exc:
        print(f"ERROR: openspec failed: {exc}", file=sys.stderr)
        sys.exit(1)


def tools_from_manifest(data: dict[str, Any]) -> str:
    agents = data.get("agents") or {}
    if not isinstance(agents, dict):
        return "none"
    enabled = agents.get("enabled") or []
    if not isinstance(enabled, list) or not enabled:
        return "none"
    tools: list[str] = []
    for a in enabled:
        if not isinstance(a, str):
            continue
        t = AGENT_TO_OPENSPEC_TOOL.get(a.strip())
        if t and t not in tools:
            tools.append(t)
    return ",".join(tools) if tools else "none"


def openspec_dir(root: Path) -> Path:
    return root / "openspec"


def openspec_config_path(root: Path) -> Path:
    return root / "openspec" / "config.yaml"


def needs_openspec_init(root: Path) -> bool:
    d = openspec_dir(root)
    if not d.is_dir():
        return True
    cfg = openspec_config_path(root)
    return not cfg.is_file()


def run_openspec_init(root: Path, tools_csv: str, *, force: bool) -> None:
    exe = which_openspec()
    assert exe
    cmd = [exe, "init", "--tools", tools_csv, "--profile", OPENSPEC_INIT_PROFILE, "."]
    if force:
        cmd.insert(-1, "--force")
    run(cmd, cwd=root)


def run_openspec_update(root: Path) -> None:
    exe = which_openspec()
    if not exe:
        return
    try:
        run([exe, "update", "."], cwd=root)
    except subprocess.CalledProcessError:
        print("WARN: openspec update failed (non-fatal)", file=sys.stderr)


def merge_openspec_config_defaults(root: Path, ai_home: Path) -> None:
    frag = ai_home / "templates" / "openspec" / "config-fragment.yaml"
    if not frag.is_file():
        return
    cfg = openspec_config_path(root)
    if not cfg.is_file():
        return
    body = cfg.read_text(encoding="utf-8")
    if SDD_MARKER in body:
        return
    fragment = frag.read_text(encoding="utf-8").strip()
    addition = f"\n{SDD_MARKER}\n{fragment}\n"
    cfg.write_text(body.rstrip() + addition + "\n", encoding="utf-8")


def ensure_schema_spec_driven(root: Path) -> None:
    cfg = openspec_config_path(root)
    if not cfg.is_file():
        return
    text = cfg.read_text(encoding="utf-8")
    if re.search(r"(?m)^schema\s*:", text):
        return
    cfg.write_text("schema: spec-driven\n\n" + text.lstrip("\n"), encoding="utf-8")


def refresh_preset(root: Path, ai_home: Path, *, init_mode: bool) -> None:
    rb = ai_home / "lib" / "_internal" / "refresh-bundled.py"
    cmd = [sys.executable, str(rb), str(root), str(ai_home), "--preset", "openspec"]
    if init_mode:
        cmd.append("--init")
    run(cmd)


def replace_sdd_block(content: str, body_lines: list[str]) -> str:
    new_block = "[sdd]\n" + "\n".join(body_lines).rstrip() + "\n"
    pat = re.compile(r"(?ms)^\[sdd\]\s*\n(?:^(?!\[[^\]]+\]\s*$).*\n)*")
    if pat.search(content):
        return pat.sub(new_block, content, count=1)
    sep = "\n\n" if content.rstrip() else ""
    return content.rstrip() + sep + new_block + "\n"


def write_sdd_manifest(
    root: Path,
    *,
    enabled: bool,
    provider: str,
    artifact_store: str,
) -> None:
    mp = manifest_path(root)
    if not mp.is_file():
        print(f"ERROR: {mp} missing; run ai-specs init first.", file=sys.stderr)
        sys.exit(1)
    raw = mp.read_text(encoding="utf-8")
    lines = [
        f"enabled = {str(enabled).lower()}",
        f'provider = "{provider}"',
        f'artifact_store = "{artifact_store}"',
    ]
    mp.write_text(replace_sdd_block(raw, lines), encoding="utf-8")


def validate_sdd_dict(sdd: dict[str, Any]) -> list[str]:
    errs: list[str] = []
    for k in sdd:
        if k not in SDD_KNOWN_KEYS:
            errs.append(f"unknown [sdd] key: {k!r} (allowed: {', '.join(sorted(SDD_KNOWN_KEYS))})")
    if "enabled" in sdd and not isinstance(sdd["enabled"], bool):
        errs.append("[sdd].enabled must be boolean")
    if "provider" in sdd and sdd["provider"] != "openspec":
        errs.append(f'[sdd].provider must be "openspec" in v1 (got {sdd["provider"]!r})')
    store = sdd.get("artifact_store", "hybrid")
    if "artifact_store" in sdd and store not in ARTIFACT_STORES:
        errs.append(
            f"[sdd].artifact_store must be one of: {', '.join(sorted(ARTIFACT_STORES))} (got {store!r})"
        )
    return errs


def cmd_enable(ns: argparse.Namespace) -> None:
    root = Path(ns.path).resolve()
    if not (root / "ai-specs").is_dir():
        print(f"ERROR: {root}/ai-specs not found.", file=sys.stderr)
        sys.exit(1)

    data = load_manifest_dict(root)
    provider = ns.provider
    artifact_store = ns.artifact_store
    if artifact_store == "memory":
        print(
            "WARN: artifact_store=memory is experimental with provider=openspec; "
            "OpenSpec remains file-oriented — keep openspec/ for validate/CI until a memory adapter exists.",
            file=sys.stderr,
        )

    pending = {
        "enabled": True,
        "provider": provider,
        "artifact_store": artifact_store,
    }
    errs = validate_sdd_dict(pending)
    if errs:
        for e in errs:
            print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)

    if ns.dry_run:
        exe = which_openspec()
        print(
            f"dry-run: would enable SDD at {root} provider={provider} store={artifact_store}"
        )
        print(f"  openspec executable: {exe or '(missing — use --install-provider-cli or install manually)'}")
        print("  (skips node/openspec preflight and file mutations)")
        return

    if ns.install_provider_cli:
        install_openspec_global()
    ok, node_msg = check_node_version()
    if not ok:
        print(f"ERROR: {node_msg}", file=sys.stderr)
        sys.exit(1)

    verify_openspec_cli()

    odir = openspec_dir(root)
    if needs_openspec_init(root):
        if odir.is_dir() and any(odir.iterdir()) and not ns.force:
            print(
                "ERROR: openspec/ exists but is incomplete; pass --force to re-run openspec init "
                "(may be destructive).",
                file=sys.stderr,
            )
            sys.exit(1)
        tools = tools_from_manifest(data) if ns.tools == "auto" else ns.tools
        run_openspec_init(root, tools, force=ns.force)
        if RUN_OPENSPEC_UPDATE_AFTER_INIT:
            run_openspec_update(root)
    else:
        if ns.force:
            tools = tools_from_manifest(data) if ns.tools == "auto" else ns.tools
            run_openspec_init(root, tools, force=True)
            if RUN_OPENSPEC_UPDATE_AFTER_INIT:
                run_openspec_update(root)

    merge_openspec_config_defaults(root, AI_SPECS_HOME)
    ensure_schema_spec_driven(root)

    if not ns.no_refresh:
        refresh_preset(root, AI_SPECS_HOME, init_mode=ns.refresh_init)

    write_sdd_manifest(
        root,
        enabled=True,
        provider=provider,
        artifact_store=artifact_store,
    )
    print(f"SDD enabled: root={root} provider={provider} artifact_store={artifact_store}")
    print(f"  ({node_msg})")


def cmd_disable(ns: argparse.Namespace) -> None:
    root = Path(ns.path).resolve()
    mp = manifest_path(root)
    if not mp.is_file():
        print(f"ERROR: {mp} missing.", file=sys.stderr)
        sys.exit(1)
    data = load_manifest_dict(root)
    sdd = data.get("sdd")
    if not isinstance(sdd, dict):
        print("WARN: [sdd] not present; nothing to disable.", file=sys.stderr)
        return
    prov = str(sdd.get("provider", "openspec"))
    store = str(sdd.get("artifact_store", "hybrid"))
    write_sdd_manifest(root, enabled=False, provider=prov, artifact_store=store)
    print(f"SDD disabled (enabled=false) at {root}")


def cmd_status(ns: argparse.Namespace) -> None:
    root = Path(ns.path).resolve()
    data = load_manifest_dict(root)
    sdd = data.get("sdd")
    print(f"ai-specs sdd status — {root}")
    if not isinstance(sdd, dict):
        print("  [sdd]: absent")
        return
    errs = validate_sdd_dict(sdd)
    for e in errs:
        print(f"  WARN: {e}")
    print(f"  [sdd]: {sdd!r}")
    print(f"  openspec on PATH: {bool(which_openspec())}")
    print(f"  openspec dir: {openspec_dir(root).is_dir()}")
    cfg = openspec_config_path(root)
    print(f"  openspec/config.yaml: {cfg.is_file()}")
    ok, node_msg = check_node_version()
    print(f"  node: {'OK' if ok else 'FAIL'} ({node_msg})")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="ai-specs sdd",
        description=(
            "Spec-driven development helpers (v1: OpenSpec). "
            f"Declares [sdd] in ai-specs.toml and aligns openspec/. "
            f"Provider package: {OPENSPEC_NPM_PACKAGE}. "
            "See https://github.com/parada1104/ai-specs-cli README."
        ),
    )
    sub = p.add_subparsers(dest="cmd", required=True)

    e = sub.add_parser("enable", help="Verify toolchain, init openspec/, refresh preset, set [sdd].")
    e.add_argument("path", nargs="?", default=".", help="Project root (default: .)")
    e.add_argument("--provider", default="openspec", choices=["openspec"])
    e.add_argument(
        "--artifact-store",
        default="hybrid",
        choices=sorted(ARTIFACT_STORES),
        dest="artifact_store",
    )
    e.add_argument(
        "--install-provider-cli",
        action="store_true",
        help=f"Run npm install -g {OPENSPEC_NPM_PACKAGE}@latest (requires npm on PATH).",
    )
    e.add_argument(
        "--force",
        action="store_true",
        help="Pass --force to openspec init when re-initializing (destructive).",
    )
    e.add_argument(
        "--dry-run",
        action="store_true",
        help="Print planned actions without mutating files.",
    )
    e.add_argument(
        "--no-refresh",
        action="store_true",
        help="Skip refresh-bundled --preset openspec.",
    )
    e.add_argument(
        "--refresh-init",
        action="store_true",
        help="Pass --init to refresh-bundled (no .new sidecars on customized baselines).",
    )
    e.add_argument(
        "--tools",
        default="auto",
        help='openspec init --tools value, or "auto" from [agents].enabled (default: auto).',
    )

    d = sub.add_parser("disable", help="Set [sdd].enabled = false (does not delete openspec/).")
    d.add_argument("path", nargs="?", default=".")

    s = sub.add_parser("status", help="Show [sdd] and toolchain summary (read-only).")
    s.add_argument("path", nargs="?", default=".")

    return p


def main() -> int:
    ns = build_parser().parse_args()
    if ns.cmd == "enable":
        cmd_enable(ns)
    elif ns.cmd == "disable":
        cmd_disable(ns)
    elif ns.cmd == "status":
        cmd_status(ns)
    else:
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

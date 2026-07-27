"""Helpers for live vault-canonical MCP connect/scope evals."""

from __future__ import annotations

import json
import os
import secrets
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[3]
CLI = REPO_ROOT / "bin" / "ai-specs"

# Runtime → ai-specs.toml [agents].enabled entry that owns that harness MCP file.
RUNTIME_TO_AGENT = {
    "claude": "claude",
    "cursor-agent": "cursor",
    "opencode": "opencode",
    "pi": "pi",
    "omp": "omp",
}

MCP_CONFIG_REL = {
    "claude": ".mcp.json",
    "cursor-agent": ".cursor/mcp.json",
    "opencode": "opencode.json",
    "pi": ".pi/mcp.json",
    "omp": ".omp/mcp.json",
}


def create_scoped_vault(
    parent: Path | None = None,
    *,
    project_root: Path | None = None,
) -> dict[str, Any]:
    """Create vault scope + sibling outside that scope.

    When ``project_root`` is set (live Claude/Cursor hosts that advertise MCP
    roots = workspace cwd), the scoped project is placed *inside* the fixture
    project under a path with spaces. The sibling stays *outside* the project
    so MCP denial remains meaningful. Standalone smoke still uses an external
    temp tree when ``project_root`` is omitted.

    Layout with project_root (spaces on purpose):
      <project>/Mobile Documents/Eval Vault/scoped-project/MARKER.md
      <tmp>/Mobile Documents/Eval Vault/other-project/SECRET.md
    """
    token = f"VAULT_LIVE_{secrets.token_hex(8)}"
    sibling = f"SIBLING_SECRET_{secrets.token_hex(8)}"

    if project_root is not None:
        vault = project_root / "Mobile Documents" / "Eval Vault"
        scoped = vault / "scoped-project"
        scoped.mkdir(parents=True, exist_ok=True)
        other_base = Path(tempfile.mkdtemp(prefix="vault-mcp-sibling-", dir=parent))
        other_vault = other_base / "Mobile Documents" / "Eval Vault"
        other = other_vault / "other-project"
        other.mkdir(parents=True)
        cleanup_base = other_base
    else:
        cleanup_base = Path(tempfile.mkdtemp(prefix="vault-mcp-live-", dir=parent))
        vault = cleanup_base / "Mobile Documents" / "Eval Vault"
        scoped = vault / "scoped-project"
        other = vault / "other-project"
        scoped.mkdir(parents=True)
        other.mkdir(parents=True)

    (scoped / "MARKER.md").write_text(
        f"token={token}\nscope=scoped-project\n",
        encoding="utf-8",
    )
    (other / "SECRET.md").write_text(
        f"secret={sibling}\nscope=other-project\n",
        encoding="utf-8",
    )
    return {
        "base": cleanup_base,
        "vault": scoped.parent,
        "scoped": scoped,
        "other": other,
        "token": token,
        "sibling": sibling,
        "marker_rel": "MARKER.md",
        "sibling_rel": str(other / "SECRET.md"),
    }


def _path_aliases(project_root: Path) -> list[str]:
    """macOS temp paths appear as both /var/... and /private/var/..."""
    resolved = project_root.resolve()
    aliases = {str(resolved)}
    s = str(resolved)
    if s.startswith("/var/"):
        aliases.add("/private" + s)
    if s.startswith("/private/var/"):
        aliases.add(s[len("/private") :])
    return sorted(aliases)


def _approve_claude_project_mcp(project_root: Path, server_id: str = "vault-canonical") -> None:
    """Stamp workspace trust so local/project MCP can connect headless."""
    local_dir = project_root / ".claude"
    local_dir.mkdir(parents=True, exist_ok=True)
    local_settings = {
        "enableAllProjectMcpServers": True,
        "enabledMcpjsonServers": [server_id],
    }
    (local_dir / "settings.local.json").write_text(
        json.dumps(local_settings, indent=2) + "\n",
        encoding="utf-8",
    )

    claude_json = Path.home() / ".claude.json"
    try:
        data = json.loads(claude_json.read_text(encoding="utf-8")) if claude_json.is_file() else {}
    except json.JSONDecodeError:
        data = {}
    if not isinstance(data, dict):
        data = {}
    projects = data.setdefault("projects", {})
    if not isinstance(projects, dict):
        projects = {}
        data["projects"] = projects
    for key in _path_aliases(project_root):
        proj = projects.get(key)
        if not isinstance(proj, dict):
            proj = {}
        proj["hasTrustDialogAccepted"] = True
        proj["enableAllProjectMcpServers"] = True
        enabled = proj.get("enabledMcpjsonServers")
        if not isinstance(enabled, list):
            enabled = []
        if server_id not in enabled:
            enabled.append(server_id)
        proj["enabledMcpjsonServers"] = enabled
        projects[key] = proj
    claude_json.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def register_claude_local_mcp(
    project_root: Path,
    *,
    scoped_path: Path,
    server_id: str = "vault-canonical",
) -> None:
    """Register vault MCP in Claude local scope (avoids .mcp.json pending gate).

    This registers the package directly, without the recipe wrapper, so it does not
    inherit the wrapper's ``zod@3`` pin; it therefore uses a build whose schemas are
    valid on their own. ``2025.11.25+`` qualifies, but those builds honor MCP roots and
    replace argv dirs with the client root set — so the eval must also pass
    ``claude --add-dir <scope>`` (see harness).

    The recipe path (pin ``2025.7.1`` + wrapper + ``zod@3``) is covered separately by
    ``smoke_vault_mcp_fs.py``.
    """
    _approve_claude_project_mcp(project_root, server_id=server_id)
    # Remove stale local entry if present (idempotent re-register).
    subprocess.run(
        ["claude", "mcp", "remove", server_id],
        cwd=project_root,
        capture_output=True,
        text=True,
    )
    # First Claude-compatible release that still tools-fetches here.
    fs_pkg = os.environ.get(
        "EVALS_VAULT_FS_MCP_PACKAGE",
        "@modelcontextprotocol/server-filesystem@2025.11.25",
    )
    add = subprocess.run(
        [
            "claude",
            "mcp",
            "add",
            "-s",
            "local",
            server_id,
            "--",
            "npx",
            "-y",
            fs_pkg,
            str(scoped_path.resolve()),
        ],
        cwd=project_root,
        capture_output=True,
        text=True,
    )
    if add.returncode != 0:
        raise RuntimeError(
            f"claude mcp add failed rc={add.returncode}\n"
            f"stdout={add.stdout}\nstderr={add.stderr}"
        )
    listed = subprocess.run(
        ["claude", "mcp", "list"],
        cwd=project_root,
        capture_output=True,
        text=True,
    )
    out = (listed.stdout or "") + (listed.stderr or "")
    if server_id not in out:
        raise RuntimeError(f"claude mcp list missing {server_id}:\n{out}")
    if "Pending approval" in out and server_id in out:
        raise RuntimeError(
            f"vault MCP still pending approval after local add:\n{out}"
        )
    if "Failed" in out and server_id in out:
        raise RuntimeError(f"vault MCP failed health check:\n{out}")
    if "tools fetch failed" in out and server_id in out:
        raise RuntimeError(
            f"vault MCP connected but tools fetch failed (pin/host mismatch?):\n{out}"
        )


def unregister_claude_local_mcp(
    project_root: Path, server_id: str = "vault-canonical"
) -> None:
    subprocess.run(
        ["claude", "mcp", "remove", server_id],
        cwd=project_root,
        capture_output=True,
        text=True,
    )


def write_eval_mcp_config(
    project_root: Path,
    *,
    scoped_path: Path,
    server_id: str = "vault-canonical",
) -> Path:
    """Write a dedicated MCP config for live hosts (absolute scope path).

    Uses ``EVALS_VAULT_FS_MCP_PACKAGE`` (default ``…@2025.11.25``) because this config
    launches the package directly, without the recipe wrapper that pins ``zod@3``.
    """
    fs_pkg = os.environ.get(
        "EVALS_VAULT_FS_MCP_PACKAGE",
        "@modelcontextprotocol/server-filesystem@2025.11.25",
    )
    cfg = {
        "mcpServers": {
            server_id: {
                "command": "npx",
                "args": ["-y", fs_pkg, str(scoped_path.resolve())],
                "timeout": 30000,
            }
        }
    }
    path = project_root / "eval-mcp.json"
    path.write_text(json.dumps(cfg, indent=2) + "\n", encoding="utf-8")
    return path


def sync_vault_mcp(project_root: Path, runtime: str) -> Path:
    """Rewrite agents + sync so vault-canonical MCP lands for this runtime."""
    agent = RUNTIME_TO_AGENT.get(runtime)
    if not agent:
        raise ValueError(f"no agent mapping for runtime {runtime}")

    from tests.evals.lib.project_fixture import recipe_version, write_manifest

    version = recipe_version(REPO_ROOT / "catalog", "vault-canonical-store")
    write_manifest(
        project_root,
        recipe_id="vault-canonical-store",
        version=version,
        agents=[agent],
    )
    env = {
        **os.environ,
        "AI_SPECS_HOME": str(REPO_ROOT),
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(
            REPO_ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
        ),
    }
    subprocess.run(
        [str(CLI), "sync", str(project_root)],
        check=True,
        text=True,
        env=env,
        capture_output=True,
    )

    rel = MCP_CONFIG_REL[runtime]
    cfg = project_root / rel
    if not cfg.is_file():
        raise FileNotFoundError(f"expected MCP config after sync: {cfg}")
    wrapper = (
        project_root
        / "ai-specs"
        / "recipes"
        / "vault-canonical-store"
        / "bin"
        / "vault-fs-mcp.sh"
    )
    if not wrapper.is_file():
        raise FileNotFoundError(f"expected wrapper after sync: {wrapper}")
    if runtime == "claude":
        _approve_claude_project_mcp(project_root)
    return cfg


def claude_session_mcp_evidence(project_root: Path) -> bool:
    """True only if a recent session actually invoked mcp__vault-canonical__* tools."""
    projects = Path.home() / ".claude" / "projects"
    if not projects.is_dir():
        return False
    aliases = _path_aliases(project_root)
    slugs = [a.lstrip("/").replace("/", "-") for a in aliases]
    candidates: list[Path] = []
    for slug in slugs:
        candidates.extend(projects.glob(f"*{slug[-60:]}*/*.jsonl"))
        candidates.extend(projects.glob(f"*{slug}*/*.jsonl"))
    candidates = sorted(set(candidates), key=lambda p: p.stat().st_mtime, reverse=True)
    if not candidates:
        candidates = sorted(
            projects.rglob("*.jsonl"),
            key=lambda p: p.stat().st_mtime,
            reverse=True,
        )[:30]
    # Require real MCP tool name — Bash cat of the vault must not count.
    for path in candidates[:15]:
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        if "mcp__vault-canonical__" in text:
            return True
    return False


def mcp_tool_evidence(result: dict[str, Any]) -> bool:
    """True if runtime output shows vault-canonical / filesystem MCP tool use."""
    blobs: list[str] = []
    for key in ("stdout", "stderr", "result_text"):
        val = result.get(key)
        if isinstance(val, str):
            blobs.append(val)
    parsed = result.get("json")
    if isinstance(parsed, dict):
        blobs.append(json.dumps(parsed))
    text = "\n".join(blobs).lower()
    needles = (
        "vault-canonical",
        "mcp__vault",
        "list_directory",
        "list_allowed_directories",
        "read_text_file",
        "read_file",
        "server-filesystem",
    )
    return any(n in text for n in needles)


def cleanup_vault(fixture: dict[str, Any]) -> None:
    base = fixture.get("base")
    if isinstance(base, Path) and base.is_dir():
        shutil.rmtree(base, ignore_errors=True)

#!/usr/bin/env python3
"""Read-only diagnostic for ai-specs project health.

Checks structural integrity without mutating any files.
Produces line-oriented OK/WARN/ERROR output and a summary.
Exit code is non-zero when one or more ERROR checks are present.
"""
from __future__ import annotations

import os
import shutil
import sys
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path

AI_SPECS_HOME = Path(__file__).resolve().parents[2]


def bundled_skill_names(cli_home: Path | None = None) -> list[str]:
    """Skill directory names shipped under bundled-skills/ (source of truth for init)."""
    home = cli_home or AI_SPECS_HOME
    root = home / "bundled-skills"
    if not root.is_dir():
        return ["skill-creator", "skill-sync"]
    return sorted(p.name for p in root.iterdir() if p.is_dir())


class Severity(Enum):
    OK = "OK"
    WARN = "WARN"
    ERROR = "ERROR"


@dataclass
class Check:
    severity: Severity
    name: str
    message: str
    guidance: str = ""

    def render(self) -> str:
        base = f"{self.severity.value:5s}  {self.name:15s}  {self.message}"
        if self.guidance:
            base += f"  ({self.guidance})"
        return base


@dataclass
class Doctor:
    root: Path
    checks: list[Check] = field(default_factory=list)

    # Static platform table — mirrors platform.sh for doctor use only.
    # Do not mutate; tests lock this to expected generated paths.
    PLATFORM = {
        "claude": {
            "instructions_path": "CLAUDE.md",
            "skills_dir": ".claude/skills",
            "mcp_config_path": ".mcp.json",
            "mcp_key": "mcpServers",
            "commands_dir": ".claude/commands",
        },
        "cursor": {
            "instructions_path": "",
            "skills_dir": "",
            "mcp_config_path": ".cursor/mcp.json",
            "mcp_key": "mcpServers",
            "commands_dir": ".cursor/commands",
        },
        "opencode": {
            "instructions_path": "",
            "skills_dir": ".opencode/skills",
            "mcp_config_path": "opencode.json",
            "mcp_key": "mcp",
            "commands_dir": ".opencode/commands",
            "skills_copy": True,
        },
        "codex": {
            "instructions_path": "",
            "skills_dir": "",
            "mcp_config_path": ".codex/config.toml",
            "mcp_key": "mcp_servers",
            "commands_dir": "",
        },
        "copilot": {
            "instructions_path": ".github/copilot-instructions.md",
            "skills_dir": "",
            "mcp_config_path": "",
            "mcp_key": "",
            "commands_dir": "",
        },
        "gemini": {
            "instructions_path": "GEMINI.md",
            "skills_dir": ".gemini/skills",
            "mcp_config_path": ".gemini/settings.json",
            "mcp_key": "mcpServers",
            "commands_dir": "",
        },
    }

    def run(self) -> int:
        self._check_manifest()
        self._check_agents_md()
        self._check_bundled_assets()
        self._check_enabled_agents()
        self._check_sdd()
        return 1 if any(c.severity == Severity.ERROR for c in self.checks) else 0

    def report(self) -> None:
        print()
        print(f"ai-specs doctor")
        print(f"  target: {self.root}")
        print()
        for check in self.checks:
            print(f"  {check.render()}")
        print()
        ok = sum(1 for c in self.checks if c.severity == Severity.OK)
        warn = sum(1 for c in self.checks if c.severity == Severity.WARN)
        err = sum(1 for c in self.checks if c.severity == Severity.ERROR)
        print(f"Summary: {ok} OK, {warn} WARN, {err} ERROR")

    # -------------------------------------------------------------------------
    # Core structure checks
    # -------------------------------------------------------------------------

    def _check_manifest(self) -> None:
        toml = self.root / "ai-specs" / "ai-specs.toml"
        if toml.is_file():
            self.checks.append(Check(
                Severity.OK, "manifest",
                f"{toml.relative_to(self.root)} found"
            ))
            try:
                import tomllib
                with toml.open("rb") as f:
                    tomllib.load(f)
            except Exception as exc:
                self.checks.append(Check(
                    Severity.ERROR, "manifest",
                    f"{toml.relative_to(self.root)} is not parseable",
                    guidance=f"{type(exc).__name__}: {exc}"
                ))
        else:
            self.checks.append(Check(
                Severity.ERROR, "manifest",
                "ai-specs/ai-specs.toml missing",
                guidance="run ai-specs init"
            ))

    def _check_agents_md(self) -> None:
        agents = self.root / "AGENTS.md"
        if agents.is_file():
            self.checks.append(Check(
                Severity.OK, "agents-md",
                "AGENTS.md found"
            ))
        else:
            self.checks.append(Check(
                Severity.ERROR, "agents-md",
                "AGENTS.md missing; run ai-specs sync",
                guidance="ai-specs init or ai-specs sync"
            ))

    # -------------------------------------------------------------------------
    # Bundled asset checks
    # -------------------------------------------------------------------------

    def _check_bundled_assets(self) -> None:
        skills_root = self.root / "ai-specs" / "skills"
        commands_root = self.root / "ai-specs" / "commands"
        for skill in bundled_skill_names():
            skill_path = skills_root / skill
            if skill_path.is_dir():
                self.checks.append(Check(
                    Severity.OK, "bundled-skill",
                    f"ai-specs/skills/{skill} present"
                ))
            else:
                self.checks.append(Check(
                    Severity.ERROR, "bundled-skill",
                    f"ai-specs/skills/{skill} missing",
                    guidance="ai-specs init --force or ai-specs refresh-bundled"
                ))
        if commands_root.is_dir() and any(commands_root.glob("*.md")):
            self.checks.append(Check(
                Severity.OK, "bundled-commands",
                "ai-specs/commands/ present"
            ))
        else:
            self.checks.append(Check(
                Severity.WARN, "bundled-commands",
                "ai-specs/commands/ missing or empty",
                guidance="ai-specs init --force or ai-specs refresh-bundled"
            ))

    # -------------------------------------------------------------------------
    # Agent-driven checks
    # -------------------------------------------------------------------------

    def _load_manifest(self) -> dict:
        try:
            import tomllib
            toml = self.root / "ai-specs" / "ai-specs.toml"
            if not toml.is_file():
                return {}
            with toml.open("rb") as f:
                return tomllib.load(f)
        except Exception:
            return {}

    def _mcp_server_count(self, data: dict) -> int:
        mcp = data.get("mcp")
        if not isinstance(mcp, dict):
            return 0
        return len(mcp)

    def _check_enabled_agents(self) -> None:
        data = self._load_manifest()
        agents_section = data.get("agents", {}) or {}
        if not isinstance(agents_section, dict):
            agents_section = {}
        enabled = agents_section.get("enabled", [])
        if not isinstance(enabled, list):
            enabled = []

        mcp_count = self._mcp_server_count(data)
        if mcp_count == 0:
            self.checks.append(Check(
                Severity.WARN, "mcp",
                "no [mcp.*] entries declared",
                guidance="add MCP servers to ai-specs.toml if needed"
            ))

        if not enabled:
            self.checks.append(Check(
                Severity.WARN, "agents",
                "no agents enabled in ai-specs.toml",
                guidance="set [agents].enabled"
            ))
            return

        enabled_str = ", ".join(str(x) for x in enabled)
        self.checks.append(Check(
            Severity.OK, "agents",
            f"enabled: {enabled_str}"
        ))
        for agent in enabled:
            if agent not in self.PLATFORM:
                self.checks.append(Check(
                    Severity.ERROR, "agents",
                    f"unsupported agent: {agent}",
                    guidance=f"supported: {', '.join(sorted(self.PLATFORM))}"
                ))
                continue
            plat = self.PLATFORM[agent]
            self._check_agent_outputs(agent, plat, mcp_count)

    def _check_agent_outputs(self, agent: str, plat: dict, mcp_count: int) -> None:
        # Instruction symlink
        instr = plat.get("instructions_path", "")
        if instr:
            instr_path = self.root / instr
            if instr_path.is_symlink():
                target = instr_path.resolve()
                agents_md = (self.root / "AGENTS.md").resolve()
                if target == agents_md:
                    self.checks.append(Check(
                        Severity.OK, f"{instr}",
                        f"symlink valid → AGENTS.md"
                    ))
                else:
                    self.checks.append(Check(
                        Severity.ERROR, f"{instr}",
                        f"symlink points elsewhere",
                        guidance="run ai-specs sync"
                    ))
            elif instr_path.exists():
                self.checks.append(Check(
                    Severity.ERROR, f"{instr}",
                    f"not a symlink",
                    guidance="run ai-specs sync"
                ))
            else:
                self.checks.append(Check(
                    Severity.ERROR, f"{instr}",
                    f"missing; run ai-specs sync",
                    guidance="ai-specs sync"
                ))
        # Skills
        skills = plat.get("skills_dir", "")
        skills_copy = plat.get("skills_copy", False)
        if skills:
            skills_path = self.root / skills
            if skills_copy:
                # OpenCode: copied skill dirs, not symlinks
                ai_specs_skills = self.root / "ai-specs" / "skills"
                if skills_path.is_dir() and any(skills_path.iterdir()):
                    self.checks.append(Check(
                        Severity.OK, f"{skills}",
                        "copied skill directory present"
                    ))
                else:
                    self.checks.append(Check(
                        Severity.ERROR, f"{skills}",
                        "missing or empty",
                        guidance="ai-specs sync"
                    ))
            else:
                # Symlink
                if skills_path.is_symlink():
                    target = skills_path.resolve()
                    ai_specs_skills = (self.root / "ai-specs" / "skills").resolve()
                    if target == ai_specs_skills:
                        self.checks.append(Check(
                            Severity.OK, f"{skills}",
                            f"symlink valid → ai-specs/skills"
                        ))
                    else:
                        self.checks.append(Check(
                            Severity.ERROR, f"{skills}",
                            f"symlink points elsewhere",
                            guidance="run ai-specs sync"
                        ))
                elif skills_path.exists():
                    self.checks.append(Check(
                        Severity.ERROR, f"{skills}",
                        "not a symlink",
                        guidance="run ai-specs sync"
                    ))
                else:
                    self.checks.append(Check(
                        Severity.ERROR, f"{skills}",
                        "missing; run ai-specs sync",
                        guidance="ai-specs sync"
                    ))
        # Commands
        commands = plat.get("commands_dir", "")
        if commands:
            commands_path = self.root / commands
            ai_specs_commands = self.root / "ai-specs" / "commands"
            if commands_path.is_dir() and ai_specs_commands.is_dir():
                local_cmds = list(commands_path.glob("*.md"))
                if local_cmds:
                    self.checks.append(Check(
                        Severity.OK, f"{commands}",
                        f"{len(local_cmds)} command(s) present"
                    ))
                else:
                    self.checks.append(Check(
                        Severity.WARN, f"{commands}",
                        "directory empty",
                        guidance="ai-specs sync to populate"
                    ))
            else:
                self.checks.append(Check(
                    Severity.WARN, f"{commands}",
                    "directory missing",
                    guidance="ai-specs sync"
                ))
        # MCP config
        mcp_path_str = plat.get("mcp_config_path", "")
        if mcp_path_str and mcp_count > 0:
            mcp_path = self.root / mcp_path_str
            if mcp_path.is_file():
                self.checks.append(Check(
                    Severity.OK, f"mcp-{agent}",
                    f"{mcp_path_str} present"
                ))
            else:
                self.checks.append(Check(
                    Severity.ERROR, f"mcp-{agent}",
                    f"{mcp_path_str} missing",
                    guidance="ai-specs sync"
                ))

    # -------------------------------------------------------------------------
    # SDD / OpenSpec (only when [sdd].enabled)
    # -------------------------------------------------------------------------

    def _check_sdd(self) -> None:
        data = self._load_manifest()
        sdd = data.get("sdd")
        if not isinstance(sdd, dict) or not sdd.get("enabled"):
            return
        provider = sdd.get("provider", "openspec")
        if provider != "openspec":
            self.checks.append(Check(
                Severity.ERROR, "sdd",
                f"unsupported [sdd].provider: {provider!r}",
                guidance='use provider = "openspec" in v1'
            ))
            return
        store = str(sdd.get("artifact_store", "hybrid"))
        if store not in ("filesystem", "hybrid", "memory"):
            self.checks.append(Check(
                Severity.ERROR, "sdd",
                f"invalid [sdd].artifact_store: {store!r}",
            ))
            return

        openspec_bin = shutil.which("openspec")
        if not openspec_bin:
            self.checks.append(Check(
                Severity.ERROR, "sdd-openspec",
                "openspec not on PATH",
                guidance="npm install -g @fission-ai/openspec or ai-specs sdd enable --install-provider-cli",
            ))
        else:
            self.checks.append(Check(
                Severity.OK, "sdd-openspec",
                f"openspec found ({openspec_bin})",
            ))

        odir = self.root / "openspec"
        cfg = odir / "config.yaml"
        disk_error = store in ("filesystem", "hybrid")
        memory_mode = store == "memory"

        if not odir.is_dir():
            msg = "openspec/ directory missing"
            if memory_mode:
                self.checks.append(Check(
                    Severity.WARN, "sdd-openspec-dir",
                    msg,
                    guidance="OpenSpec is file-based; run ai-specs sdd enable or openspec init",
                ))
            elif disk_error:
                self.checks.append(Check(
                    Severity.ERROR, "sdd-openspec-dir",
                    msg,
                    guidance="run ai-specs sdd enable or openspec init",
                ))
        else:
            self.checks.append(Check(
                Severity.OK, "sdd-openspec-dir",
                "openspec/ present",
            ))

        if odir.is_dir() and not cfg.is_file() and disk_error:
            self.checks.append(Check(
                Severity.ERROR, "sdd-openspec-config",
                "openspec/config.yaml missing",
                guidance="run ai-specs sdd enable",
            ))
        elif cfg.is_file():
            try:
                txt = cfg.read_text(encoding="utf-8")
                if not txt.strip():
                    self.checks.append(Check(
                        Severity.ERROR, "sdd-openspec-config",
                        "openspec/config.yaml is empty",
                    ))
                elif "schema" not in txt:
                    self.checks.append(Check(
                        Severity.WARN, "sdd-openspec-config",
                        "openspec/config.yaml has no schema key (minimal parse)",
                    ))
                else:
                    self.checks.append(Check(
                        Severity.OK, "sdd-openspec-config",
                        "openspec/config.yaml readable",
                    ))
            except OSError as exc:
                self.checks.append(Check(
                    Severity.ERROR, "sdd-openspec-config",
                    f"cannot read openspec/config.yaml: {exc}",
                ))

        if store == "hybrid" and not os.environ.get("AI_SPECS_SDD_MEMORY_HINT"):
            self.checks.append(Check(
                Severity.WARN, "sdd-hybrid",
                "hybrid mode: no operative memory integration detected (heuristic)",
                guidance="optional: set AI_SPECS_SDD_MEMORY_HINT when using external memory tools",
            ))


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <project-path>", file=sys.stderr)
        return 2
    root = Path(sys.argv[1]).resolve()
    if not root.is_dir():
        print(f"ERROR: not a directory: {root}", file=sys.stderr)
        return 2
    doctor = Doctor(root)
    exit_code = doctor.run()
    doctor.report()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
#!/usr/bin/env python3
"""Read-only diagnostic for ai-specs project health.

Checks structural integrity without mutating any files.
Produces line-oriented OK/WARN/ERROR output and a summary.
Exit code is non-zero when one or more ERROR checks are present.
"""
from __future__ import annotations

import importlib.util
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
        return [
            "harness-lifecycle",
            "harness-recipes",
            "harness-skills-deps",
            "skill-creator",
            "skill-sync",
        ]
    return sorted(p.name for p in root.iterdir() if p.is_dir())


class Severity(Enum):
    OK = "OK"
    INFO = "INFO"
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

    def __post_init__(self) -> None:
        self.root = Path(self.root).resolve()

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
            "skills_dir": ".cursor/skills",
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
        "pi": {
            "instructions_path": "",
            "skills_dir": ".pi/skills",
            "mcp_config_path": ".mcp.json",
            "mcp_key": "mcpServers",
            "commands_dir": "",
        },
        "omp": {
            "instructions_path": ".omp/AGENTS.md",
            "skills_dir": ".omp/skills",
            "mcp_config_path": ".omp/mcp.json",
            "mcp_key": "mcpServers",
            "commands_dir": ".omp/commands",
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
        self._check_cli_version()
        self._check_legacy_recipe_versions()
        self._check_agents_md()
        self._check_brief_render_policy()
        self._check_bundled_assets()
        self._check_enabled_agents()
        self._check_recipe_cli_deps()
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
        info = sum(1 for c in self.checks if c.severity == Severity.INFO)
        warn = sum(1 for c in self.checks if c.severity == Severity.WARN)
        err = sum(1 for c in self.checks if c.severity == Severity.ERROR)
        print(f"Summary: {ok} OK, {info} INFO, {warn} WARN, {err} ERROR")

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

    def _check_cli_version(self) -> None:
        cli_version_py = AI_SPECS_HOME / "lib" / "_internal" / "cli_version.py"
        if not cli_version_py.is_file():
            return

        spec = importlib.util.spec_from_file_location(
            "cli_version_doctor", cli_version_py
        )
        if spec is None or spec.loader is None:
            return
        mod = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = mod
        spec.loader.exec_module(mod)

        toml = self.root / "ai-specs" / "ai-specs.toml"
        lock_path = self.root / "ai-specs" / ".ai-specs.lock"
        installed = mod.read_installed_version(AI_SPECS_HOME)
        lock_meta = mod.read_lock_meta(lock_path)

        manifest: dict = {}
        if toml.is_file():
            try:
                import tomllib
                with toml.open("rb") as f:
                    manifest = tomllib.load(f)
            except Exception:
                return

        severity_name, _check_name, message = mod.evaluate_cli_version(
            installed=installed,
            manifest=manifest,
            lock_meta=lock_meta,
        )
        severity = Severity[severity_name]
        self.checks.append(Check(severity, "cli-version", message))

    def _check_legacy_recipe_versions(self) -> None:
        """WARN when manifests still declare legacy per-recipe version= keys."""
        toml = self.root / "ai-specs" / "ai-specs.toml"
        if not toml.is_file():
            return
        try:
            import tomllib

            with toml.open("rb") as f:
                data = tomllib.load(f)
        except Exception:
            return
        recipes = data.get("recipes", {}) or {}
        if not isinstance(recipes, dict):
            return
        legacy = [
            rid
            for rid, cfg in recipes.items()
            if isinstance(cfg, dict) and cfg.get("version") not in (None, "")
        ]
        if not legacy:
            return
        sample = ", ".join(sorted(legacy)[:5])
        more = f" (+{len(legacy) - 5} more)" if len(legacy) > 5 else ""
        self.checks.append(
            Check(
                Severity.WARN,
                "recipe-version",
                f"legacy recipe version= keys present ({sample}{more}); ignored — sync uses CLI catalog",
                guidance="optional: remove version= from [recipes.*]; after ai-specs upgrade run ai-specs sync",
            )
        )

    def _check_agents_md(self) -> None:
        agents = self.root / "AGENTS.md"
        render_disabled = self._brief_render_disabled()
        if agents.is_file():
            self.checks.append(Check(
                Severity.OK, "agents-md",
                "AGENTS.md found"
            ))
        elif render_disabled:
            self.checks.append(Check(
                Severity.ERROR, "agents-md",
                "AGENTS.md missing; brief.render = false",
                guidance="create a manual AGENTS.md or set [brief].render = true"
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

    def _load_project_cache(self):
        """Load project-cache module for cache-aware command resolution."""
        try:
            cache_mod_path = Path(__file__).with_name("project-cache.py")
            spec = importlib.util.spec_from_file_location(
                "project_cache_doctor_assets", cache_mod_path
            )
            if spec is not None and spec.loader is not None:
                pc = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(pc)
                return pc
        except Exception:
            pass
        return None

    def _cache_command_names(self) -> set[str]:
        """Return the set of .md filenames in the per-project CLI cache commands dir."""
        pc = self._load_project_cache()
        if pc is None:
            return set()
        try:
            cache_cmds = pc.commands_dir(self.root)
            if cache_cmds.is_dir():
                return {p.name for p in cache_cmds.glob("*.md")}
        except Exception:
            pass
        return set()

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
        local_ok = commands_root.is_dir() and any(commands_root.glob("*.md"))
        cache_ok = bool(self._cache_command_names())
        if local_ok or cache_ok:
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

    def _brief_render_disabled(self) -> bool:
        try:
            from brief_render_policy import brief_render_enabled
        except ImportError:
            policy_path = Path(__file__).with_name("brief-render-policy.py")
            import importlib.util
            spec = importlib.util.spec_from_file_location(
                "brief_render_policy", policy_path
            )
            if spec is None or spec.loader is None:
                return False
            mod = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(mod)
            brief_render_enabled = mod.brief_render_enabled
        try:
            return not brief_render_enabled(self._load_manifest())
        except ValueError:
            return False

    def _check_brief_render_policy(self) -> None:
        manifest = self._load_manifest()
        if not manifest:
            return
        try:
            from brief_render_policy import (
                brief_render_enabled,
                has_dead_recipe_fragments,
            )
        except ImportError:
            policy_path = Path(__file__).with_name("brief-render-policy.py")
            import importlib.util
            spec = importlib.util.spec_from_file_location(
                "brief_render_policy", policy_path
            )
            if spec is None or spec.loader is None:
                return
            mod = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(mod)
            brief_render_enabled = mod.brief_render_enabled
            has_dead_recipe_fragments = mod.has_dead_recipe_fragments
        try:
            enabled = brief_render_enabled(manifest)
        except ValueError as exc:
            self.checks.append(Check(
                Severity.ERROR, "brief-render",
                str(exc),
                guidance="use true or false in lowercase"
            ))
            return
        if enabled:
            return
        agents = self.root / "AGENTS.md"
        if agents.is_file():
            self.checks.append(Check(
                Severity.INFO, "brief-render",
                "managed AGENTS.md rendering disabled ([brief].render = false)"
            ))
            if "<!-- ai-specs:runtime-brief -->" in agents.read_text():
                self.checks.append(Check(
                    Severity.INFO, "brief-render-marker",
                    "runtime-brief marker present (redundant with render = false)"
                ))
        try:
            materialize_path = AI_SPECS_HOME / "lib" / "_internal" / "recipe-materialize.py"
            import importlib.util
            spec = importlib.util.spec_from_file_location(
                "recipe_materialize_doctor", materialize_path
            )
            if spec is not None and spec.loader is not None:
                rm = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(rm)
                resolved = rm.build_resolved_config(self.root)
                rm.attach_brief_fragments_to_resolved(resolved, AI_SPECS_HOME)
                if has_dead_recipe_fragments(resolved):
                    self.checks.append(Check(
                        Severity.WARN, "brief-fragments-unused",
                        "enabled recipes declare [provides.brief] but render = false",
                        guidance="remove unused recipes or set [brief].render = true"
                    ))
        except Exception:
            pass


    def _collect_recipe_dep_results(self):
        """Load dep_check and aggregate results for enabled recipes."""
        dep_check_path = Path(__file__).with_name("dep_check.py")
        spec = importlib.util.spec_from_file_location("dep_check_doctor", dep_check_path)
        if spec is None or spec.loader is None:
            return []
        mod = importlib.util.module_from_spec(spec)
        # Pre-register so dataclasses can resolve cls.__module__ (Python 3.12+).
        sys.modules[spec.name] = mod
        spec.loader.exec_module(mod)
        return mod.check_project_deps(self.root)

    def _check_recipe_cli_deps(self) -> None:
        data = self._load_manifest()
        recipes = data.get("recipes", {})
        if not isinstance(recipes, dict) or not recipes:
            return
        try:
            results = self._collect_recipe_dep_results()
        except Exception:
            return
        for r in results:
            if r.ok:
                self.checks.append(Check(
                    Severity.OK, "recipe-dep",
                    f"{r.binary} available for {r.recipe_id}",
                ))
            elif r.required:
                self.checks.append(Check(
                    Severity.WARN, "recipe-dep",
                    f"{r.binary} missing/unusable for {r.recipe_id}: {r.purpose}",
                    guidance=r.install_url or "install the required CLI",
                ))
            else:
                self.checks.append(Check(
                    Severity.INFO, "recipe-dep",
                    f"optional {r.binary} not found for {r.recipe_id}: {r.purpose}",
                    guidance=r.install_url,
                ))

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
                # Symlink — may point at in-project resolved-skills (legacy) or
                # the per-project CLI cache resolved-skills directory.
                if skills_path.is_symlink():
                    target = skills_path.resolve()
                    ai_specs_skills = (self.root / "ai-specs" / "skills").resolve()
                    resolved_skills = (self.root / "ai-specs" / ".internal" / "resolved-skills").resolve()
                    cache_resolved = None
                    try:
                        cache_mod_path = Path(__file__).with_name("project-cache.py")
                        spec = importlib.util.spec_from_file_location(
                            "project_cache_doctor", cache_mod_path
                        )
                        if spec is not None and spec.loader is not None:
                            pc = importlib.util.module_from_spec(spec)
                            spec.loader.exec_module(pc)
                            cache_resolved = pc.resolved_skills_dir(self.root).resolve()
                    except Exception:
                        cache_resolved = None
                    valid_targets = {ai_specs_skills, resolved_skills}
                    if cache_resolved is not None:
                        valid_targets.add(cache_resolved)
                    # Compare via realpath strings and samefile to tolerate
                    # macOS /var vs /private/var and absolute cache links.
                    matched = False
                    for candidate in valid_targets:
                        try:
                            if target == candidate or (
                                candidate.exists() and target.exists() and target.samefile(candidate)
                            ):
                                matched = True
                                break
                        except OSError:
                            continue
                        if os.path.realpath(str(target)) == os.path.realpath(str(candidate)):
                            matched = True
                            break
                    # Accept any existing cache resolved-skills target even if the
                    # computed cache key differs (path string /var vs /private/var).
                    if not matched and target.exists():
                        parts = target.parts
                        if "cache" in parts and "projects" in parts and target.name == "resolved-skills":
                            matched = True
                    if matched:
                        display = (
                            str(target.relative_to(self.root))
                            if target.is_relative_to(self.root)
                            else str(target)
                        )
                        self.checks.append(Check(
                            Severity.OK, f"{skills}",
                            f"symlink valid → {display}"
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
        # Commands: expected = hand-authored ai-specs/commands/ ∪ cache commands/
        commands = plat.get("commands_dir", "")
        if commands:
            commands_path = self.root / commands
            ai_specs_commands = self.root / "ai-specs" / "commands"
            if commands_path.is_dir():
                local_names: set[str] = set()
                if ai_specs_commands.is_dir():
                    local_names = {p.name for p in ai_specs_commands.glob("*.md")}
                expected = local_names | self._cache_command_names()
                actual = {p.name for p in commands_path.glob("*.md")}
                missing = expected - actual
                extra = actual - expected
                if not actual and not expected:
                    self.checks.append(Check(
                        Severity.OK, f"{commands}",
                        "no commands configured"
                    ))
                elif not actual:
                    self.checks.append(Check(
                        Severity.WARN, f"{commands}",
                        "directory empty",
                        guidance="ai-specs sync to populate"
                    ))
                elif missing:
                    self.checks.append(Check(
                        Severity.ERROR, f"{commands}",
                        f"missing {len(missing)} command(s): {', '.join(sorted(missing))}",
                        guidance="run ai-specs sync"
                    ))
                elif extra:
                    self.checks.append(Check(
                        Severity.WARN, f"{commands}",
                        f"{len(extra)} stale command(s): {', '.join(sorted(extra))}",
                        guidance="run ai-specs sync"
                    ))
                else:
                    self.checks.append(Check(
                        Severity.OK, f"{commands}",
                        f"{len(actual)} command(s) in sync"
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
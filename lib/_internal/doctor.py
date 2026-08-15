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


def bundled_command_names(cli_home: Path | None = None) -> list[str]:
    """Command ids (``.md`` stems) shipped under bundled-commands/ (source of
    truth for the per-bundled-command-id doctor check)."""
    home = cli_home or AI_SPECS_HOME
    root = home / "bundled-commands"
    if not root.is_dir():
        return [
            "rules-audit",
            "skills-as-rules",
        ]
    return sorted(p.stem for p in root.glob("*.md") if p.is_file())


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
        self._check_tracked_bundled_leftovers()
        self._check_enabled_agents()
        self._check_recipe_cli_deps()
        self._check_tracker_card_link()
        self._check_harness_env_layout()
        self._check_worktree_gate()
        self._check_repo_topology()
        self._check_stale_template_overrides()
        self._check_gate_provenance()
        self._check_worktree_flow_assets()
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

    def _bundled_command_names(self) -> set[str]:
        """Return the set of .md filenames flattened into {cache}/.bundled/commands/."""
        pc = self._load_project_cache()
        if pc is None:
            return set()
        try:
            bundled_cmds = pc.bundled_commands_root(self.root)
            if bundled_cmds.is_dir():
                return {p.name for p in bundled_cmds.glob("*.md")}
        except Exception:
            pass
        return set()

    def _check_bundled_assets(self) -> None:
        # CLI-bundled skills resolve from the cache ({cache}/.bundled/skills/),
        # not the project surface. Sync/refresh-bundled flattens them there.
        pc = self._load_project_cache()
        bundled_skills_root = None
        if pc is not None:
            try:
                bundled_skills_root = pc.bundled_skills_root(self.root) / "skills"
            except Exception:
                bundled_skills_root = None
        for skill in bundled_skill_names():
            skill_path = (bundled_skills_root / skill) if bundled_skills_root is not None else None
            if skill_path is not None and skill_path.is_dir():
                self.checks.append(Check(
                    Severity.OK, "bundled-skill",
                    f"cache .bundled/skills/{skill} present"
                ))
            else:
                self.checks.append(Check(
                    Severity.ERROR, "bundled-skill",
                    f"cache .bundled/skills/{skill} missing",
                    guidance="ai-specs sync (flattens CLI-bundled skills into the cache)"
                ))
        # CLI-bundled commands resolve from the cache ({cache}/.bundled/commands/)
        # too — never the project surface. An empty hand-authored
        # ai-specs/commands/ is healthy on its own (same as ai-specs/skills/);
        # it is not checked here.
        bundled_commands_root = None
        if pc is not None:
            try:
                bundled_commands_root = pc.bundled_commands_root(self.root)
            except Exception:
                bundled_commands_root = None
        for command in bundled_command_names():
            command_path = (
                (bundled_commands_root / f"{command}.md")
                if bundled_commands_root is not None else None
            )
            if command_path is not None and command_path.is_file():
                self.checks.append(Check(
                    Severity.OK, "bundled-commands",
                    f"cache .bundled/commands/{command}.md present"
                ))
            else:
                self.checks.append(Check(
                    Severity.ERROR, "bundled-commands",
                    f"cache .bundled/commands/{command}.md missing",
                    guidance="ai-specs sync (flattens CLI-bundled commands into the cache)"
                ))

    def _check_tracked_bundled_leftovers(self) -> None:
        """WARN when git still tracks CLI-bundled skills/commands removed from disk.

        Never runs ``git rm`` — only guides the developer.
        """
        pc = self._load_project_cache()
        if pc is None:
            return
        try:
            skill_ids = pc.tracked_bundled_skill_leftovers(self.root)
        except Exception:
            skill_ids = []
        if skill_ids:
            paths = " ".join(f"ai-specs/skills/{sid}" for sid in skill_ids)
            self.checks.append(Check(
                Severity.WARN,
                "tracked-bundled-leftover",
                f"{len(skill_ids)} removed CLI-bundled skill(s) still tracked in git",
                guidance=(
                    f"git rm -r --cached {paths}  "
                    "# then commit; ai-specs never modifies the index"
                ),
            ))
        try:
            command_ids = pc.tracked_bundled_command_leftovers(self.root)
        except Exception:
            command_ids = []
        if command_ids:
            paths = " ".join(f"ai-specs/commands/{cid}.md" for cid in command_ids)
            self.checks.append(Check(
                Severity.WARN,
                "tracked-bundled-leftover",
                f"{len(command_ids)} removed CLI-bundled command(s) still tracked in git",
                guidance=(
                    f"git rm --cached {paths}  "
                    "# then commit; ai-specs never modifies the index"
                ),
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


    def _load_trello_link(self):
        """Sibling-load lib/_internal/trello_link.py for the shared validity predicate."""
        try:
            path = Path(__file__).with_name("trello_link.py")
            spec = importlib.util.spec_from_file_location("trello_link_doctor", path)
            if spec is None or spec.loader is None:
                return None
            mod = importlib.util.module_from_spec(spec)
            sys.modules[spec.name] = mod
            spec.loader.exec_module(mod)
            return mod
        except Exception:
            return None

    def _check_tracker_card_link(self) -> None:
        """WARN when active changes lack a ## Tracker section (recipe+marker)."""
        data = self._load_manifest()
        recipes = data.get("recipes", {}) or {}
        tr = recipes.get("trello-mcp-workflow") or {}
        if not isinstance(tr, dict) or tr.get("enabled") is not True:
            return

        pc = self._load_project_cache()
        marker = None
        if pc is not None:
            try:
                marker = (
                    pc.recipe_skills_root(self.root)
                    / "trello-mcp-workflow"
                    / "bootstrap-ready"
                )
            except Exception:
                marker = None
        local_marker = self.root / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
        if not ((marker is not None and marker.is_file()) or local_marker.is_file()):
            return

        link = self._load_trello_link()
        changes_dir = self.root / "openspec" / "changes"
        deficient: list[str] = []
        if changes_dir.is_dir():
            for change in sorted(p for p in changes_dir.iterdir() if p.is_dir()):
                if change.name == "archive":
                    continue
                if not any(
                    (change / f).is_file()
                    for f in ("proposal.md", "tasks.md", "spec.md", "design.md")
                ):
                    continue
                if (change / "tracker.none").is_file():
                    continue
                if link is not None and link.is_valid_link(
                    [change / "proposal.md", change / "tasks.md"]
                ):
                    try:
                        parsed = link.parse_tracker_section(
                            [change / "proposal.md", change / "tasks.md"]
                        )
                        card_id = parsed.get("card_id", "")
                        if card_id and not link.card_id_looks_canonical(card_id):
                            self.checks.append(Check(
                                Severity.INFO, "tracker-card",
                                f"{change.name}: card_id is non-canonical (not 24-hex)",
                                guidance="prefer the 24-hex Trello card id",
                            ))
                        if "url" not in parsed or not parsed.get("url"):
                            self.checks.append(Check(
                                Severity.INFO, "tracker-card",
                                f"{change.name}: ## Tracker section missing url",
                                guidance="add url alongside card_id",
                            ))
                    except Exception:
                        pass
                    continue
                deficient.append(change.name)

        if deficient:
            sample = ", ".join(deficient[:5])
            more = f" (+{len(deficient) - 5})" if len(deficient) > 5 else ""
            self.checks.append(Check(
                Severity.WARN, "tracker-card",
                f"{len(deficient)} active change(s) missing a valid ## Tracker link section: {sample}{more}",
                guidance=(
                    "create/link a Trello card and write the ## Tracker section "
                    "of the change's proposal.md (card_id + url), or add tracker.none"
                ),
            ))
        else:
            self.checks.append(Check(
                Severity.OK, "tracker-card",
                "all active changes carry a valid ## Tracker link section (or tracker.none)",
            ))

    def _load_gate_binary(self):
        """Sibling-load lib/_internal/gate_binary.py for the gate check."""
        try:
            path = Path(__file__).with_name("gate_binary.py")
            spec = importlib.util.spec_from_file_location("gate_binary_doctor", path)
            if spec is None or spec.loader is None:
                return None
            mod = importlib.util.module_from_spec(spec)
            sys.modules[spec.name] = mod
            spec.loader.exec_module(mod)
            return mod
        except Exception:
            return None

    def _check_worktree_gate(self) -> None:
        """Diagnose the worktree-gate implementation (design §6.5).

        Severity table:
          OK    Go binary resolved, version matches the stamp, selftest passes
          INFO  gate_impl=bash configured explicitly (rollback lever)
          WARN  gate_impl=auto silently falling back to Bash
          WARN  binary version does not match the stamped version
          ERROR gate_impl=go with no usable binary (gate failing open)
          ERROR digest mismatch recorded at the last acquisition

        A fail-open gate is invisible by construction, so this ERROR is the
        only place a user can discover it.
        """
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        if not manifest.is_file():
            return
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        except Exception:
            return
        recipes = data.get("recipes") or {}
        wf = recipes.get("worktree-flow") or {}
        if not isinstance(wf, dict) or wf.get("enabled") is not True:
            return
        cfg = wf.get("config") or {}
        impl = str(cfg.get("gate_impl") or "auto")

        launcher = self.root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        stamped_version = ""
        if launcher.is_file():
            for line in launcher.read_text(encoding="utf-8").splitlines():
                if line.startswith('stamped_gate_version="'):
                    stamped_version = line.split('"', 2)[1]
                    break

        gb = self._load_gate_binary()
        if gb is None:
            return

        # Recorded digest mismatch from the last acquisition → ERROR.
        mismatch_path = gb.digest_mismatch_record_path(AI_SPECS_HOME)
        if mismatch_path.is_file():
            try:
                mismatch_text = mismatch_path.read_text(encoding="utf-8").strip()
            except OSError:
                mismatch_text = ""
            self.checks.append(Check(
                Severity.ERROR, "worktree-gate",
                mismatch_text or "gate binary digest mismatch recorded at last acquisition",
                guidance="run ai-specs sync to re-acquire; the rejected artifact was never executed",
            ))
            return

        if impl == "bash":
            self.checks.append(Check(
                Severity.INFO, "worktree-gate",
                "gate_impl=bash configured explicitly; frozen Bash reference in effect (rollback lever)",
            ))
            return

        goos, goarch = gb.detect_platform()
        binary = gb.cache_bin_path(AI_SPECS_HOME, goos=goos, goarch=goarch)
        usable = binary.is_file() and os.access(binary, os.X_OK)

        if not usable:
            expected = str(binary)
            if impl == "go":
                self.checks.append(Check(
                    Severity.ERROR, "worktree-gate",
                    f"gate_impl=go and no usable binary at {expected}; the gate is failing open",
                    guidance="run ai-specs sync (network) or AI_SPECS_GATE_BUILD=1 ai-specs sync (local build)",
                ))
            else:
                self.checks.append(Check(
                    Severity.WARN, "worktree-gate",
                    f"gate_impl=auto and no usable binary at {expected}; falling back to the Bash implementation",
                    guidance="run ai-specs sync to acquire the Go binary",
                ))
            return

        verify = getattr(gb, "verify_cached_binary", None)
        if callable(verify):
            version_key = stamped_version
            version_reader = getattr(gb, "cli_version", None)
            if callable(version_reader):
                version_key = version_reader(AI_SPECS_HOME)
            evidence = verify(
                binary,
                AI_SPECS_HOME,
                version_key,
                goos,
                goarch,
                require_receipt=True,
            )
            if not evidence.get("verified"):
                self.checks.append(Check(
                    Severity.ERROR,
                    "worktree-gate",
                    f"cached Go gate at {binary} is unverified: "
                    f"{evidence.get('reason', 'verification failed')} "
                    f"(expected_digest={evidence.get('expected_digest', 'unknown')}, "
                    f"observed_digest={evidence.get('observed_digest', 'unknown')}, "
                    f"version={evidence.get('version', 'unknown')}, "
                    f"selftest={evidence.get('selftest', 'unknown')}, "
                    f"receipt={evidence.get('receipt', 'unknown')})",
                    guidance=(
                        "run ai-specs sync to force re-acquisition; the unverified "
                        "candidate is never executed"
                    ),
                ))
                return

        version = gb.binary_version(binary)
        selftest = gb._run_selftest(binary)
        if selftest is not None:
            self.checks.append(Check(
                Severity.ERROR, "worktree-gate",
                f"gate binary at {binary} failed --selftest: {selftest}; the gate is not enforcing",
                guidance="run ai-specs sync to re-acquire or re-build",
            ))
            return

        if stamped_version and version != stamped_version:
            self.checks.append(Check(
                Severity.WARN, "worktree-gate",
                f"gate binary version {version} does not match the stamped version {stamped_version}",
                guidance="run ai-specs sync to re-acquire for the installed CLI version",
            ))
            return

        size_kb = gb.cache_size(AI_SPECS_HOME) // 1024
        self.checks.append(Check(
            Severity.OK, "worktree-gate",
            f"Go binary {version} at {binary}; selftest passed (cache {size_kb} KiB)",
        ))

    def _load_env_scaffold(self):
        path = Path(__file__).with_name("env_scaffold.py")
        spec = importlib.util.spec_from_file_location("env_scaffold_doctor", path)
        if spec is None or spec.loader is None:
            return None
        mod = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = mod
        spec.loader.exec_module(mod)
        return mod

    def _check_harness_env_layout(self) -> None:
        """WARN on direnv / managed .envrc / missing harness env keys when MCP env needed."""
        env_mod = self._load_env_scaffold()
        if env_mod is None:
            return
        try:
            vars_map = env_mod.collect_env_vars(self.root)
        except Exception:
            return
        if not vars_map:
            return

        if shutil.which("direnv") is None:
            self.checks.append(Check(
                Severity.WARN, "direnv",
                "direnv not on PATH (needed to load harness MCP env for shells)",
                guidance="brew install direnv && direnv allow  # or see https://direnv.net",
            ))
        else:
            self.checks.append(Check(
                Severity.OK, "direnv",
                "direnv available on PATH",
            ))

        envrc = self.root / ".envrc"
        envrc_text = envrc.read_text(encoding="utf-8") if envrc.is_file() else ""
        is_current = False
        if envrc.is_file():
            checker = getattr(env_mod, "managed_block_is_current", None)
            if callable(checker):
                is_current = checker(envrc_text)
            else:
                is_current = env_mod.has_managed_block(envrc_text)
        if not is_current:
            stale = bool(envrc_text) and env_mod.has_managed_block(envrc_text)
            self.checks.append(Check(
                Severity.WARN, "envrc-managed",
                (
                    "project-root .envrc has stale ai-specs managed block"
                    if stale
                    else "project-root .envrc missing ai-specs managed block"
                ),
                guidance="run ai-specs configure-recipes to ensure root .envrc",
            ))
        else:
            self.checks.append(Check(
                Severity.OK, "envrc-managed",
                "project-root .envrc has ai-specs managed block",
            ))

        try:
            present = env_mod.load_harness_env(self.root)
        except Exception:
            present = {}
        missing = [v for v in sorted(vars_map) if not (present.get(v) or "").strip()]
        if missing:
            self.checks.append(Check(
                Severity.WARN, "harness-env",
                f"missing/empty in ai-specs.env: {', '.join(missing)}",
                guidance="run ai-specs configure-recipes to set harness env values",
            ))
        else:
            self.checks.append(Check(
                Severity.OK, "harness-env",
                f"ai-specs.env has {len(vars_map)} required MCP env key(s)",
            ))

    def _mcp_server_count(self, data: dict) -> int:
        mcp = data.get("mcp")
        if not isinstance(mcp, dict):
            return 0
        return len(mcp)


    def _load_util(self):
        path = Path(__file__).with_name("util.py")
        spec = importlib.util.spec_from_file_location("util_doctor", path)
        if spec is None or spec.loader is None:
            return None
        mod = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = mod
        spec.loader.exec_module(mod)
        return mod

    def _check_repo_topology(self) -> None:
        """INFO: echo resolved worktree-flow topology + initialized submodule count."""
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        if not manifest.is_file():
            return
        util = self._load_util()
        if util is None:
            return
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        except Exception:
            return
        recipes = data.get("recipes") or {}
        wf = recipes.get("worktree-flow") or {}
        if not isinstance(wf, dict) or wf.get("enabled") is not True:
            return
        cfg = wf.get("config") or {}
        configured = str(cfg.get("repo_topology") or "auto")
        try:
            res = util.resolve_repo_topology(self.root, configured)
        except Exception:
            return
        n = len(res.submodules)
        self.checks.append(Check(
            Severity.INFO,
            "repo-topology",
            f"{res.resolved} (via {res.via}; {n} initialized submodule(s))",
        ))

    def _check_stale_template_overrides(self) -> None:
        """Diagnose governed templates using lock-backed ownership state."""
        util = self._load_util()
        if util is None:
            return
        catalog = AI_SPECS_HOME / "catalog" / "recipes"
        if not catalog.is_dir():
            return
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        if not manifest.is_file():
            return
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        except Exception:
            return

        lock_path = self.root / "ai-specs" / ".ai-specs.lock"
        lock_mod_path = Path(__file__).with_name("lock.py")
        lock_spec = importlib.util.spec_from_file_location("lock_doctor", lock_mod_path)
        if lock_spec is None or lock_spec.loader is None:
            return
        lock_mod = importlib.util.module_from_spec(lock_spec)
        sys.modules[lock_spec.name] = lock_mod
        try:
            lock_spec.loader.exec_module(lock_mod)
            managed = lock_mod.load_lock(lock_path).get("managed", {})
        except Exception:
            managed = {}

        recipes = data.get("recipes") or {}
        schema_path = Path(__file__).with_name("recipe_schema.py")
        spec = importlib.util.spec_from_file_location("recipe_schema_doctor", schema_path)
        if spec is None or spec.loader is None:
            return
        schema = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = schema
        try:
            spec.loader.exec_module(schema)
        except Exception:
            return

        for rid, val in recipes.items():
            if not isinstance(val, dict) or val.get("enabled") is not True:
                continue
            recipe_dir = catalog / rid
            recipe_toml = recipe_dir / "recipe.toml"
            if not recipe_toml.is_file():
                continue
            try:
                recipe = schema.load_recipe_toml(recipe_toml)
            except Exception:
                continue
            merged_cfg = val.get("config") if isinstance(val.get("config"), dict) else {}
            for tpl in getattr(recipe, "templates", []) or []:
                if getattr(tpl, "condition", None) != "not_exists":
                    continue
                src = recipe_dir / tpl.source
                dest = self.root / tpl.target
                if not dest.is_file() or not src.is_file():
                    continue
                target = Path(tpl.target).as_posix()
                if (
                    rid == "worktree-flow"
                    and target
                    == "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
                ):
                    continue
                state = util.classify_managed_override(
                    dest,
                    managed.get(target),
                    would_write=util.render_override_bytes(src, merged_cfg),
                )
                policy = getattr(tpl, "update_policy", "auto") or "auto"
                if state == "user_modified":
                    message = f"{tpl.target} is user-modified; sync will preserve it"
                elif state == "untracked":
                    disk_sha = util.sha256_bytes(dest.read_bytes())
                    rendered_sha = util.sha256_bytes(
                        util.render_override_bytes(src, merged_cfg)
                    )
                    legacy_catalog_sha = util.sha256_bytes(src.read_bytes())
                    if disk_sha in (rendered_sha, legacy_catalog_sha):
                        continue
                    message = (
                        f"{tpl.target} has missing ownership metadata; sync will preserve the existing file "
                        "without assigning ownership. Leave it unchanged to preserve it, or remove it and "
                        "rerun sync to restore the current recipe version"
                    )
                elif state == "managed_stale" and policy != "auto":
                    message = f"{tpl.target} is managed-stale and policy is {policy}; sync will preserve it"
                else:
                    continue
                self.checks.append(Check(
                    Severity.WARN,
                    "stale-override",
                    message,
                    guidance=f"rm {tpl.target} && ai-specs sync",
                ))

    def _check_gate_provenance(self) -> None:
        """Diagnose generated runtime hook (gate) provenance from lock baselines.

        Warns when a materialized gate's current bytes differ from its recorded
        baseline (user-modified) or when no baseline exists (unknown
        provenance); stays quiet for gates whose baseline matches. Mirrors the
        sync-side classifier exactly; never rewrites anything.
        """
        util = self._load_util()
        if util is None:
            return
        catalog = AI_SPECS_HOME / "catalog" / "recipes"
        if not catalog.is_dir():
            return
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        if not manifest.is_file():
            return
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        except Exception:
            return

        lock_path = self.root / "ai-specs" / ".ai-specs.lock"
        lock_mod_path = Path(__file__).with_name("lock.py")
        lock_spec = importlib.util.spec_from_file_location("lock_doctor_gate", lock_mod_path)
        if lock_spec is None or lock_spec.loader is None:
            return
        lock_mod = importlib.util.module_from_spec(lock_spec)
        sys.modules[lock_spec.name] = lock_mod
        try:
            lock_spec.loader.exec_module(lock_mod)
            managed = lock_mod.load_lock(lock_path).get("managed", {})
        except Exception:
            managed = {}

        recipes = data.get("recipes") or {}
        schema_path = Path(__file__).with_name("recipe_schema.py")
        spec = importlib.util.spec_from_file_location("recipe_schema_doctor_gate", schema_path)
        if spec is None or spec.loader is None:
            return
        schema = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = schema
        try:
            spec.loader.exec_module(schema)
        except Exception:
            return

        for rid, val in recipes.items():
            if not isinstance(val, dict) or val.get("enabled") is not True:
                continue
            recipe_dir = catalog / rid
            recipe_toml = recipe_dir / "recipe.toml"
            if not recipe_toml.is_file():
                continue
            try:
                recipe = schema.load_recipe_toml(recipe_toml)
            except Exception:
                continue
            if rid == "worktree-flow":
                continue
            for hook in getattr(recipe, "runtime_hooks", []) or []:
                rel = (
                    f"ai-specs/recipes/{rid}/hooks/"
                    f"{Path(hook.script).name}"
                )
                dest = self.root / rel
                if not dest.is_file():
                    continue
                state = util.classify_managed_override(dest, managed.get(rel))
                if state == "user_modified":
                    message = f"{rel} is user-modified; sync will preserve it"
                elif state == "untracked":
                    message = (
                        f"{rel} has no recorded provenance; sync will preserve it "
                        "and record a baseline only when the CLI renders the gate"
                    )
                else:
                    continue
                self.checks.append(Check(
                    Severity.WARN,
                    "gate-provenance",
                    message,
                    guidance=f"rm {rel} && ai-specs sync (or ai-specs sync --refresh-gates)",
                ))

    def _check_worktree_flow_assets(self) -> None:
        """Report governed worktree-flow freshness without repairing anything."""
        util = self._load_util()
        if util is None:
            return
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        if not manifest.is_file():
            return
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
        except Exception:
            return
        wf = (data.get("recipes") or {}).get("worktree-flow") or {}
        if not isinstance(wf, dict) or wf.get("enabled") is not True:
            return
        cfg = wf.get("config") if isinstance(wf.get("config"), dict) else {}

        lock_mod_path = Path(__file__).with_name("lock.py")
        lock_spec = importlib.util.spec_from_file_location("lock_doctor_wf_assets", lock_mod_path)
        if lock_spec is None or lock_spec.loader is None:
            return
        lock_mod = importlib.util.module_from_spec(lock_spec)
        sys.modules[lock_spec.name] = lock_mod
        try:
            lock_spec.loader.exec_module(lock_mod)
            managed = lock_mod.load_lock(
                self.root / "ai-specs" / ".ai-specs.lock"
            ).get("managed", {})
        except Exception:
            managed = {}

        recipe_dir = AI_SPECS_HOME / "catalog" / "recipes" / "worktree-flow"
        version_path = AI_SPECS_HOME / "VERSION"
        version = version_path.read_text(encoding="utf-8").strip() if version_path.is_file() else "dev"
        values = {
            "__WORKTREE_GATE_MODE__": str(cfg.get("gate_mode") or "always"),
            "__WORKTREE_GATE_SCOPE__": str(cfg.get("gate_scope") or "auto"),
            "__WORKTREE_REPO_TOPOLOGY__": str(cfg.get("repo_topology") or "auto"),
            "__WORKTREE_GATE_IMPL__": str(cfg.get("gate_impl") or "auto"),
            "__WORKTREE_GATE_VERSION__": version,
        }
        assets = (
            (
                "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh",
                recipe_dir / "templates" / "worktree-cleanup.sh",
                "template",
            ),
            (
                "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh",
                recipe_dir / "hooks" / "worktree-gate.sh",
                "launcher",
            ),
            (
                "ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh",
                recipe_dir / "hooks" / "worktree-gate-legacy.sh",
                "legacy gate",
            ),
        )
        for rel, source, kind in assets:
            if not source.is_file():
                self.checks.append(Check(
                    Severity.ERROR,
                    "worktree-flow-freshness",
                    f"canonical {kind} source is missing at {source}",
                    guidance="run ai-specs upgrade, then ai-specs sync",
                ))
                continue
            desired = source.read_bytes()
            if kind == "template":
                desired = desired.replace(
                    b"__WORKTREE_REPO_TOPOLOGY__",
                    values["__WORKTREE_REPO_TOPOLOGY__"].encode(),
                )
            elif kind == "launcher":
                text = desired.decode("utf-8")
                for token, value in values.items():
                    text = text.replace(token, value)
                desired = text.encode("utf-8")
            dest = self.root / rel
            state = util.classify_managed_override(
                dest,
                managed.get(rel),
                would_write=desired,
            )
            if state == "managed_current":
                continue
            observed = "missing" if not dest.is_file() else util.sha256_bytes(dest.read_bytes())
            desired_sha = util.sha256_bytes(desired)
            self.checks.append(Check(
                Severity.ERROR,
                "worktree-flow-freshness",
                f"{rel} state={state} observed_digest={observed} "
                f"desired_digest={desired_sha}; ordinary sync will force the "
                "latest verified replacement (cache-only backup where supported)",
                guidance="run ai-specs sync (or ai-specs sync --refresh-gates)",
            ))

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
        # ∪ CLI-bundled commands (all three are CLI-driven or merge inputs).
        commands = plat.get("commands_dir", "")
        if commands:
            commands_path = self.root / commands
            ai_specs_commands = self.root / "ai-specs" / "commands"
            if commands_path.is_dir():
                local_names: set[str] = set()
                if ai_specs_commands.is_dir():
                    local_names = {p.name for p in ai_specs_commands.glob("*.md")}
                expected = local_names | self._cache_command_names() | self._bundled_command_names()
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

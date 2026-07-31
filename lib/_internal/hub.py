#!/usr/bin/env python3
"""Interactive front door for bare `ai-specs` (status + command menu).

Import-time contract: pure stdlib. rich/questionary are imported lazily only
after the deps gate passes. Sibling modules (util, doctor, recipe-list,
skill-resolution) load via absolute path.
"""

from __future__ import annotations

import argparse
import importlib.util
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from enum import Enum
from pathlib import Path


def _load_sibling(name: str):
    """Load a same-directory _internal module by absolute path (sys.path-independent)."""
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_util = _load_sibling("util")
_doctor = _load_sibling("doctor")
_recipes = _load_sibling("recipe-list")
_skillres = _load_sibling("skill-resolution")


class Mode(Enum):
    INTERACTIVE_HUB = "interactive-hub"
    NONINTERACTIVE_STATUS = "noninteractive-status"
    OFFER_INIT = "offer-init"
    ERROR_UNINITIALIZED = "error-uninitialized"


def decide_mode(*, initialized: bool, tty: bool) -> Mode:
    if initialized:
        return Mode.INTERACTIVE_HUB if tty else Mode.NONINTERACTIVE_STATUS
    return Mode.OFFER_INIT if tty else Mode.ERROR_UNINITIALIZED


class Action(Enum):
    SYNC = "sync"
    DOCTOR = "doctor"
    AGENTS = "agents"
    SKILLS = "skills"
    RECIPES = "recipe"
    CONFIGURE_RECIPES = "configure-recipes"
    RULES_AUDIT = "rules-audit"
    UPGRADE = "upgrade"
    VERSION = "version"
    HELP = "help"
    INIT = "init"
    QUIT = "quit"


_MENU: list[tuple[Action, str, str]] = [
    (Action.SYNC, "Sync", "Reconcile manifest → bundled + vendor + AGENTS.md + agents"),
    (Action.DOCTOR, "Doctor", "Full project health report (read-only)"),
    (Action.AGENTS, "Agents", "Select which AI agents to enable"),
    (Action.SKILLS, "Skills", "List / inspect project skills by origin"),
    (Action.RECIPES, "Recipes", "List / add / remove / configure catalog recipes"),
    (Action.RULES_AUDIT, "Rules audit", "Inventory legacy rules for migration"),
    (Action.UPGRADE, "Upgrade", "Upgrade the global ai-specs installation"),
    (Action.VERSION, "Version", "Print the CLI version"),
    (Action.HELP, "Help", "Show ai-specs command help"),
    (Action.INIT, "Init wizard", "Re-run interactive onboarding"),
    (Action.QUIT, "Quit", "Exit the hub"),
]


# ── Pure layer (dep-free) ────────────────────────────────────────────────────


def _read_version() -> str:
    p = _util.ai_specs_home() / "VERSION"
    return p.read_text(encoding="utf-8").strip() if p.is_file() else "unknown"


def _print_version() -> None:
    print(_read_version())


def recipe_add_choices(recipes: list[dict]) -> list[tuple[str, str]]:
    """Recipes installable now = status == 'available'."""
    out: list[tuple[str, str]] = []
    for r in recipes:
        if r.get("status") != "available":
            continue
        rid = r["id"]
        name = r.get("name") or rid
        version = r.get("version") or ""
        out.append((f"{name} ({rid})  v{version}", rid))
    return out


def recipe_remove_choices(recipes: list[dict]) -> list[tuple[str, str]]:
    """Recipes in the manifest = status in {'installed','disabled'}."""
    out: list[tuple[str, str]] = []
    for r in recipes:
        status = r.get("status")
        if status not in {"installed", "disabled"}:
            continue
        rid = r["id"]
        name = r.get("name") or rid
        out.append((f"{name} ({rid})  [{status}]", rid))
    return out


def _skill_description(skill_dir: Path) -> str:
    """Read SKILL.md front-matter description: (mirrors skills-list.sh)."""
    path = skill_dir / "SKILL.md"
    if not path.is_file():
        return ""
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return ""
    m = re.match(r"^---\s*\n(.*?)\n---", text, re.DOTALL)
    if not m:
        return ""
    dm = re.search(r"^description:\s*(.+?)\s*$", m.group(1), re.MULTILINE)
    if not dm:
        return ""
    desc = dm.group(1).strip()
    if desc.startswith('"') and desc.endswith('"'):
        desc = desc[1:-1].replace('\\"', '"').replace("\\\\", "\\")
    elif desc.startswith("'") and desc.endswith("'"):
        desc = desc[1:-1]
    return desc[:80]

_SKILL_BUCKET_KEYS: dict[str, str] = {
    "bundled": "Bundled (CLI-shipped)",
    "local": "Local / vendored (project)",
    "recipe": "Provided by recipes / catalog",
    "dep": "Registered deps",
}


def categorize_skills(project_root: Path, cli_home: Path) -> dict[str, list[dict]]:
    """Partition project skills into bundled / local / recipe / dep buckets."""
    bundled_names = set(_doctor.bundled_skill_names(cli_home))
    collected = _skillres.collect_skills(project_root, cli_home=cli_home)
    buckets: dict[str, list[dict]] = {k: [] for k in _SKILL_BUCKET_KEYS}
    for skill_id, (source_type, path) in collected.items():
        entry = {"id": skill_id, "path": path, "desc": _skill_description(path)}
        if source_type == "local":
            if skill_id in bundled_names:
                buckets["bundled"].append(entry)
            else:
                buckets["local"].append(entry)
        elif source_type in buckets:
            buckets[source_type].append(entry)
    for key in buckets:
        buckets[key].sort(key=lambda e: e["id"])
    return buckets


@dataclass(frozen=True)
class StatusSummary:
    root: Path
    ok: int
    info: int
    warn: int
    error: int
    exit_code: int
    headline: str
    checks: list
    version: str
    topology: str = ""
    topology_via: str = ""


def status_summary(root: Path) -> StatusSummary:
    doc = _doctor.Doctor(root)
    exit_code = doc.run()
    Sev = _doctor.Severity
    counts = {s: sum(1 for c in doc.checks if c.severity == s) for s in Sev}
    error = counts[Sev.ERROR]
    warn = counts[Sev.WARN]
    if error:
        headline = f"{error} error(s)"
    elif warn:
        headline = f"{warn} warning{'s' if warn != 1 else ''}"
    else:
        headline = "healthy"
    topology = ""
    topology_via = ""
    manifest = root / "ai-specs" / "ai-specs.toml"
    if manifest.is_file():
        try:
            import tomllib
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
            wf = (data.get("recipes") or {}).get("worktree-flow") or {}
            if isinstance(wf, dict) and wf.get("enabled") is True:
                cfg_val = str((wf.get("config") or {}).get("repo_topology") or "auto")
                res = _util.resolve_repo_topology(root, cfg_val)
                topology = res.resolved
                topology_via = res.via
        except Exception:
            pass
    return StatusSummary(
        root=root,
        ok=counts[Sev.OK],
        info=counts[Sev.INFO],
        warn=warn,
        error=error,
        exit_code=exit_code,
        headline=headline,
        checks=list(doc.checks),
        version=_read_version(),
        topology=topology,
        topology_via=topology_via,
    )


def _print_uninit_error(target: Path) -> None:
    print(f"ERROR: no ai-specs project at {target}", file=sys.stderr)
    print("Run 'ai-specs init' to create one.", file=sys.stderr)


def _run_noninteractive(target: Path) -> int:
    summary = status_summary(target)
    print(f"ai-specs status — {summary.headline}")
    print(f"  version: {summary.version}")
    print(f"  target: {summary.root}")
    if summary.topology:
        via = summary.topology_via or "auto"
        print(f"  topology: {summary.topology} ({via})")
    print(
        f"  Summary: {summary.ok} OK, {summary.info} INFO, "
        f"{summary.warn} WARN, {summary.error} ERROR"
    )
    print()
    print("Commands:")
    for _act, title, desc in _MENU:
        print(f"  {title:12s}  {desc}")
    return 0


# ── Widget layer (lazy questionary / input) ──────────────────────────────────


def pick_one(
    message: str,
    options: list[tuple[str, str]],
    *,
    default: str | None = None,
) -> str | None:
    """questionary.select. options = [(label, value), ...]. None if aborted/empty."""
    if not options:
        return None
    import questionary

    choices = [questionary.Choice(title=label, value=value) for label, value in options]
    kwargs: dict = {}
    if default is not None:
        kwargs["default"] = default
    return questionary.select(message, choices=choices, **kwargs).ask()


def pick_many(message: str, options: list[tuple[str, str, bool]]) -> list[str] | None:
    """questionary.checkbox. options = [(label, value, checked), ...]."""
    if not options:
        return None
    import questionary

    choices = [
        questionary.Choice(title=label, value=value, checked=checked)
        for label, value, checked in options
    ]
    return questionary.checkbox(message, choices=choices).ask()


def confirm_action(message: str, *, default: bool = True) -> bool | None:
    """questionary.confirm. Returns bool, or None if aborted."""
    import questionary

    return questionary.confirm(message, default=default).ask()


def pause(message: str = "Press Enter to return…") -> bool:
    """Lightweight blocking pause. True normally, False on EOFError."""
    try:
        input(message)
        return True
    except EOFError:
        return False


@dataclass
class CommandMenu:
    def prompt(self) -> Action:
        import questionary

        # questionary<2.1 has no Choice.description — embed in title.
        choices = [
            questionary.Choice(title=f"{title} — {desc}", value=act)
            for act, title, desc in _MENU
        ]
        answer = questionary.select("What do you want to do?", choices=choices).ask()
        return answer if answer is not None else Action.QUIT


@dataclass
class DelegateRunner:
    cli: Path
    target: Path

    def run(self, action: Action, extra: list[str] | None = None) -> int:
        argv = [str(self.cli), action.value]
        if extra:
            # Subcommand-based dispatchers (recipe, skills) want subcommand before target
            argv.extend(extra)
            argv.append(str(self.target))
        else:
            argv.append(str(self.target))
        return subprocess.run(argv).returncode


@dataclass
class StatusPanel:
    summary: StatusSummary

    def render(self):
        from rich.panel import Panel
        from rich.table import Table

        table = Table.grid(padding=(0, 1))
        table.add_column(style="bold")
        table.add_column()
        table.add_row("version", self.summary.version)
        table.add_row("target", str(self.summary.root))
        if self.summary.topology:
            via = self.summary.topology_via or "auto"
            label = self.summary.topology
            if via == "auto":
                label = f"{self.summary.topology} (auto→{self.summary.topology})"
            else:
                label = f"{self.summary.topology} (via {via})"
            table.add_row("topology", label)
        table.add_row(
            "Summary:",
            f"{self.summary.ok} OK, {self.summary.info} INFO, "
            f"{self.summary.warn} WARN, {self.summary.error} ERROR "
            f"({self.summary.headline})",
        )
        Sev = _doctor.Severity
        for check in self.summary.checks:
            if check.severity is Sev.OK:
                continue
            table.add_row(check.severity.value, f"{check.name}: {check.message}")

        if self.summary.error:
            border = "red"
        elif self.summary.warn:
            border = "yellow"
        else:
            border = "green"
        return Panel(table, title="ai-specs", border_style=border)
def _render_skills_buckets(console, buckets: dict[str, list[dict]]) -> None:
    for key, title in _SKILL_BUCKET_KEYS.items():
        console.print(f"[bold]── {title} ──[/bold]")
        entries = buckets.get(key) or []
        if not entries:
            console.print("  (none)")
        else:
            for e in entries:
                line = f"  {e['id']}"
                if e.get("desc"):
                    line += f"  — {e['desc']}"
                from rich.markup import escape
                console.print(escape(line))
        console.print()


def _run_skills_submenu(console, target: Path) -> int | None:
    """Interactive Skills submenu (mirrors Recipes: one action then back)."""
    sub = pick_one(
        "Skills:",
        [
            ("List skills (categorized)", "list"),
            ("Inspect a skill", "inspect"),
            ("Back", "back"),
        ],
    )
    if sub is None or sub == "back":
        return None

    cli_home = _util.ai_specs_home()
    buckets = categorize_skills(target, cli_home)

    if sub == "list":
        _render_skills_buckets(console, buckets)
        if not pause():
            return 0
        return None
    if sub == "inspect":
        options: list[tuple[str, str]] = []
        for key in _SKILL_BUCKET_KEYS:
            for e in buckets.get(key) or []:
                options.append((f"{e['id']}  [{key}]", e["id"]))
        if not options:
            console.print("[yellow]No skills found to inspect.[/yellow]")
            if not pause():
                return 0
            return None
        sid = pick_one("Skill to inspect:", options)
        if sid is None:
            return None
        found = None
        for entries in buckets.values():
            for e in entries:
                if e["id"] == sid:
                    found = e
                    break
            if found:
                break
        if found is None:
            console.print(f"[red]Skill not found: {sid}[/red]")
        else:
            from rich.markup import escape
            console.print(f"[bold]{escape(found['id'])}[/bold]")
            console.print(f"  path: {escape(str(found['path'] / 'SKILL.md'))}")
            console.print(f"  desc: {escape(found['desc'] or '(none)')}")
        if not pause():
            return 0
        return None

    return None


def _run_recipes_submenu(console, runner: DelegateRunner, target: Path) -> int | None:
    """Interactive Recipes submenu. Returns 0 to quit hub, None to continue."""
    sub = pick_one(
        "Recipes:",
        [
            ("List recipes", "list"),
            ("Add recipe", "add"),
            ("Remove recipe", "remove"),
            ("Configure recipe", "configure"),
            ("Back", "back"),
        ],
    )
    if sub is None or sub == "back":
        return None

    if sub == "list":
        rc = runner.run(Action.RECIPES, extra=["list"])
    elif sub == "add":
        try:
            recipes = _recipes.list_recipes(target)
        except Exception as exc:
            console.print(f"[yellow]Recipes unavailable: {exc}[/yellow]")
            if not pause():
                return 0
            return None
        choices = recipe_add_choices(recipes)
        if not choices:
            console.print("[yellow]No catalog recipes available to add.[/yellow]")
            if not pause():
                return 0
            return None
        rid = pick_one("Recipe to add:", choices)
        if rid is None:
            return None
        rc = runner.run(Action.RECIPES, extra=["add", rid])
    elif sub == "remove":
        try:
            recipes = _recipes.list_recipes(target)
        except Exception as exc:
            console.print(f"[yellow]Recipes unavailable: {exc}[/yellow]")
            if not pause():
                return 0
            return None
        choices = recipe_remove_choices(recipes)
        if not choices:
            console.print("[yellow]No recipes installed to remove.[/yellow]")
            if not pause():
                return 0
            return None
        rid = pick_one("Recipe to remove:", choices)
        if rid is None:
            return None
        rc = runner.run(Action.RECIPES, extra=["remove", rid])
    elif sub == "configure":
        rc = runner.run(Action.CONFIGURE_RECIPES)
    else:
        return None

    print("✓ done" if rc == 0 else f"✗ exited {rc}")
    if not pause():
        return 0
    return None


def _run_agents_submenu(console, target: Path) -> int | None:
    """Interactive Agents picker. Returns 0 to quit hub, None to continue."""
    manifest_path = target / "ai-specs" / "ai-specs.toml"
    if not manifest_path.is_file():
        console.print("[red]Manifest not found — run ai-specs init first[/red]")
        return None

    toml_path = Path(__file__).with_name("toml-read.py")
    spec = importlib.util.spec_from_file_location("toml_read_inline", toml_path)
    if spec and spec.loader:
        tr_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = tr_module
        spec.loader.exec_module(tr_module)
        data = tr_module.load_toml(manifest_path)
        current = tr_module.read_agents(data).get("enabled", [])
    else:
        current = []

    supported = ["claude", "cursor", "opencode", "codex", "copilot", "gemini", "pi", "omp"]
    options = [(a, a, a in current) for a in supported]
    selected = pick_many("Select agents to enable:", options)
    if selected is None:
        return None

    text = manifest_path.read_text(encoding="utf-8")
    lines = text.splitlines(keepends=True)
    in_agents = False
    written = False
    new_lines: list[str] = []
    for line in lines:
        stripped = line.strip()
        if stripped == "[agents]":
            in_agents = True
            new_lines.append(line)
            continue
        if in_agents and stripped.startswith("enabled"):
            new_lines.append(f'enabled = [{", ".join(repr(a) for a in selected)}]\n')
            in_agents = False
            written = True
            continue
        if in_agents and stripped.startswith("["):
            new_lines.append(f'enabled = [{", ".join(repr(a) for a in selected)}]\n')
            in_agents = False
            written = True
        new_lines.append(line)

    if not written:
        new_lines.append(f"[agents]\nenabled = [{', '.join(repr(a) for a in selected)}]\n")

    manifest_path.write_text("".join(new_lines), encoding="utf-8")
    print(f"  ✓ agents updated: {', '.join(selected) if selected else '(none)'}")
    print("  Run sync to regenerate agent configs.")
    if not pause():
        return 0
    return None


def _run_interactive_hub(target: Path) -> int:
    from rich.console import Console

    console = Console()
    cli = _util.ai_specs_home() / "bin" / "ai-specs"
    runner = DelegateRunner(cli=cli, target=target)

    while True:
        summary = status_summary(target)
        console.print(StatusPanel(summary).render())
        action = CommandMenu().prompt()

        if action is Action.QUIT:
            return 0

        if action is Action.RECIPES:
            result = _run_recipes_submenu(console, runner, target)
            if result is not None:
                return result
            continue

        if action is Action.SKILLS:
            result = _run_skills_submenu(console, target)
            if result is not None:
                return result
            continue

        if action is Action.AGENTS:
            result = _run_agents_submenu(console, target)
            if result is not None:
                return result
            continue

        if action is Action.VERSION:
            _print_version()
            continue

        rc = runner.run(action)
        if rc == 0:
            print("✓ done")
        else:
            print(f"✗ exited {rc}")
        if not pause():
            return 0


def _offer_init(target: Path) -> bool:
    print(f"No ai-specs project found at {target}.")
    answer = confirm_action("Run the init wizard now?", default=True)
    if not answer:
        return False
    cli = _util.ai_specs_home() / "bin" / "ai-specs"
    rc = DelegateRunner(cli=cli, target=target).run(Action.INIT)
    if rc != 0 or not _util.is_initialized(target):
        return False
    # After a successful init, auto-run sync so the hub shows a clean status
    # instead of doctor errors about missing agent configs.
    print("✓ init complete — running sync to generate agent configs…")
    rc = DelegateRunner(cli=cli, target=target).run(Action.SYNC)
    return rc == 0


def _parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="ai-specs hub — status + command menu")
    parser.add_argument(
        "target",
        nargs="?",
        default=os.getcwd(),
        help="Target project root (default: current directory)",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    target = Path(args.target).resolve()
    if not target.is_dir():
        print(f"ERROR: target is not a directory: {target}", file=sys.stderr)
        return 2

    tty = sys.stdin.isatty() and sys.stdout.isatty()
    mode = decide_mode(initialized=_util.is_initialized(target), tty=tty)

    if mode is Mode.ERROR_UNINITIALIZED:
        _print_uninit_error(target)
        return 2
    if mode is Mode.NONINTERACTIVE_STATUS:
        return _run_noninteractive(target)

    err = _util.ensure_deps(_util.vendor_dir())
    if err is not None:
        return err

    if mode is Mode.OFFER_INIT:
        if not _offer_init(target):
            return 0

    return _run_interactive_hub(target)


if __name__ == "__main__":
    sys.exit(main())

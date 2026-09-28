"""Doctor renders the Go `tracker-ledger` finding; it no longer grades links.

The tracker ledger has exactly one authoritative grader — the Go `--ledger`
predicate — and doctor is a JSON-consumption host with no write side effect
(A8/A10/D15). These tests pin, through the `bin/ai-specs doctor` process
boundary:

  * doctor renders the verdict's ``doctor`` finding under the check name
    ``tracker-ledger`` with the A10 severities (INFO unbound, WARN ambiguous /
    declared-not-bound / missing-witness / conflict, ERROR infra);
  * doctor fails the check closed to ERROR when no verified binary resolves;
  * a legacy ``## Tracker`` section never changes the verdict, because doctor
    stopped grading it;
  * the runtime brief and generated agent files gain no static dormancy line.
"""
from __future__ import annotations

import json
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    cache_project_dir,
    invoke,
    isolated_home,
    snapshot,
    tree_diff,
)

ROOT = Path(__file__).resolve().parents[1]
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"

BUNDLED_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
BUNDLED_COMMANDS = ("rules-audit", "skills-as-rules")

_LINE_RE = re.compile(r"^\s*(OK|INFO|WARN|ERROR)\s+(?P<name>\S+)\s+(?P<body>.*)$")


def _platform() -> tuple[str, str]:
    """Mirror gate_binary.detect_platform for staging cache paths in tests."""
    goos = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system(), "")
    machine = platform.machine()
    goarch = "arm64" if machine in ("arm64", "aarch64") else (
        "amd64" if machine in ("x86_64", "amd64") else "")
    return goos, goarch


def _make_home(base: Path) -> Path:
    """Isolated CLI home with a REAL lib copy and an empty catalog.

    doctor.py derives its cache root from its own realpath, so a symlinked
    lib would resolve back into the repository and make the gate checks read
    (never write) repo cache state. A real copy keeps every lookup in temp;
    the empty catalog keeps the SHA256SUMS trust root out of the way so a
    receipt-backed stub binary is accepted exactly like an acquired one.
    """
    home = isolated_home(base, catalog=False)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    return home


def _seed_clean_cache(root: Path, home: Path) -> None:
    """Pre-seed the per-project bundled cache so only the check under test can
    influence the frozen exit-code contract (exit 1 iff any ERROR)."""
    bundled = cache_project_dir(root, home) / ".bundled"
    for skill in BUNDLED_SKILLS:
        (bundled / "skills" / skill).mkdir(parents=True, exist_ok=True)
    for command in BUNDLED_COMMANDS:
        path = bundled / "commands" / f"{command}.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("# bundled\n", encoding="utf-8")


def _stage_gate_stub(
    home: Path,
    *,
    ledger_payload: dict | None = None,
    ledger_stdout: str | None = None,
) -> None:
    """Install a receipt-backed stub binary at the version-keyed cache path.

    The CLI environment is fixed, so the canned verdict is embedded in the
    stub script itself instead of being handed over via an env pin.
    """
    version = (home / "VERSION").read_text(encoding="utf-8").strip()
    goos, goarch = _platform()
    bindir = home / "cache" / "bin" / "worktree-gate" / version / f"{goos}-{goarch}"
    bindir.mkdir(parents=True, exist_ok=True)
    stub = bindir / "worktree-gate"
    lines = ["#!/bin/sh"]
    if ledger_payload is not None:
        lines += ["cat <<'JSON'", json.dumps(ledger_payload), "JSON"]
    elif ledger_stdout is not None:
        lines.append(f"printf '%s\\n' '{ledger_stdout}'")
    lines.append("exit 0")
    stub.write_text("\n".join(lines) + "\n", encoding="utf-8")
    stub.chmod(0o755)
    (bindir / "worktree-gate.verified").write_text("status=verified\n", encoding="utf-8")


def _named_checks(stdout: str, name: str) -> list[tuple[str, str]]:
    """The rendered (severity, message+guidance) lines for one check name."""
    found = []
    for line in stdout.splitlines():
        match = _LINE_RE.match(line)
        if match and match.group("name") == name:
            found.append((match.group(1), match.group("body")))
    return found


def verdict(*, severity: str, message: str, reason: str = "") -> dict:
    return {
        "capability": "tracker",
        "active": severity == "OK",
        "checkpoint": "work-start",
        "mode": "warn",
        "decision": "allow",
        "reason": reason,
        "identity": {"common_dir": "", "branch": "", "change": None, "key": ""},
        "item": None,
        "conflict": None,
        "prompt": None,
        "doctor": {"severity": severity, "name": "tracker-ledger", "message": message},
    }


class DoctorTrackerLedgerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base)

    def _project(
        self,
        *,
        recipe_enabled: bool = True,
        declaration: bool = False,
        init_git: bool = False,
    ) -> Path:
        root = self.base / "prj"
        root.mkdir()
        ai = root / "ai-specs"
        ai.mkdir()
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            "[recipes.trello-mcp-workflow]\n"
            f"enabled = {'true' if recipe_enabled else 'false'}\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        (root / "AGENTS.md").write_text("# agents\n")
        if declaration:
            openspec = root / "openspec"
            openspec.mkdir()
            (openspec / "config.yaml").write_text("tracking: trello\n")
        if init_git:
            subprocess.run(["git", "-C", str(root), "init", "-q"], check=True,
                           capture_output=True, text=True)
        _seed_clean_cache(root, self.home)
        return root

    def _write_witness(self, root: Path, payload: object) -> None:
        ledger_dir = root / ".git" / "ai-specs" / "ledger"
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "witness.json").write_text(json.dumps(payload))

    def _render(self, root: Path) -> tuple[list[tuple[str, str]], int]:
        result = invoke(root, "doctor", cli_home=self.home)
        return _named_checks(result.stdout, "tracker-ledger"), result.returncode

    # --- severity rendering (one grader: doctor renders the Go finding) ---

    def test_bound_healthy_renders_ok(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "OK")

    def test_unbound_renders_info_with_guidance(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="INFO", reason="unbound",
            message="no tracker recipe bound; enable one or ignore"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "INFO")
        self.assertIn("no tracker recipe bound", checks[0][1])
        self.assertIn("enable one tracker recipe or ignore", checks[0][1])
        # Frozen exit contract: INFO never affects the exit code.
        self.assertEqual(code, 0)

    def test_missing_witness_renders_warn(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="witness-missing",
            message="witness missing; run ai-specs sync"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("ai-specs sync", checks[0][1])
        # Frozen exit contract: WARN never affects the exit code.
        self.assertEqual(code, 0)

    def test_ambiguous_renders_warn_with_binding_guidance(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="ambiguous",
            message="ambiguous tracker binding"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn('[[bindings]] capability="tracker"', checks[0][1])
        self.assertEqual(code, 0)

    def test_declared_not_bound_renders_warn(self):
        root = self._project(recipe_enabled=False, declaration=True)
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="declared-not-bound",
            message="tracking is declared but no tracker recipe is bound"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("remove the tracking declaration", checks[0][1])
        self.assertEqual(code, 0)

    def test_conflict_renders_warn(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="conflict",
            message="conflict recorded; adjudicate at the next checkpoint"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("adjudicate", checks[0][1])
        self.assertEqual(code, 0)

    def test_infra_unevaluable_renders_error(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="ERROR", reason="store-corrupt",
            message="ledger store unreadable; run ai-specs sync"))
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "ERROR")
        # Frozen exit contract: exit 1 iff at least one ERROR was rendered.
        self.assertEqual(code, 1)

    # --- relevance gate: nothing to report when the ledger is not in play ---

    def test_irrelevant_project_is_silent(self):
        root = self._project(recipe_enabled=False)
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        self.assertEqual(checks, [])
        self.assertEqual(code, 0)

    def test_declaration_keeps_the_ledger_visible(self):
        root = self._project(recipe_enabled=False, declaration=True)
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="declared-not-bound",
            message="tracking is declared but no tracker recipe is bound"))
        checks, code = self._render(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(code, 0)

    def test_witness_keeps_the_ledger_visible(self):
        root = self._project(recipe_enabled=False, init_git=True)
        self._write_witness(root, {"v": 1, "capability": "tracker", "state": "bound"})
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        # The Go finding plus the unhosted work-start INFO (task 3.3).
        self.assertTrue(any(sev == "OK" for sev, _ in checks), checks)
        self.assertTrue(any("work-start is unhosted" in body for _, body in checks), checks)
        self.assertEqual(code, 0)

    # --- witness-derived recipe lookup and the unhosted work-start (3.1/3.3) ---

    def _project_with_fixture_recipe(self) -> Path:
        root = self.base / "fixture-prj"
        root.mkdir()
        ai = root / "ai-specs"
        ai.mkdir()
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = []\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = false\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
            "[recipes.fixture-tracker]\nenabled = true\n"
        )
        (root / "AGENTS.md").write_text("# agents\n")
        subprocess.run(["git", "-C", str(root), "init", "-q"], check=True,
                       capture_output=True, text=True)
        _seed_clean_cache(root, self.home)
        return root

    def _bound_fixture_witness(self, root: Path) -> None:
        self._write_witness(root, {
            "v": 1, "capability": "tracker", "state": "bound",
            "recipe_id": "fixture-tracker", "candidates": [],
        })

    def test_doctor_resolves_the_recipe_id_from_the_witness(self):
        # CLI-observable surface: with the witness bound to the custom recipe
        # (and the legacy recipe disabled), the ledger stays in play and the
        # Go finding renders under its witness-bound recipe id.
        root = self._project_with_fixture_recipe()
        self._bound_fixture_witness(root)
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        findings = [(sev, body) for sev, body in checks if "work-start is unhosted" not in body]
        unhosted = [body for _, body in checks if "work-start is unhosted" in body]
        self.assertEqual(len(findings), 1, checks)
        self.assertEqual(findings[0][0], "OK")
        self.assertEqual(len(unhosted), 1, checks)
        self.assertEqual(code, 0)

    def test_doctor_falls_back_to_the_legacy_recipe_id_without_a_witness(self):
        # CLI-observable surface: without a witness the legacy recipe id is the
        # relevance fallback. It is disabled in this manifest, so the ledger is
        # not in play and nothing renders — proving the fallback selected the
        # legacy id rather than any enabled recipe.
        root = self._project_with_fixture_recipe()
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        self.assertEqual(checks, [])
        self.assertEqual(code, 0)

    def test_bound_witness_without_plan_build_reports_unhosted_work_start(self):
        root = self._project_with_fixture_recipe()
        self._bound_fixture_witness(root)
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        unhosted = [body for _, body in checks if "work-start is unhosted" in body]
        self.assertEqual(len(unhosted), 1, checks)
        self.assertIn("enable plan-build-flow", unhosted[0])
        self.assertEqual(code, 0)

    def test_no_unhosted_info_when_plan_build_flow_is_enabled(self):
        root = self._project_with_fixture_recipe()
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text() + "[recipes.plan-build-flow]\nenabled = true\n")
        self._bound_fixture_witness(root)
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        checks, code = self._render(root)
        self.assertFalse(any("work-start is unhosted" in body for _, body in checks), checks)
        self.assertEqual(code, 0)

    def test_no_unhosted_info_without_a_bound_witness(self):
        root = self._project_with_fixture_recipe()
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="witness-missing",
            message="witness missing; run ai-specs sync"))
        checks, code = self._render(root)
        self.assertFalse(any("work-start is unhosted" in body for _, body in checks), checks)
        self.assertEqual(code, 0)

    def test_doctor_still_issues_no_writes(self):
        root = self._project_with_fixture_recipe()
        self._bound_fixture_witness(root)
        _stage_gate_stub(self.home, ledger_payload=verdict(severity="OK", message=""))
        before = snapshot(root)
        self._render(root)
        self.assertEqual(tree_diff(before, snapshot(root)),
                         {"created": [], "deleted": [], "modified": []})

    # --- infra fail-closed ---

    def test_missing_binary_renders_error(self):
        # CLI-observable equivalent of the WORKTREE_GATE_BIN pin to a missing
        # path: with no verified binary resolvable at all, the check fails
        # closed to ERROR.
        root = self._project()
        checks, code = self._render(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "ERROR")
        self.assertIn("failing open", checks[0][1])
        self.assertEqual(code, 1)

    def test_unparseable_verdict_renders_error(self):
        root = self._project()
        _stage_gate_stub(self.home, ledger_stdout="not json at all")
        checks, code = self._render(root)
        self.assertEqual(checks[0][0], "ERROR")
        self.assertIn("failing open", checks[0][1])
        self.assertEqual(code, 1)

    # --- legacy compatibility: doctor no longer grades the ## Tracker section ---

    def test_legacy_tracker_link_section_is_not_graded(self):
        root = self._project()
        for slug, body in (
            ("bad", "# proposal\n"),
            ("good", "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"),
        ):
            change = root / "openspec" / "changes" / slug
            change.mkdir(parents=True)
            (change / "proposal.md").write_text(body)
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="witness-missing",
            message="witness missing; run ai-specs sync"))
        checks, code = self._render(root)
        self.assertEqual(len(checks), 1)
        blob = " ".join(body for _, body in checks)
        self.assertNotIn("## Tracker", blob)
        self.assertNotIn("link section", blob)
        self.assertEqual(code, 0)

    def test_legacy_grader_is_gone(self):
        source = DOCTOR_PY.read_text(encoding="utf-8")
        self.assertNotIn("_check_tracker_card_link", source)
        self.assertNotIn("_load_trello_link", source)
        self.assertNotIn("is_valid_link", source)

    def test_doctor_is_read_only(self):
        root = self._project()
        (root / "openspec" / "changes" / "no-card").mkdir(parents=True)
        (root / "openspec" / "changes" / "no-card" / "proposal.md").write_text("# p\n")
        _stage_gate_stub(self.home, ledger_payload=verdict(
            severity="WARN", reason="witness-missing",
            message="witness missing; run ai-specs sync"))
        before = snapshot(root)
        self._render(root)
        self.assertEqual(tree_diff(before, snapshot(root)),
                         {"created": [], "deleted": [], "modified": []})

    # --- D15: no static dormancy line in the runtime brief / agent files ---

    def test_runtime_brief_sources_add_no_dormancy_line(self):
        sources = list((ROOT / "catalog" / "recipes").rglob("recipe.toml"))
        sources.append(ROOT / "lib" / "_internal" / "agents-render.py")
        for path in sources:
            with self.subTest(path=path.name):
                self.assertNotIn("tracker-ledger", path.read_text(encoding="utf-8"))
        agents = ROOT / "AGENTS.md"
        if agents.is_file():
            self.assertNotIn("tracker-ledger", agents.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()

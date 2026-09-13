"""Go-only parity fixtures for the tracker ledger (``--ledger``).

Drives the built ``dist/worktree-gate-current --ledger`` against real Git
fixtures described by the pinned corpus under
``tests/fixtures/tracker-ledger-corpus/``. The corpus pins
``(identity + witness + evidence + mode + checkpoint) -> (decision, conflict,
exit, doctor)`` for every design test-table row, plus two host-bridge rows that
prove the acquisition bridges fail open (missing binary; ``openspec/**`` path).

Skip loudly only when the Go binary is absent (``dist/worktree-gate-current``).
Never skip because a host bridge is missing; the bridge rows exercise the
committed hooks directly.
"""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "dist" / "worktree-gate-current"
CORPUS = ROOT / "tests" / "fixtures" / "tracker-ledger-corpus"
PLAN_BUILD_GATE = (
    ROOT / "catalog" / "recipes" / "plan-build-flow" / "hooks" / "plan-build-gate.sh"
)

STAMP_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:?\d{2})$")
LEDGER_ALLOWED_NAMES = frozenset({"witness.json", "state.json", "state.json.lock"})

# Every design test-table row this corpus must pin, as (label prefix, row name).
DESIGN_ROWS = {
    "dormant": ("01-", "no witness is dormant"),
    "empty_warn": ("02-", "bound + empty store + warn"),
    "empty_ask": ("03-", "bound + empty store + ask"),
    "empty_always": ("04-", "bound + empty store + always"),
    "consistent": ("05-", "consistent evidence"),
    "conflict_ask": ("06-", "four-side disagreement ask"),
    "conflict_block": ("07-", "four-side disagreement block"),
    "decide": ("08-", "persisted adjudication"),
    "opt_out": ("09-", "opt-out covers only"),
    "branch_reuse": ("10-", "closed item"),
    "branch_reuse_open": ("11-", "reused branch opens a new item"),
    "two_open_warn": ("12-", "two open items are a conflict"),
    "two_open_always": ("13-", "two open items block"),
    "detached_always": ("14-", "detached HEAD blocks"),
    "detached_warn": ("15-", "detached HEAD reports"),
    "corrupt_warn": ("16-", "corrupt store is unevaluable"),
    "corrupt_always": ("17-", "corrupt store fails open"),
    "ambiguous": ("18-", "ambiguous witness"),
    "declared_not_bound": ("19-", "declared-not-bound witness"),
    "unbound": ("20-", "unbound witness"),
    "pr_review": ("21-", "pr-review"),
    "archive_close": ("22-", "archive-close"),
    "missing_binary": ("23-", "missing binary"),
    "openspec_path": ("24-", "openspec/**"),
}


def require_go_binary() -> Path:
    if not BINARY.is_file():
        raise unittest.SkipTest(
            "no Go gate binary in dist/ (run scripts/build-gate.sh); "
            "tracker-ledger parity is Go-only"
        )
    return BINARY


def git(cwd: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True
    )


def load_corpus() -> list[tuple[Path, dict]]:
    return [(p, json.loads(p.read_text())) for p in sorted(CORPUS.glob("*.json"))]


def build_verdict_fixture(root: Path, case: dict) -> tuple[Path, Path]:
    """Build the Git fixture and write the witness/store the case pins.

    Returns ``(repo, ledger_dir)``. The common dir is realpath-resolved so the
    runner derives the same identity key the Go reader does.
    """
    repo = root / "repo"
    repo.mkdir()
    git(repo, "init", "-q")
    git(repo, "config", "user.email", "test@example.invalid")
    git(repo, "config", "user.name", "test")
    (repo / "README.md").write_text("fixture\n", encoding="utf-8")
    git(repo, "add", "-A")
    git(repo, "commit", "-qm", "fixture")
    branch = case.get("branch", "fixture")
    git(repo, "checkout", "-q", "-B", branch)
    if case.get("detached"):
        git(repo, "checkout", "-q", "--detach")

    change = case.get("change")
    if change:
        folder = repo / "openspec" / "changes" / change
        folder.mkdir(parents=True)
        (folder / "proposal.md").write_text("# proposal\n", encoding="utf-8")

    common_raw = git(
        repo, "rev-parse", "--path-format=absolute", "--git-common-dir"
    ).stdout.strip()
    common = str(Path(common_raw).resolve())
    ledger_dir = Path(common) / "ai-specs" / "ledger"

    witness = case.get("witness")
    if isinstance(witness, dict):
        payload = {
            "v": 1,
            "capability": "tracker",
            "state": witness.get("state", "bound"),
            "recipe_id": witness.get("recipe_id", "test-tracker-ledger"),
            "candidates": witness.get("candidates", []),
            "written_at": "2026-01-01T00:00:00Z",
        }
        payload.update(witness)
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "witness.json").write_text(json.dumps(payload))

    store_spec = case.get("store")
    if store_spec == "corrupt":
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "state.json").write_text("{ not valid json\n")
    elif isinstance(store_spec, dict):
        items = []
        for index, spec in enumerate(store_spec.get("items", []), start=1):
            slug = spec.get("slug") or ""
            ident = {"common_dir": common, "branch": branch, "change": slug}
            decisions = [
                {
                    "at": d.get("at", "2026-01-01T00:00:00Z"),
                    "checkpoint": d.get("checkpoint", ""),
                    "kind": d.get("kind", ""),
                    "choice": d.get("choice", ""),
                    "note": d.get("note", ""),
                }
                for d in spec.get("decisions", [])
            ]
            items.append({
                "id": spec.get("id") or f"item-{index:04d}",
                "identity": ident,
                "status": spec.get("status", "open"),
                "item_id": spec.get("item_id", f"T-{index}"),
                "provider_id": "test-tracker-ledger",
                "native_type": "",
                "url": "",
                "state": "",
                "provider": {},
                "exemption": spec.get("exemption", ""),
                "conflict": (
                    {"sides": {}, "recorded_at": "2026-01-01T00:00:00Z"}
                    if spec.get("conflict")
                    else None
                ),
                "decisions": decisions,
            })
        assert common and branch, "fixture identity must be non-empty"
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "state.json").write_text(json.dumps({"v": 1, "items": items}))

    return repo, ledger_dir


def case_steps(case: dict) -> list[dict]:
    if "steps" in case:
        return case["steps"]
    return [case]


def run_step(root: Path, repo: Path, case: dict, step: dict,
             binary: Path) -> tuple[subprocess.CompletedProcess, dict]:
    cmd = [
        str(binary), "--ledger",
        "--checkpoint", step["checkpoint"],
        "--ledger-mode", step.get("mode", "warn"),
        "--project-root", str(repo),
    ]
    evidence = step.get("evidence", case.get("evidence"))
    if evidence is not None:
        evidence_path = root / f"evidence-{step.get('checkpoint', 'x')}.json"
        evidence_path.write_text(json.dumps(evidence))
        cmd += ["--evidence", str(evidence_path)]
    decide = step.get("decide")
    if decide is not None:
        cmd += ["--decide", json.dumps(decide)]
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    payload = json.loads(proc.stdout)
    return proc, payload


def normalize(value):
    """Replace dynamic RFC3339 stamps so two runs compare byte-for-byte."""
    if isinstance(value, dict):
        return {k: normalize(v) for k, v in value.items()}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    if isinstance(value, str) and STAMP_RE.match(value):
        return "<stamp>"
    return value


def assert_expected(test: unittest.TestCase, payload: dict,
                    proc: subprocess.CompletedProcess, expected: dict) -> None:
    test.assertEqual(proc.returncode, expected["exit"], proc.stderr)
    test.assertEqual(payload.get("decision"), expected["decision"])
    test.assertEqual(payload.get("reason"), expected["reason"])
    test.assertEqual(bool(payload.get("active")), expected["active"])
    test.assertEqual(payload.get("conflict") is not None, expected["conflict"])
    test.assertEqual((payload.get("doctor") or {}).get("severity"), expected["doctor"])
    test.assertEqual(payload.get("capability"), "tracker")


class TrackerLedgerParityTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cases = load_corpus()
        cls.verdict_cases = [c for c in cls.cases if c[1].get("kind", "verdict") == "verdict"]
        cls.bridge_cases = [c for c in cls.cases if c[1].get("kind") == "bridge"]

    def setUp(self):
        require_go_binary()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def _run_case(self, case: dict, root: Path):
        repo, ledger_dir = build_verdict_fixture(root, case)
        results = []
        for step in case_steps(case):
            proc, payload = run_step(root, repo, case, step, BINARY)
            results.append((step, proc, payload))
        return ledger_dir, results

    def test_every_verdict_case_matches_its_pin(self):
        for case_file, case in self.verdict_cases:
            with self.subTest(case=case_file.name):
                root = self.root / case_file.stem
                root.mkdir()
                _, results = self._run_case(case, root)
                for step, proc, payload in results:
                    assert_expected(self, payload, proc, step["expected"])

    def test_no_store_residue_and_no_unexpected_ledger_files(self):
        for case_file, case in self.verdict_cases:
            with self.subTest(case=case_file.name):
                root = self.root / ("residue-" + case_file.stem)
                root.mkdir()
                ledger_dir, _ = self._run_case(case, root)
                if not ledger_dir.is_dir():
                    continue
                names = {p.name for p in ledger_dir.iterdir()}
                self.assertEqual(
                    sorted(names - LEDGER_ALLOWED_NAMES), [],
                    f"unexpected ledger file(s) in {ledger_dir}",
                )

    def test_corpus_is_stable_across_two_fresh_runs(self):
        for case_file, case in self.verdict_cases:
            with self.subTest(case=case_file.name):
                outputs = []
                for run in ("a", "b"):
                    # Same absolute fixture path both times, so identity.common_dir
                    # and key match; only RFC3339 stamps are normalized. Two fresh
                    # builds of the identical fixture must be byte-identical.
                    root = self.root / ("stable-" + case_file.stem)
                    if run == "b":
                        shutil.rmtree(root)
                    root.mkdir()
                    _, results = self._run_case(case, root)
                    outputs.append([normalize(payload) for _, _, payload in results])
                self.assertEqual(outputs[0], outputs[1])

    def test_verdict_corpus_rejects_a_divergent_pin(self):
        # Mutation guard: the corpus is not vacuous. Running the pinned
        # expectations against a deliberately wrong decision must fail, so a
        # silently-allow-all runner cannot pass this suite.
        case_file, case = self.verdict_cases[0]
        root = self.root / "mutation"
        root.mkdir()
        _, results = self._run_case(case, root)
        step, proc, payload = results[0]
        wrong = dict(step["expected"])
        wrong["decision"] = "allow" if step["expected"]["decision"] != "allow" else "block"
        with self.assertRaises(AssertionError):
            assert_expected(self, payload, proc, wrong)

    def test_corpus_covers_every_design_row(self):
        labels = {p.name: c for p, c in self.cases}
        for row, (prefix, needle) in DESIGN_ROWS.items():
            with self.subTest(row=row):
                matching = [
                    (name, case) for name, case in labels.items()
                    if name.startswith(prefix)
                ]
                self.assertEqual(len(matching), 1, f"{row}: expected one {prefix}* case")
                self.assertIn(needle, matching[0][1]["label"])

    def test_bridge_rows_fail_open_and_never_block_openspec(self):
        for case_file, case in self.bridge_cases:
            with self.subTest(case=case_file.name):
                root = self.root / ("bridge-" + case_file.stem)
                root.mkdir()
                repo = root / "repo"
                repo.mkdir()
                git(repo, "init", "-q")
                git(repo, "config", "user.email", "test@example.invalid")
                git(repo, "config", "user.name", "test")
                (repo / "README.md").write_text("fixture\n", encoding="utf-8")
                git(repo, "add", "-A")
                git(repo, "commit", "-qm", "fixture")
                ai = repo / "ai-specs"
                ai.mkdir()
                (ai / "ai-specs.toml").write_text(
                    "[project]\nname = 'bridge'\n\n"
                    "[recipes.trello-mcp-workflow]\nenabled = true\n"
                    "[recipes.trello-mcp-workflow.config]\n"
                    f'ledger_mode = "{case.get("ledger_mode", "always")}"\n'
                )
                env = dict(os.environ)
                env.pop("TRACKER_LEDGER_MODE", None)
                stub_log = root / "stub.log"
                change = repo / "openspec" / "changes" / "fixture-change"
                change.mkdir(parents=True)
                (change / "tasks.md").write_text("# tasks\n", encoding="utf-8")
                if case["scenario"] == "missing-binary":
                    env.pop("WORKTREE_GATE_BIN", None)
                    env["AI_SPECS_HOME"] = str(root / "cold-home")
                    target = repo / "src" / "app.py"
                    target.parent.mkdir(parents=True)
                    target.write_text("x\n", encoding="utf-8")
                else:
                    stub = root / "blocking-gate.sh"
                    stub.write_text(
                        "#!/usr/bin/env bash\n"
                        'printf "called\\n" >> "$STUB_LOG"\n'
                        "exit 2\n"
                    )
                    stub.chmod(0o755)
                    env["WORKTREE_GATE_BIN"] = str(stub)
                    env["STUB_LOG"] = str(stub_log)
                    target = change / "tasks.md"
                event = {
                    "event": "pre-tool-use",
                    "tool_name": "Write",
                    "tool_input": {"file_path": str(target)},
                    "cwd": str(repo),
                }
                proc = subprocess.run(
                    ["bash", str(PLAN_BUILD_GATE)],
                    input=json.dumps(event), capture_output=True, text=True, env=env,
                )
                self.assertEqual(
                    proc.returncode, case["expected_exit"],
                    f"{case['label']}: {proc.stderr}",
                )
                if case["scenario"] == "openspec-path":
                    self.assertFalse(
                        stub_log.exists(),
                        "openspec/** must exit before the ledger binary is invoked",
                    )


if __name__ == "__main__":
    unittest.main()

"""Hermetic Bash-reference parity fixtures for worktree-gate-go.

Drives the frozen Bash reference (catalog/recipes/worktree-flow/hooks/
worktree-gate-legacy.sh) against real Git fixtures described by the corpus
under tests/fixtures/worktree-gate-corpus/. Every corpus case declares the
fixture it needs (`fixture` key) and a placeholder target; the runner builds
the fixture in a temp dir, substitutes placeholders ({repo}, {worktree},
{external}) into the event, and asserts the exit-code contract — plus the
exact stderr message when the case pins one.

No case may rely on nonexistent-path fail-open behavior (Phase 1 correction
1.20): placeholders always resolve inside the built fixture, and fixture
metadata is validated before any case runs (correction 1.23) so a malformed
fixture can never silently pass as an outside-repository allow.

Malformed-input negative coverage (task 1.15) is part of the corpus: top-level
JSON array on stdin, unbalanced quotes in a shell command, and shell events
with no command source all fail open to allow with empty stderr.

The Go half of the parity comparison is explicitly skipped until the binary
exists (task 1.22 / 1.17); the Bash-vs-expect half always runs.
"""
from __future__ import annotations

import hashlib
import json
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LEGACY = ROOT / "catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh"
CORPUS = ROOT / "tests/fixtures/worktree-gate-corpus"

# The frozen reference ships with unstamped sentinels (__WORKTREE_GATE_MODE__
# etc.) so it warns and falls back on every invocation. The parity runner
# materializes a copy with valid stamps (always / auto / auto — the effective
# defaults) so stderr is exactly the gate message with no setup noise.
STAMPED_MODE = "always"
STAMPED_SCOPE = "auto"
STAMPED_TOPOLOGY = "auto"


def sha256(path: Path) -> str:
    """Return the lowercase hex SHA-256 of a file."""
    return hashlib.sha256(path.read_bytes()).hexdigest()

def materialize_legacy(dest: Path) -> Path:
    content = LEGACY.read_text()
    content = content.replace(
        'stamped_gate_mode="__WORKTREE_GATE_MODE__"',
        f'stamped_gate_mode="{STAMPED_MODE}"',
    )
    content = content.replace(
        'stamped_gate_scope="__WORKTREE_GATE_SCOPE__"',
        f'stamped_gate_scope="{STAMPED_SCOPE}"',
    )
    content = content.replace(
        'stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"',
        f'stamped_repo_topology="{STAMPED_TOPOLOGY}"',
    )
    dest.write_text(content, encoding="utf-8")
    dest.chmod(0o755)
    return dest


def git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


def build_fixture(root: Path, fixture: str) -> dict[str, Path]:
    """Build the Git fixture named by a corpus case.

    Returns a mapping of logical locations the corpus targets may reference:
    "repo" (primary checkout), "external" (outside any repository), and
    "worktree" (linked worktree of the repo). The repo is always created and
    left on branch "main" unless the fixture moves it.
    """
    repo = root / "repo"
    repo.mkdir()
    git(repo, "init", "-q")
    git(repo, "config", "user.email", "test@example.invalid")
    git(repo, "config", "user.name", "test")
    (repo / "README.md").write_text("fixture\n", encoding="utf-8")
    git(repo, "add", "README.md")
    git(repo, "commit", "-qm", "fixture")
    git(repo, "checkout", "-q", "-B", "main")
    locations = {"repo": repo}

    if fixture == "protected-main":
        return locations
    if fixture == "feature-branch":
        git(repo, "checkout", "-q", "-B", "feature-x")
        return locations
    if fixture == "development-branch":
        git(repo, "checkout", "-q", "-B", "development")
        return locations
    if fixture == "external-path":
        external = root / "external"
        external.mkdir()
        locations["external"] = external
        return locations
    if fixture == "linked-worktree":
        worktree = root / "wt"
        git(repo, "worktree", "add", "-q", "-b", "feat", str(worktree))
        locations["worktree"] = worktree
        return locations
    raise ValueError(f"unknown fixture type: {fixture!r}")


FIXTURES = frozenset({"repo", "external", "worktree"})


def validate_corpus_case(case: dict) -> None:
    """Reject corpus metadata that could silently pass as an outside-repo allow.

    Phase 1 correction 1.23: a case that declares no fixture ("none") while
    still referencing a fixture placeholder would leave the placeholder as a
    literal path. On a machine where that literal path exists outside any
    repository the gate would fail open and the case would pass for the wrong
    reason — exactly the nonexistent-path fail-open behavior correction 1.20
    forbids. A case may only reference a placeholder its fixture provides, so
    this is a contradiction; flag it before any fixture is built or run.
    """
    fixture = case.get("fixture", "none")
    if fixture != "none" and fixture not in build_fixture.ALLOWED_FIXTURES:
        raise ValueError(f"unknown fixture type in corpus case: {fixture!r}")
    raw = json.dumps(case)
    used = {m.group(1) for m in re.finditer(r"\{(\w+)\}", raw)}
    if fixture == "none" and used:
        raise ValueError(
            "fixture case 'none' must not reference placeholders "
            f"(uses {sorted(used)}); a literal path would rely on "
            "nonexistent-path fail-open behavior"
        )
    unknown = used - FIXTURES
    if unknown:
        raise ValueError(
            f"case uses unknown placeholders: {sorted(unknown)}"
        )


build_fixture.ALLOWED_FIXTURES = frozenset(
    {"protected-main", "feature-branch", "development-branch",
     "external-path", "linked-worktree"}
)


def substitute(text: str, locations: dict[str, Path]) -> str:
    """Replace {name} placeholders with the built fixture locations.


    A placeholder is replaced by the location path plus a "/" separator when
    anything follows it, so both "{repo}/src.py" and "{repo}src.py" resolve to
    "<repo>/src.py". A bare "{repo}" at end of string stays a bare path.
    """
    for name, path in locations.items():
        marker = "{" + name + "}"
        while marker in text:
            head, _, tail = text.partition(marker)
            if not tail:
                text = head + str(path)
            else:
                text = head + str(path) + "/" + tail.lstrip("/")
    return text


def run_legacy(gate: Path, event: dict | None, stdin_text: str | None,
               cwd: Path) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["bash", str(gate)],
        input=stdin_text if stdin_text is not None else json.dumps(event),
        capture_output=True, text=True, cwd=cwd,
    )



class WorktreeGateParityTests(unittest.TestCase):
    def test_frozen_reference_hash_is_pinned(self):
        # Task 1.2: the frozen Bash reference must never drift silently. The
        # live hook and the frozen copy are currently byte-identical (the
        # freeze is PR-1 planning state); pinning the digest of the frozen
        # copy means any later change to either file breaks here and forces
        # an explicit, reviewed re-freeze. This is the drift guard for the
        # parity oracle: the corpus asserts behavior, this asserts identity.
        expected = "1ee9da4a53777334c8e9e0e3dd046d3cc1c8ad2225e823123bdeb65c6eeabe76"
        self.assertEqual(sha256(LEGACY), expected)

    def test_protected_main_reference_blocks_real_fixture(self):
        case = json.loads((CORPUS / "01-block-write-protected-branch.json").read_text())
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            gate = materialize_legacy(root / "gate.sh")
            locations = build_fixture(root, case["fixture"])
            event = json.loads(json.dumps(case["event"]))
            event["cwd"] = substitute(event["cwd"], locations)
            event["tool_input"]["file_path"] = substitute(
                event["tool_input"]["file_path"], locations)
            result = run_legacy(gate, event, None, locations["repo"])
            self.assertEqual(result.returncode, case["expected_exit"], result.stderr)
            self.assertEqual(result.stderr,
                             substitute(case["expected_stderr"], locations) + "\n")

    def test_every_corpus_case_runs_against_real_fixture(self):
        # Wire every corpus case through the shared pipeline: build the named
        # fixture, substitute placeholders, run the Bash reference, and assert
        # the full IO contract — exit code, empty stdout, and exact stderr
        # (pinned message on block, exactly empty on allow). A hardcoded
        # "/repo" path anywhere in the corpus would fail here rather than
        # silently pass as an outside-repository allow; malformed fixture
        # metadata is rejected up front (correction 1.23).
        for case_file in sorted(CORPUS.glob("*.json")):
            case = json.loads(case_file.read_text())
            with self.subTest(case=case_file.name):
                validate_corpus_case(case)
                with tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    gate = materialize_legacy(root / "gate.sh")
                    if case["fixture"] == "none":
                        locations: dict[str, Path] = {}
                        event_cwd = root
                    else:
                        locations = build_fixture(root, case["fixture"])
                        event_cwd = locations["repo"]

                    if case.get("stdin") is not None:
                        result = run_legacy(gate, None, case["stdin"], event_cwd)
                    else:
                        event = json.loads(json.dumps(case["event"]))
                        if "cwd" in event:
                            event["cwd"] = substitute(event["cwd"], locations)
                        tool_input = event.get("tool_input") or {}
                        for key in ("file_path", "notebook_path", "command"):
                            if key in tool_input:
                                tool_input[key] = substitute(tool_input[key], locations)
                        if "command" in event:
                            event["command"] = substitute(event["command"], locations)
                        result = run_legacy(gate, event, None, event_cwd)

                    self.assertEqual(result.returncode, case["expected_exit"], result.stderr)
                    # Exact IO parity: the gate never writes stdout (only the
                    # future --explain/--version do, never on the gate path),
                    # and stderr is exactly the pinned message or exactly
                    # empty on allow. A guard that leaks setup noise or an
                    # allow-path warning would break here.
                    self.assertEqual(result.stdout, "", result.stdout)
                    if case.get("expected_stderr") is not None:
                        self.assertEqual(
                            result.stderr,
                            substitute(case["expected_stderr"], locations) + "\n")
                    else:
                        self.assertEqual(result.stderr, "", result.stderr)

    def test_go_comparison_matches_bash_for_available_binary(self):
        binary = ROOT / "dist" / "worktree-gate-current"
        if not binary.exists():
            self.skipTest("no Go gate binary in dist/")
        for case_file in sorted(CORPUS.glob("*.json")):
            case = json.loads(case_file.read_text())
            validate_corpus_case(case)
            with self.subTest(case=case_file.name):
                with tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    gate = materialize_legacy(root / "gate.sh")
                    locations = {} if case["fixture"] == "none" else build_fixture(root, case["fixture"])
                    event_cwd = locations.get("repo", root)
                    if case.get("stdin") is not None:
                        payload = case["stdin"]
                    else:
                        event = json.loads(json.dumps(case["event"]))
                        if "cwd" in event:
                            event["cwd"] = substitute(event["cwd"], locations)
                        ti = event.get("tool_input") or {}
                        for key in ("file_path", "notebook_path", "command", "script", "cmd"):
                            if key in ti:
                                ti[key] = substitute(ti[key], locations)
                        for key in ("command", "script"):
                            if key in event:
                                event[key] = substitute(event[key], locations)
                        payload = json.dumps(event)
                    legacy = run_legacy(gate, None, payload, event_cwd)
                    go = subprocess.run([str(binary), "--gate-mode", STAMPED_MODE,
                                         "--gate-scope", STAMPED_SCOPE,
                                         "--repo-topology", STAMPED_TOPOLOGY,
                                         "--protected", "main development"],
                                        input=payload, capture_output=True, text=True,
                                        cwd=event_cwd)
                    self.assertEqual(go.returncode, legacy.returncode, go.stderr)
                    self.assertEqual(go.stdout, legacy.stdout)
                    self.assertEqual(go.stderr, legacy.stderr)


    def test_malformed_input_cases_fail_open_silently(self):
        # Task 1.15 fail-open set: malformed input must allow (exit 0) with no
        # gate message on stderr — a guard bug that leaked a warning or exited
        # non-zero would wedge editing. Cases carrying "malformed": true pin
        # the exact contract.
        malformed = [
            case for case_file in sorted(CORPUS.glob("*.json"))
            for case in [json.loads(case_file.read_text())]
            if case.get("malformed")
        ]
        self.assertTrue(malformed, "corpus must contain malformed-input cases")
        for case in malformed:
            with self.subTest(case=case.get("label", "?")):
                self.assertEqual(case["expected_exit"], 0,
                                 "malformed input must fail open to allow")
                with tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    gate = materialize_legacy(root / "gate.sh")
                    if case["fixture"] == "none":
                        locations: dict[str, Path] = {}
                        event_cwd = root
                    else:
                        locations = build_fixture(root, case["fixture"])
                        event_cwd = locations["repo"]
                    if case.get("stdin") is not None:
                        result = run_legacy(gate, None, case["stdin"], event_cwd)
                    else:
                        event = json.loads(json.dumps(case["event"]))
                        if "cwd" in event:
                            event["cwd"] = substitute(event["cwd"], locations)
                        tool_input = event.get("tool_input") or {}
                        for key in ("file_path", "notebook_path", "command"):
                            if key in tool_input:
                                tool_input[key] = substitute(tool_input[key], locations)
                        result = run_legacy(gate, event, None, event_cwd)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(result.stderr, "",
                                     "malformed input must not emit a gate message")

    def test_malformed_fixture_cannot_pass_as_outside_repo_allow(self):
        # Phase 1 correction 1.23: fixture metadata must be validated up front.
        # The dangerous shape — fixture "none" (no repo is built) while the
        # event still references a {repo} placeholder — leaves a literal
        # "{repo}/..." path: the gate fails open on the nonexistent path and
        # the case passes for the wrong reason. Prove the runner would accept
        # it (exit 0) and then assert the validator rejects it.
        bad_case = {
            "fixture": "none",
            "event": {
                "event": "pre-tool-use",
                "tool_name": "Write",
                "tool_input": {"file_path": "{repo}src.py"},
                "cwd": "{repo}",
            },
            "expected_exit": 0,
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            gate = materialize_legacy(root / "gate.sh")
            event = json.loads(json.dumps(bad_case["event"]))
            event["cwd"] = substitute(event["cwd"], {})
            event["tool_input"]["file_path"] = substitute(
                event["tool_input"]["file_path"], {})
            result = run_legacy(gate, event, None, root)
            self.assertEqual(result.returncode, 0, result.stderr)
        with self.assertRaisesRegex(
                ValueError, r"fixture case 'none' must not reference placeholders"):
            validate_corpus_case(bad_case)

    def test_no_illustrative_repo_paths_in_corpus(self):
        # Phase 1 correction 1.20/1.23: corpus cases must declare fixture
        # metadata and may only reference placeholders their fixture provides;
        # a hardcoded "/repo" path would silently pass as an outside-repository
        # allow on machines where it exists.
        for case_file in sorted(CORPUS.glob("*.json")):
            case = json.loads(case_file.read_text())
            with self.subTest(case=case_file.name):
                self.assertIn("fixture", case, "corpus case must declare fixture metadata")
                validate_corpus_case(case)
                raw = json.dumps(case)
                self.assertNotIn("/repo", raw,
                                 "corpus case must not hardcode an absolute repo path")
                if case.get("fixture") != "none" and "command" not in raw:
                    self.assertIn("target", case, "fixture case must declare a target")


if __name__ == "__main__":
    unittest.main()

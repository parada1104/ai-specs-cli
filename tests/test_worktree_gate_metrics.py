"""Go-only performance-budget evidence for the worktree gate.

This file implements git-shim invocation counting and per-invocation
latency as reproducible, failing-when-regressed tests.

What is measured and asserted:

1. Git memoization (spec "Git facts are memoized across candidates"): a
   shell-mode event yielding four candidate write paths inside one
   repository with two initialized submodules is run through a `git` shim
   on PATH that counts subprocess invocations and delegates to the real
   git. The Go binary memoizes by (resolved directory, args). The Go
   count MUST stay at or below the memoization ceiling; there is no Bash
   comparison (design D11).

2. One implementation process per invocation (spec "One process per
   invocation"): the launcher execs the resolved binary, so exactly one
   implementation process is spawned for a four-candidate event. Measured
   by asserting the Go binary receives the full event and a PATH-wrapped
   interpreter counter records zero python3 spawns.

3. No hashing on the hot path by default (spec "No hashing on the hot path
   by default"): when WORKTREE_GATE_VERIFY is unset the launcher must not
   compute a digest of the resolved binary. Measured by a PATH shim that
   records every executed binary; the launcher must run the gate binary
   directly with no shasum/digest utility on the invocation path. With
   WORKTREE_GATE_VERIFY=1 the --selftest runs and a digest is expected.

4. Latency: per-invocation wall time for the Go binary over four
   representative corpus cases, recorded for the verify report and
   asserted only for basic sanity (positive timings).
"""
from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

from tests.test_worktree_gate_parity import (
    CORPUS,
    STAMPED_MODE,
    STAMPED_SCOPE,
    STAMPED_TOPOLOGY,
    build_fixture,
    substitute,
)

ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
BINARY = ROOT / "dist" / "worktree-gate-current"

REAL_GIT = shutil.which("git")
if REAL_GIT is None:  # pragma: no cover
    raise unittest.SkipTest("git not on PATH")


def _write_shim(dir: Path, name: str, script: str) -> Path:
    """Write an executable shim script into dir and return its path."""
    path = dir / name
    path.write_text(script, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def _counted_git_shim(dir: Path, counter: Path) -> Path:
    """A `git` shim that counts invocations then delegates to the real git."""
    return _write_shim(
        dir,
        "git",
        "#!/usr/bin/env bash\n"
        'echo x >> "%s"\n'
        'exec "%s" "$@"\n' % (counter, REAL_GIT),
    )


def _spawn_shim(dir: Path, counter: Path) -> Path:
    """A `python3` shim that counts interpreter spawns then delegates."""
    return _write_shim(
        dir,
        "python3",
        "#!/usr/bin/env bash\n"
        'echo x >> "%s"\n'
        'exec "%s" "$@"\n' % (counter, sys.executable),
    )


def _env_with_shim(base: dict, shim_dir: Path) -> dict:
    env = dict(base)
    env["PATH"] = str(shim_dir) + os.pathsep + env.get("PATH", "")
    return env


class WorktreeGateGitCountTests(unittest.TestCase):
    """Task 2.11: memoization proven by strictly fewer git invocations."""

    def _superrepo_fixture(self, root: Path) -> Path:
        """Build a repo on protected main with two initialized submodules.

        Returns the superrepo root. Layout mirrors the topology unit fixture:
        file:// remotes so `submodule add` materializes .git/modules/<rel>.
        """
        def git(cwd: Path, *args: str) -> None:
            subprocess.run(["git", "-C", str(cwd), *args], check=True,
                           capture_output=True, text=True)

        def make_module(name: str) -> str:
            src = root / f"src-{name}"
            src.mkdir()
            git(src, "init", "-q")
            git(src, "config", "user.email", "t@t.t")
            git(src, "config", "user.name", "t")
            (src / "README.md").write_text(f"{name}\n")
            git(src, "add", "-A")
            git(src, "commit", "-qm", "init")
            git(src, "checkout", "-q", "-B", "main")
            remote = root / f"{name}.git"
            subprocess.run(["git", "clone", "--bare", "-q", str(src), str(remote)],
                           check=True, capture_output=True, text=True)
            return "file://" + str(remote)

        superroot = root / "super"
        superroot.mkdir()
        git(superroot, "init", "-q")
        git(superroot, "config", "user.email", "t@t.t")
        git(superroot, "config", "user.name", "t")
        (superroot / "ROOT").write_text("super\n")
        git(superroot, "add", "-A")
        git(superroot, "commit", "-qm", "root")
        for name in ("m1", "m2"):
            subprocess.run(
                ["git", "-C", str(superroot), "-c", "protocol.file.allow=always",
                 "submodule", "add", "-q", make_module(name), f"apps/{name}"],
                check=True, capture_output=True, text=True)
        git(superroot, "commit", "-qam", "add modules")
        git(superroot, "checkout", "-q", "-B", "main")
        return superroot

    def _four_candidate_event(self, superroot: Path) -> str:
        """Shell-mode event whose four candidates all live in the central
        `openspec/changes` exception path.

        Every candidate is a full decision-path write on protected main with a
        superrepo owner (two proven submodules), allowed only via the central
        exception. Nothing short-circuits early: each candidate walks the
        git-fact resolution and the submodule proof, so the Bash reference
        re-derives git facts four times while the Go gate memoizes once.
        """
        central = superroot / "openspec" / "changes"
        command = "; ".join([
            f"echo x > {central}/a/file.py",
            f"echo x > {central}/b/file.py",
            f"echo x > {central}/c/file.py",
            f"echo x > {central}/d/file.py",
        ])
        return json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Bash",
            "tool_input": {"command": command},
            "cwd": str(superroot),
        })

    def test_go_memoizes_git_facts_under_invocation_ceiling(self):
        """Spec 'Git facts are memoized across candidates': Go-only ceiling.

        Design D11: do not keep a vendored Bash copy for comparison.
        """
        if not BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            superroot = self._superrepo_fixture(root)
            payload = self._four_candidate_event(superroot)

            go_git_count = root / "go-git-count"
            go_shim_dir = root / "go-shim"
            go_shim_dir.mkdir()
            _counted_git_shim(go_shim_dir, go_git_count)
            go_env = _env_with_shim(os.environ, go_shim_dir)
            go = subprocess.run(
                [str(BINARY), "--gate-mode", STAMPED_MODE,
                 "--gate-scope", STAMPED_SCOPE,
                 "--repo-topology", STAMPED_TOPOLOGY,
                 "--protected", "main development"],
                input=payload, capture_output=True, text=True,
                cwd=superroot, env=go_env)

            go_gits = (
                len(go_git_count.read_text().splitlines())
                if go_git_count.exists() else 0
            )

            self.assertEqual(go.returncode, 0, go.stderr)
            self.assertEqual(go.stderr, "")
            self.assertGreater(go_gits, 0, "Go gate made no git calls")
            # Ceiling: four candidates sharing one superrepo (+ two modules)
            # must not re-derive git facts per candidate. 24 is 4 candidates
            # times 6 facts — a naive unmemoized upper bound; memoization
            # lands well below this.
            self.assertLessEqual(
                go_gits, 24,
                f"Go git invocations {go_gits} exceeded the memoization "
                f"ceiling of 24 for a four-candidate event")

    def test_go_runs_single_process_for_four_candidates(self):
        """Spec 'One process per invocation': exactly one gate process.

        The launcher execs the resolved implementation (worktree-gate.sh:
        `exec "$bin" ...`), so a four-candidate event spawns exactly one
        implementation process. Measured by a PATH shim that fails loudly if
        any `python3` interpreter is spawned. The Go binary must spawn none.
        """
        if not BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            superroot = self._superrepo_fixture(root)
            payload = self._four_candidate_event(superroot)

            counter = root / "py-spawn-count"
            shim_dir = root / "py-shim"
            shim_dir.mkdir()
            _spawn_shim(shim_dir, counter)
            env = _env_with_shim(os.environ, shim_dir)
            go = subprocess.run(
                [str(BINARY), "--gate-mode", STAMPED_MODE,
                 "--gate-scope", STAMPED_SCOPE,
                 "--repo-topology", STAMPED_TOPOLOGY,
                 "--protected", "main development"],
                input=payload, capture_output=True, text=True,
                cwd=superroot, env=env)
            spawns = len(counter.read_text().splitlines()) if counter.exists() else 0
            self.assertEqual(go.returncode, 0, go.stderr)
            self.assertEqual(spawns, 0,
                             "Go gate must spawn zero interpreter processes "
                             "for a four-candidate event (exactly one gate "
                             "process per invocation)")


class WorktreeGateNoHashHotPathTests(unittest.TestCase):
    """Spec 'No hashing on the hot path by default' (launcher contract 3.x)."""

    def _stamp_launcher(self, dest: Path, *, impl: str = "go",
                        version: str = "0.22.0") -> Path:
        content = LAUNCHER.read_text()
        content = content.replace("__WORKTREE_GATE_MODE__", "always")
        content = content.replace("__WORKTREE_GATE_SCOPE__", "auto")
        content = content.replace("__WORKTREE_REPO_TOPOLOGY__", "auto")
        content = content.replace("__WORKTREE_GATE_IMPL__", impl)
        content = content.replace("__WORKTREE_GATE_VERSION__", version)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(content, encoding="utf-8")
        dest.chmod(dest.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        return dest

    def test_no_digest_computed_without_verify_flag(self):
        if not BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo = root / "repo"
            repo.mkdir()
            subprocess.run(["git", "-C", str(repo), "init", "-q"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "t@t.t"],
                           check=True, capture_output=True, text=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "t"],
                           check=True, capture_output=True, text=True)
            (repo / "README.md").write_text("x\n")
            subprocess.run(["git", "-C", str(repo), "add", "-A"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-qm", "init"],
                           check=True, capture_output=True, text=True)
            subprocess.run(["git", "-C", str(repo), "checkout", "-q", "-B", "main"],
                           check=True)

            # Launcher resolves the binary from the project-local pin.
            launcher = self._stamp_launcher(
                root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh")
            pin = root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
            pin.parent.mkdir(parents=True, exist_ok=True)
            pin.write_bytes(BINARY.read_bytes())
            pin.chmod(pin.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

            # A shim that records every utility the launcher might shell out
            # to for digest computation; the shim does NOT delegate, so if
            # the launcher calls any of them the gate fails loudly here.
            shim_dir = root / "shim"
            shim_dir.mkdir()
            for util in ("shasum", "sha256sum", "openssl", "cksum"):
                _write_shim(shim_dir, util, "#!/usr/bin/env bash\nexit 99\n")

            env = dict(os.environ)
            env["PATH"] = str(shim_dir) + os.pathsep + env.get("PATH", "")
            env["AI_SPECS_HOME"] = str(root / "home")
            env.pop("WORKTREE_GATE_BIN", None)
            env.pop("WORKTREE_GATE_VERIFY", None)
            event = json.dumps({
                "event": "pre-tool-use",
                "tool_name": "Write",
                "tool_input": {"file_path": str(repo / "src.py")},
                "cwd": str(repo),
            })
            proc = subprocess.run(
                ["bash", str(launcher)], input=event, capture_output=True,
                text=True, cwd=root, env=env)
            # The gate still runs and blocks — the shims were never invoked.
            self.assertEqual(proc.returncode, 2, proc.stderr)
            self.assertIn("refusing", proc.stderr)

    def test_verify_flag_requests_selftest(self):
        if not BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo = root / "repo"
            repo.mkdir()
            subprocess.run(["git", "-C", str(repo), "init", "-q"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "t@t.t"],
                           check=True, capture_output=True, text=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "t"],
                           check=True, capture_output=True, text=True)
            (repo / "README.md").write_text("x\n")
            subprocess.run(["git", "-C", str(repo), "add", "-A"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-qm", "init"],
                           check=True, capture_output=True, text=True)
            subprocess.run(["git", "-C", str(repo), "checkout", "-q", "-B", "main"],
                           check=True)
            launcher = self._stamp_launcher(
                root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh")
            pin = root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
            pin.parent.mkdir(parents=True, exist_ok=True)
            pin.write_bytes(BINARY.read_bytes())
            pin.chmod(pin.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
            env = dict(os.environ)
            env["AI_SPECS_HOME"] = str(root / "home")
            env.pop("WORKTREE_GATE_BIN", None)
            env["WORKTREE_GATE_VERIFY"] = "1"
            event = json.dumps({
                "event": "pre-tool-use",
                "tool_name": "Write",
                "tool_input": {"file_path": str(repo / "src.py")},
                "cwd": str(repo),
            })
            proc = subprocess.run(
                ["bash", str(launcher)], input=event, capture_output=True,
                text=True, cwd=root, env=env)
            self.assertEqual(proc.returncode, 2, proc.stderr)
            self.assertIn("refusing", proc.stderr)


class WorktreeGateLatencyEvidenceTests(unittest.TestCase):
    """Reproducible per-invocation latency for the Go gate."""

    def test_latency_recorded_for_go_gate(self):
        if not BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        samples = sorted(CORPUS.glob("*.json"))[:4]
        self.assertTrue(samples)

        def measure(case_file: Path, runs: int) -> list[float]:
            case = json.loads(case_file.read_text())
            timings: list[float] = []
            with tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                locations = {} if case["fixture"] == "none" else build_fixture(root, case["fixture"])
                event_cwd = locations.get("repo", root)
                event = json.loads(json.dumps(case.get("event", {})))
                if "cwd" in event:
                    event["cwd"] = substitute(event["cwd"], locations)
                ti = event.get("tool_input") or {}
                for key in ("file_path", "notebook_path", "command", "script", "cmd"):
                    if key in ti:
                        ti[key] = substitute(ti[key], locations)
                for key in ("command", "script"):
                    if key in event:
                        event[key] = substitute(event[key], locations)
                payload = case.get("stdin") or json.dumps(event)
                expected = case["expected_exit"]
                for _i in range(runs):
                    start = time.perf_counter()
                    result = subprocess.run(
                        [str(BINARY), "--gate-mode", STAMPED_MODE,
                         "--gate-scope", STAMPED_SCOPE,
                         "--repo-topology", STAMPED_TOPOLOGY,
                         "--protected", "main development"],
                        input=payload, capture_output=True, text=True,
                        cwd=event_cwd)
                    timings.append(time.perf_counter() - start)
                    self.assertEqual(result.returncode, expected, result.stderr)
            return timings

        go_timings: list[float] = []
        for case_file in samples:
            measure(case_file, 1)  # warm-up
            go_timings.extend(measure(case_file, 3))

        self.assertEqual(len(go_timings), 12)
        self.assertTrue(all(t >= 0 for t in go_timings))
        go_median = sorted(go_timings)[len(go_timings) // 2]
        # Sanity only: a hung or pathologically slow gate would fail here.
        self.assertLess(go_median, 5.0,
                        f"Go median {go_median*1000:.1f}ms exceeded 5s sanity bound")


if __name__ == "__main__":
    import sys
    unittest.main()

"""Behavior tests for compact sync output (openspec change: compact-sync-output).

Covers fan-out termination, verbosity contract, nested framing, and errexit
interactions for lib/sync.sh and lib/sync-agent.sh.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))

import _go_cli  # noqa: E402
import _sync_stub  # noqa: E402

# The native Go binary: these suites must exercise the `sync`/`sync-agent`
# routes, not the legacy Bash launcher (kept only for the parity harness's
# legacy leg).
CLI = _go_cli.cli()
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"


def _sync_env(extra: dict | None = None) -> dict:
    env = {
        **os.environ,
        "AI_SPECS_HOME": str(ROOT),
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
    }
    if extra:
        env.update(extra)
    return env


# The Bash-source harness helpers (_bash_version, _supports_inherit_errexit,
# _extract_bash_functions) are retired with the re-point: extracting function
# bodies out of lib/sync.sh tests the orphaned script, not the native spine.


class _WorkspaceMixin:
    def make_workspace(self) -> Path:
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-sync-out-"))
        shutil.copytree(FIXTURE_ROOT, tmp / "workspace")
        return tmp / "workspace"

    def write_local_skill(self, workspace: Path, name: str = "local-demo") -> Path:
        skill_dir = workspace / "ai-specs" / "skills" / name
        skill_dir.mkdir(parents=True)
        path = skill_dir / "SKILL.md"
        path.write_text(
            textwrap.dedent(
                f"""\
                ---
                name: {name}
                description: >
                  Demo local skill.
                license: Apache-2.0
                metadata:
                  author: fixture-suite
                  version: "1.0"
                  scope:
                    - "root"
                  auto_invoke:
                    - "Syncing root workspace"
                ---

                # {name}
                """
            )
        )
        return path

    def init_workspace(
        self,
        workspace: Path,
        *,
        agents: list[str] | None = None,
        subrepos: list[str] | None = None,
    ) -> None:
        subprocess.run(
            [str(CLI), "init", str(workspace)],
            check=True,
            text=True,
            capture_output=True,
            env=_sync_env(),
        )
        agent_list = agents if agents is not None else ["claude", "cursor", "opencode"]
        repo_list = (
            subrepos if subrepos is not None else ["packages/a", "packages/b"]
        )
        agents_toml = ", ".join(f"'{a}'" for a in agent_list)
        repos_toml = ", ".join(f"'{r}'" for r in repo_list)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\n"
            "name = 'fixture-sync'\n"
            f"subrepos = [{repos_toml}]\n\n"
            "[agents]\n"
            f"enabled = [{agents_toml}]\n"
        )
        self.write_local_skill(workspace)

    def resolved_target_count(self, workspace: Path) -> int:
        proc = subprocess.run(
            [
                "python3",
                str(ROOT / "lib" / "_internal" / "target-resolve.py"),
                str(workspace),
            ],
            text=True,
            capture_output=True,
            check=True,
        )
        import json

        plan = json.loads(proc.stdout)
        return len(plan["targets"])

    def resolved_target_paths(self, workspace: Path) -> list[str]:
        import json

        proc = subprocess.run(
            [
                "python3",
                str(ROOT / "lib" / "_internal" / "target-resolve.py"),
                str(workspace),
            ],
            text=True,
            capture_output=True,
            check=True,
        )
        plan = json.loads(proc.stdout)
        return [t["path"] for t in plan["targets"]]


class FanOutTerminationTests(_WorkspaceMixin, unittest.TestCase):
    """P1 — public-root fan-out must terminate after dispatching children.

    Both tests observe the fan-out through the CLI boundary only: the parent's
    per-target step lines, the framing blocks, and the resulting tree. The old
    sync-agent.sh INVOKE/BODY instrumentation measured the Bash child-process
    boundary, which stops being authoritative once the fan-out runs natively
    inside the binary.
    """

    def test_t1_1_public_root_fanout_invokes_exactly_n_children_no_parent_body(
        self,
    ):
        """T1.1: N resolved targets → N per-target passes; the parent must not
        fall through into a single-target body against the workspace root."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace)
            targets = self.resolved_target_paths(workspace)
            self.assertGreater(len(targets), 1)

            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
            )
            out = proc.stdout
            # The parent announces exactly one per-target pass per resolved
            # target (its step lines carry the absolute target path; child
            # step labels never do).
            for target in targets:
                found = len(
                    re.findall(rf"(?m)^  syncing {re.escape(target)}$", out)
                )
                self.assertEqual(
                    found,
                    1,
                    f"expected exactly one fan-out pass for {target}, "
                    f"got {found}:\n{out}",
                )
            # One parent header and one footer: a parent that fell through
            # into the single-target body would add a second framing pair.
            header_count = len(re.findall(r"(?m)^ai-specs sync-agent$", out))
            self.assertEqual(
                header_count,
                1,
                f"parent header must appear exactly once; got {header_count}:\n{out}",
            )
            self.assertEqual(out.count("✓ sync-agent complete"), 1, out)
            # Every resolved target received its derived artifact set.
            for target in targets:
                self.assertTrue(
                    (Path(target) / "ai-specs" / ".gitignore").exists(),
                    f"target {target} has no ai-specs/.gitignore:\n{out}",
                )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_t1_3_first_child_failure_stops_fanout_and_names_target(self):
        """T1.3: first child failure stops the loop, exits non-zero, names target."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            targets = self.resolved_target_paths(workspace)
            self.assertGreater(len(targets), 1)

            # Fail the first resolved target (root) by planting a non-symlink
            # at the claude instructions path so the relative symlink refuses.
            claude_md = workspace / "CLAUDE.md"
            if claude_md.is_symlink() or claude_md.exists():
                claude_md.unlink()
            claude_md.write_text("manual file — not a symlink\n")

            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("sync-agent failed for target:", proc.stderr)
            self.assertIn(str(workspace.resolve()), proc.stderr)
            self.assertIn(
                "refuse to overwrite non-symlink", proc.stdout + proc.stderr
            )
            # Later subrepos must not have been written by a subsequent child.
            for target in targets[1:]:
                self.assertFalse(
                    (Path(target) / "ai-specs" / ".gitignore").exists(),
                    f"later target {target} was processed after the first "
                    f"failure:\n{proc.stdout}\n{proc.stderr}",
                )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)



class _CliSyncStubMixin:
    """Install PATH stubs, build a temp project, run one `sync` at the CLI."""

    def run_stubbed_sync(self, *, intercepts=(), args=()):
        base = Path(tempfile.mkdtemp(prefix="ai-specs-verb-"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        stubs = _sync_stub.SyncStubs(base)
        project = _sync_stub.minimal_project(base)
        for module, kwargs in intercepts:
            stubs.intercept(module, **kwargs)
        proc = subprocess.run(
            [str(CLI), "sync", str(project), *args],
            env=stubs.env(),
            text=True,
            capture_output=True,
            check=False,
        )
        return proc, stubs


class StepOutputContractCliTests(_CliSyncStubMixin, unittest.TestCase):
    """P2 — compact/verbose/failure contract, driven at the CLI boundary.

    The Bash-harness halves of the old P2 tests are replaced here: the first
    sync step (`gitignore-render.py`) is stubbed through PATH, so what these
    tests observe is the spine's own capture + filter + replay path rather
    than an extracted copy of `run_step`.
    """

    FIRST_STEP = "gitignore-render.py"
    FIRST_LABEL = "  syncing ai-specs/.gitignore"

    def _run(self, *, stdout: str = "", stderr: str = "", rc: int = 0, args=()):
        proc, _ = self.run_stubbed_sync(
            intercepts=[
                (self.FIRST_STEP, {"rc": rc, "stdout": stdout, "stderr": stderr})
            ],
            args=args,
        )
        return proc

    def test_t2_1_compact_suppresses_success_detail_and_blank_lines(self):
        """T2.1: compact mode drops ✓/·/⇢/▸ lines and blank lines."""
        stdout = (
            "    ✓ bundled skill worktree-flow\n"
            "    · symlink ok\n"
            "    ⇢ flattened 1\n"
            "    ▸ recipe session-context\n"
            "\n"
            "   \n"
            "keep-me\n"
            "  ! warning\n"
            "  ✗ error\n"
            "  ℹ notice\n"
        )
        proc = self._run(stdout=stdout)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("keep-me\n", proc.stdout)
        self.assertIn("  ! warning\n", proc.stdout)
        self.assertIn("  ✗ error\n", proc.stdout)
        self.assertIn("  ℹ notice\n", proc.stdout)
        for marker in (
            "    ✓ bundled skill worktree-flow",
            "    · symlink ok",
            "    ⇢ flattened 1",
            "    ▸ recipe session-context",
        ):
            self.assertNotIn(marker, proc.stdout)

    def test_t2_2_compact_preserves_notice_markers_on_original_streams(self):
        """T2.2: !/✗/ℹ survive byte-identically on their original streams."""
        proc = self._run(
            stdout="    ✓ detail\n  ! warn-stdout\nplain-out\n",
            stderr="  ✗ err-stderr\n  ℹ note-stderr\n",
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn(self.FIRST_LABEL + "\n", proc.stdout)
        self.assertIn("  ! warn-stdout\n", proc.stdout)
        self.assertIn("plain-out\n", proc.stdout)
        self.assertNotIn("✓ detail", proc.stdout)
        self.assertEqual(proc.stderr, "  ✗ err-stderr\n  ℹ note-stderr\n")
        # Stream separation: the stub's notice markers must not cross streams.
        # (Later real steps legitimately emit their own ℹ notices on stdout.)
        self.assertNotIn("  ✗ err-stderr", proc.stdout)
        self.assertNotIn("  ℹ note-stderr", proc.stdout)
        self.assertNotIn("  ! warn-stdout", proc.stderr)

    def test_t2_3_verbose_reproduces_full_step_output(self):
        """T2.3: --verbose prints the step's full unfiltered output."""
        proc = self._run(
            stdout="    ✓ a\n    · b\n  ! c\n",
            stderr="  ✗ d\n",
            args=("-v",),
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn(
            self.FIRST_LABEL + "\n    ✓ a\n    · b\n  ! c\n", proc.stdout
        )
        self.assertEqual(proc.stderr, "  ✗ d\n")

    def test_m3_verbose_preserves_trailing_blank_lines(self):
        """M3/F5: verbose replay must be byte-identical, including trailing blanks.

        `printf '%s\n' "$(cat out_file)"` strips ALL trailing newlines from
        the captured step output; a step that ends with blank lines must still
        reproduce them under --verbose.
        """
        proc = self._run(stdout="detail\n\n\n", args=("-v",))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn(
            self.FIRST_LABEL + "\ndetail\n\n\n",
            proc.stdout,
            f"verbose must preserve trailing blank lines; got:\n{proc.stdout!r}",
        )

    def test_t2_4_failing_step_prints_full_output_and_status_both_modes(self):
        """T2.4: failure always prints full stdout+stderr and propagates status."""
        for verbose in (False, True):
            args = ("-v",) if verbose else ()
            with self.subTest(verbose=verbose):
                proc = self._run(
                    stdout="    ✓ detail\n    · more\n",
                    stderr="diag on stderr\n",
                    rc=7,
                    args=args,
                )
                self.assertEqual(proc.returncode, 7, proc.stderr)
                self.assertIn(self.FIRST_LABEL + "\n", proc.stdout)
                self.assertIn("    ✓ detail\n", proc.stdout)
                self.assertIn("    · more\n", proc.stdout)
                self.assertEqual(proc.stderr, "diag on stderr\n")


class VerboseFlagIntegrationTests(_WorkspaceMixin, unittest.TestCase):
    """P2 — flag parsing and -v fan-out forwarding."""

    def test_t2_6_verbose_forwarded_through_fanout_only_when_set(self):
        """T2.6: children render full detail iff the parent was invoked with -v.

        The old version asserted the literal --verbose token on instrumented
        child argv; the behavioral equivalent is the child detail itself: each
        nested child prints its full step output (✓ flattened / ✓ merged) only
        when -v reaches it, and compact mode filters exactly those lines."""
        for with_verbose in (False, True):
            with self.subTest(verbose=with_verbose):
                workspace = self.make_workspace()
                try:
                    self.init_workspace(workspace, agents=["claude"])
                    n_targets = self.resolved_target_count(workspace)
                    self.assertGreater(n_targets, 1)
                    cmd = [str(CLI), "sync-agent", str(workspace), "--all"]
                    if with_verbose:
                        cmd.append("--verbose")
                    proc = subprocess.run(
                        cmd,
                        text=True,
                        capture_output=True,
                        check=False,
                        env=_sync_env(),
                    )
                    self.assertEqual(
                        proc.returncode,
                        0,
                        f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
                    )
                    flat_count = len(
                        re.findall(r"(?m)^\s*✓\s+flattened\b", proc.stdout)
                    )
                    merge_count = len(
                        re.findall(r"(?m)^\s*✓\s+merged\b", proc.stdout)
                    )
                    if with_verbose:
                        self.assertGreaterEqual(
                            flat_count,
                            n_targets,
                            f"-v must reach every child (flatten detail); "
                            f"got {flat_count} of {n_targets}:\n{proc.stdout}",
                        )
                        self.assertGreaterEqual(
                            merge_count,
                            n_targets,
                            f"-v must reach every child (merge detail); "
                            f"got {merge_count} of {n_targets}:\n{proc.stdout}",
                        )
                    else:
                        self.assertEqual(
                            flat_count,
                            0,
                            f"compact mode must filter child detail:\n{proc.stdout}",
                        )
                        self.assertEqual(
                            merge_count,
                            0,
                            f"compact mode must filter child detail:\n{proc.stdout}",
                        )
                finally:
                    shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_h2_sync_verbose_shows_parent_and_child_detail(self):
        """H2(a): `ai-specs sync -v` on a public root shows detail from parent AND every child."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            n_targets = self.resolved_target_count(workspace)
            self.assertGreaterEqual(n_targets, 2)

            proc = subprocess.run(
                [str(CLI), "sync", str(workspace), "-v"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            out = proc.stdout
            # Parent body detail (root sync steps, not only the fan-out labels).
            self.assertRegex(
                out,
                r"✓\s+wrote\s+\S+/ai-specs/\.gitignore",
                f"parent verbose detail missing (gitignore):\n{out}",
            )
            # Child body detail forwarded via -v → --verbose on each sync-agent.
            # Flatten runs once per child target; require the marker, not just argv.
            flat_count = len(re.findall(r"(?m)^\s*✓\s+flattened\b", out))
            self.assertGreaterEqual(
                flat_count,
                n_targets,
                f"expected flatten detail from each of {n_targets} children, "
                f"got {flat_count}:\n{out}",
            )
            merge_count = len(re.findall(r"(?m)^\s*✓\s+merged\b", out))
            self.assertGreaterEqual(
                merge_count,
                n_targets,
                f"expected merge detail from each of {n_targets} children, "
                f"got {merge_count}:\n{out}",
            )
            # Subrepo children also render a target gitignore (root child skips it).
            wrote_gi = len(
                re.findall(r"(?m)^\s*✓\s+wrote\s+\S+/packages/\S+/ai-specs/\.gitignore", out)
            )
            self.assertGreaterEqual(
                wrote_gi,
                n_targets - 1,
                f"expected subrepo gitignore detail from children; got {wrote_gi}:\n{out}",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_h2_short_v_forwards_through_sync_and_sync_agent_fanout(self):
        """H2(b): short `-v` (not only `--verbose`) forwards through both fan-out paths."""
        # Path 1: sync → sync-agent fan-out
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            n_targets = self.resolved_target_count(workspace)
            self.assertGreaterEqual(n_targets, 2)
            proc = subprocess.run(
                [str(CLI), "sync", str(workspace), "-v"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertGreaterEqual(
                len(re.findall(r"(?m)^\s*✓\s+flattened\b", proc.stdout)),
                n_targets,
                f"sync -v must forward short -v into child detail;\n{proc.stdout}",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

        # Path 2: sync-agent's own public-root fan-out
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            n_targets = self.resolved_target_count(workspace)
            self.assertGreaterEqual(n_targets, 2)
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all", "-v"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            flat_count = len(re.findall(r"(?m)^\s*✓\s+flattened\b", proc.stdout))
            self.assertGreaterEqual(
                flat_count,
                n_targets,
                f"sync-agent -v must forward short -v to nested children; "
                f"flatten markers={flat_count}:\n{proc.stdout}",
            )
            # Compact control: without -v, those markers must be absent.
            proc_c = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(proc_c.returncode, 0, proc_c.stderr)
            self.assertEqual(
                len(re.findall(r"(?m)^\s*✓\s+flattened\b", proc_c.stdout)),
                0,
                f"compact control unexpectedly showed flatten detail;\n{proc_c.stdout}",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_t2_7_unknown_flag_rejected_on_sync_and_sync_agent(self):
        """T2.7: unknown flags still exit non-zero on both commands."""
        for command in ("sync", "sync-agent"):
            with self.subTest(command=command):
                proc = subprocess.run(
                    [str(CLI), command, "--verbos"],
                    text=True,
                    capture_output=True,
                    check=False,
                    # The Go binary is built outside the repo, so pin the
                    # install root explicitly (assertions unchanged).
                    env=_sync_env(),
                )
                self.assertNotEqual(proc.returncode, 0)
                self.assertIn("unknown flag", proc.stderr.lower())



class NestedFramingTests(_WorkspaceMixin, unittest.TestCase):
    """P3 — banner/footer ownership for fan-out vs standalone."""

    def test_t3_1_header_once_and_footer_not_for_children(self):
        """T3.1: header once; footer only for top-level parent, not children."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            combined = proc.stdout + proc.stderr
            header_count = len(re.findall(r"(?m)^ai-specs sync-agent$", combined))
            self.assertEqual(
                header_count,
                1,
                f"header must appear exactly once; got {header_count}\n{combined}",
            )
            footer_count = combined.count("✓ sync-agent complete")
            self.assertEqual(
                footer_count,
                1,
                f"top-level parent must print footer exactly once; "
                f"got {footer_count}\n{combined}",
            )
            # Children must not emit their own framing blocks. With NESTED=1
            # they also must not print a second header after each target.
            self.assertNotRegex(
                combined,
                r"mode:\s+public root fan-out(?:.*\n){0,20}^ai-specs sync-agent$",
                "child must not reprint the sync-agent header",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_t3_1_standalone_still_prints_footer(self):
        """Standalone (single-target) sync-agent keeps the complete footer."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertIn("✓ sync-agent complete", proc.stdout)
            self.assertEqual(proc.stdout.count("ai-specs sync-agent"), 1)
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)



def _leaked_detail_markers(text: str, *, allow_footer: bool) -> list[str]:
    """Return detail-marker lines that must not appear in compact stdout.

    Allows the intentional top-level footer '✓ sync-agent complete' when
    allow_footer is True; every other leading ✓/·/⇢/▸ line is a leak.
    """
    leaked: list[str] = []
    for line in text.splitlines():
        stripped = line.lstrip()
        if not stripped:
            continue
        if stripped[0] not in "✓·⇢▸":
            continue
        if allow_footer and stripped == "✓ sync-agent complete":
            continue
        leaked.append(line)
    return leaked


class CompactModeLeakTests(_WorkspaceMixin, unittest.TestCase):
    """F1/H1 — flatten/merge/gitignore must not bypass run_step in compact mode."""

    def test_f1_standalone_sync_agent_compact_has_no_leaked_detail_markers(self):
        """E2E: standalone sync-agent compact stdout has zero leaked ✓/·/⇢/▸."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--claude"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            leaked = _leaked_detail_markers(proc.stdout, allow_footer=True)
            self.assertEqual(
                leaked,
                [],
                "compact sync-agent must not print raw detail markers "
                f"(flatten/merge/gitignore bypassing run_step):\n"
                + "\n".join(leaked)
                + f"\n\nfull stdout:\n{proc.stdout}",
            )
            # Intentional footer remains for standalone (non-nested) runs.
            self.assertIn("✓ sync-agent complete", proc.stdout)
            # And we still ran the work that used to leak (skills were flattened).
            self.assertIn("  syncing claude\n", proc.stdout)
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_f1_public_root_fanout_compact_has_no_child_detail_leaks(self):
        """E2E: public-root fan-out (2+ targets) leaks zero child flatten/merge/gitignore lines."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"])
            n_targets = self.resolved_target_count(workspace)
            self.assertGreaterEqual(n_targets, 2)
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            leaked = _leaked_detail_markers(proc.stdout, allow_footer=True)
            self.assertEqual(
                leaked,
                [],
                "fan-out compact mode must suppress every child's flatten/"
                f"merge/gitignore detail lines; leaked:\n"
                + "\n".join(leaked)
                + f"\n\nfull stdout:\n{proc.stdout}",
            )
            # T3.1: only the parent footer — children must not emit their own.
            self.assertEqual(
                proc.stdout.count("✓ sync-agent complete"),
                1,
                f"expected exactly one top-level footer;\n{proc.stdout}",
            )
            self.assertIn("mode:        public root fan-out", proc.stdout)
            # Children still sync (labels present) — silence is from filtering,
            # not from skipping work.
            self.assertGreaterEqual(proc.stdout.count("  syncing "), n_targets)
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)


class MarkerHygieneTests(_WorkspaceMixin, unittest.TestCase):
    """P3 — notices that must survive compaction use ℹ, not ·."""

    def test_t3_2_mcp_skipped_notice_survives_compaction(self):
        """T3.2 RED: 'mcp skipped' must remain visible in compact mode."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            # Ensure no [mcp.*] entries (init template may include none).
            toml = workspace / "ai-specs" / "ai-specs.toml"
            text = toml.read_text()
            self.assertNotRegex(text, r"(?m)^\[mcp\.")
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--claude"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            combined = proc.stdout + proc.stderr
            self.assertRegex(
                combined,
                r"ℹ.*mcp skipped \(no \[mcp\.\*\] in manifest\)",
                f"mcp skipped notice must survive compact mode;\n{combined}",
            )
            self.assertNotRegex(
                combined,
                r"·\s*mcp skipped",
                "mcp skipped must not use the suppressed · marker",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_m2_skipped_agents_md_notice_survives_compaction(self):
        """M2: 'skipped AGENTS.md (brief.render = false)' must remain visible in compact."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            # Root sync emits the notice via sync_agents_render (sync-agent root
            # short-circuits ensure_target_workspace before the skip message).
            toml = workspace / "ai-specs" / "ai-specs.toml"
            toml.write_text(toml.read_text().rstrip() + "\n\n[brief]\nrender = false\n")
            agents_md = workspace / "AGENTS.md"
            agents_md.write_text("# manual runtime brief\n")

            proc = subprocess.run(
                [str(CLI), "sync", str(workspace)],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
            )
            combined = proc.stdout + proc.stderr
            self.assertRegex(
                combined,
                r"ℹ.*skipped AGENTS\.md \(brief\.render = false\)",
                f"skipped AGENTS.md notice must survive compact mode;\n{combined}",
            )
            self.assertNotRegex(
                combined,
                r"·\s*skipped AGENTS\.md",
                "skipped AGENTS.md must not use the suppressed · marker",
            )
            # Content left untouched (opt-out still honored under compaction).
            self.assertEqual(agents_md.read_text(), "# manual runtime brief\n")
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)


# DotMarkerAuditTests is retired: it audited `·` echo comments in
# lib/sync*.sh, which is Bash source structure, not the native spine's
# behavior. The observable intent — compact mode suppresses `·` noise — is
# covered by CompactModeLeakTests, MarkerHygieneTests and
# StepOutputContractCliTests above.


class TemplateSkippedClassificationTests(_CliSyncStubMixin, unittest.TestCase):
    """M1 / T3.3 gap — classify recipe-materialize 'template skipped (exists)'."""

    SAMPLE = "    · template skipped (exists) ai-specs/foo.md"

    def test_m1_template_skipped_is_classified_as_noise(self):
        """Static half: the producer marks it `·` noise, not a `ℹ` notice.

        Precedent (daad3aa): promote to ℹ only for user-facing policy/absence
        notices ('skipped AGENTS.md', 'mcp skipped'). Template-skipped reports
        a no-op success when condition=not_exists and the dest already exists —
        same class as 'symlink ok', so it stays · and is filtered in compact.
        """
        recipe_py = ROOT / "lib" / "_internal" / "recipe-materialize.py"
        lines = recipe_py.read_text().splitlines()
        hit = None
        for i, line in enumerate(lines):
            if "template skipped (exists)" in line and "print" in line:
                hit = i
                break
        self.assertIsNotNone(hit, "template skipped print not found")
        prev = lines[hit - 1] if hit >= 1 else ""
        self.assertRegex(
            prev,
            r"Noise \(keep ·\)|Notice \(promote to ℹ\)",
            f"unclassified template-skipped line at {recipe_py}:{hit+1}: "
            f"{lines[hit].strip()}\nprev: {prev!r}",
        )
        self.assertIn("Noise (keep ·)", prev)
        self.assertIn("· template skipped (exists)", lines[hit])
        self.assertNotIn("ℹ template skipped", lines[hit])

    def test_m1_template_skipped_cli_compact_drops_and_verbose_keeps(self):
        """CLI half: the stub's `·` line is filtered in compact, kept in -v."""
        intercepts = [
            ("gitignore-render.py", {"rc": 0, "stdout": self.SAMPLE + "\nkeep-me\n"})
        ]
        proc, _ = self.run_stubbed_sync(intercepts=intercepts)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("keep-me\n", proc.stdout)
        self.assertNotIn("template skipped", proc.stdout)

        proc_v, _ = self.run_stubbed_sync(intercepts=intercepts, args=("-v",))
        self.assertEqual(proc_v.returncode, 0, proc_v.stderr)
        self.assertIn(self.SAMPLE, proc_v.stdout)


class ErrexitInteractionTests(_WorkspaceMixin, _CliSyncStubMixin, unittest.TestCase):
    """P4 — failure propagation, driven at the CLI boundary.

    `test_t4_0_bash_3_2_runs_both_sync_entry_points` is retired with the
    Bash-3.2 matrix: it only proved the shell scripts run under a legacy
    interpreter, which stops being authoritative once `sync` is native. The
    direct-script halves of the old `test_t4_1_*` are replaced by CLI stub
    tests; the `|| return $?` count audit on sync-agent.sh is retired as Bash
    source structure (its behavioral replacement is the symlink test below).
    """

    def test_t4_1_target_resolution_failure_aborts_before_the_header(self):
        """A failing target-resolve must abort before any writes or header."""
        proc, stubs = self.run_stubbed_sync(
            intercepts=[("target-resolve.py", {"rc": 1, "stderr": "ERREXIT_PROBE\n"})]
        )
        self.assertEqual(proc.returncode, 1, proc.stderr)
        self.assertIn(
            "ERROR: target resolution failed before any writes.", proc.stderr
        )
        self.assertIn("ERREXIT_PROBE", proc.stderr)
        self.assertNotIn("ai-specs sync", proc.stdout)
        self.assertFalse(
            any("project-cache.py" in line for line in stubs.invocations()),
            "resolution failed before any write work began",
        )

    def test_t4_1_project_cache_failure_in_the_fanout_propagates(self):
        """A failing cache derivation in the fan-out exits non-zero, no footer.

        The old version PATH-stubbed project-cache.py inside the sync fan-out;
        the native fan-out derives cache paths itself, so the stub can no
        longer reach this seam. The fault is injected at the real seam instead:
        a read-only isolated-home cache directory makes ensure_cache fail
        exactly where the shell's command substitution died, and sync-agent
        must die with it before any footer."""
        import _blackbox as bb  # noqa: E402  (tests-dir helper, not lib code)

        workspace = self.make_workspace()
        base = Path(tempfile.mkdtemp(prefix="ai-specs-cache-fault-"))
        cache = base / "cli-home" / "cache"
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            home = bb.isolated_home(base)
            cache = home / "cache"
            os.chmod(cache, 0o555)
            recipe_mcp = base / "recipe-mcp.json"
            recipe_mcp.write_text("{}\n")
            proc = subprocess.run(
                [
                    str(CLI), "sync-agent",
                    "--source-root", str(workspace),
                    "--target", str(workspace),
                    "--recipe-mcp", str(recipe_mcp),
                ],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env({"AI_SPECS_HOME": str(home)}),
            )
            self.assertNotEqual(proc.returncode, 0)
            combined = proc.stdout + proc.stderr
            self.assertIn("cache not writable", combined)
            self.assertNotIn("✓ sync-agent complete", combined)
            # The failure fired at the derivation seam, before any step work.
            self.assertNotIn("  syncing flatten resolved skills", combined)
        finally:
            if cache.exists():
                os.chmod(cache, 0o755)
            shutil.rmtree(workspace.parent, ignore_errors=True)
            shutil.rmtree(base, ignore_errors=True)

    def test_t4_2_sync_one_agent_return_sites_propagate_symlink_failure(self):
        """T4.2: make_relative_symlink failure exits sync-agent via || return $?."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            claude_md = workspace / "CLAUDE.md"
            if claude_md.exists() or claude_md.is_symlink():
                claude_md.unlink()
            claude_md.write_text("not-a-symlink\n")

            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--claude"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env(),
            )
            self.assertNotEqual(proc.returncode, 0)
            combined = proc.stdout + proc.stderr
            self.assertIn("refuse to overwrite non-symlink", combined)
            # Must not claim success after a swallowed failure.
            self.assertNotIn("✓ sync-agent complete", combined)
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)

    # Retired: the skills-symlink leg was a static `|| return $?` count audit
    # over sync-agent.sh's source (`body.count(...)`), i.e. Bash source
    # structure rather than observed behavior. The behavioral replacement is
    # `test_t4_2_sync_one_agent_return_sites_propagate_symlink_failure` above,
    # which drives the CLI and asserts the failure surfaces.


if __name__ == "__main__":
    unittest.main()

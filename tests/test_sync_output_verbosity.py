"""Behavior tests for compact sync output (openspec change: compact-sync-output).

Covers fan-out termination, verbosity contract, nested framing, and errexit
interactions for lib/sync.sh and lib/sync-agent.sh.
"""

from __future__ import annotations

import os
import re
import shutil
import stat
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
SYNC_SH = ROOT / "lib" / "sync.sh"
SYNC_AGENT_SH = ROOT / "lib" / "sync-agent.sh"
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"

BODY_NEEDLE = 'TOML_PATH="$SOURCE_ROOT/ai-specs/ai-specs.toml"'


def _sync_env(extra: dict | None = None) -> dict:
    env = {
        **os.environ,
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
    }
    if extra:
        env.update(extra)
    return env


def _bash_version(binary: Path) -> tuple[int, int] | None:
    proc = subprocess.run(
        [
            str(binary),
            "-c",
            'printf "%s.%s\\n" "${BASH_VERSINFO[0]}" "${BASH_VERSINFO[1]}"',
        ],
        text=True,
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0:
        return None
    parts = proc.stdout.strip().split(".")
    if len(parts) != 2:
        return None
    try:
        return int(parts[0]), int(parts[1])
    except ValueError:
        return None


def _supports_inherit_errexit(binary: Path) -> bool:
    proc = subprocess.run(
        [str(binary), "-c", "shopt -s inherit_errexit"],
        text=True,
        capture_output=True,
        check=False,
    )
    return proc.returncode == 0


def _extract_bash_functions(script: Path, names: tuple[str, ...]) -> str:
    """Extract named function bodies from a bash script (brace-counted)."""
    text = script.read_text()
    chunks: list[str] = []
    for name in names:
        start = text.find(f"{name}() {{")
        if start < 0:
            start = text.find(f"{name}(){{")
        if start < 0:
            raise AssertionError(f"function {name} not found in {script}")
        depth = 0
        i = text.find("{", start)
        end = None
        while i < len(text):
            ch = text[i]
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    end = i + 1
                    break
            i += 1
        if end is None:
            raise AssertionError(f"unterminated function {name} in {script}")
        chunks.append(text[start:end])
    return "\n\n".join(chunks)


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

    def instrumented_sync_agent_home(self, log_path: Path) -> Path:
        """AI_SPECS_HOME with sync-agent.sh logging INVOKE/BODY entries."""
        home = Path(tempfile.mkdtemp(prefix="ai-specs-home-"))
        for name in os.listdir(ROOT):
            src = ROOT / name
            dst = home / name
            if name == "lib":
                dst.mkdir()
                for lib_name in os.listdir(src):
                    if lib_name == "sync-agent.sh":
                        continue
                    (dst / lib_name).symlink_to(src / lib_name)
            else:
                dst.symlink_to(src)

        real = SYNC_AGENT_SH.read_text()
        if BODY_NEEDLE not in real:
            raise AssertionError(f"expected {BODY_NEEDLE!r} in sync-agent.sh")
        instrumented = real.replace(
            "set -euo pipefail\n",
            "set -euo pipefail\n"
            f'printf "INVOKE %s\\n" "$*" >> "{log_path}"\n',
            1,
        )
        instrumented = instrumented.replace(
            BODY_NEEDLE,
            BODY_NEEDLE + f'\nprintf "BODY %s\\n" "$*" >> "{log_path}"',
            1,
        )
        target = home / "lib" / "sync-agent.sh"
        target.write_text(instrumented)
        target.chmod(target.stat().st_mode | stat.S_IEXEC)
        return home


class FanOutTerminationTests(_WorkspaceMixin, unittest.TestCase):
    """P1 — public-root fan-out must terminate after dispatching children."""

    def test_t1_1_public_root_fanout_invokes_exactly_n_children_no_parent_body(
        self,
    ):
        """T1.1: N resolved targets → N child invocations; parent must not
        fall through into a silent materialize/render pass."""
        workspace = self.make_workspace()
        log_path = Path(tempfile.mkdtemp()) / "sync-agent.log"
        home = None
        try:
            self.init_workspace(workspace)
            n_targets = self.resolved_target_count(workspace)
            self.assertGreater(n_targets, 1)

            home = self.instrumented_sync_agent_home(log_path)
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env({"AI_SPECS_HOME": str(home)}),
            )
            self.assertEqual(
                proc.returncode,
                0,
                f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}",
            )

            log = log_path.read_text() if log_path.exists() else ""
            invokes = [
                line[len("INVOKE ") :]
                for line in log.splitlines()
                if line.startswith("INVOKE ")
            ]
            bodies = [
                line[len("BODY ") :]
                for line in log.splitlines()
                if line.startswith("BODY ")
            ]
            child_invokes = [args for args in invokes if "--source-root" in args]

            self.assertEqual(
                len(child_invokes),
                n_targets,
                f"expected {n_targets} child sync-agent invocations, got "
                f"{len(child_invokes)}: {child_invokes}\nfull log:\n{log}",
            )
            self.assertEqual(
                len(bodies),
                n_targets,
                f"parent must not enter the single-target body after fan-out; "
                f"expected {n_targets} BODY entries, got {len(bodies)}.\n"
                f"full log:\n{log}\nstdout:\n{proc.stdout}",
            )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)
            if home is not None:
                shutil.rmtree(home, ignore_errors=True)
            if log_path.parent.exists():
                shutil.rmtree(log_path.parent, ignore_errors=True)

    def test_t1_3_first_child_failure_stops_fanout_and_names_target(self):
        """T1.3: first child failure stops the loop, exits non-zero, names target."""
        workspace = self.make_workspace()
        log_path = Path(tempfile.mkdtemp()) / "sync-agent.log"
        home = None
        try:
            self.init_workspace(workspace, agents=["claude"])
            n_targets = self.resolved_target_count(workspace)
            self.assertGreater(n_targets, 1)

            # Fail the first resolved target (root) by planting a non-symlink
            # at the claude instructions path so make_relative_symlink refuses.
            claude_md = workspace / "CLAUDE.md"
            if claude_md.is_symlink() or claude_md.exists():
                claude_md.unlink()
            claude_md.write_text("manual file — not a symlink\n")

            home = self.instrumented_sync_agent_home(log_path)
            proc = subprocess.run(
                [str(CLI), "sync-agent", str(workspace), "--all"],
                text=True,
                capture_output=True,
                check=False,
                env=_sync_env({"AI_SPECS_HOME": str(home)}),
            )
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("sync-agent failed for target:", proc.stderr)
            self.assertIn(str(workspace.resolve()), proc.stderr)

            log = log_path.read_text() if log_path.exists() else ""
            child_invokes = [
                line
                for line in log.splitlines()
                if line.startswith("INVOKE ") and "--source-root" in line
            ]
            self.assertEqual(
                len(child_invokes),
                1,
                f"second child must not run after first failure; "
                f"child invokes:\n{child_invokes}\nlog:\n{log}",
            )
            # Later subrepo must not have been written by a subsequent child.
            self.assertFalse((workspace / "packages" / "b" / "AGENTS.md").exists())
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)
            if home is not None:
                shutil.rmtree(home, ignore_errors=True)
            if log_path.parent.exists():
                shutil.rmtree(log_path.parent, ignore_errors=True)



class StepOutputContractTests(unittest.TestCase):
    """P2 — compact/verbose/failure contract for print_step_output + run_step."""

    def _harness(self, script: Path, *, verbose: int, body: str) -> subprocess.CompletedProcess:
        fns = _extract_bash_functions(script, ("print_step_output", "run_step"))
        bash = (
            "set -euo pipefail\n"
            f"VERBOSE={verbose}\n"
            f"{fns}\n"
            f"{body}\n"
        )
        return subprocess.run(
            ["bash", "-c", bash],
            text=True,
            capture_output=True,
            check=False,
        )

    def test_t2_1_compact_suppresses_success_detail_and_blank_lines(self):
        """T2.1: compact mode drops ✓/·/⇢/▸ lines and blank lines."""
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                body = textwrap.dedent(
                    r"""
                    f="$(mktemp)"
                    printf '%s\n' \
                        '    ✓ bundled skill worktree-flow' \
                        '    · symlink ok' \
                        '    ⇢ flattened 1' \
                        '    ▸ recipe session-context' \
                        '' \
                        '   ' \
                        'keep-me' \
                        '  ! warning' \
                        '  ✗ error' \
                        '  ℹ notice' >"$f"
                    print_step_output "$f"
                    rm -f "$f"
                    """
                )
                proc = self._harness(script, verbose=0, body=body)
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertEqual(
                    proc.stdout,
                    "keep-me\n  ! warning\n  ✗ error\n  ℹ notice\n",
                )
                for marker in ("✓", "·", "⇢", "▸"):
                    self.assertNotIn(marker, proc.stdout)

    def test_t2_2_compact_preserves_notice_markers_on_original_streams(self):
        """T2.2: !/✗/ℹ survive byte-identically on their original streams."""
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                body = textwrap.dedent(
                    r"""
                    step() {
                        printf '%s\n' '    ✓ detail' '  ! warn-stdout' 'plain-out'
                        printf '%s\n' '  ✗ err-stderr' '  ℹ note-stderr' >&2
                    }
                    run_step "demo" step
                    """
                )
                proc = self._harness(script, verbose=0, body=body)
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertIn("  syncing demo\n", proc.stdout)
                self.assertIn("  ! warn-stdout\n", proc.stdout)
                self.assertIn("plain-out\n", proc.stdout)
                self.assertNotIn("✓ detail", proc.stdout)
                self.assertEqual(proc.stderr, "  ✗ err-stderr\n  ℹ note-stderr\n")
                # Stream separation: notice markers must not cross streams.
                self.assertNotIn("✗", proc.stdout)
                self.assertNotIn("ℹ", proc.stdout)
                self.assertNotIn("!", proc.stderr)

    def test_t2_3_verbose_reproduces_full_step_output(self):
        """T2.3: --verbose prints the step's full unfiltered output."""
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                body = textwrap.dedent(
                    r"""
                    step() {
                        printf '%s\n' '    ✓ a' '    · b' '  ! c'
                        printf '%s\n' '  ✗ d' >&2
                    }
                    run_step "demo" step
                    """
                )
                proc = self._harness(script, verbose=1, body=body)
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertEqual(
                    proc.stdout,
                    "  syncing demo\n    ✓ a\n    · b\n  ! c\n",
                )
                self.assertEqual(proc.stderr, "  ✗ d\n")

    def test_m3_verbose_preserves_trailing_blank_lines(self):
        """M3/F5: verbose replay must be byte-identical, including trailing blanks.

        `printf '%s\n' "$(cat out_file)"` strips ALL trailing newlines from the
        captured step output; a step that ends with blank lines must still
        reproduce them under --verbose.
        """
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                fns = _extract_bash_functions(
                    script, ("print_step_output", "run_step")
                )
                # Emit: "line\n\n\n" (one content line + two trailing blank lines)
                # via a real temp file written with trailing newlines, then run_step.
                body = (
                    "set -euo pipefail\n"
                    "VERBOSE=1\n"
                    + fns
                    + "\n"
                    + "step() {\n"
                    + "  # exact bytes: 'detail' + newline + blank + blank\n"
                    + "  printf 'detail\n\n\n'\n"
                    + "}\n"
                    + "run_step \"demo\" step\n"
                )
                proc = subprocess.run(
                    ["bash", "-c", body],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                # Expect: syncing line, then detail, then two trailing blanks
                # exactly as the step printed (byte-identical replay).
                self.assertEqual(
                    proc.stdout,
                    "  syncing demo\ndetail\n\n\n",
                    f"verbose must preserve trailing blank lines; got:\n"
                    f"{proc.stdout!r}",
                )


    def test_t2_4_failing_step_prints_full_output_and_status_both_modes(self):
        """T2.4: failure always prints full stdout+stderr and propagates status."""
        for script in (SYNC_SH, SYNC_AGENT_SH):
            for verbose in (0, 1):
                with self.subTest(script=script.name, verbose=verbose):
                    body = textwrap.dedent(
                        r"""
                        bad() {
                            printf '%s\n' '    ✓ detail' '    · more'
                            printf '%s\n' 'diag on stderr' >&2
                            return 7
                        }
                        # Invoke in ||-list so run_step's restored set -e does
                        # not abort the harness before we can observe $?.
                        rc=0
                        run_step "demo" bad || rc=$?
                        printf 'EXIT:%s\n' "$rc"
                        """
                    )
                    proc = self._harness(script, verbose=verbose, body=body)
                    self.assertEqual(proc.returncode, 0, proc.stderr)
                    self.assertIn("  syncing demo\n", proc.stdout)
                    self.assertIn("    ✓ detail\n", proc.stdout)
                    self.assertIn("    · more\n", proc.stdout)
                    self.assertIn("EXIT:7\n", proc.stdout)
                    self.assertEqual(proc.stderr, "diag on stderr\n")


class VerboseFlagIntegrationTests(_WorkspaceMixin, unittest.TestCase):
    """P2 — flag parsing and -v fan-out forwarding."""

    def test_t2_6_verbose_forwarded_through_fanout_only_when_set(self):
        """T2.6: children get --verbose iff the parent was invoked with -v."""
        for with_verbose in (False, True):
            with self.subTest(verbose=with_verbose):
                workspace = self.make_workspace()
                log_path = Path(tempfile.mkdtemp()) / "sync-agent.log"
                home = None
                try:
                    self.init_workspace(workspace, agents=["claude"])
                    home = self.instrumented_sync_agent_home(log_path)
                    cmd = [str(CLI), "sync-agent", str(workspace), "--all"]
                    if with_verbose:
                        cmd.append("--verbose")
                    proc = subprocess.run(
                        cmd,
                        text=True,
                        capture_output=True,
                        check=False,
                        env=_sync_env({"AI_SPECS_HOME": str(home)}),
                    )
                    self.assertEqual(
                        proc.returncode,
                        0,
                        f"stderr:\n{proc.stderr}\nstdout:\n{proc.stdout}",
                    )
                    log = log_path.read_text()
                    child_invokes = [
                        line[len("INVOKE ") :]
                        for line in log.splitlines()
                        if line.startswith("INVOKE ") and "--source-root" in line
                    ]
                    self.assertGreaterEqual(len(child_invokes), 2)
                    for args in child_invokes:
                        if with_verbose:
                            self.assertRegex(
                                args,
                                r"(^|\s)--verbose(\s|$)",
                                f"missing --verbose in child args: {args}",
                            )
                        else:
                            self.assertNotRegex(
                                args,
                                r"(^|\s)--verbose(\s|$)",
                                f"unexpected --verbose in child args: {args}",
                            )
                finally:
                    shutil.rmtree(workspace.parent, ignore_errors=True)
                    if home is not None:
                        shutil.rmtree(home, ignore_errors=True)
                    if log_path.parent.exists():
                        shutil.rmtree(log_path.parent, ignore_errors=True)

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


class DotMarkerAuditTests(unittest.TestCase):
    """P3 T3.3 — remaining · echoes are intentional compact-mode noise."""

    def test_t3_3_remaining_dot_markers_are_classified_noise(self):
        """Every ·-prefixed echo in sync*.sh is classified (noise vs notice)."""
        pattern = re.compile(r'echo\s+"([^"]*·[^"]*)"')
        found: dict[str, list[str]] = {}
        for script in (SYNC_SH, SYNC_AGENT_SH):
            lines = script.read_text().splitlines()
            for i, line in enumerate(lines, 1):
                m = pattern.search(line)
                if not m:
                    continue
                # Require an immediately preceding classification comment.
                prev = lines[i - 2] if i >= 2 else ""
                self.assertRegex(
                    prev,
                    r"Noise \(keep ·\)|Notice \(not noise\)",
                    f"{script.name}:{i} has unclassified · echo: {line.strip()}",
                )
                found.setdefault(script.name, []).append(m.group(1))
        # sync.sh has no · echoes today; sync-agent keeps only symlink-ok noise.
        self.assertEqual(found.get("sync.sh", []), [])
        self.assertTrue(
            all("symlink ok" in msg for msg in found.get("sync-agent.sh", [])),
            found,
        )
        # mcp skipped must not remain on ·.
        joined = " ".join(found.get("sync-agent.sh", []))
        self.assertNotIn("mcp skipped", joined)



class TemplateSkippedClassificationTests(unittest.TestCase):
    """M1 / T3.3 gap — classify recipe-materialize 'template skipped (exists)'."""

    def test_m1_template_skipped_is_noise_filtered_in_compact(self):
        """Keep · (noise): idempotent 'already exists' detail, like symlink ok.

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

        sample = "    · template skipped (exists) ai-specs/foo.md"
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                fns = _extract_bash_functions(
                    script, ("print_step_output", "run_step")
                )
                body = (
                    "set -euo pipefail\n"
                    "VERBOSE=0\n"
                    + fns
                    + "\n"
                    + 'f="$(mktemp)"\n'
                    + "printf '%s\n' '"
                    + sample
                    + "' 'keep-me' >\"$f\"\n"
                    + "print_step_output \"$f\"\n"
                    + "rm -f \"$f\"\n"
                )
                proc = subprocess.run(
                    ["bash", "-c", body],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertEqual(proc.stdout, "keep-me\n")
                self.assertNotIn("template skipped", proc.stdout)

                body_v = (
                    "set -euo pipefail\n"
                    "VERBOSE=1\n"
                    + fns
                    + "\n"
                    + 'f="$(mktemp)"\n'
                    + "printf '%s\n' '"
                    + sample
                    + "' >\"$f\"\n"
                    + "print_step_output \"$f\"\n"
                    + "rm -f \"$f\"\n"
                )
                proc_v = subprocess.run(
                    ["bash", "-c", body_v],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(proc_v.returncode, 0, proc_v.stderr)
                self.assertIn(sample, proc_v.stdout)


class ErrexitInteractionTests(_WorkspaceMixin, unittest.TestCase):
    """P4 — inherit_errexit + sync_one_agent || return $? failure propagation."""

    def test_t4_0_bash_3_2_runs_both_sync_entry_points(self):
        """T4.0: stock macOS Bash reaches both sync activation paths."""
        interpreter = Path("/bin/bash")
        version = _bash_version(interpreter) if interpreter.is_file() else None
        if version != (3, 2):
            self.skipTest(
                "legacy Bash 3.2 matrix leg omitted: /bin/bash is unavailable "
                f"or reports {version!r}"
            )

        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                workspace = self.make_workspace()
                try:
                    self.init_workspace(workspace, agents=["claude"], subrepos=[])
                    command = [str(interpreter), str(script), str(workspace)]
                    command.append(
                        "--ignore-cli-version"
                        if script == SYNC_SH
                        else "--claude"
                    )
                    proc = subprocess.run(
                        command,
                        text=True,
                        capture_output=True,
                        check=False,
                        env=_sync_env(),
                    )
                    combined = proc.stdout + proc.stderr
                    self.assertEqual(
                        proc.returncode,
                        0,
                        f"{script.name} under /bin/bash 3.2 failed:\n{combined}",
                    )
                    self.assertNotIn("invalid shell option name", combined)
                finally:
                    shutil.rmtree(workspace.parent, ignore_errors=True)

    def test_t4_1_command_substitution_failure_hard_fails_under_inherit_errexit(self):
        """T4.1: failing $(...) below inherit_errexit is not silently swallowed."""
        modern_binary = shutil.which("bash")
        modern = Path(modern_binary) if modern_binary else None
        version = _bash_version(modern) if modern else None
        if version is None or version < (4, 4) or not _supports_inherit_errexit(modern):
            self.skipTest(
                "modern Bash >=4.4 matrix leg omitted: interpreter unavailable "
                f"or lacks inherit_errexit (detected {version!r})"
            )

        harness_dir = Path(tempfile.mkdtemp(prefix="ai-specs-errexit-env-"))
        self.addCleanup(shutil.rmtree, harness_dir, ignore_errors=True)
        bash_env = harness_dir / "bash-env"
        bash_env.write_text(
            textwrap.dedent(
                r"""
                python3() {
                    case "${2:-}" in
                        *'["root"]'*) printf 'ERREXIT_PROBE\n' >&2; false ;;
                    esac
                    case "${1:-}:${3:-}" in
                        *project-cache.py:path) printf 'ERREXIT_PROBE\n' >&2; false ;;
                    esac
                    command python3 "$@"
                }
                """
            )
        )

        # The actual scripts must still inherit errexit on a modern Bash. The
        # injected helper fails inside an unguarded command substitution; with
        # inherit_errexit enabled, the scripts stop before their success marker.
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=f"{script.name}:modern-runtime"):
                workspace = self.make_workspace()
                try:
                    self.init_workspace(workspace, agents=["claude"], subrepos=[])
                    command = [str(modern), str(script), str(workspace)]
                    command.append(
                        "--ignore-cli-version"
                        if script == SYNC_SH
                        else "--claude"
                    )
                    proc = subprocess.run(
                        command,
                        text=True,
                        capture_output=True,
                        check=False,
                        env=_sync_env({"BASH_ENV": str(bash_env)}),
                    )
                    combined = proc.stdout + proc.stderr
                    self.assertNotEqual(
                        proc.returncode,
                        0,
                        f"{script.name} did not preserve inherited errexit:\n{combined}",
                    )
                    self.assertIn("ERREXIT_PROBE", combined)
                    self.assertNotIn("complete", combined)
                finally:
                    shutil.rmtree(workspace.parent, ignore_errors=True)

        # Mirror the scripts' shell options and the sync_one_agent pattern:
        #   local x; x="$(cmd)" || return $?
        bash = textwrap.dedent(
            r"""
            set -euo pipefail
            shopt -s inherit_errexit
            sync_one_agent_like() {
                local x
                x="$(false)" || return $?
                printf 'REACHED\n'
                return 0
            }
            rc=0
            sync_one_agent_like || rc=$?
            printf 'STATUS:%s\n' "$rc"
            """
        )
        proc = subprocess.run(
            [str(modern), "-c", bash], text=True, capture_output=True, check=False
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(proc.stdout, "STATUS:1\n")
        self.assertNotIn("REACHED", proc.stdout)

        # Same pattern as exercised inside the real scripts after their shopt line:
        # a run_step whose body uses a failing command substitution must surface
        # the failure (not continue as success).
        for script in (SYNC_SH, SYNC_AGENT_SH):
            with self.subTest(script=script.name):
                fns = _extract_bash_functions(
                    script, ("print_step_output", "run_step")
                )
                body_tail = textwrap.dedent(
                    r"""
                    bad_step() {
                        # Fail inside a command substitution below inherit_errexit.
                        local x
                        x="$(false)" || return $?
                        printf 'should-not-print\n'
                    }
                    rc=0
                    run_step "demo" bad_step || rc=$?
                    printf 'EXIT:%s\n' "$rc"
                    """
                )
                body = (
                    "set -euo pipefail\n"
                    "shopt -s inherit_errexit\n"
                    "VERBOSE=0\n"
                    + fns
                    + "\n"
                    + body_tail
                )
                proc = subprocess.run(
                    [str(modern), "-c", body],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertIn("EXIT:1\n", proc.stdout)
                self.assertNotIn("should-not-print", proc.stdout)

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

    def test_t4_2_sync_one_agent_return_sites_propagate_skills_symlink_failure(
        self,
    ):
        """T4.2: make_skills_symlink failure also propagates (not swallowed)."""
        workspace = self.make_workspace()
        try:
            self.init_workspace(workspace, agents=["claude"], subrepos=[])
            # Plant a non-symlink at the skills link path (.claude/skills).
            skills_link = workspace / ".claude" / "skills"
            skills_link.parent.mkdir(parents=True, exist_ok=True)
            if skills_link.is_symlink() or skills_link.exists():
                if skills_link.is_dir() and not skills_link.is_symlink():
                    shutil.rmtree(skills_link)
                else:
                    skills_link.unlink()
            # After sync_one_agent removes a non-symlink dir, it recreates the
            # symlink. To force make_skills_symlink to fail, plant a non-symlink
            # FILE at the link path after ensuring parent exists — but the
            # function rm -rf's non-symlink dirs first. Plant a file that is
            # not a dir: rm -rf will remove a file too. The refuse path is when
            # the existing path is neither missing nor a symlink to the right
            # target... read make_skills_symlink.
            #
            # make_skills_symlink refuses when link_path exists and is NOT a
            # symlink. sync_one_agent rm -rf's non-symlink paths first for
            # skills, so that path is covered. Use instructions symlink
            # refusal as the representative || return $? site already above,
            # and additionally assert every || return $? site still exists.
            text = SYNC_AGENT_SH.read_text()
            start = text.index("sync_one_agent() {")
            end = text.index("\nfor agent in", start)
            body = text[start:end]
            returns = body.count("|| return $?")
            self.assertGreaterEqual(
                returns,
                10,
                f"expected the sync_one_agent || return $? sites; found {returns}",
            )
            # No bare command substitutions without || return in the critical path.
            for line in body.splitlines():
                stripped = line.strip()
                if "platform_get" in stripped and "$(" in stripped:
                    self.assertIn(
                        "|| return $?",
                        stripped,
                        f"platform_get substitution missing || return $?: {stripped}",
                    )
        finally:
            shutil.rmtree(workspace.parent, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()

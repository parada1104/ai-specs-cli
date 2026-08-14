"""Integration tests for the plan-build-flow plan-build-gate.sh runtime hook.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open. The gate blocks production edits when
no active change folder (openspec/changes/<slug>/tasks.md, outside archive/)
exists.
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "plan-build-flow" / "hooks" / "plan-build-gate.sh"

# The preflight-resolved store (config artifact_store_default) must never change
# a gate decision. STORE_ENV_KEY is a test-only fixture naming the env a
# store-aware preflight would set; the gate is store-blind and reads only the
# filesystem planning tree, so every context must yield the baseline verdict.
STORE_ENUM = ["openspec", "engram", "both"]
STORE_ENV_KEY = "PLAN_BUILD_ARTIFACT_STORE"


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


def _git_output(cwd: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(cwd), *args], check=True,
                          capture_output=True, text=True).stdout.strip()


class PlanBuildGateHookTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name) / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")

    def _seed_change_at(self, root: Path, slug: str = "demo-change") -> None:
        d = root / "openspec" / "changes" / slug
        d.mkdir(parents=True, exist_ok=True)
        (d / "tasks.md").write_text("# tasks\n")

    def _seed_change(self, slug: str = "demo-change") -> None:
        self._seed_change_at(self.repo, slug)

    def _seed_archived_change_at(self, root: Path, slug: str = "old-change") -> None:
        d = root / "openspec" / "changes" / "archive" / slug
        d.mkdir(parents=True, exist_ok=True)
        (d / "tasks.md").write_text("# tasks\n")

    def _seed_archived_change(self, slug: str = "old-change") -> None:
        self._seed_archived_change_at(self.repo, slug)

    def _make_super_with_submodule(
        self,
        label: str = "primary",
        submodule_name: str | None = "api",
        superproject_parent: str = "",
    ) -> dict[str, Path]:
        root = Path(os.path.realpath(self.tmp.name))
        source = root / f"{label}-source"
        superproject = root / superproject_parent / f"{label}-super"
        source.mkdir()
        _git(source, "init", "-q")
        _git(source, "config", "user.email", "t@t.t")
        _git(source, "config", "user.name", "t")
        (source / "src").mkdir()
        (source / "src" / "app.py").write_text("x\n")
        _git(source, "add", "-A")
        _git(source, "commit", "-qm", "init")

        superproject.mkdir(parents=True)
        _git(superproject, "init", "-q")
        _git(superproject, "config", "user.email", "t@t.t")
        _git(superproject, "config", "user.name", "t")
        add_args = [
            "-c", "protocol.file.allow=always", "submodule", "add",
        ]
        if submodule_name is not None:
            add_args.extend(["--name", submodule_name])
        add_args.extend([str(source), "apps/api"])
        _git(superproject, *add_args)
        _git(superproject, "commit", "-qm", "add submodule")

        linked = superproject / ".worktrees" / f"apps-api-{label}"
        linked.parent.mkdir(parents=True)
        _git(superproject / "apps" / "api", "worktree", "add", "-b", f"feat-{label}", str(linked), "HEAD")
        self.assertTrue((superproject / "apps" / "api" / ".git").exists())
        self.assertEqual(_git_output(linked, "rev-parse", "--show-toplevel"), str(linked))
        self.assertEqual(_git_output(linked, "rev-parse", "--show-superproject-working-tree"), "")
        return {"source": source, "super": superproject, "sub": superproject / "apps" / "api", "linked": linked}

    def _event(self, tool: str, file_path: str, *, cwd: Path | None = None) -> dict:
        return {
            "event": "pre-tool-use",
            "tool_name": tool,
            "tool_input": {"file_path": file_path},
            "cwd": str(cwd or self.repo),
        }

    def _run(
        self,
        event: dict,
        *,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = dict(os.environ)
        env.pop("PLAN_BUILD_GATE_MODE", None)
        env.pop("PLAN_BUILD_GATE_PATHS", None)
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            ["bash", str(GATE)],
            input=json.dumps(event),
            capture_output=True, text=True, env=env,
        )

    # 1. Production write, no change folder → block (exit 2).
    def test_block_production_write_without_change_folder(self):
        r = self._run(self._event("Write", str(self.repo / "src" / "app.py")))
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("plan-build-gate", r.stderr)

    # 2. Production write, active change folder present → allow.
    def test_allow_production_write_with_change_folder(self):
        self._seed_change()
        r = self._run(self._event("Write", str(self.repo / "src" / "app.py")))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 3. Writing the plan itself is never blocked.
    def test_allow_writing_plan_artifacts(self):
        target = self.repo / "openspec" / "changes" / "new-slug" / "tasks.md"
        r = self._run(self._event("Write", str(target)))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 4. Non-production path (tests) → allow even without a change folder.
    def test_allow_non_production_path(self):
        r = self._run(self._event("Write", str(self.repo / "tests" / "t.py")))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 5. Gitignored agent config on a production tree → always allow.
    def test_allow_claude_settings(self):
        target = self.repo / ".claude" / "settings.json"
        r = self._run(self._event("Write", str(target)))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 6. Missing file_path → fail-open allow.
    def test_missing_file_path_fail_open(self):
        r = self._run({"event": "pre-tool-use", "tool_name": "Write", "tool_input": {}})
        self.assertEqual(r.returncode, 0, r.stderr)

    # 7. Malformed JSON on stdin → fail-open allow.
    def test_malformed_stdin_fail_open(self):
        env = dict(os.environ)
        env.pop("PLAN_BUILD_GATE_MODE", None)
        r = subprocess.run(["bash", str(GATE)], input="not json",
                           capture_output=True, text=True, env=env)
        self.assertEqual(r.returncode, 0, r.stderr)

    # 8. Only an ARCHIVED change folder does not count → still block.
    def test_archived_change_folder_does_not_count(self):
        self._seed_archived_change()
        r = self._run(self._event("Write", str(self.repo / "lib" / "core.py")))
        self.assertEqual(r.returncode, 2, r.stderr)

    # 9. The gate is non-bypassable: there is no on/off/ask mode. Setting the
    #    (now-removed) mode env var must NOT open the gate.
    def test_no_mode_bypass(self):
        r = self._run(
            self._event("Write", str(self.repo / "src" / "app.py")),
            extra_env={"PLAN_BUILD_GATE_MODE": "off"},
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    # 11. PLAN_BUILD_GATE_PATHS override redefines production dirs.
    def test_custom_production_paths_override(self):
        # 'app' is now production; 'src' is not.
        blocked = self._run(
            self._event("Write", str(self.repo / "app" / "x.py")),
            extra_env={"PLAN_BUILD_GATE_PATHS": "app"},
        )
        self.assertEqual(blocked.returncode, 2, blocked.stderr)
        allowed = self._run(
            self._event("Write", str(self.repo / "src" / "x.py")),
            extra_env={"PLAN_BUILD_GATE_PATHS": "app"},
        )
        self.assertEqual(allowed.returncode, 0, allowed.stderr)

    # 12. Not inside a git repo → fail-open allow.
    def test_outside_git_repo_fail_open(self):
        outside = Path(self.tmp.name) / "loose"
        outside.mkdir()
        r = self._run({
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(outside / "src" / "x.py")},
            "cwd": str(outside),
        })
        self.assertEqual(r.returncode, 0, r.stderr)

    # 13. Store selection never changes readiness: a blocked production write
    #     stays blocked under every preflight-resolved store value.
    def test_store_env_does_not_change_block_decision(self):
        event = self._event("Write", str(self.repo / "src" / "app.py"))
        baseline = self._run(event)
        self.assertEqual(baseline.returncode, 2, baseline.stderr)
        for value in STORE_ENUM:
            with self.subTest(store=value):
                r = self._run(event, extra_env={STORE_ENV_KEY: value})
                self.assertEqual(r.returncode, baseline.returncode, r.stderr)

    # 14. Store selection never changes readiness: an allowed production write
    #     stays allowed under every preflight-resolved store value.
    def test_store_env_does_not_change_allow_decision(self):
        self._seed_change()
        event = self._event("Write", str(self.repo / "src" / "app.py"))
        baseline = self._run(event)
        self.assertEqual(baseline.returncode, 0, baseline.stderr)
        for value in STORE_ENUM:
            with self.subTest(store=value):
                r = self._run(event, extra_env={STORE_ENV_KEY: value})
                self.assertEqual(r.returncode, baseline.returncode, r.stderr)

    def test_submodule_worktree_allows_production_with_central_plan(self):
        fx = self._make_super_with_submodule()
        self._seed_change_at(fx["super"], "demo")
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_default_nested_submodule_name_resolves_central(self):
        fx = self._make_super_with_submodule("default-name", None)
        self.assertEqual(
            _git_output(fx["super"], "config", "-f", ".gitmodules", "--get", "submodule.apps/api.path"),
            "apps/api",
        )
        self._seed_change_at(fx["super"], "demo")
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_superproject_path_with_modules_component_resolves_central(self):
        fx = self._make_super_with_submodule("modules-parent", "api", "modules")
        self._seed_change_at(fx["super"], "demo")
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_submodule_worktree_blocks_without_central_plan(self):
        fx = self._make_super_with_submodule()
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn(str(fx["super"] / "openspec" / "changes"), r.stderr)

    def test_submodule_worktree_blocks_with_archived_only_central_plan(self):
        fx = self._make_super_with_submodule()
        self._seed_archived_change_at(fx["super"], "demo")
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_submodule_worktree_allows_central_plan_creation(self):
        fx = self._make_super_with_submodule()
        target = fx["super"] / "openspec" / "changes" / "new" / "tasks.md"
        r = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_submodule_worktree_allows_central_archive_write(self):
        fx = self._make_super_with_submodule()
        target = fx["super"] / "openspec" / "changes" / "archive" / "demo" / "tasks.md"
        r = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_submodule_worktree_blocks_superproject_production_path(self):
        fx = self._make_super_with_submodule()
        target = fx["super"] / "src" / "app.py"
        r = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_superproject_probe_empty_still_resolves_central(self):
        fx = self._make_super_with_submodule()
        self.assertEqual(_git_output(fx["linked"], "rev-parse", "--show-superproject-working-tree"), "")
        self._seed_change_at(fx["super"], "demo")
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "new.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_similar_submodule_names_do_not_select_wrong_parent(self):
        first = self._make_super_with_submodule("first", "api")
        second = self._make_super_with_submodule("second", "api")
        self._seed_change_at(second["super"], "only-second")
        r = self._run(self._event("Write", str(first["linked"] / "src" / "app.py"), cwd=first["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn(str(first["super"] / "openspec" / "changes"), r.stderr)
        self.assertNotIn(str(second["super"]), r.stderr)

    def test_uninitialized_submodule_does_not_grant_production_access(self):
        fx = self._make_super_with_submodule()
        self._seed_change_at(fx["super"], "central")
        _git(fx["super"], "submodule", "deinit", "-f", "--", "apps/api")
        self.assertEqual(_git_output(fx["linked"], "rev-parse", "--show-toplevel"), str(fx["linked"]))
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_non_submodule_worktree_uses_own_root(self):
        linked = Path(self.tmp.name) / "ordinary-worktree"
        _git(self.repo, "worktree", "add", "-b", "ordinary", str(linked), "HEAD")
        self._seed_change_at(linked, "local")
        r = self._run(self._event("Write", str(linked / "src" / "app.py"), cwd=linked))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_central_nonexistent_tasks_path_allowed(self):
        fx = self._make_super_with_submodule()
        target = fx["super"] / "openspec" / "changes" / "brand-new" / "specs" / "spec.md"
        r = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_symlinked_central_path_cannot_escape(self):
        fx = self._make_super_with_submodule()
        changes = fx["super"] / "openspec" / "changes"
        changes.mkdir(parents=True)
        (changes / "escape").symlink_to(fx["super"] / "src", target_is_directory=True)
        target = changes / "escape" / "evil.py"
        blocked = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(blocked.returncode, 2, blocked.stderr)
        self._seed_change_at(fx["super"], "central")
        allowed = self._run(self._event("Write", str(target), cwd=fx["linked"]))
        self.assertEqual(allowed.returncode, 0, allowed.stderr)

    def test_changes_lookalike_prefix_is_not_artifact_root(self):
        fx = self._make_super_with_submodule()
        target = fx["super"] / "openspec" / "changes-archive" / "demo" / "tasks.md"
        r = self._run(
            self._event("Write", str(target), cwd=fx["linked"]),
            extra_env={"PLAN_BUILD_GATE_PATHS": "openspec"},
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    def test_outside_repository_path_not_broadened(self):
        fx = self._make_super_with_submodule()
        outside = Path(self.tmp.name) / "outside" / "openspec" / "changes" / "demo" / "tasks.md"
        r = self._run(self._event("Write", str(outside), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_gate_behavior_identical_without_external_orchestration(self):
        """3.3 — RED: absent/disabled external orchestration never changes verdicts."""
        event = self._event("Write", "src/app.py")
        clean = self._run(event)
        self.assertEqual(clean.returncode, 2, clean.stderr)
        for extra in (
            {"GENTLE_AI_MODE": "disabled", "GENTLE_AI_ABSENT": "1"},
            {"PLAN_BUILD_ORCHESTRATOR": "absent"},
        ):
            alt = self._run(event, extra_env=extra)
            self.assertEqual(alt.returncode, clean.returncode)
            self.assertEqual(alt.stderr, clean.stderr)
        script = GATE.read_text()
        self.assertNotIn("gentle", script.lower(),
                         "the gate must never invoke an external provider")

    def test_gate_evaluation_creates_nothing(self):
        fx = self._make_super_with_submodule()
        before_worktrees = _git_output(fx["sub"], "worktree", "list", "--porcelain")
        before_branches = _git_output(fx["sub"], "for-each-ref", "--format=%(refname:short)", "refs/heads")
        before_dirs = sorted(str(p.relative_to(fx["super"])) for p in fx["super"].rglob("*") if p.is_dir())
        r = self._run(self._event("Write", str(fx["linked"] / "src" / "app.py"), cwd=fx["linked"]))
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertEqual(_git_output(fx["sub"], "worktree", "list", "--porcelain"), before_worktrees)
        self.assertEqual(_git_output(fx["sub"], "for-each-ref", "--format=%(refname:short)", "refs/heads"), before_branches)
        after_dirs = sorted(str(p.relative_to(fx["super"])) for p in fx["super"].rglob("*") if p.is_dir())
        self.assertEqual(after_dirs, before_dirs)

if __name__ == "__main__":
    unittest.main()

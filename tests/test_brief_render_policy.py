"""Black-box tests for the doctor-visible half of the brief render policy.

The [brief].render verdict matrix (default enabled, explicit true/false, the
fail-safe non-validate default, the --validate rejection and the TOML-decode
exit) is native: ``internal/sync/briefpolicy_test.go`` asserts the verdicts and
its frozen differential pins the rc/stdout/stderr of every manifest × mode pair
that the retired ``lib/_internal/brief-render-policy.py`` CLI produced. What is
left here is the doctor-level contract — an enabled recipe whose brief
fragments can never render is reported as ``brief-fragments-unused``.
"""
from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    """One shared isolated install root per command sequence."""
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = isolated_home(base)
    return _HOMES[key]


class BriefRenderPolicyDoctorTests(unittest.TestCase):
    """Dead-recipe-fragment detection through the doctor verb."""

    def _doctor_project(self, toml: str) -> Path:
        td = tempfile.TemporaryDirectory(prefix="ai-specs-doctor-")
        self.addCleanup(td.cleanup)
        project = Path(td.name) / "project"
        (project / "ai-specs" / "skills").mkdir(parents=True)
        (project / "ai-specs" / "commands").mkdir()
        (project / "ai-specs" / "ai-specs.toml").write_text(toml)
        # A pre-existing AGENTS.md keeps the unrelated agents-md check quiet.
        (project / "AGENTS.md").write_text("# manual brief\n")
        return project

    def test_has_dead_recipe_fragments_true(self):
        """An enabled recipe declaring brief fragments under render=false is dead.

        doctor surfaces has_dead_recipe_fragments as the brief-fragments-unused
        WARN — that is the CLI-observable form of this contract.
        """
        project = self._doctor_project(
            "[project]\nname = 'dead-fragments'\n\n"
            "[brief]\nrender = false\n\n"
            "[recipes.session-context]\nenabled = true\nversion = '2.0.0'\n"
        )
        result = invoke(project, "doctor", cli_home=_home_for(project))
        self.assertIn(
            "brief-fragments-unused",
            result.stdout + result.stderr,
            "doctor must WARN on enabled recipes whose brief fragments can never render",
        )

    def test_has_dead_recipe_fragments_false(self):
        """An enabled recipe WITHOUT brief fragments is not dead.

        Uses a fresh custom recipe (no [provides.brief]) inside the isolated
        home so no catalog recipe's fragments leak into the assertion.
        """
        project = self._doctor_project(
            "[project]\nname = 'live-fragments'\n\n"
            "[brief]\nrender = false\n\n"
            "[recipes.policy-plain-recipe]\nenabled = true\n"
        )
        home = _home_for(project)
        populate_catalog(home, "policy-plain-recipe")
        result = invoke(project, "doctor", cli_home=home)
        combined = result.stdout + result.stderr
        self.assertIn("brief-provenance", combined, "doctor must run")
        self.assertNotIn(
            "brief-fragments-unused",
            combined,
            "a recipe without brief fragments must not be reported dead",
        )


if __name__ == "__main__":
    unittest.main()

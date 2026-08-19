"""Black-box tests for the [brief].render AGENTS.md rendering policy.

Each test drives `bin/ai-specs sync` / `bin/ai-specs doctor` against a
per-test project manifest and observes the runtime brief rendering policy
through the shipped CLI. No lib/_internal imports, no direct lib/*.sh runs.
"""

# Probe report — every literal asserted below was captured from a real
# `bin/ai-specs` invocation during this conversion (worktree bb-misc):
#   - sync, no [brief] table / empty [brief] / render = true / render = "false":
#       rc 0, AGENTS.md rendered at the project root                (stdout/stderr clean)
#   - sync, [brief] render = false (agent enabled): rc 1, stdout `  ℹ skipped
#       AGENTS.md (brief.render = false)`; root sync-agent fails because
#       AGENTS.md is missing -> stderr `ERROR: <TEMP>/AGENTS.md not found. Run
#       'ai-specs init <TEMP>' first.` Then AGENTS.md absent.
#   - sync, render = True (uppercase, invalid TOML): rc 1, stderr
#       `error: Invalid value (at line N, column 10)` then
#       `ERROR: target resolution failed before any writes.`
#   - doctor, render = "false" (string): stdout (doctor table)
#       `  ERROR  brief-render     [brief].render must be a boolean (true or
#       false); got str  (use true or false in lowercase)`
#   - doctor, render = 1 (int): stdout `... got int ...` (same shape as above)
#   - doctor, render = false + manual AGENTS.md (no recipe): stdout
#       `  INFO   brief-render  managed AGENTS.md rendering disabled
#       ([brief].render = false)`
#   - doctor, render = false + manual AGENTS.md + enabled recipe that declares
#       [provides.brief]: stdout `  WARN   brief-fragments-unused  enabled
#       recipes declare [provides.brief] but render = false` (no such WARN in
#       the no-recipe control).
# Streams: doctor diagnostics are emitted on STDOUT (the doctor table); sync
# value-resolution failures on STDERR; sync step lines on STDOUT.
import tempfile
import unittest
from pathlib import Path

from _blackbox import CLIResult, invoke, isolated_home

BASE_MANIFEST = (
    '[project]\n'
    'name = "brief-policy"\n'
    '[agents]\n'
    'enabled = ["claude"]\n'
)


class BriefRenderPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # Shared isolated install home, held on the class so it is not GC'd mid-
        # test (a GC'd home breaks bin/ai-specs with rc 127).
        cls._home_holder = tempfile.TemporaryDirectory(prefix="brief-policy-home-")
        cls.home = isolated_home(Path(cls._home_holder.name))

    @classmethod
    def tearDownClass(cls):
        cls._home_holder.cleanup()

    def _proj_dir(self) -> Path:
        # A fresh per-test project directory (one per test method name).
        proj = Path(self._home_holder.name) / "projects" / self._testMethodName
        (proj / "ai-specs").mkdir(parents=True, exist_ok=True)
        return proj

    def _sync(self, proj: Path) -> CLIResult:
        return invoke(proj, "sync", cli_home=self.home)

    def _doctor(self, proj: Path) -> CLIResult:
        return invoke(proj, "doctor", cli_home=self.home)

    def test_no_brief_table_defaults_true(self):
        # Observed probe: no [brief] table -> sync rc 0, AGENTS.md rendered at
        # the project root.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST)
        r = self._sync(proj)
        self.assertEqual(r.returncode, 0)
        self.assertTrue((proj / "AGENTS.md").is_file())

    def test_brief_without_render_defaults_true(self):
        # Observed probe: empty [brief] table -> sync rc 0, AGENTS.md rendered.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\n")
        r = self._sync(proj)
        self.assertEqual(r.returncode, 0)
        self.assertTrue((proj / "AGENTS.md").is_file())

    def test_render_true(self):
        # Observed probe: [brief] render = true -> sync rc 0, AGENTS.md rendered.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = true\n")
        r = self._sync(proj)
        self.assertEqual(r.returncode, 0)
        self.assertTrue((proj / "AGENTS.md").is_file())

    def test_render_false(self):
        # Observed probe: [brief] render = false on a clean project -> sync
        # skips AGENTS.md and the root sync-agent target then fails because the
        # file is missing -> rc 1, AGENTS.md not rendered.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = false\n")
        r = self._sync(proj)
        self.assertEqual(r.returncode, 1)
        self.assertIn("  ℹ skipped AGENTS.md (brief.render = false)", r.stdout)
        self.assertFalse((proj / "AGENTS.md").exists())

    def test_render_string_raises(self):
        # Observed probe: doctor rejects a string-shaped render value with an
        # ERROR titled brief-render whose message (on stdout, doctor table)
        # names [brief].render.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + '\n[brief]\nrender = "false"\n')
        r = self._doctor(proj)
        self.assertIn("brief-render", r.stdout)
        self.assertIn("[brief].render", r.stdout)

    def test_render_int_raises(self):
        # Observed probe: doctor rejects an int-shaped render value with the
        # same brief-render ERROR naming [brief].render.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = 1\n")
        r = self._doctor(proj)
        self.assertIn("brief-render", r.stdout)
        self.assertIn("[brief].render", r.stdout)

    def test_load_from_toml_file(self):
        # A manifest file carrying [brief] render = false loads as "disabled":
        # doctor reports the managed AGENTS.md rendering disabled INFO line
        # (observed when a manual AGENTS.md is present alongside render=false).
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = false\n")
        (proj / "AGENTS.md").write_text("# Manual runtime brief\n")
        r = self._doctor(proj)
        self.assertIn("brief-render", r.stdout)
        self.assertIn("managed AGENTS.md rendering disabled ([brief].render = false)", r.stdout)

    def test_cli_prints_false(self):
        # TRIAGE: the internal policy script (lib/_internal/brief-render-policy.py)
        # has NO bin/ai-specs verb, so there is no standalone CLI to print
        # "false". Through the shipped CLI the equivalent observable is the sync
        # skip line emitted when a manifest sets render = false — the policy
        # value "false" is what drives the skip decision.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = false\n")
        r = self._sync(proj)
        self.assertEqual(r.returncode, 1)
        self.assertIn("skipped AGENTS.md (brief.render = false)", r.stdout)

    def test_render_uppercase_true_is_toml_error(self):
        # Observed probe: `render = True` (capitalized) is invalid TOML; sync
        # fails before any writes with "Invalid value" / "target resolution
        # failed", rc 1, AGENTS.md never rendered.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = True\n")
        r = self._sync(proj)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("Invalid value", r.stderr)
        self.assertIn("target resolution failed", r.stderr)

    def test_cli_non_validate_invalid_render_defaults_to_true(self):
        # Observed probe: a string render value ("false") is treated as the
        # fail-safe default (enabled): sync rc 0 and AGENTS.md rendered.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + '\n[brief]\nrender = "false"\n')
        r = self._sync(proj)
        self.assertEqual(r.returncode, 0)
        self.assertTrue((proj / "AGENTS.md").is_file())

    def test_cli_validate_rejects_string(self):
        # The validate path (doctor always validates): a string render value is
        # rejected with the brief-render ERROR and guidance telling the user to
        # use a lowercase boolean.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + '\n[brief]\nrender = "false"\n')
        r = self._doctor(proj)
        self.assertIn("brief-render", r.stdout)
        self.assertIn("use true or false in lowercase", r.stdout)

    def test_has_dead_recipe_fragments_true(self):
        # Observed probe: doctor emits a brief-fragments-unused WARNING when an
        # enabled recipe declares [provides.brief] and render = false.
        toml = (
            BASE_MANIFEST
            + "\n[brief]\nrender = false\n"
            + "\n[recipes.session-context]\nenabled = true\n"
        )
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(toml)
        (proj / "AGENTS.md").write_text("# Manual runtime brief\n")
        r = self._doctor(proj)
        self.assertIn("brief-fragments-unused", r.stdout)
        self.assertIn("enabled recipes declare [provides.brief] but render = false", r.stdout)

    def test_has_dead_recipe_fragments_false(self):
        # Control: with no recipe declaring [provides.brief], doctor emits no
        # brief-fragments-unused line even though render = false.
        proj = self._proj_dir()
        (proj / "ai-specs" / "ai-specs.toml").write_text(BASE_MANIFEST + "\n[brief]\nrender = false\n")
        (proj / "AGENTS.md").write_text("# Manual runtime brief\n")
        r = self._doctor(proj)
        self.assertNotIn("brief-fragments-unused", r.stdout)


if __name__ == "__main__":
    unittest.main()

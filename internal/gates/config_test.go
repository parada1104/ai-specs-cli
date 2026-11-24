package gates

// Project config (plan-build-openspec-odd-align P5/S11): openspec/config.yaml is
// the project's configuration. The CLI validates only the keys it consumes --
// containment, `schema:`, `testing.runner:` and a boolean `strict_tdd:` -- and
// keeps the declared runner fresh against the plan's `## Verification` rows.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// projectConfig is a valid config whose opaque prose is deliberately messy: the
// CLI must ignore `context`, `rules.*` and `ai_specs_guidance`.
const projectConfig = `schema: spec-driven

context: |
  Tech stack: Go CLI with a Python parity baseline.
  Testing: ./tests/run.sh is not the runner of this project.

strict_tdd: true

testing:
  detected: 2026-04-24
  runner: go test ./internal/...
  layers:
    unit:
      tool: go test

rules:
  apply:
    - Use ./tests/run.sh for focused feedback (opaque prose, never validated)

ai_specs_guidance:
  apply:
    tdd: true
    test_command: ./tests/run.sh
`

// writeProjectConfig writes the fixture config at the mode's resolved path.
func writeProjectConfig(t *testing.T, root, body string) {
	t.Helper()
	writeFile(t, planbuild.ConfigPath(root, planbuild.ModeOpenspec), body)
}

// A relative root is graded exactly like the absolute one (R4): the config
// containment resolves both through planbuild's helper.
func TestReadProjectConfigAcceptsARelativeRoot(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, projectConfig)
	t.Chdir(root)
	if _, err := ReadProjectConfig(".", planbuild.ModeOpenspec); err != nil {
		t.Fatalf("ReadProjectConfig(\".\") = %v, want no error", err)
	}
}

func TestReadProjectConfigReadsTheConsumedKeys(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, projectConfig)

	cfg, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err != nil {
		t.Fatalf("ReadProjectConfig = %v, want no error", err)
	}
	if cfg.Schema != "spec-driven" {
		t.Errorf("Schema = %q, want spec-driven", cfg.Schema)
	}
	if cfg.Runner != "go test ./internal/..." {
		t.Errorf("Runner = %q, want go test ./internal/...", cfg.Runner)
	}
	if cfg.StrictTDD == nil || !*cfg.StrictTDD {
		t.Errorf("StrictTDD = %v, want true", cfg.StrictTDD)
	}
}

// The opaque prose is never validated: a `context` block whose lines look like
// keys must not be read as configuration.
func TestReadProjectConfigIgnoresOpaqueProse(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, "schema: spec-driven\n\ncontext: |\n  runner: this is prose\n"+
		"\nstrict_tdd: false\n\ntesting:\n  runner: go test ./internal/...\n")

	cfg, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err != nil {
		t.Fatalf("ReadProjectConfig = %v, want no error", err)
	}
	if cfg.Runner != "go test ./internal/..." {
		t.Errorf("Runner = %q, want the testing block value", cfg.Runner)
	}
	if cfg.StrictTDD == nil || *cfg.StrictTDD {
		t.Errorf("StrictTDD = %v, want false", cfg.StrictTDD)
	}
}

func TestReadProjectConfigBlocksWithoutTestingRunner(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, "schema: spec-driven\n\ntesting:\n  detected: 2026-04-24\n")

	_, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err == nil {
		t.Fatal("a config without testing.runner must block")
	}
	want := "config.yaml: testing.runner is missing: declare the command that runs the project tests"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestReadProjectConfigBlocksAnEmptySchema(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, "schema:\n\ntesting:\n  runner: go test ./internal/...\n")

	if _, err := ReadProjectConfig(root, planbuild.ModeOpenspec); err == nil ||
		!strings.Contains(err.Error(), "config.yaml: schema is missing") {
		t.Fatalf("error = %v, want the schema blocker", err)
	}
}

func TestReadProjectConfigValidatesStrictTDD(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, "schema: spec-driven\nstrict_tdd: maybe\n\n"+
		"testing:\n  runner: go test ./internal/...\n")
	_, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err == nil {
		t.Fatal("strict_tdd: maybe must block")
	}
	want := "config.yaml: strict_tdd must be true or false: write strict_tdd: true or strict_tdd: false"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}

	// Absent is accepted: the key is optional.
	writeProjectConfig(t, root, "schema: spec-driven\n\ntesting:\n  runner: go test ./internal/...\n")
	cfg, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err != nil {
		t.Fatalf("ReadProjectConfig without strict_tdd = %v, want no error", err)
	}
	if cfg.StrictTDD != nil {
		t.Errorf("StrictTDD = %v, want nil when absent", cfg.StrictTDD)
	}
}

func TestReadProjectConfigBlocksASymlinkOutsideTheRepository(t *testing.T) {
	outside := t.TempDir()
	external := filepath.Join(outside, "config.yaml")
	if err := os.WriteFile(external, []byte(projectConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "openspec"))
	if err := os.Symlink(external, planbuild.ConfigPath(root, planbuild.ModeOpenspec)); err != nil {
		t.Fatal(err)
	}

	_, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err == nil {
		t.Fatal("a config symlinked outside the repository must block")
	}
	want := "config.yaml: the project config is missing or is not a regular file inside the " +
		"repository: write openspec/config.yaml as a file of this repository"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// A Light change carries no project config duty: that is the ceremony reduction
// of S12.
func TestLightChangePassesWithoutAProjectConfig(t *testing.T) {
	root := newBareRepo(t)
	archiveWith(t, root, "light-no-config", "Depth: light\n", archiveOpts{})

	if result := CheckPremerge(root, "light-no-config", tierOpt(TierLight)); !result.OK {
		t.Fatalf("light = %+v, want OK without a project config", result)
	}
}

// A Standard change whose rows name the configured runner passes.
func TestStandardChangeWithAConfiguredGoTestPasses(t *testing.T) {
	root := newRepo(t)
	writeProjectConfig(t, root, projectConfig)
	archiveWith(t, root, "std-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-config", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK with a fresh config", result)
	}
}

// A Standard change without a config blocks: the file is required from Standard
// up, and the blocker names the path.
func TestStandardChangeBlocksWithoutAProjectConfig(t *testing.T) {
	root := newBareRepo(t)
	archiveWith(t, root, "std-no-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-no-config", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a Standard change without a project config must block")
	}
	assertBlockerContains(t, result,
		"config.yaml: the project config is missing or is not a regular file inside the repository")
}

// A verification row naming a command that cannot run blocks.
func TestVerificationNamingAMissingCommandBlocks(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-missing-command", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `./nope.sh`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-missing-command", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a verification row naming a missing command must block")
	}
	assertBlockerContains(t, result,
		`verification for S1 names command "./nope.sh" which does not exist: `+
			"run the configured tests and record the real command")
}

// A verification set that never names the configured runner blocks.
func TestVerificationThatNeverRunsTheConfiguredRunnerBlocks(t *testing.T) {
	root := newRepo(t)
	writeProjectConfig(t, root, "schema: spec-driven\n\ntesting:\n  runner: ./tests/run.sh\n")
	archiveWith(t, root, "std-stale-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-stale-config", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a verification set that never runs the configured runner must block")
	}
	assertBlockerContains(t, result,
		`verification never runs the configured testing.runner "./tests/run.sh": `+
			"name it in at least one row or update config.yaml")
}

// One matching row is enough: the runner is a declaration, not every row.
func TestConfiguredRunnerMatchingOneRowPasses(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	root := newRepo(t)
	writeProjectConfig(t, root, "schema: spec-driven\n\ntesting:\n  runner: go test ./internal/...\n")
	archiveWith(t, root, "std-mixed-rows", "Depth: standard\n\n"+verificationPlan(2,
		"- **S1: PASS** — `go vet ./internal/...`, exit 0\n"+
			"- **S2: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-mixed-rows", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK because S2 runs the configured runner", result)
	}
}

// A row with no command now blocks at Standard (F4), and the freshness rules
// themselves keep their "nothing to keep fresh" case: a plan with no rows is not
// kept fresh by either rule (ceremony reduction, S12).
func TestChangeWithoutCommandsBlocksAndTheFreshnessRulesStayQuiet(t *testing.T) {
	root := newRepo(t)
	writeProjectConfig(t, root, "schema: spec-driven\n\ntesting:\n  runner: ./tests/run.sh\n")
	archiveWith(t, root, "std-no-commands", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — covered by hand\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-no-commands", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("standard with no backticked command must block")
	}
	assertBlockerContains(t, result,
		`verification for S1 must name the command it ran in backticks: record "- **S1: PASS** — `+
			"`go test ./...`\"")
	assertBlockerLacks(t, result, "never runs the configured testing.runner")

	cfg, err := ReadProjectConfig(root, planbuild.ModeOpenspec)
	if err != nil {
		t.Fatal(err)
	}
	if blockers := checkConfigFreshness(root, "openspec/config.yaml", cfg, Plan{}); len(blockers) != 0 {
		t.Fatalf("a plan with no rows = %q, want no blockers", blockers)
	}
}

// This repository's own config must satisfy the validator it ships: the stale
// Bash/Python file is the reason the freshness loop exists (S11).
func TestRepositoryProjectConfigIsValid(t *testing.T) {
	cfg, err := ReadProjectConfig(docsRepoRoot(t), planbuild.ModeOpenspec)
	if err != nil {
		t.Fatalf("openspec/config.yaml: %v", err)
	}
	if cfg.Runner != "go test ./internal/..." {
		t.Errorf("Runner = %q, want the Go runner of this repository", cfg.Runner)
	}
}

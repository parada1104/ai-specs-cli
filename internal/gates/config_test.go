package gates

// plan-build-openspec-odd-align D1: `openspec/config.yaml` is an optional
// project artefact that OpenSpec and agents may read. The CLI neither requires
// nor grades it, so the former schema/testing.runner/strict_tdd validation and
// the runner freshness matching are gone. The one surviving rule is that every
// command recorded in a `## Verification` row must resolve on disk.

import (
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// writeProjectConfig writes a config at the openspec config path.
func writeProjectConfig(t *testing.T, root, body string) {
	t.Helper()
	writeFile(t, planbuild.ConfigPath(root), body)
}

// A Light change carries no verification duty at all.
func TestLightChangePassesWithoutAProjectConfig(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "light-no-config", "Depth: light\n", archiveOpts{})

	if result := CheckPremerge(root, "light-no-config", tierOpt(TierLight)); !result.OK {
		t.Fatalf("light = %+v, want OK without a project config", result)
	}
}

// A Standard change whose rows record resolvable commands passes, with or
// without a config file beside it.
func TestStandardChangeWithAProjectConfigStillPasses(t *testing.T) {
	root := newRepo(t)
	writeProjectConfig(t, root, "schema: spec-driven\n\nstrict_tdd: true\n\n"+
		"testing:\n  runner: go test ./internal/...\n")
	archiveWith(t, root, "std-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-config", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK with a config present", result)
	}
}

// A Standard change no longer needs a project config: the premerge does not
// depend on the OpenSpec CLI or on openspec/config.yaml.
func TestStandardChangePassesWithoutAProjectConfig(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-no-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-no-config", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK without a project config", result)
	}
}

// A config that would have blocked is dead weight: the file is a project
// artefact the CLI neither requires nor grades.
func TestProjectConfigIsAnOptionalUngradedFile(t *testing.T) {
	root := newRepo(t)
	writeProjectConfig(t, root, "schema:\n\nstrict_tdd: maybe\n\ntesting:\n  runner: ./tests/run.sh\n")
	archiveWith(t, root, "std-ignored-config", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-ignored-config", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want the config ignored", result)
	}
}

// The one surviving rule: every command recorded in a `## Verification` row must
// resolve on disk, and the blocker names the plan that records it.
func TestVerificationCommandMustResolveAgainstThePlanPath(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-missing-command", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `./nope.sh`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-missing-command", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a verification row naming a missing command must block")
	}
	want := `openspec/changes/archive/std-missing-command/tasks.md: verification for S1 ` +
		`names command "./nope.sh" which does not exist: record the real command`
	if got := joined(result); !strings.Contains(got, strings.ToLower(want)) {
		t.Fatalf("blockers %q do not contain %q", result.Blockers, want)
	}
}

// A bare word that is not on PATH does not resolve either.
func TestVerificationBareWordNotOnPathBlocks(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-missing-binary", "Depth: standard\n\n"+verificationPlan(1,
		"- **S1: PASS** — `definitely-not-a-real-binary --run`, exit 0\n"), archiveOpts{spec: true})

	result := CheckPremerge(root, "std-missing-binary", tierOpt(TierStandard))
	if result.OK {
		t.Fatal("a bare word not on PATH must block")
	}
	assertBlockerContains(t, result, `names command "definitely-not-a-real-binary --run" which does not exist`)
}

// An N/A row records no run, so its backticked text is never resolved (C3).
func TestVerificationNARowCommandIsNotResolved(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-na-command", "Depth: standard\n\n"+verificationPlan(2,
		"- **S1: PASS** — `go test ./internal/gates/...`, exit 0\n"+
			"- **S2: N/A** — `./nope.sh` is not run\n"), archiveOpts{spec: true})

	if result := CheckPremerge(root, "std-na-command", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK: an N/A row records no run", result)
	}
}

// A legacy report with no `## Verification` rows records no command, so the
// command rule stays quiet (ceremony reduction).
func TestLegacyChangeWithoutRowsIsNotSubjectToTheCommandRule(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "std-no-rows", "Depth: standard\n", archiveOpts{
		spec: true, report: repoString(standardReport),
	})

	if result := CheckPremerge(root, "std-no-rows", tierOpt(TierStandard)); !result.OK {
		t.Fatalf("standard = %+v, want OK without verification rows", result)
	}
}

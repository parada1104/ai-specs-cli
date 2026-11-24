package cli

// `ai-specs gates premerge <slug> --root <planning-root> [--tier ...] [--stage ...]`
// (GO-20 U2e, unit U-B): the native verb for the pre-merge guardian, the port of
// lib/_internal/premerge_guardian.py's main(). The stdout/stderr bytes and exit
// codes pinned here are the Python contract's, so the verb is a drop-in
// replacement for the consumer docs (plan-build-flow SKILL.md).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gatesStandardReport = "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
	"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n"

// gatesPlanBody is the conforming plan every conforming fixture carries; the
// grammar itself is covered by internal/gates. `## Log` is present because
// standard/full require it (R7).
const gatesPlanBody = "## Specs\n\n- **S1.** Fixture requirement\n\n## Tasks\n\n" +
	"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
	"| T1 | S1 | Fixture task | inline | pending |\n\n" +
	"## Log\n\n- **L1** — fixture request\n"

// gatesConfig is a valid project config: `go` is on PATH, so a fixture that
// records a command keeps the freshness rules quiet.
const gatesConfig = "schema: spec-driven\n\nstrict_tdd: true\n\ntesting:\n  runner: go test ./internal/...\n"

// gatesRepo is an empty planning tree. It carries a valid project config, since
// the CLI requires one from Standard up; a test about the config rules removes
// that duty by using its own root.
func gatesRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	gatesWrite(t, filepath.Join(root, "openspec", "config.yaml"), gatesConfig)
	return root
}

func gatesWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gatesConformingArchive writes a Standard archive that passes every gate.
func gatesConformingArchive(t *testing.T, root, slug string) string {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", "archive", slug)
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: standard\n\n"+gatesPlanBody)
	gatesWrite(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	gatesWrite(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	gatesWrite(t, filepath.Join(folder, "verify-report.md"), gatesStandardReport)
	return folder
}

func runGatesCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, t.TempDir(), nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// A relative root grades exactly like the absolute one (R4): CI ships `--root .`.
func TestGatesPremergeRelativeRootAgreesWithAbsolute(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "ok-change")

	absoluteCode, absoluteOut, absoluteErr := runGatesCLI(t,
		"gates", "premerge", "ok-change", "--root", root)
	t.Chdir(root)
	relativeCode, relativeOut, relativeErr := runGatesCLI(t,
		"gates", "premerge", "ok-change", "--root", ".")

	if relativeCode != absoluteCode || relativeOut != absoluteOut || relativeErr != absoluteErr {
		t.Fatalf("--root . = (%d, %q, %q), --root %s = (%d, %q, %q); want the same verdict",
			relativeCode, relativeOut, relativeErr, root, absoluteCode, absoluteOut, absoluteErr)
	}
	if relativeCode != 0 {
		t.Fatalf("exit = %d, stderr = %q; want 0", relativeCode, relativeErr)
	}
}

// The graded change becomes a path inside the planning tree, so a name that is
// not a single segment is a usage error, never a folder outside the change tree
// (R5).
func TestGatesPremergeRefusesAnInvalidChangeName(t *testing.T) {
	root := gatesRepo(t)
	// Without the refusal, ../../../offtree resolves to a real graded folder.
	gatesWrite(t, filepath.Join(root, "offtree", "tasks.md"), "Depth: light\n\n"+gatesPlanBody)

	for _, args := range [][]string{
		{"gates", "premerge", "--change", "../../../offtree", "--root", root},
		{"gates", "premerge", "..", "--root", root},
	} {
		code, stdout, stderr := runGatesCLI(t, args...)
		if code != 2 || stdout != "" {
			t.Fatalf("%q = (%d, %q); want 2/empty", args, code, stdout)
		}
		if !strings.Contains(stderr, "ERROR: invalid change name:") ||
			!strings.Contains(stderr, "(expected a single path segment)") {
			t.Fatalf("%q stderr = %q; want the invalid-name usage error", args, stderr)
		}
	}
}

func TestGatesPremergeOKPrintsTheTier(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "ok-change")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
	if stdout != "premerge-guardian: OK (standard)\n" {
		t.Errorf("stdout = %q, want the OK line with the resolved tier", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestGatesPremergeBlockedPrintsTheBlockerList(t *testing.T) {
	root := gatesRepo(t)
	gatesWrite(t, filepath.Join(root, "openspec", "changes", "archive", "2026-01-02-bad-change", "tasks.md"),
		"Depth: standard\n")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "bad-change", "--root", root)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "premerge-guardian: BLOCKED\n  - ") {
		t.Fatalf("stderr = %q, want the BLOCKED header then indented blockers", stderr)
	}
	for _, want := range []string{
		"  - archive/2026-01-02-bad-change/specs/ must contain at least one *.md for tier standard\n",
		"  - archive/2026-01-02-bad-change/tasks.md: `## Specs` section is missing: add the `## Specs` section above `## Tasks`\n",
		"  - archive/2026-01-02-bad-change/tasks.md: `## Tasks` section is missing: add the `## Tasks` table\n",
		"  - standard verify evidence requires dedicated verify-report.md\n",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q does not contain %q", stderr, want)
		}
	}
}

func TestGatesPremergePreArchiveStageChecksTheActiveFolder(t *testing.T) {
	root := gatesRepo(t)
	active := filepath.Join(root, "openspec", "changes", "active-change")
	gatesWrite(t, filepath.Join(active, "tasks.md"), "Depth: light\n\n"+gatesPlanBody)
	gatesWrite(t, filepath.Join(active, "proposal.md"), "# proposal\n")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "active-change", "--root", root,
		"--stage", "pre-archive")
	if code != 0 || stdout != "premerge-guardian: OK (light)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want 0 and the light OK line", code, stdout, stderr)
	}

	gatesWrite(t, filepath.Join(root, "openspec", "changes", "active-change", "tasks.md"),
		"Depth: standard\n\n"+gatesPlanBody)
	code, stdout, stderr = runGatesCLI(t, "gates", "premerge", "active-change", "--root", root,
		"--stage", "pre-archive")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1 and empty stdout", code, stdout)
	}
	if !strings.Contains(stderr, "standard verify evidence requires dedicated verify-report.md") {
		t.Errorf("stderr = %q, want the verify-report blocker", stderr)
	}
}

func TestGatesPremergeExplicitTierOverridesTheDepthLine(t *testing.T) {
	root := gatesRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "archive", "full-change")
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: light\n\n"+gatesPlanBody)
	gatesWrite(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	gatesWrite(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	gatesWrite(t, filepath.Join(folder, "verify-report.md"), gatesStandardReport)

	code, stdout, _ := runGatesCLI(t, "gates", "premerge", "full-change", "--root", root, "--tier", "standard")
	if code != 0 || stdout != "premerge-guardian: OK (standard)\n" {
		t.Fatalf("exit = %d, stdout = %q; want the explicit tier", code, stdout)
	}

	code, stdout, _ = runGatesCLI(t, "gates", "premerge", "full-change", "--root", root, "--tier", "full")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want the full tier to block", code, stdout)
	}
}

func TestGatesPremergeRequiresTheRootFlagAndNeverFallsBackToCwd(t *testing.T) {
	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "some-slug")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "--root") || !strings.Contains(stderr, "--help") {
		t.Errorf("stderr = %q, want the --root requirement and a --help pointer", stderr)
	}
}

func TestGatesPremergeArgumentErrors(t *testing.T) {
	root := gatesRepo(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing slug", []string{"gates", "premerge", "--root", root}, "missing required argument: <slug>"},
		{"extra positional", []string{"gates", "premerge", "a", "b", "--root", root}, "unexpected positional argument: b"},
		{"unknown flag", []string{"gates", "premerge", "a", "--root", root, "--bogus"}, "unknown flag: --bogus"},
		{"root without value", []string{"gates", "premerge", "a", "--root"}, "--root requires a value"},
		{"bad tier", []string{"gates", "premerge", "a", "--root", root, "--tier", "deep"}, "invalid --tier value"},
		{"bad stage", []string{"gates", "premerge", "a", "--root", root, "--stage", "post-merge"}, "invalid --stage value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runGatesCLI(t, tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.want) || !strings.Contains(stderr, "--help") {
				t.Errorf("stderr = %q, want %q and a --help pointer", stderr, tc.want)
			}
		})
	}
}

func TestGatesPremergeRejectedInputWritesNothing(t *testing.T) {
	root := gatesRepo(t)
	code, _, _ := runGatesCLI(t, "gates", "premerge", "slug", "--root", filepath.Join(root, "absent-root"))
	if code != 2 && code != 1 {
		t.Fatalf("exit = %d, want a refusal", code)
	}
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "changes", "archive"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("archive entries = %v (%v), want none written", entries, err)
	}
}

// gatesGitRepo is a repository on the given branch, so the change ladder can
// pair a branch with a change folder.
func gatesGitRepo(t *testing.T, branch string) string {
	t.Helper()
	root := t.TempDir()
	gitTest(t, root, "init", "-b", branch)
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	gitTest(t, root, "config", "user.name", "test")
	gitTest(t, root, "commit", "--allow-empty", "-m", "initial")
	gatesWrite(t, filepath.Join(root, "openspec", "config.yaml"), gatesConfig)
	return root
}

// A branch-shaped --change (CI passes GITHUB_HEAD_REF) is normalized to its last
// path segment before use (F6).
func TestGatesPremergeAcceptsABranchShapedChangeOverride(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "alpha")

	code, stdout, stderr := runGatesCLI(t,
		"gates", "premerge", "--change", "feat/alpha", "--root", root)

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if stdout != "premerge-guardian: OK (standard, change alpha)\n" {
		t.Errorf("stdout = %q, want the normalized change in the verdict", stdout)
	}
}

// An empty positional or an explicitly empty --change= is a usage error, never
// "omitted": only a genuinely absent argument means absent (F2).
func TestGatesPremergeRefusesAnEmptyChangeArgument(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "alpha")

	for _, tc := range []struct {
		name   string
		args   []string
		needle string
	}{
		{"empty positional", []string{"gates", "premerge", ""}, "empty change argument"},
		{"empty --change=", []string{"gates", "premerge", "--change="}, "--change requires a non-empty value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--root", root)
			code, stdout, stderr := runGatesCLI(t, args...)
			if code != 2 || stdout != "" {
				t.Fatalf("exit = %d, stdout = %q; want 2/empty", code, stdout)
			}
			if !strings.Contains(stderr, tc.needle) {
				t.Errorf("stderr = %q, want %q", stderr, tc.needle)
			}
		})
	}
}

// A positional slug keeps the legacy verdict byte for byte; --change instead
// prints the derived change.
func TestGatesPremergeChangeFlagPrintsTheDerivedChange(t *testing.T) {
	root := gatesGitRepo(t, "feat/other")
	gatesConformingArchive(t, root, "ok-change")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root)
	if code != 0 || stdout != "premerge-guardian: OK (standard)\n" || stderr != "" {
		t.Fatalf("positional = (%d, %q, %q), want the legacy OK line", code, stdout, stderr)
	}
	for _, args := range [][]string{
		{"gates", "premerge", "--change", "ok-change", "--root", root},
		{"gates", "premerge", "--change=ok-change", "--root", root},
	} {
		code, stdout, stderr = runGatesCLI(t, args...)
		if code != 0 || stdout != "premerge-guardian: OK (standard, change ok-change)\n" || stderr != "" {
			t.Fatalf("%q = (%d, %q, %q), want the derived change in the verdict", args, code, stdout, stderr)
		}
	}
}

// Without a positional slug the change comes from the branch ladder and the
// guardian grades that change instead of failing on a missing argument. The
// active delta folder it finds is now a pre-merge blocker (the archive must run
// first); once archived, the positional form passes.
func TestGatesPremergeResolvesTheChangeWithoutASlug(t *testing.T) {
	root := gatesGitRepo(t, "feat/ok-change")
	planBuildWriteMode(t, root, "odd")
	planBuildWriteOddPlan(t, root, "ok-change")
	active := filepath.Join(root, "odd", "changes", "ok-change")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "--root", root)
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
	}
	if !strings.Contains(stderr, "run `ai-specs changes archive`") {
		t.Fatalf("stderr = %q, want the archive blocker for the branch-resolved change", stderr)
	}

	archive := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-ok-change")
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(active, archive); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root)
	if code != 0 || stderr != "" {
		t.Fatalf("archived: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if stdout != "premerge-guardian: OK (odd)\n" {
		t.Errorf("stdout = %q, want the OK verdict", stdout)
	}
}

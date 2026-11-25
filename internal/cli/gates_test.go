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

// gatesRepo is an empty planning tree. It carries no project config: the
// config is optional and ungraded.
func gatesRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
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

// gatesNotePrefix is the line the guardian prints for a refutation it could not
// apply; the verdict and the exit code are unaffected.
const gatesNotePrefix = "premerge-guardian: NOTE: R-a and R-b were not applied: "

// --base is the ref the objective refutations diff against; CI passes the pull
// request's base sha. A base that does not resolve refutes nothing and says so.
func TestGatesPremergeAcceptsABase(t *testing.T) {
	root := gatesRepo(t)
	for _, args := range [][]string{
		{"gates", "premerge", "nothing", "--root", root, "--base", "deadbeef"},
		{"gates", "premerge", "nothing", "--root", root, "--base=deadbeef"},
	} {
		code, stdout, stderr := runGatesCLI(t, args...)
		if code != 0 || stdout != "premerge-guardian: OK (none)\n" ||
			!strings.HasPrefix(stderr, gatesNotePrefix) || !strings.Contains(stderr, "`--base deadbeef`") {
			t.Fatalf("%q = (%d, %q, %q); want the none verdict and the note", args, code, stdout, stderr)
		}
	}
	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "nothing", "--root", root, "--base")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "ERROR: --base requires a value") {
		t.Fatalf("bare --base = (%d, %q, %q); want the usage error", code, stdout, stderr)
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
	// The temporary root is not a repository, so the diff is ungraded and the
	// guardian says which refutations it skipped instead of claiming it applied them.
	if !strings.HasPrefix(stderr, gatesNotePrefix+"no base ref resolves") {
		t.Errorf("stderr = %q, want the note naming the ungraded diff", stderr)
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
		"  - archive/2026-01-02-bad-change: the tier is derived from the artifacts of the change, and they derive 'light', not the declared 'standard' — add a specs/ delta to reach standard, or drop the declaration\n",
		"  - archive/2026-01-02-bad-change/tasks.md: `## Specs` section is missing: add the `## Specs` section above `## Tasks`\n",
		"  - archive/2026-01-02-bad-change/tasks.md: `## Tasks` section is missing: add the `## Tasks` table\n",
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
	// The delta is what derives standard; the declaration alone no longer counts.
	gatesWrite(t, filepath.Join(active, "specs", "cap", "spec.md"), "# cap\n")
	code, stdout, stderr = runGatesCLI(t, "gates", "premerge", "active-change", "--root", root,
		"--stage", "pre-archive")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1 and empty stdout", code, stdout)
	}
	if !strings.Contains(stderr, "standard verify evidence requires dedicated verify-report.md") {
		t.Errorf("stderr = %q, want the verify-report blocker", stderr)
	}
}

// The tier is derived from the artifacts, so a declared tier is only a
// coherence assertion: one that contradicts them blocks even without --tier.
func TestGatesPremergeBlocksOnATierDeclarationThatContradictsTheArtifacts(t *testing.T) {
	root := gatesRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "archive", "artifact-change")
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: light\n\n"+gatesPlanBody)
	gatesWrite(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	gatesWrite(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")
	gatesWrite(t, filepath.Join(folder, "verify-report.md"), gatesStandardReport)

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "artifact-change", "--root", root)
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1 and empty stdout", code, stdout)
	}
	if !strings.Contains(stderr, "they derive 'standard', not the declared 'light'") {
		t.Fatalf("stderr = %q, want the tier mismatch", stderr)
	}

	code, stdout, _ = runGatesCLI(t, "gates", "premerge", "artifact-change", "--root", root, "--tier", "full")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want the full tier to block", code, stdout)
	}

	gatesWrite(t, filepath.Join(folder, "tasks.md"), gatesPlanBody)
	code, stdout, stderr = runGatesCLI(t, "gates", "premerge", "artifact-change", "--root", root)
	if code != 0 || stdout != "premerge-guardian: OK (standard)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want the derived standard", code, stdout, stderr)
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

// A rejected input writes nothing: the guardian is read-only, so a blocked
// verdict leaves the archive tree untouched.
func TestGatesPremergeRejectedInputWritesNothing(t *testing.T) {
	root := gatesRepo(t)
	gatesWrite(t, filepath.Join(root, "openspec", "changes", "slug", "tasks.md"),
		"Depth: light\n\n"+gatesPlanBody)
	code, _, _ := runGatesCLI(t, "gates", "premerge", "slug", "--root", root)
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
	return root
}

// A branch-shaped --change (CI passes GITHUB_HEAD_REF) is normalized to its last
// path segment before use (F6).
func TestGatesPremergeAcceptsABranchShapedChangeOverride(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "alpha")

	code, stdout, stderr := runGatesCLI(t,
		"gates", "premerge", "--change", "feat/alpha", "--root", root)

	if code != 0 || !strings.HasPrefix(stderr, gatesNotePrefix) {
		t.Fatalf("exit = %d, stderr = %q; want 0 and the note", code, stderr)
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
	if code != 0 || stdout != "premerge-guardian: OK (standard)\n" ||
		!strings.HasPrefix(stderr, gatesNotePrefix) {
		t.Fatalf("positional = (%d, %q, %q), want the legacy OK line and the note", code, stdout, stderr)
	}
	for _, args := range [][]string{
		{"gates", "premerge", "--change", "ok-change", "--root", root},
		{"gates", "premerge", "--change=ok-change", "--root", root},
	} {
		code, stdout, stderr = runGatesCLI(t, args...)
		if code != 0 || stdout != "premerge-guardian: OK (standard, change ok-change)\n" ||
			!strings.HasPrefix(stderr, gatesNotePrefix) {
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
	// The archive must hold something of the change: the marker `changes archive`
	// writes for a delta-less change.
	gatesWrite(t, filepath.Join(archive, "archived.md"), "ok-change\n2026-01-02\nodd/tasks/ok-change.md\n")
	code, stdout, stderr = runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root)
	if code != 0 || !strings.HasPrefix(stderr, gatesNotePrefix) {
		t.Fatalf("archived: exit = %d, stderr = %q; want 0 and the note", code, stderr)
	}
	if stdout != "premerge-guardian: OK (odd)\n" {
		t.Errorf("stdout = %q, want the OK verdict", stdout)
	}
}

// The former full tier is removed: --tier full blocks with a migration message
// that points at standard instead of silently downgrading the change.
func TestGatesPremergeFullTierBlocksWithAMigrationMessage(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "ok-change")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root, "--tier", "full")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
	}
	for _, want := range []string{"the full tier was removed", "`Depth: standard`"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	}
}

// A plan declaring Depth: full blocks at the default tier resolution too.
func TestGatesPremergeDepthFullPlanBlocksWithAMigrationMessage(t *testing.T) {
	root := gatesRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "archive", "full-depth")
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: full\n\n"+gatesPlanBody)
	gatesWrite(t, filepath.Join(folder, "specs", "cap", "spec.md"), "# cap\n")

	code, _, stderr := runGatesCLI(t, "gates", "premerge", "full-depth", "--root", root)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "the full tier was removed") {
		t.Errorf("stderr = %q, want the migration message", stderr)
	}
}

// pre-archive grades openspec/changes/<slug>/, which odd mode does not use; run
// there it blocked every odd change, so it is a usage error naming the mode's own
// archive verb.
func TestGatesPremergeRefusesThePreArchiveStageInOddMode(t *testing.T) {
	root := gatesRepo(t)
	gatesWrite(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
		"[recipes.plan-build-flow.config]\nmode = \"odd\"\n")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "thing", "--root", root,
		"--stage", "pre-archive")
	if code != 2 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 2 and empty stdout", code, stdout)
	}
	if !strings.Contains(stderr, "--stage pre-archive is an openspec stage") {
		t.Fatalf("stderr = %q, want the odd refusal", stderr)
	}
}

// A refutation the guardian could not apply is said out loud, and the verdict —
// with its exit code — is exactly the one it would have been without the note.
func TestGatesPremergeNotesARefutationItCouldNotApply(t *testing.T) {
	t.Run("explicit base that does not resolve", func(t *testing.T) {
		code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "nothing",
			"--root", gatesRepo(t), "--base", "no-such-ref")
		if code != 0 || stdout != "premerge-guardian: OK (none)\n" {
			t.Fatalf("exit = %d, stdout = %q; want the none verdict", code, stdout)
		}
		if stderr != gatesNotePrefix+"the explicit `--base no-such-ref` does not resolve to a commit "+
			"in this root, so there is no diff to grade. Pass a ref that exists "+
			"(`git rev-parse --verify no-such-ref`) or omit `--base` to use the project's base "+
			"branch.\n" {
			t.Fatalf("stderr = %q, want the note naming the unresolved --base", stderr)
		}
	})

	t.Run("root that is not a git repository", func(t *testing.T) {
		code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "nothing", "--root", gatesRepo(t))
		if code != 0 || stdout != "premerge-guardian: OK (none)\n" {
			t.Fatalf("exit = %d, stdout = %q; want the none verdict", code, stdout)
		}
		if !strings.Contains(stderr, "not a git repository") {
			t.Fatalf("stderr = %q, want the note naming the missing repository", stderr)
		}
	})

	t.Run("note never blocks", func(t *testing.T) {
		root := gatesRepo(t)
		gatesConformingArchive(t, root, "ok-change")
		code, _, stderr := runGatesCLI(t, "gates", "premerge", "ok-change", "--root", root)
		if code != 0 {
			t.Fatalf("exit = %d, stderr = %q; a note must not turn into a blocker", code, stderr)
		}
	})
}

// pre-archive is not exempt from the refutations: a canonical spec edit with no
// delta is the same promotion bypass before the archive as after it.
func TestGatesPremergePreArchiveGradesTheRefutations(t *testing.T) {
	root := gatesGitRepo(t, "development")
	gitTest(t, root, "checkout", "-q", "-b", "feat/active-change")
	active := filepath.Join(root, "openspec", "changes", "active-change")
	gatesWrite(t, filepath.Join(active, "tasks.md"), gatesPlanBody)
	gatesWrite(t, filepath.Join(root, "openspec", "specs", "cap", "spec.md"), "# cap\n")
	gitTest(t, root, "add", "-A")
	gitTest(t, root, "commit", "-q", "-m", "docs: edit the canonical spec")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "active-change", "--root", root,
		"--stage", "pre-archive")
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1 and empty stdout", code, stdout)
	}
	if !strings.Contains(stderr, "R-a:") || !strings.Contains(stderr, "carries no delta") {
		t.Fatalf("stderr = %q, want the R-a blocker at pre-archive", stderr)
	}
}

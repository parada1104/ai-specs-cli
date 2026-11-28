package cli

// `ai-specs changes archive [<change>] --root <planning-root> [--date YYYY-MM-DD]`
// (plan-build-openspec-odd-align I5/P4, S10): the mechanical archive step the
// pre-merge guardian now requires in both modes. The verb refuses while the
// promotion is pending, refuses to create a second archive for one change, and
// moves the change folder with a single os.Rename. In odd mode the plan stays at
// odd/tasks/<change>.md; in openspec it lives inside the change folder and moves
// with it.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/specs"
)

const changesDelta = "## ADDED Requirements\n\n### Requirement: Archive rule\n\nBody.\n\n" +
	"#### Scenario: archive\n\n- **WHEN** archive\n- **THEN** archive\n"

const changesPlan = "## Specs\n\n- **S1.** Archive rule\n\n## Tasks\n\n" +
	"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
	"| T1 | S1 | Archive it | inline | pending |\n"

func runChangesCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, t.TempDir(), nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func changesWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// changesOddRepo is a real git repository in odd mode: the Gentle AI state
// directory under the common dir is what ResolveRepo probes.
func changesOddRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "gentle-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// changesOpenSpecChange writes a promoted openspec change: plan, delta and the
// canonical spec it composes into.
func changesOpenSpecChange(t *testing.T, root, slug string) string {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", slug)
	changesWrite(t, filepath.Join(folder, "tasks.md"), changesPlan)
	changesWrite(t, filepath.Join(folder, "specs", "capability", "spec.md"), changesDelta)
	if report := specs.PromoteChangeForMode(root, slug, planbuild.ModeOpenspec, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	return folder
}

// changesOddChange writes an unpromoted odd change folder and its plan.
func changesOddChange(t *testing.T, root, slug string) string {
	t.Helper()
	changesWrite(t, filepath.Join(root, "odd", "tasks", slug+".md"), changesPlan)
	folder := filepath.Join(root, "odd", "changes", slug)
	changesWrite(t, filepath.Join(folder, "specs", "capability", "spec.md"), changesDelta)
	return folder
}

// openspec: the change folder moves to <date>-<change> and the plan goes with it.
func TestChangesArchiveMovesTheOpenSpecFolderWithItsPlan(t *testing.T) {
	root := t.TempDir()
	src := changesOpenSpecChange(t, root, "thing")
	dst := filepath.Join(root, "openspec", "changes", "archive", "2026-01-02-thing")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if want := "changes archive: moved " + src + " to " + dst + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("archive folder missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "tasks.md")); err != nil {
		t.Errorf("plan did not move with the openSpec change: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source folder still present: %v", err)
	}
}

// odd: only odd/changes/<change>/ moves; odd/tasks/<change>.md stays.
func TestChangesArchiveMovesOnlyTheOddDeltaFolder(t *testing.T) {
	root := changesOddRepo(t)
	src := changesOddChange(t, root, "thing")
	if report := specs.PromoteChangeForMode(root, "thing", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	dst := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if want := "changes archive: moved " + src + " to " + dst + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("delta folder still present: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("archive folder missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "odd", "tasks", "thing.md")); err != nil {
		t.Errorf("odd plan must stay at odd/tasks/thing.md: %v", err)
	}
}

// A pending promotion refuses the archive and moves nothing.
func TestChangesArchiveRefusesWhileThePromotionIsPending(t *testing.T) {
	root := changesOddRepo(t)
	src := changesOddChange(t, root, "thing") // delta written, never promoted
	archiveBase := filepath.Join(root, "odd", "changes", "archive")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
	}
	if !strings.Contains(stderr, "is not promoted") {
		t.Errorf("stderr = %q, want the promotion blocker", stderr)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("delta folder moved on a refusal: %v", err)
	}
	if _, err := os.Stat(archiveBase); !os.IsNotExist(err) {
		t.Errorf("archive tree was created on a refusal: %v", err)
	}
}

// A dated archive for the same change refuses instead of creating a second one.
func TestChangesArchiveRefusesAnExistingDatedArchive(t *testing.T) {
	for _, tc := range []struct {
		name       string
		existing   string
		wantNeedle string
	}{
		{"same date", "2026-01-02-thing", "already exists"},
		{"other date", "2025-12-31-thing", "already exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			src := changesOpenSpecChange(t, root, "thing")
			existing := filepath.Join(root, "openspec", "changes", "archive", tc.existing)
			if err := os.MkdirAll(existing, 0o755); err != nil {
				t.Fatal(err)
			}

			code, stdout, stderr := runChangesCLI(t,
				"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

			if code != 1 || stdout != "" {
				t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
			}
			if !strings.Contains(stderr, tc.wantNeedle) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantNeedle)
			}
			if _, err := os.Stat(src); err != nil {
				t.Errorf("delta folder moved on a refusal: %v", err)
			}
			if tc.existing == "2026-01-02-thing" {
				if info, err := os.Stat(existing); err != nil || !info.IsDir() {
					t.Errorf("pre-existing archive was disturbed: %v", err)
				}
				return
			}
			if _, err := os.Stat(filepath.Join(root, "openspec", "changes", "archive", "2026-01-02-thing")); !os.IsNotExist(err) {
				t.Errorf("a second archive was created: %v", err)
			}
		})
	}
}

// An impossible calendar date is rejected before any move; a valid one is the
// archive prefix.
func TestChangesArchiveValidatesTheDate(t *testing.T) {
	root := t.TempDir()
	src := changesOpenSpecChange(t, root, "thing")

	code, _, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-02-30")
	if code == 0 {
		t.Fatalf("exit = 0, want a usage error")
	}
	if !strings.Contains(stderr, "invalid --date value: 2026-02-30") {
		t.Errorf("stderr = %q, want the --date error", stderr)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("folder moved on an invalid date: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes", "archive")); !os.IsNotExist(err) {
		t.Errorf("archive tree was created on an invalid date: %v", err)
	}

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2020-12-31")
	if code != 0 || stderr != "" {
		t.Fatalf("valid date: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if !strings.Contains(stdout, "2020-12-31-thing") {
		t.Errorf("stdout = %q, want the 2020-12-31 prefix", stdout)
	}
}

// A delta-less change is still archived: the verb creates the dated folder
// holding a three-line archived.md marker and leaves the original plan where it
// is (F3). The plan is never copied, so a long plan is not duplicated.
func TestChangesArchiveWritesTheMarkerForADeltaLessChange(t *testing.T) {
	root := changesOddRepo(t)
	plan := filepath.Join(root, "odd", "tasks", "thing.md")
	changesWrite(t, plan, changesPlan)
	dst := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	want := "changes archive: archived the plan of thing at " + dst + " (no delta folder)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	marker, err := os.ReadFile(filepath.Join(dst, "archived.md"))
	if err != nil {
		t.Fatalf("archive folder does not hold the marker: %v", err)
	}
	if got, wantMarker := string(marker), "# archived: thing\ndate: 2026-01-02\n"; got != wantMarker {
		t.Errorf("marker = %q, want %q", got, wantMarker)
	}
	if _, err := os.Stat(filepath.Join(dst, "tasks.md")); !os.IsNotExist(err) {
		t.Errorf("the plan must not be copied into the archive: %v", err)
	}
	if original, err := os.ReadFile(plan); err != nil || string(original) != changesPlan {
		t.Errorf("odd plan must stay at odd/tasks/thing.md: %q (%v)", original, err)
	}
}

// An active change folder that holds only notes archives to a folder the
// guardian would reject as empty; the marker keeps it content-provable (C1).
func TestChangesArchiveWritesTheMarkerForANotesOnlyFolder(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "openspec", "changes", "thing")
	changesWrite(t, filepath.Join(src, "notes.md"), "# notes\n")
	dst := filepath.Join(root, "openspec", "changes", "archive", "2026-01-02-thing")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if want := "changes archive: moved " + src + " to " + dst + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	marker, err := os.ReadFile(filepath.Join(dst, "archived.md"))
	if err != nil {
		t.Fatalf("archive does not hold the marker: %v", err)
	}
	if got, wantMarker := string(marker), "# archived: thing\ndate: 2026-01-02\n"; got != wantMarker {
		t.Errorf("marker = %q, want %q", got, wantMarker)
	}
}

// An archive destination that resolves through a symlink or outside the
// repository is refused before any move (F1).
func TestChangesArchiveRefusesASymlinkedArchiveDestination(t *testing.T) {
	for _, tc := range []struct {
		name string
		link string
	}{
		{"archive tree", filepath.Join("odd", "changes", "archive")},
		{"destination folder", filepath.Join("odd", "changes", "archive", "2026-01-02-thing")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := changesOddRepo(t)
			src := changesOddChange(t, root, "thing")
			outside := t.TempDir()
			link := filepath.Join(root, tc.link)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			code, stdout, stderr := runChangesCLI(t,
				"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

			if code != 1 || stdout != "" {
				t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
			}
			if !strings.Contains(stderr, "resolves through a symlink or outside the repository") {
				t.Errorf("stderr = %q, want the containment refusal", stderr)
			}
			if _, err := os.Stat(src); err != nil {
				t.Errorf("delta folder moved on a refusal: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Errorf("outside tree = %v (%v), want nothing written", entries, err)
			}
		})
	}
}

// A symlink at the change path is not the change directory: os.Stat would call
// it active and os.Rename would move the link instead of the change (C1).
func TestChangesArchiveRefusesASymlinkedSource(t *testing.T) {
	root := changesOddRepo(t)
	link := changesOddChange(t, root, "thing")
	if report := specs.PromoteChangeForMode(root, "thing", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	real := filepath.Join(root, "odd", "changes", "thing-real")
	if err := os.Rename(link, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02")

	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
	}
	if !strings.Contains(stderr, "is a symlink, not a change directory") {
		t.Errorf("stderr = %q, want the symlinked-source refusal", stderr)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("the link was moved on a refusal: %v", err)
	}
	if _, err := os.Stat(real); err != nil {
		t.Errorf("the real directory was moved on a refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "odd", "changes", "archive")); !os.IsNotExist(err) {
		t.Errorf("archive tree was created on a refusal: %v", err)
	}
}

// An explicitly empty --date is a usage error, never "omitted": only a
// genuinely absent flag means today (C2).
func TestChangesArchiveRefusesAnEmptyDate(t *testing.T) {
	root := t.TempDir()
	src := changesOpenSpecChange(t, root, "thing")

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date=")

	if code != 2 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 2/empty", code, stdout)
	}
	if !strings.Contains(stderr, "--date requires a non-empty value") {
		t.Errorf("stderr = %q, want the empty --date usage error", stderr)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("folder moved on an empty --date: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes", "archive")); !os.IsNotExist(err) {
		t.Errorf("archive tree was created on a usage error: %v", err)
	}
}

// An empty positional or an explicitly empty --change= is a usage error, never
// "omitted": only a genuinely absent argument means absent (F2).
func TestChangesArchiveRefusesAnEmptyChangeArgument(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		needle string
	}{
		{"empty positional", []string{"changes", "archive", ""}, "empty change argument"},
		{"empty --change=", []string{"changes", "archive", "--change="}, "--change requires a non-empty value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := changesOddRepo(t)
			src := changesOddChange(t, root, "thing")
			args := append([]string{}, tc.args...)
			args = append(args, "--root", root)

			code, stdout, stderr := runChangesCLI(t, args...)

			if code != 2 || stdout != "" {
				t.Fatalf("exit = %d, stdout = %q; want 2/empty", code, stdout)
			}
			if !strings.Contains(stderr, tc.needle) {
				t.Errorf("stderr = %q, want %q", stderr, tc.needle)
			}
			if _, err := os.Stat(src); err != nil {
				t.Errorf("delta folder moved on an empty change argument: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "odd", "changes", "archive")); !os.IsNotExist(err) {
				t.Errorf("archive tree was created on a usage error: %v", err)
			}
		})
	}
}

// The root help announces the changes family, and both parity goldens still
// match the Go help after the harness normalizations (tests/parity/parity.py
// N2 collapses the hub command-substitution region, N14 drops the worktree-gate
// line the frozen legacy goldens predate).
func TestChangesRootHelpMatchesTheParityGoldens(t *testing.T) {
	code, stdout, stderr := runChangesCLI(t, "help")
	if code != 0 || stderr != "" {
		t.Fatalf("help: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if !strings.Contains(stdout, "changes archive") {
		t.Fatalf("root help %q does not announce the changes family", stdout)
	}
	snapshot := parityHelpSnapshot(stdout)
	for _, variant := range []string{"gate-present", "gate-absent"} {
		path := filepath.Join("..", "..", "tests", "parity", "goldens", variant, "surface-verbs.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var doc struct {
			Steps []struct {
				Args   []string `json:"argv"`
				Stdout string   `json:"stdout"`
			} `json:"steps"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(doc.Steps) == 0 || len(doc.Steps[0].Args) == 0 || doc.Steps[0].Args[0] != "help" {
			t.Fatalf("%s: first step is not the help verb", path)
		}
		if doc.Steps[0].Stdout != snapshot {
			t.Errorf("%s help golden no longer matches the binary output", path)
		}
	}
}

// parityHelpSnapshot is the Go side of the two harness normalizations the help
// leg applies, so the comparison is against the same normalized surface.
func parityHelpSnapshot(stdout string) string {
	const gateLine = "  worktree-gate [gate args...]  Run the worktree gate " +
		"(arguments passed through unchanged)\n"
	stdout = strings.ReplaceAll(stdout, gateLine, "")
	// N16 in tests/parity/parity.py: `changes start --none` is a verb the frozen
	// legacy goldens predate, so the same line is dropped from the Go side.
	const startLine = "  changes start --none --root <planning-root>  " +
		"Deprecated: the tier is inferred from the artifacts\n"
	stdout = strings.ReplaceAll(stdout, startLine, "")
	// N17 in tests/parity/parity.py: `changes workstream` is the other verb the
	// frozen legacy goldens predate (change-ledger-v2 T3).
	const workstreamLine = "  changes workstream --root <repo-root>  Report a change or a " +
		"workstream, or declare the workstream (--reason, --end)\n"
	stdout = strings.ReplaceAll(stdout, workstreamLine, "")
	// N18 in tests/parity/parity.py: `changes status` is the verb T7 adds, and the
	// frozen legacy goldens predate it too.
	const statusLine = "  changes status --root <repo-root>  Report the phase and what the " +
		"worktree owes, composed read-only\n"
	stdout = strings.ReplaceAll(stdout, statusLine, "")
	const hubPrefix = "  hub [path] Interactive status + command menu (also bare "
	const helpAnchor = "\n  help Show this help"
	if start := strings.Index(stdout, hubPrefix); start >= 0 {
		if end := strings.Index(stdout[start:], helpAnchor); end >= 0 {
			stdout = stdout[:start] + hubPrefix + "<HELP_CMD_SUBST>" + stdout[start+end:]
		}
	}
	return stdout
}

// Re-running the verb on an already archived change refuses instead of
// reporting a second no-op: an existing archive is the "only condition" the
// no-delta exit 0 does not cover.
func TestChangesArchiveRefusesWhenTheChangeIsAlreadyArchived(t *testing.T) {
	root := t.TempDir()
	src := changesOpenSpecChange(t, root, "thing")
	if code, _, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-02"); code != 0 {
		t.Fatalf("first archive: exit = %d, stderr = %q", code, stderr)
	}

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "thing", "--root", root, "--date", "2026-01-03")

	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1/empty", code, stdout)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr = %q, want the existing-archive refusal", stderr)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source reappeared: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes", "archive", "2026-01-03-thing")); !os.IsNotExist(err) {
		t.Errorf("a second archive was created: %v", err)
	}
}

// Without --date the prefix is today in UTC.
func TestChangesArchiveDefaultsToTodayUTC(t *testing.T) {
	root := t.TempDir()
	src := changesOpenSpecChange(t, root, "thing")
	today := time.Now().UTC().Format("2006-01-02")

	code, stdout, stderr := runChangesCLI(t, "changes", "archive", "thing", "--root", root)

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	want := filepath.Join(root, "openspec", "changes", "archive", today+"-thing")
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want the %s prefix", stdout, today)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("archive at the UTC date missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source folder still present: %v", err)
	}
}

// The change name becomes a directory path, so it must stay one segment: an
// override like ../.. can never move a tree outside the repository.
func TestChangesArchiveRefusesAChangeNameWithAPathSeparator(t *testing.T) {
	root := changesOddRepo(t)
	src := changesOddChange(t, root, "thing")

	for _, name := range []string{"../thing", "..", ".", "a/../b"} {
		code, _, stderr := runChangesCLI(t,
			"changes", "archive", name, "--root", root, "--date", "2026-01-02")
		if code != 2 {
			t.Fatalf("%q: exit = %d; want 2", name, code)
		}
		if !strings.Contains(stderr, "invalid change name") {
			t.Errorf("%q: stderr = %q, want the invalid-name refusal", name, stderr)
		}
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("delta folder moved on a refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "odd", "changes", "archive")); !os.IsNotExist(err) {
		t.Errorf("archive tree was created on a refusal: %v", err)
	}
}

// A branch-shaped --change (CI passes GITHUB_HEAD_REF) keeps its last path
// segment before use (F6).
func TestChangesArchiveNormalizesABranchShapedChange(t *testing.T) {
	root := changesOddRepo(t)
	src := changesOddChange(t, root, "thing")
	if report := specs.PromoteChangeForMode(root, "thing", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}

	code, stdout, stderr := runChangesCLI(t,
		"changes", "archive", "--change", "feat/thing", "--root", root, "--date", "2026-01-02")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	dst := filepath.Join(root, "odd", "changes", "archive", "2026-01-02-thing")
	if !strings.Contains(stdout, dst) || !strings.Contains(stdout, src) {
		t.Errorf("stdout = %q, want the move to %s", stdout, dst)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("archive folder missing: %v", err)
	}
}

// --- changes start --none (plan-build-openspec-odd-align T14/S19) ----------

// changesStartRepo is a real repository on the change branch.
func changesStartRepo(t *testing.T, branch string) string {
	t.Helper()
	root := t.TempDir()
	gitTest(t, root, "init", "-b", branch)
	return root
}

// changesCeremonyValue reads the local declaration through git, so the test
// asserts the stored state and not only the confirmation line.
func changesCeremonyValue(t *testing.T, root, branch string) (string, bool) {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "config", "--get", "branch."+branch+".aiSpecsCeremony").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// changesStartNotice is the verb's one deprecation line, verbatim.
const changesStartNotice = "changes start: deprecated — the tier is inferred from the change's " +
	"artifacts, so this verb no longer affects any gate, and it records nothing. A worktree that is " +
	"not a change is classified from facts; run 'ai-specs changes workstream --help'.\n"

// The verb still runs for one version, prints its deprecation notice, exits 0
// and records nothing the gate reads.
func TestChangesStartDeclaresTheLocalNoPlanCeremony(t *testing.T) {
	root := changesStartRepo(t, "feat/thing")

	code, stdout, stderr := runChangesCLI(t, "changes", "start", "--none", "--root", root)

	if code != 0 || stdout != "" || stderr != changesStartNotice {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want 0/empty/notice", code, stdout, stderr)
	}
	if value, found := changesCeremonyValue(t, root, "feat/thing"); found {
		t.Errorf("git config branch.feat/thing.aiSpecsCeremony = %q; want nothing written", value)
	}
}

// Running it twice is idempotent and prints the same notice, with no side effect.
func TestChangesStartIsIdempotent(t *testing.T) {
	root := changesStartRepo(t, "feat/thing")
	first, firstStdout, firstStderr := runChangesCLI(t, "changes", "start", "--none", "--root", root)
	second, secondStdout, secondStderr := runChangesCLI(t, "changes", "start", "--none", "--root", root)

	if first != 0 || second != 0 || firstStdout != "" || secondStdout != "" {
		t.Fatalf("exit = (%d, %d), stdout = (%q, %q); want 0/0 and empty", first, second, firstStdout, secondStdout)
	}
	if firstStderr != changesStartNotice || secondStderr != changesStartNotice {
		t.Errorf("stderr = (%q, %q), want the same notice twice", firstStderr, secondStderr)
	}
	if value, found := changesCeremonyValue(t, root, "feat/thing"); found {
		t.Errorf("git config value = %q; want nothing written", value)
	}
}

// Without --none the verb names nothing it could declare: it prints its usage,
// exits 2 and writes no config.
func TestChangesStartWithoutNoneIsAUsageError(t *testing.T) {
	root := changesStartRepo(t, "feat/thing")

	code, stdout, stderr := runChangesCLI(t, "changes", "start", "--root", root)

	if code != 2 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 2/empty", code, stdout)
	}
	if !strings.Contains(stderr, "Usage: ai-specs changes start") {
		t.Errorf("stderr = %q, want the start usage", stderr)
	}
	if value, found := changesCeremonyValue(t, root, "feat/thing"); found {
		t.Errorf("git config branch.feat/thing.aiSpecsCeremony = %q; want nothing written", value)
	}
}

// A worktree that is not a Git repository is still served: the verb is inert, so
// it warns and exits 0 instead of demanding a branch to declare on.
func TestChangesStartOutsideARepositoryIsAUsageError(t *testing.T) {
	root := t.TempDir()

	code, stdout, stderr := runChangesCLI(t, "changes", "start", "--none", "--root", root)

	if code != 0 || stdout != "" || stderr != changesStartNotice {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want 0/empty/notice", code, stdout, stderr)
	}
}

// The verb is registered as a native route with its own kind.
func TestChangesStartRouteIsNative(t *testing.T) {
	if got := Route([]string{"changes", "start"}).kind; got != routeChangesStart {
		t.Fatalf("route = %v, want routeChangesStart", got)
	}
}

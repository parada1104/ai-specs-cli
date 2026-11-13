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
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: standard\n")
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
	gatesWrite(t, filepath.Join(root, "openspec", "changes", "archive", "bad-change", "tasks.md"),
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
		"  - archive/bad-change/proposal.md is required for tier standard\n",
		"  - archive/bad-change/specs/ must contain at least one *.md for tier standard\n",
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
	gatesWrite(t, filepath.Join(active, "tasks.md"), "Depth: light\n")
	gatesWrite(t, filepath.Join(active, "proposal.md"), "# proposal\n")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "active-change", "--root", root,
		"--stage", "pre-archive")
	if code != 0 || stdout != "premerge-guardian: OK (light)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want 0 and the light OK line", code, stdout, stderr)
	}

	gatesWrite(t, filepath.Join(root, "openspec", "changes", "active-change", "tasks.md"),
		"Depth: standard\n")
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
	gatesWrite(t, filepath.Join(folder, "tasks.md"), "Depth: light\n")
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

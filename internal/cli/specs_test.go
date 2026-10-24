package cli

// `ai-specs specs promote <slug> --root <planning-root>` (GO-20 U2e, unit U-A):
// the native verb for the canonical spec promoter. The stdout/stderr bytes and
// exit codes pinned here are the ones lib/_internal/spec_promotion.py main()
// prints, so the verb is a drop-in replacement for the consumer docs.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func specsRequirement(name, body string) string {
	return "### Requirement: " + name + "\n\n" + body + "\n\n#### Scenario: behaviour\n\n" +
		"- **WHEN** behaviour\n- **THEN** behaviour\n"
}

func specsDelta(blocks ...string) string {
	return "# Delta for capability\n\n## ADDED Requirements\n\n" + strings.Join(blocks, "\n")
}

// specsRepo builds a minimal planning root: openspec/changes/archive and
// openspec/specs both exist.
func specsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"openspec/changes/archive", "openspec/specs"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func specsChange(t *testing.T, root, slug, domain, text string) {
	t.Helper()
	dir := filepath.Join(root, "openspec", "changes", slug, "specs", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func specCanonicalPath(root, domain string) string {
	return filepath.Join(root, "openspec", "specs", domain, "spec.md")
}

func runSpecsCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, t.TempDir(), nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestSpecsPromoteComposesAndRerunsAsAlreadyPromoted(t *testing.T) {
	root := specsRepo(t)
	added := specsRequirement("Added requirement", "Added body.")
	specsChange(t, root, "cli-change", "capability", specsDelta(added))

	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "cli-change", "--root", root)

	if code != 0 || stderr != "" {
		t.Fatalf("first run: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	want := "spec-promotion: OK (1 domain spec(s) composed)\n  - capability: ADDED Added requirement\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if _, err := os.Stat(specCanonicalPath(root, "capability")); err != nil {
		t.Fatalf("canonical spec not written: %v", err)
	}

	code, stdout, stderr = runSpecsCLI(t, "specs", "promote", "cli-change", "--root", root)

	if code != 0 || stderr != "" {
		t.Fatalf("rerun: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	want = "spec-promotion: OK (already promoted: capability: ADDED Added requirement (already canonical))\n"
	if stdout != want {
		t.Errorf("rerun stdout = %q, want %q", stdout, want)
	}
}

func TestSpecsPromoteNothingToPromote(t *testing.T) {
	root := specsRepo(t)
	specsChange(t, root, "nothing", "capability", "# Delta for capability\n")

	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "nothing", "--root", root)

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if want := "spec-promotion: OK (nothing to promote)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestSpecsPromoteBlockedExitsOne(t *testing.T) {
	root := specsRepo(t)
	canonical := specCanonicalPath(root, "capability")
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical,
		[]byte("# capability Specification\n\n## Requirements\n\n"+
			specsRequirement("Existing requirement", "Existing body.")), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	specsChange(t, root, "blocked", "capability",
		specsDelta(specsRequirement("Existing requirement", "Conflicting body.")))

	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "blocked", "--root", root)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	want := "spec-promotion: BLOCKED\n  - specs/capability/spec.md: ADDED requirement " +
		`"Existing requirement" collides with an existing canonical requirement with different ` +
		"content; use MODIFIED for an intentional replacement, or rename the requirement\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	after, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Errorf("blocked promotion wrote the canonical spec: %q, want %q", after, before)
	}
}

func TestSpecsPromoteRemovedNeedsTheExplicitFlag(t *testing.T) {
	root := specsRepo(t)
	canonical := specCanonicalPath(root, "capability")
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# capability Specification\n\n## Requirements\n\n" +
		specsRequirement("Existing requirement", "Existing body.")
	if err := os.WriteFile(canonical, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	specsChange(t, root, "destructive", "capability",
		"# Delta for capability\n\n## REMOVED Requirements\n\n"+
			specsRequirement("Existing requirement", "Existing body."))

	code, _, stderr := runSpecsCLI(t, "specs", "promote", "destructive", "--root", root)
	if code != 1 || !strings.Contains(stderr, "spec-promotion: BLOCKED") {
		t.Fatalf("without --allow-removed: exit = %d, stderr = %q; want 1/BLOCKED", code, stderr)
	}

	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "destructive", "--root", root, "--allow-removed")
	if code != 0 || stderr != "" {
		t.Fatalf("with --allow-removed: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if want := "spec-promotion: OK (1 domain spec(s) composed)\n  - capability: REMOVED Existing requirement\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	got, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Existing requirement") {
		t.Errorf("canonical still holds the removed requirement: %q", got)
	}
}

func TestSpecsPromoteRequiresTheRootFlag(t *testing.T) {
	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "cli-change")

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "--root") {
		t.Errorf("stderr = %q, want the --root requirement", stderr)
	}
}

func TestSpecsPromoteArgumentErrors(t *testing.T) {
	root := specsRepo(t)
	cases := []struct {
		name string
		args []string
	}{
		{"missing slug", []string{"specs", "promote", "--root", root}},
		{"extra positional", []string{"specs", "promote", "a", "b", "--root", root}},
		{"unknown flag", []string{"specs", "promote", "a", "--root", root, "--bogus"}},
		{"root without value", []string{"specs", "promote", "a", "--root"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runSpecsCLI(t, tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if stderr == "" {
				t.Error("stderr is empty, want a usage diagnostic")
			}
		})
	}
}

func TestSpecsPromoteHelp(t *testing.T) {
	code, stdout, stderr := runSpecsCLI(t, "specs", "promote", "--help")

	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	for _, want := range []string{
		"Usage: ai-specs specs promote <slug> --root <planning-root>",
		"--root",
		"--allow-removed",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output %q does not contain %q", stdout, want)
		}
	}
}

func TestSpecsFamilyUsageAndRoute(t *testing.T) {
	cases := []struct {
		args     []string
		wantKind routeKind
	}{
		{[]string{"specs", "promote"}, routeSpecsPromote},
		{[]string{"specs", "promote", "slug", "--root", "/tmp"}, routeSpecsPromote},
		{[]string{"specs"}, routeSpecs},
		{[]string{"specs", "--help"}, routeSpecs},
		{[]string{"specs", "help"}, routeSpecs},
		{[]string{"specs", "promotee"}, routeSpecs},
	}
	for _, tc := range cases {
		if got := Route(tc.args).kind; got != tc.wantKind {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, got, tc.wantKind)
		}
	}

	code, stdout, stderr := runSpecsCLI(t, "specs", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs specs <subcommand>") {
		t.Errorf("specs --help: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runSpecsCLI(t, "specs")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "Usage: ai-specs specs <subcommand>") {
		t.Errorf("bare specs: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runSpecsCLI(t, "specs", "bogus")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown subcommand 'bogus'") {
		t.Errorf("specs bogus: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// TestRootHelpAnnouncesTheSpecsFamily pins the one-line root announcement: the
// root dispatcher has no `promote` verb, and the family is reachable through
// `specs promote` (help.txt line before `version`).
func TestRootHelpAnnouncesTheSpecsFamily(t *testing.T) {
	code, stdout, stderr := runSpecsCLI(t, "help")

	if code != 0 || stderr != "" {
		t.Fatalf("help: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if !strings.Contains(stdout, "specs promote <slug> --root <planning-root>") {
		t.Errorf("root help does not announce the specs family: %q", stdout)
	}
}

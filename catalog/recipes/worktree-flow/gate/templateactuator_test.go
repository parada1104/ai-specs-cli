package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// RED contract suite for strangler slice 6 (GO-09): the Go template actuator
// (`worktree-gate --materialize-template`) that will replace the Python
// materialize_template path in lib/_internal/recipe-materialize.py:829-948
// and its renderer resolve_template_dest/render_template_bytes (:829/:989).
//
// The symbols referenced here (runMaterializeTemplate, resolveTemplateDest,
// renderTemplateBytes) do not exist yet, so this package intentionally fails
// to compile until the implementation work unit lands templateactuator.go.
// These tests ARE the contract: the implementation must make them pass
// byte-for-byte and string-for-string, and must REUSE the already-ported
// classifyManagedOverride pure core (classify.go) for the not_exists
// decision — the ownership classification is never re-ported in slice 6
// (single shared port with slice 7, util.py:651/808 via --plan-classify).
//
// Bridge contract (fail-open, matching GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK):
// one JSON envelope on stdin, one JSON envelope on stdout, exit 0/2. Exit 2
// or any bridge failure makes the Python bridge warn once with the fallback
// token and run the historical Python path; Go never escalates the failure.

// templateInput mirrors the --materialize-template stdin contract. The zero
// values Python sends are: condition "not_exists" (or the recipe's declared
// condition), update_policy "auto" (or declared), and config carrying the
// merged recipe config values (only repo_topology, worktrees_dir, and
// integration_branch are read). managed_entry is the lock's
// managed[target] subset — only sha256 is read; nil means untracked.
type templateInput struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Source       string                `json:"source"`
	Target       string                `json:"target"`
	Condition    string                `json:"condition,omitempty"`
	UpdatePolicy string                `json:"update_policy,omitempty"`
	Config       map[string]any        `json:"config,omitempty"`
	ManagedEntry *templateManagedEntry `json:"managed_entry,omitempty"`
}

// templateManagedEntry is the subset of [managed.<path>] lock metadata the
// actuator reads (same shape as classifyManagedEntry).
type templateManagedEntry struct {
	SHA256 string `json:"sha256"`
}

// templateRecord is the lock payload the Python bridge must hand to
// lock.set_managed_override: target is the posix form of tpl.target, sha256
// is the normalized (CRLF-folded) sha of the bytes actually on disk after the
// actuation, kind is always "template". A nil record means "write nothing to
// the lock".
type templateRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// templateOutput is the --materialize-template stdout contract.
//   - dest: the resolved absolute destination (what resolve_template_dest
//     produced, anchored when git emitted a repo-relative path).
//   - wrote: Go wrote the file itself (mkdir -p parent, rendered bytes,
//     chmod to the source permission bits). Python never re-writes.
//   - record: the managed-override payload, nil when nothing is recorded.
//   - message: the per-target detail line WITHOUT the print indentation
//     ("✓ template {target}" or "· template skipped (exists) {target}").
//   - info: optional info() line ("refreshed managed template {target}").
//   - warnings: exact warn() strings, in emission order.
//   - error: set with exit 2; no file is touched on refusal paths.
type templateOutput struct {
	Dest     string          `json:"dest"`
	Wrote    bool            `json:"wrote"`
	Record   *templateRecord `json:"record"`
	Message  string          `json:"message"`
	Info     string          `json:"info,omitempty"`
	Warnings []string        `json:"warnings"`
	Error    *string         `json:"error"`
}

func requireGit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("template dest resolution runs git subprocesses")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available for --git-path resolution")
	}
}

// writeTemplateSource writes the recipe source file and returns the fixture
// input with the standard defaults for a non-.git target.
func writeTemplateSource(t *testing.T, body string, mode os.FileMode) templateInput {
	t.Helper()
	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	recipeDir := filepath.Join(base, "catalog", "recipes", "worktree-flow")
	if err := os.MkdirAll(filepath.Join(recipeDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir recipe dir: %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	src := filepath.Join(recipeDir, "templates", "post-merge.sh")
	if err := os.WriteFile(src, []byte(body), mode); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return templateInput{
		ProjectRoot:  projectRoot,
		RecipeDir:    recipeDir,
		RecipeID:     "worktree-flow",
		Source:       "templates/post-merge.sh",
		Target:       "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh",
		Condition:    "not_exists",
		UpdatePolicy: "auto",
		Config: map[string]any{
			"repo_topology":      "auto",
			"worktrees_dir":      ".worktrees",
			"integration_branch": "main",
		},
	}
}

// seedDest writes a pre-existing destination for the not_exists cases. The
// actuator creates parent directories for its own writes, but a seeded
// destination needs its parent to exist before the file itself can be written.
func seedDest(t *testing.T, dest string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir dest parent: %v", err)
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
}

func runTemplateActuatorCLI(t *testing.T, in templateInput) (int, templateOutput, string) {
	t.Helper()
	payload, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runMaterializeTemplate(bytes.NewReader(payload), &stdout, &stderr)
	var out templateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %q: %v", stdout.String(), err)
	}
	return code, out, stderr.String()
}

func wantRecord(in templateInput, sha string) *templateRecord {
	return &templateRecord{
		Target: in.Target,
		SHA256: sha,
		Recipe: in.RecipeID,
		Source: in.Source,
		Kind:   "template",
		Policy: in.UpdatePolicy,
	}
}

// --- pure unit tests: rendering and dest resolution, no CLI envelope ---

// TestRenderTemplateBytesTopologyToken pins the shared
// util.render_override_bytes contract: __WORKTREE_REPO_TOPOLOGY__ is replaced
// with cfg.repo_topology, defaulting to "auto" for a nil config or a missing
// key.
func TestRenderTemplateBytesTopologyToken(t *testing.T) {
	src := []byte("mode=__WORKTREE_REPO_TOPOLOGY__\n")
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{"configured", map[string]any{"repo_topology": "superrepo"}, "mode=superrepo\n"},
		{"default key", map[string]any{}, "mode=auto\n"},
		{"nil config", nil, "mode=auto\n"},
	}
	for _, tc := range cases {
		if got := string(renderTemplateBytes(src, tc.cfg)); got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := string(renderTemplateBytes([]byte("plain\n"), map[string]any{"repo_topology": "subrepo"})); got != "plain\n" {
		t.Errorf("tokenless body modified: %q", got)
	}
}

// TestRenderTemplateBytesCleanupTokens pins the narrow worktree-flow stamps:
// __WORKTREE_WORKTREES_DIR__ and __WORKTREE_INTEGRATION_BRANCH__ use the
// config value, falling back to ".worktrees"/"main" on a missing OR empty
// value (Python's `str(cfg.get(key) or default)`). With a nil config the
// topology token still resolves to "auto" but the cleanup tokens stay
// LITERAL — the early return fires after the shared render.
func TestRenderTemplateBytesCleanupTokens(t *testing.T) {
	src := []byte("dir=__WORKTREE_WORKTREES_DIR__\nbranch=__WORKTREE_INTEGRATION_BRANCH__\n")
	cases := []struct {
		name string
		cfg  map[string]any
		want string
	}{
		{
			"configured",
			map[string]any{"worktrees_dir": "trees", "integration_branch": "development"},
			"dir=trees\nbranch=development\n",
		},
		{
			"defaults on empty",
			map[string]any{"worktrees_dir": "", "integration_branch": ""},
			"dir=.worktrees\nbranch=main\n",
		},
		{
			"defaults on missing",
			map[string]any{},
			"dir=.worktrees\nbranch=main\n",
		},
	}
	for _, tc := range cases {
		if got := string(renderTemplateBytes(src, tc.cfg)); got != tc.want {
			t.Errorf("%s: render = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := string(renderTemplateBytes(src, nil)); got != string(src) {
		t.Errorf("nil config must leave cleanup tokens literal, got %q", got)
	}
}

// TestRenderTemplateBytesCRFLiteral pins that rendering is byte surgery only:
// CRLF content is replaced in place and never normalized by the renderer
// (normalization happens only inside the sha256).
func TestRenderTemplateBytesCRFLiteral(t *testing.T) {
	src := []byte("a=__WORKTREE_REPO_TOPOLOGY__\r\n")
	got := string(renderTemplateBytes(src, map[string]any{"repo_topology": "auto"}))
	if got != "a=auto\r\n" {
		t.Errorf("render = %q, want CRLF preserved %q", got, "a=auto\r\n")
	}
}

// TestResolveTemplateDestLiteralJoin pins the non-.git rule: the target is
// joined under the project root, and only the exact ".git/" prefix triggers
// git resolution ("​.githooks/..." stays literal).
func TestResolveTemplateDestLiteralJoin(t *testing.T) {
	root := t.TempDir()
	cases := []struct{ target, wantRel string }{
		{"ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh", "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"},
		{"docs/readme.md", "docs/readme.md"},
		{".githooks/post-merge", ".githooks/post-merge"},
	}
	for _, tc := range cases {
		want := filepath.Join(root, filepath.FromSlash(tc.wantRel))
		got, gitResolved := resolveTemplateDest(root, tc.target)
		if gitResolved {
			t.Errorf("resolveTemplateDest(%q) reported gitResolved for a literal target", tc.target)
		}
		if got != want {
			t.Errorf("resolveTemplateDest(%q) = %q, want %q", tc.target, got, want)
		}
	}
}

// TestResolveTemplateDestGitPathPlainRepo pins the plain-repo .git/ rule: git
// rev-parse --git-path emits a repo-relative path that is anchored at the
// project root.
func TestResolveTemplateDestGitPathPlainRepo(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	want := filepath.Join(root, ".git", "hooks", "post-merge")
	got, gitResolved := resolveTemplateDest(root, ".git/hooks/post-merge")
	if !gitResolved {
		t.Errorf("resolveTemplateDest = %q, gitResolved = false, want a clean git resolution", got)
	}
	if got != want {
		t.Errorf("resolveTemplateDest = %q, want %q", got, want)
	}
}

// TestResolveTemplateDestGitPathUncleanRemainder pins the containment
// semantics of the git resolution: inside a REAL repository,
// `git rev-parse --git-path` honors parent-directory components (it exits 0
// emitting an escaping path for "../../outside/evil"), so an unclean
// ".git/" remainder must never count as git-resolved and must fall back to
// the literal join — keeping the target subject to containment.
func TestResolveTemplateDestGitPathUncleanRemainder(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	cases := []struct{ target, wantRel string }{
		{".git/../../escape.sh", "../escape.sh"},
		{".git/hooks/../../../escape.sh", "../escape.sh"},
		{".git/../hooks/x", "hooks/x"},
	}
	for _, tc := range cases {
		want := filepath.Join(root, filepath.FromSlash(tc.wantRel))
		got, gitResolved := resolveTemplateDest(root, tc.target)
		if gitResolved {
			t.Errorf("resolveTemplateDest(%q) reported gitResolved for an unclean remainder", tc.target)
		}
		if got != want {
			t.Errorf("resolveTemplateDest(%q) = %q, want literal join %q", tc.target, got, want)
		}
	}
}

// TestResolveTemplateDestGitPathLinkedWorktree is the reason the function
// exists: in a linked worktree `.git` is a gitfile, so the hook must resolve
// through git to the SHARED hooks directory of the main repository.
func TestResolveTemplateDestGitPathLinkedWorktree(t *testing.T) {
	requireGit(t)
	base := t.TempDir()
	main := filepath.Join(base, "main")
	wt := filepath.Join(base, "wt")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("-C", main, "init", "-q")
	run("-C", main, "config", "user.email", "t@example.com")
	run("-C", main, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(main, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("-C", main, "add", ".")
	run("-C", main, "commit", "-qm", "init")
	run("-C", main, "worktree", "add", "-q", "-b", "feature", wt)

	want := filepath.Join(main, ".git", "hooks", "post-merge")
	got, gitResolved := resolveTemplateDest(wt, ".git/hooks/post-merge")
	if !gitResolved {
		t.Errorf("resolveTemplateDest(worktree) = %q, gitResolved = false, want a clean git resolution", got)
	}
	if got != want {
		t.Errorf("resolveTemplateDest(worktree) = %q, want shared hooks dir %q", got, want)
	}
}

// TestResolveTemplateDestFallback pins the fail-open rule: outside any
// repository git rev-parse fails and the literal project-relative path is
// returned, so fixtures without a repo still materialize.
func TestResolveTemplateDestFallback(t *testing.T) {
	requireGit(t)
	root := t.TempDir() // not a git repository
	want := filepath.Join(root, ".git", "hooks", "post-merge")
	got, gitResolved := resolveTemplateDest(root, ".git/hooks/post-merge")
	if gitResolved {
		t.Errorf("resolveTemplateDest(non-repo) reported gitResolved on the fallback path")
	}
	if got != want {
		t.Errorf("resolveTemplateDest(non-repo) = %q, want literal %q", got, want)
	}
}

// --- end-to-end actuator tests through the CLI contract ---

// TestTemplateActuatorFreshWrite materializes into a missing destination:
// rendered bytes on disk, source permission bits copied, exact record
// payload, and the "✓ template" message.
func TestTemplateActuatorFreshWrite(t *testing.T) {
	body := "#!/bin/sh\necho hi\n"
	in := writeTemplateSource(t, body, 0o644)
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	if out.Dest != dest {
		t.Errorf("dest = %q, want %q", out.Dest, dest)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want true")
	}
	wantSHA := sha256Bytes([]byte(body))
	if out.Record == nil || *out.Record != *wantRecord(in, wantSHA) {
		t.Errorf("record = %#v, want %#v", out.Record, wantRecord(in, wantSHA))
	}
	if out.Message != fmt.Sprintf("✓ template %s", in.Target) {
		t.Errorf("message = %q", out.Message)
	}
	if out.Info != "" || len(out.Warnings) != 0 || out.Error != nil {
		t.Errorf("unexpected extras: info %q warnings %#v error %#v", out.Info, out.Warnings, out.Error)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != body {
		t.Errorf("dest bytes = %q, want %q", got, body)
	}
}

// TestTemplateActuatorModeCopied pins the os.chmod(dest, src.st_mode) parity:
// an executable source template lands executable.
func TestTemplateActuatorModeCopied(t *testing.T) {
	in := writeTemplateSource(t, "#!/bin/sh\n", 0o755)
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	info, err := os.Stat(filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target)))
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dest mode = %o, want 755", info.Mode().Perm())
	}
}

// TestTemplateActuatorNotExistsSeedsRenderedCopy: an untracked existing file
// whose bytes match the render is seeded (its sha recorded, no rewrite) so
// the next sync reconciles it.
func TestTemplateActuatorNotExistsSeedsRenderedCopy(t *testing.T) {
	body := "echo seed\n"
	in := writeTemplateSource(t, body, 0o644)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte(body))
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want seeding without rewrite")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
		t.Errorf("record = %#v, want seeded sha", out.Record)
	}
	if out.Message != fmt.Sprintf("· template skipped (exists) %s", in.Target) {
		t.Errorf("message = %q", out.Message)
	}
}

// TestTemplateActuatorNotExistsSeedsLegacyPlaceholder: a pre-render
// placeholder copy (raw catalog bytes still carrying the tokens) is seeded
// with its ACTUAL bytes — the legacy_catalog_sha comparison.
func TestTemplateActuatorNotExistsSeedsLegacyPlaceholder(t *testing.T) {
	body := "topology=__WORKTREE_REPO_TOPOLOGY__\n"
	in := writeTemplateSource(t, body, 0o644)
	in.Config["repo_topology"] = "superrepo" // rendered differs from the raw source
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte(body))
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want seeding without rewrite")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
		t.Errorf("record = %#v, want the legacy placeholder sha", out.Record)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != body {
		t.Errorf("dest bytes changed during seeding: %q", got)
	}
}

// TestTemplateActuatorNotExistsPreservesUntracked pins the exact warning for
// an untracked existing file that matches neither the render nor the legacy
// catalog copy: no write, no lock record, and the skip line still prints.
func TestTemplateActuatorNotExistsPreservesUntracked(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte("user stuff\n"))
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote || out.Record != nil {
		t.Errorf("wrote = %v record = %#v, want none", out.Wrote, out.Record)
	}
	want := fmt.Sprintf(
		"override metadata missing for %s; preserving existing file without assigning ownership. "+
			"To preserve this local file, leave it unchanged. To replace it with the current recipe version, "+
			"remove it and run sync again:\n  rm %s && ai-specs sync", in.Target, in.Target)
	if len(out.Warnings) != 1 || out.Warnings[0] != want {
		t.Errorf("warnings = %#v, want [%q]", out.Warnings, want)
	}
	if out.Message != fmt.Sprintf("· template skipped (exists) %s", in.Target) {
		t.Errorf("message = %q", out.Message)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "user stuff\n" {
		t.Errorf("dest changed: %q (%v)", got, err)
	}
}

// TestTemplateActuatorManagedStaleAutoRefresh: managed_stale with policy auto
// refreshes in place, records the new sha, emits the info line, and STILL
// prints the "· template skipped (exists)" detail line (Python fall-through).
func TestTemplateActuatorManagedStaleAutoRefresh(t *testing.T) {
	body := "echo new\n"
	in := writeTemplateSource(t, body, 0o644)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte("echo old\n"))
	in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte("echo old\n"))}
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want refresh")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
		t.Errorf("record = %#v, want refreshed sha", out.Record)
	}
	if out.Info != fmt.Sprintf("refreshed managed template %s", in.Target) {
		t.Errorf("info = %q", out.Info)
	}
	if out.Message != fmt.Sprintf("· template skipped (exists) %s", in.Target) {
		t.Errorf("message = %q, want the fall-through skip line", out.Message)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != body {
		t.Errorf("dest = %q (%v), want %q", got, err, body)
	}
}

// TestTemplateActuatorStaleRefusalPolicies: managed_stale under confirm and
// never-force refuses with the exact managed-stale warning and no write.
func TestTemplateActuatorStaleRefusalPolicies(t *testing.T) {
	for _, policy := range []string{"confirm", "never-force"} {
		t.Run(policy, func(t *testing.T) {
			in := writeTemplateSource(t, "echo new\n", 0o644)
			in.UpdatePolicy = policy
			dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
			seedDest(t, dest, []byte("echo old\n"))
			in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte("echo old\n"))}
			code, out, stderr := runTemplateActuatorCLI(t, in)
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
			}
			if out.Wrote || out.Record != nil {
				t.Errorf("wrote = %v record = %#v, want refusal", out.Wrote, out.Record)
			}
			want := fmt.Sprintf(
				"override managed-stale (%s-required): %s was not refreshed. "+
					"Refresh with:\n  rm %s && ai-specs sync", policy, in.Target, in.Target)
			if len(out.Warnings) != 1 || out.Warnings[0] != want {
				t.Errorf("warnings = %#v, want [%q]", out.Warnings, want)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != "echo old\n" {
				t.Errorf("dest changed: %q (%v)", got, err)
			}
		})
	}
}

// TestTemplateActuatorUserModifiedRefusal: a user-edited managed target is
// never refreshed, with the exact user-modified warning.
func TestTemplateActuatorUserModifiedRefusal(t *testing.T) {
	in := writeTemplateSource(t, "echo new\n", 0o644)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte("user edited\n"))
	in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte("echo old\n"))}
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote || out.Record != nil {
		t.Errorf("wrote = %v record = %#v, want refusal", out.Wrote, out.Record)
	}
	want := fmt.Sprintf(
		"override user-modified: %s was not refreshed. "+
			"Refresh with:\n  rm %s && ai-specs sync", in.Target, in.Target)
	if len(out.Warnings) != 1 || out.Warnings[0] != want {
		t.Errorf("warnings = %#v, want [%q]", out.Warnings, want)
	}
}

// TestTemplateActuatorManagedCurrentBackfill: a current managed target is
// left byte-identical while the provenance record is backfilled.
func TestTemplateActuatorManagedCurrentBackfill(t *testing.T) {
	body := "echo same\n"
	in := writeTemplateSource(t, body, 0o644)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte(body))
	in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte(body))}
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if out.Wrote {
		t.Errorf("wrote = true, want backfill only")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
		t.Errorf("record = %#v, want backfilled record", out.Record)
	}
	if out.Message != fmt.Sprintf("· template skipped (exists) %s", in.Target) {
		t.Errorf("message = %q", out.Message)
	}
}

// TestTemplateActuatorCRLFShaParity pins that classification hashes
// CRLF-normalized bytes: a managed target whose disk bytes differ from the
// lock sha only by line endings counts as managed_current (backfill, no
// rewrite), and a fresh write keeps the literal CRLF bytes on disk while
// recording the normalized sha.
func TestTemplateActuatorCRLFShaParity(t *testing.T) {
	t.Run("backfill normalizes", func(t *testing.T) {
		in := writeTemplateSource(t, "echo same\n", 0o644)
		dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
		seedDest(t, dest, []byte("echo same\r\n"))
		in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte("echo same\n"))}
		code, out, _ := runTemplateActuatorCLI(t, in)
		if code != 0 {
			t.Fatalf("exit = %d, out %#v", code, out)
		}
		if out.Wrote {
			t.Errorf("wrote = true, want backfill only")
		}
		if out.Record == nil {
			t.Errorf("record = nil, want backfilled record")
		}
	})
	t.Run("fresh write keeps CRLF bytes", func(t *testing.T) {
		body := "echo crlf\r\n"
		in := writeTemplateSource(t, body, 0o644)
		code, out, _ := runTemplateActuatorCLI(t, in)
		if code != 0 {
			t.Fatalf("exit = %d, out %#v", code, out)
		}
		dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
		got, err := os.ReadFile(dest)
		if err != nil || string(got) != body {
			t.Errorf("dest = %q (%v), want literal CRLF %q", got, err, body)
		}
		if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
			t.Errorf("record = %#v, want normalized sha", out.Record)
		}
	})
}

// TestTemplateActuatorAlwaysConditionOverwrites: any condition other than
// not_exists rewrites an existing destination regardless of ownership.
func TestTemplateActuatorAlwaysConditionOverwrites(t *testing.T) {
	body := "echo new\n"
	in := writeTemplateSource(t, body, 0o644)
	in.Condition = "always"
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
	seedDest(t, dest, []byte("user stuff\n"))
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if !out.Wrote {
		t.Errorf("wrote = false, want overwrite")
	}
	if out.Record == nil || out.Record.SHA256 != sha256Bytes([]byte(body)) {
		t.Errorf("record = %#v, want written sha", out.Record)
	}
	if out.Message != fmt.Sprintf("✓ template %s", in.Target) {
		t.Errorf("message = %q", out.Message)
	}
}

// TestTemplateActuatorInvalidPolicy refuses an update_policy outside
// auto | confirm | never-force with the exact reference string, exit 2, and
// no destination created.
func TestTemplateActuatorInvalidPolicy(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	in.UpdatePolicy = "force"
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
	}
	want := fmt.Sprintf("invalid update policy 'force' for template '%s'; expected auto | confirm | never-force", in.Target)
	if out.Error == nil || *out.Error != want {
		t.Fatalf("error = %#v, want %q", out.Error, want)
	}
	if _, err := os.Stat(filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))); !os.IsNotExist(err) {
		t.Errorf("destination created despite refusal: %v", err)
	}
}

// TestTemplateActuatorMissingSource refuses a missing source file with the
// exact reference string naming the absolute joined path, exit 2.
func TestTemplateActuatorMissingSource(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	in.Source = "templates/missing.sh"
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
	}
	want := fmt.Sprintf("template source not found: %s", filepath.Join(in.RecipeDir, "templates", "missing.sh"))
	if out.Error == nil || *out.Error != want {
		t.Fatalf("error = %#v, want %q", out.Error, want)
	}
}

// TestTemplateActuatorSymlinkDestRefused pins the candidate-1 findings fix
// (R1-symlink-following-dest-write, R3-1): os.WriteFile follows a symlink at
// the destination, so a link planted at the managed target would redirect the
// refresh outside the project (and for a dangling link, os.WriteFile CREATES
// the link target). The actuator refuses instead: exit 2 with the exact
// refusal, the link target's bytes untouched, and the link itself still a
// symlink. Symlink creation failure fails loudly (no silent skip).
func TestTemplateActuatorSymlinkDestRefused(t *testing.T) {
	linkBody := "echo victim\n"
	plant := func(t *testing.T, in templateInput, targetPath string, createTarget bool) string {
		t.Helper()
		dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(in.Target))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatalf("mkdir dest parent: %v", err)
		}
		if createTarget {
			if err := os.WriteFile(targetPath, []byte(linkBody), 0o644); err != nil {
				t.Fatalf("write link target: %v", err)
			}
		}
		if err := os.Symlink(targetPath, dest); err != nil {
			t.Fatalf("symlink creation denied on this platform: %v", err)
		}
		return dest
	}
	assertSymlinkIntact := func(t *testing.T, dest, targetPath string, targetCreated bool) {
		t.Helper()
		info, lstatErr := os.Lstat(dest)
		if lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("dest no longer a symlink: %v (%v)", info, lstatErr)
		}
		_, statErr := os.Lstat(targetPath)
		if targetCreated && statErr != nil {
			t.Fatalf("link target missing: %v", statErr)
		}
		if !targetCreated && !os.IsNotExist(statErr) {
			t.Fatalf("dangling link target was created: %v", statErr)
		}
	}

	t.Run("regular target, overwrite path", func(t *testing.T) {
		in := writeTemplateSource(t, "echo new\n", 0o644)
		in.Condition = "always"
		targetPath := filepath.Join(in.ProjectRoot, "victim.sh")
		dest := plant(t, in, targetPath, true)
		code, out, stderr := runTemplateActuatorCLI(t, in)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
		}
		if out.Error == nil || *out.Error != templateSymlinkRefusal(in.Target) {
			t.Fatalf("error = %#v, want %q", out.Error, templateSymlinkRefusal(in.Target))
		}
		assertSymlinkIntact(t, dest, targetPath, true)
	})

	t.Run("regular target, managed_stale auto refresh path", func(t *testing.T) {
		in := writeTemplateSource(t, "echo new\n", 0o644)
		targetPath := filepath.Join(in.ProjectRoot, "victim.sh")
		dest := plant(t, in, targetPath, true)
		in.ManagedEntry = &templateManagedEntry{SHA256: sha256Bytes([]byte(linkBody))}
		code, out, stderr := runTemplateActuatorCLI(t, in)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
		}
		if out.Error == nil || *out.Error != templateSymlinkRefusal(in.Target) {
			t.Fatalf("error = %#v, want %q", out.Error, templateSymlinkRefusal(in.Target))
		}
		assertSymlinkIntact(t, dest, targetPath, true)
	})

	t.Run("dangling link, not_exists fall-through", func(t *testing.T) {
		in := writeTemplateSource(t, "echo new\n", 0o644)
		targetPath := filepath.Join(in.ProjectRoot, "victim.sh")
		dest := plant(t, in, targetPath, false) // the link target is never created
		code, out, stderr := runTemplateActuatorCLI(t, in)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
		}
		if out.Error == nil || *out.Error != templateSymlinkRefusal(in.Target) {
			t.Fatalf("error = %#v, want %q", out.Error, templateSymlinkRefusal(in.Target))
		}
		assertSymlinkIntact(t, dest, targetPath, false)
	})
}

// TestWriteTemplateContentRefusesSymlinkDest pins the destination guard on
// writeTemplateContent itself (R3-toctou-go-dest-guard): a symlink at the
// destination — to a regular file or dangling — is refused with errDestSymlink
// (errors.Is), the link stays a symlink, and the link target is neither
// created nor modified.
func TestWriteTemplateContentRefusesSymlinkDest(t *testing.T) {
	for _, tc := range []struct {
		name         string
		createTarget bool
	}{
		{"regular-file target", true},
		{"dangling target", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "dest.sh")
			targetPath := filepath.Join(base, "victim.sh")
			if tc.createTarget {
				if err := os.WriteFile(targetPath, []byte("echo victim\n"), 0o644); err != nil {
					t.Fatalf("write link target: %v", err)
				}
			}
			if err := os.Symlink(targetPath, dest); err != nil {
				t.Fatalf("symlink creation denied on this platform: %v", err)
			}
			err := writeTemplateContent(dest, []byte("echo new\n"), 0o644)
			if !errors.Is(err, errDestSymlink) {
				t.Fatalf("err = %v, want errDestSymlink", err)
			}
			info, lstatErr := os.Lstat(dest)
			if lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("dest no longer a symlink: %v (%v)", info, lstatErr)
			}
			_, statErr := os.Lstat(targetPath)
			if tc.createTarget {
				got, readErr := os.ReadFile(targetPath)
				if readErr != nil || string(got) != "echo victim\n" {
					t.Fatalf("link target modified: %q (%v)", got, readErr)
				}
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("dangling link target was created: %v", statErr)
			}
		})
	}
}

// TestTemplateActuatorEscapingTargetRefused pins R1-dest-containment-absent:
// a target containing `..` that resolves OUTSIDE the project root is refused
// with an exit-2 decision envelope, and nothing is written outside the
// project (the literal join used to clean the path and write wherever it
// landed). A target whose `..` stays inside the root ("a/../b.sh") remains
// allowed — only escapes are refusals.
func TestTemplateActuatorEscapingTargetRefused(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	in.Target = "../outside/escape.sh"
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
	}
	if out.Error == nil || *out.Error != templateEscapingTargetRefusal(in.Target) {
		t.Fatalf("error = %#v, want %q", out.Error, templateEscapingTargetRefusal(in.Target))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(in.ProjectRoot), "outside", "escape.sh")); !os.IsNotExist(err) {
		t.Errorf("file written outside the project root: %v", err)
	}
	// The in-root twin of the same escape shape must keep materializing.
	passed := writeTemplateSource(t, "echo hi\n", 0o644)
	passed.Target = "ai-specs/../stays.sh"
	code, out, stderr = runTemplateActuatorCLI(t, passed)
	if code != 0 {
		t.Fatalf("in-root `..` target: exit = %d, stderr %q, out %#v", code, stderr, out)
	}
	if _, err := os.Stat(filepath.Join(passed.ProjectRoot, "stays.sh")); err != nil {
		t.Errorf("in-root target not materialized: %v", err)
	}
}

// TestTemplateActuatorGitPrefixEscapeRefused pins R1-git-prefix-containment-bypass:
// a ".git/"-prefixed target whose remainder is an UNCLEAN path (carries `..`
// components) must be refused. `git rev-parse --git-path` honors
// parent-directory components (it exits 0 emitting an escaping path for
// "../../outside/evil"), and both git-failure fallbacks return the literal
// join, which filepath.Join also cleans — so a target like
// ".git/../../escape.sh" must never claim the ".git/" prefix exemption from
// destination containment.
func TestTemplateActuatorGitPrefixEscapeRefused(t *testing.T) {
	for _, target := range []string{
		".git/../../escape.sh",
		".git/hooks/../../../escape.sh",
	} {
		t.Run(target, func(t *testing.T) {
			in := writeTemplateSource(t, "echo hi\n", 0o644)
			in.Target = target
			code, out, stderr := runTemplateActuatorCLI(t, in)
			if code != 2 {
				t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
			}
			if out.Error == nil || *out.Error != templateEscapingTargetRefusal(in.Target) {
				t.Fatalf("error = %#v, want %q", out.Error, templateEscapingTargetRefusal(in.Target))
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(in.ProjectRoot), "escape.sh")); !os.IsNotExist(err) {
				t.Errorf("file written outside the project root: %v", err)
			}
		})
	}
}

// TestTemplateActuatorSymlinkedAncestorRefused pins R1-ancestor-symlink-traversal-deferred
// end to end: a symlinked ANCESTOR directory of the destination (ai-specs →
// outside the project) redirects the write outside the project even though
// the destination path is lexically contained. The actuator refuses with an
// exit-2 decision envelope and nothing is created through the link.
func TestTemplateActuatorSymlinkedAncestorRefused(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	outside := filepath.Join(filepath.Dir(in.ProjectRoot), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(in.ProjectRoot, "ai-specs")); err != nil {
		t.Fatalf("symlink creation denied on this platform: %v", err)
	}
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
	}
	if out.Error == nil || *out.Error != templateAncestorSymlinkRefusal(in.Target) {
		t.Fatalf("error = %#v, want %q", out.Error, templateAncestorSymlinkRefusal(in.Target))
	}
	if _, err := os.Lstat(filepath.Join(outside, "recipes")); !os.IsNotExist(err) {
		t.Errorf("directory created through the symlinked ancestor: %v", err)
	}
}

// TestWriteTemplateContentRefusesSymlinkedAncestor pins the ancestor walk on
// its guard unit (R1-ancestor-symlink-traversal-deferred): an ancestor
// directory of dest below the containment root that is a symlink is refused
// with errAncestorSymlink before MkdirAll can create anything through the
// link, and a real-directory ancestor chain still writes.
func TestWriteTemplateContentRefusesSymlinkedAncestor(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "ai-specs")); err != nil {
		t.Fatalf("symlink creation denied on this platform: %v", err)
	}
	dest := filepath.Join(root, "ai-specs", "recipes", "x.sh")
	err := ensureTemplateAncestorsReal(root, dest)
	if !errors.Is(err, errAncestorSymlink) {
		t.Fatalf("err = %v, want errAncestorSymlink", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "recipes")); !os.IsNotExist(statErr) {
		t.Errorf("directory created through the symlinked ancestor: %v", statErr)
	}
	// The happy path is unchanged: a real-directory ancestor chain passes the
	// walk and writeTemplateContent writes through it.
	ok := filepath.Join(root, "ai-specs-real", "recipes", "x.sh")
	if err := ensureTemplateAncestorsReal(root, ok); err != nil {
		t.Fatalf("real ancestors: err = %v", err)
	}
	if err := writeTemplateContent(ok, []byte("echo x\n"), 0o644); err != nil {
		t.Fatalf("real ancestors: write err = %v", err)
	}
	if got, readErr := os.ReadFile(ok); readErr != nil || string(got) != "echo x\n" {
		t.Errorf("real-ancestor write = %q (%v)", got, readErr)
	}
}

// TestTemplateActuatorDirectorySourceRefused pins the non-regular-source arm
// of the source guard: a source that exists but is a directory refuses with
// the same "template source not found" reference string (the
// srcInfo.Mode().IsRegular() check), exit 2.
func TestTemplateActuatorDirectorySourceRefused(t *testing.T) {
	in := writeTemplateSource(t, "echo hi\n", 0o644)
	if err := os.MkdirAll(filepath.Join(in.RecipeDir, "templates", "missing.sh"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	in.Source = "templates/missing.sh"
	code, out, stderr := runTemplateActuatorCLI(t, in)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, out %#v", code, stderr, out)
	}
	want := fmt.Sprintf("template source not found: %s", filepath.Join(in.RecipeDir, "templates", "missing.sh"))
	if out.Error == nil || *out.Error != want {
		t.Fatalf("error = %#v, want %q", out.Error, want)
	}
}

// TestPyConfigStringDomain pins the Python str() parity of pyConfigString
// over the whole config value domain the JSON decoder can deliver: strings
// verbatim, capitalized bools, None for null, and numbers as their literal
// text (json.Number preserves the decoded literal).
func TestPyConfigStringDomain(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "standalone", "standalone"},
		{"true", true, "True"},
		{"false", false, "False"},
		{"null", nil, "None"},
		{"int literal", json.Number("42"), "42"},
		{"float literal", json.Number("1.5"), "1.5"},
	}
	for _, tc := range cases {
		if got := pyConfigString(tc.in); got != tc.want {
			t.Errorf("%s: pyConfigString(%v) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestTemplateConfigOrNumericZeroFallback pins `str(cfg.get(key) or default)`
// falsiness for numbers: a numeric zero config value is falsy in Python and
// falls back to the default, while any other number is used verbatim.
func TestTemplateConfigOrNumericZeroFallback(t *testing.T) {
	cfg := map[string]any{"worktrees_dir": json.Number("0"), "integration_branch": json.Number("7")}
	if got := templateConfigOr(cfg, "worktrees_dir", ".worktrees"); got != ".worktrees" {
		t.Errorf("numeric zero = %q, want the default", got)
	}
	if got := templateConfigOr(cfg, "integration_branch", "main"); got != "7" {
		t.Errorf("numeric non-zero = %q, want \"7\"", got)
	}
}

// TestTemplateActuatorMalformedEnvelope rejects input that is not the
// documented envelope with exit 2 and a stderr diagnostic (the bridge then
// falls back to Python).
func TestTemplateActuatorMalformedEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMaterializeTemplate(strings.NewReader("not json"), &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2 (stdout %q)", code, stdout.String())
	}
	if strings.TrimSpace(stderr.String()) == "" {
		t.Errorf("expected a stderr diagnostic, got %q", stderr.String())
	}
}

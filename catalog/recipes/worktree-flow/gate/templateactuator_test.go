package main

import (
	"bytes"
	"encoding/json"
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
		if got := resolveTemplateDest(root, tc.target); got != want {
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
	if got := resolveTemplateDest(root, ".git/hooks/post-merge"); got != want {
		t.Errorf("resolveTemplateDest = %q, want %q", got, want)
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
	if got := resolveTemplateDest(wt, ".git/hooks/post-merge"); got != want {
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
	if got := resolveTemplateDest(root, ".git/hooks/post-merge"); got != want {
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
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
	if err := os.WriteFile(dest, []byte("user stuff\n"), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
	if err := os.WriteFile(dest, []byte("echo old\n"), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
			if err := os.WriteFile(dest, []byte("echo old\n"), 0o644); err != nil {
				t.Fatalf("write dest: %v", err)
			}
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
	if err := os.WriteFile(dest, []byte("user edited\n"), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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
		if err := os.WriteFile(dest, []byte("echo same\r\n"), 0o644); err != nil {
			t.Fatalf("write dest: %v", err)
		}
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
	if err := os.WriteFile(dest, []byte("user stuff\n"), 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}
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

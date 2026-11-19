package sync

// T5 lock hygiene: generated hook shims record their owner in the lock and are
// pruned by that record, and sync drops [deps.*] / [managed.*] entries whose
// source no longer exists.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

func lockhygienePtr(s string) *string { return &s }

// hookShimFixture installs one hook-declaring catalog recipe in an isolated
// CLI home and returns the home and project paths.
func hookShimFixture(t *testing.T, agents string) (home, project string) {
	t.Helper()
	source, _ := spineEnv(t)
	home = t.TempDir()
	for _, name := range []string{"templates", "bundled-skills", "bundled-commands", "VERSION"} {
		if err := os.Symlink(filepath.Join(source, name), filepath.Join(home, name)); err != nil {
			t.Fatal(err)
		}
	}
	recipe := filepath.Join(home, "catalog/recipes/hookdemo")
	spineWrite(t, filepath.Join(recipe, "recipe.toml"),
		"[recipe]\nid = 'hookdemo'\nname = 'Demo'\ndescription = 'Fixture'\nversion = '1.0.0'\n"+
			"[[provides.hooks]]\nid = 'main'\nevent = 'pre-tool-use'\nscript = 'hooks/main.sh'\nmatcher = 'Bash'\n", 0o644)
	spineWrite(t, filepath.Join(recipe, "hooks/main.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	project = spineProject(t, "[project]\nname = 'shim-owner'\n[brief]\nrender = false\n[agents]\nenabled = "+agents+"\n[recipes.hookdemo]\nenabled = true\n")
	return home, project
}

func runSyncCase(t *testing.T, home, project string) string {
	t.Helper()
	var out, stderr bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &stderr); rc != 0 {
		t.Fatalf("sync rc=%d, stderr=%q", rc, stderr.String())
	}
	return stderr.String()
}

func readLockText(t *testing.T, project string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, "ai-specs/.ai-specs.lock"))
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	return string(data)
}

// T5.1: a generated shim is recorded with its owning hook when written, and
// the record authorizes deleting it even after it was edited or its marker was
// stripped.
func TestSyncPrunesEditedHookShimByOwnershipRecord(t *testing.T) {
	home, project := hookShimFixture(t, "['cursor', 'pi']")
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("seed sync stderr = %q", stderr)
	}
	const owner = "ai-specs:hooks:hookdemo:main"
	shims := []string{".cursor/hooks/hookdemo-main.sh", ".pi/extensions/hookdemo-main.ts"}
	text := readLockText(t, project)
	for _, rel := range shims {
		if !strings.Contains(text, "["+"hooks]") || !strings.Contains(text, `"`+rel+`" = "`+owner+`"`) {
			t.Fatalf("lock does not record %s as %s:\n%s", rel, owner, text)
		}
	}
	for _, rel := range shims {
		spineWrite(t, filepath.Join(project, rel), "locally edited, marker stripped\n", 0o644)
	}
	spineWrite(t, filepath.Join(project, "ai-specs/ai-specs.toml"),
		"[project]\nname = 'shim-owner'\n[brief]\nrender = false\n[agents]\nenabled = ['cursor', 'pi']\n", 0o644)
	if stderr := runSyncCase(t, home, project); stderr != "" {
		t.Fatalf("prune sync stderr = %q", stderr)
	}
	for _, rel := range shims {
		if _, err := os.Lstat(filepath.Join(project, rel)); !os.IsNotExist(err) {
			t.Errorf("%s still exists after its owner disappeared: %v", rel, err)
		}
	}
	if text := readLockText(t, project); strings.Contains(text, "hookdemo-main") {
		t.Errorf("pruned shims kept lock records:\n%s", text)
	}
}

// T5.1: an unrecorded legacy shim is never deleted, and sync says so once.
func TestSyncKeepsUnrecordedLegacyHookShimWithOneNotice(t *testing.T) {
	home, _ := spineEnv(t)
	const rel = ".pi/extensions/hookdemo-main.ts"
	const edited = hooksTSHeader + "hookdemo:main (event tool_call).\n// locally edited\n"
	project := spineProject(t, "[project]\nname = 'shim-owner'\n[brief]\nrender = false\n[agents]\nenabled = ['pi']\n")
	spineWrite(t, filepath.Join(project, rel), edited, 0o644)
	for attempt := 1; attempt <= 2; attempt++ {
		stderr := runSyncCase(t, home, project)
		if got, err := os.ReadFile(filepath.Join(project, rel)); err != nil || string(got) != edited {
			t.Fatalf("attempt %d: unrecorded legacy shim was not preserved: %v", attempt, err)
		}
		lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
		if len(lines) != 1 || !strings.Contains(lines[0], "not recorded as ai-specs-managed") || !strings.Contains(lines[0], rel) {
			t.Fatalf("attempt %d: want exactly one ownership notice for %s, got %q", attempt, rel, stderr)
		}
	}
}

// T5.2: sync drops a [deps.*] entry the manifest no longer declares, together
// with its in-project vendored tree.
func TestSyncPrunesLockDepsForUndeclaredDep(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'dep-prune'\n[brief]\nrender = false\n[agents]\nenabled = ['claude']\n")
	spineWrite(t, filepath.Join(project, "ai-specs/.deps/gone/skills/gone/SKILL.md"), "gone\n", 0o644)
	lk := lock.NewLock()
	lock.SetDepSkillHashes(lk, "gone", "gone", map[string]string{"skills/gone/SKILL.md": "hash"})
	lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	runSyncCase(t, home, project)
	after, err := lock.LoadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Deps["gone"]; ok {
		t.Errorf("undeclared dep kept its lock record: %v", after.Deps)
	}
	if _, err := os.Lstat(filepath.Join(project, "ai-specs/.deps/gone")); !os.IsNotExist(err) {
		t.Errorf("undeclared dep kept its vendored tree: %v", err)
	}
}

// T5.3: a [managed.*] entry whose recipe is no longer produced is dropped once
// its artifact is already gone, and kept (with the file untouched) while the
// artifact still exists.
func TestSyncDropsStaleManagedRecordOnlyWhenArtifactIsGone(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'stale-managed'\n[brief]\nrender = false\n[agents]\nenabled = ['claude']\n")
	spineWrite(t, filepath.Join(project, "docs/kept.md"), "user kept\n", 0o644)
	lk := lock.NewLock()
	lock.SetManagedOverride(lk, "docs/gone.md", "sha-gone", lockhygienePtr("retired"), lockhygienePtr("doc.md"), lockhygienePtr("doc"), lockhygienePtr("auto"))
	lock.SetManagedOverride(lk, "docs/kept.md", "sha-kept", lockhygienePtr("retired"), lockhygienePtr("doc.md"), lockhygienePtr("doc"), lockhygienePtr("auto"))
	lock.SetBriefBaseline(lk, "AGENTS.md", "sha-brief")
	lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	runSyncCase(t, home, project)
	after, err := lock.LoadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Managed["docs/gone.md"]; ok {
		t.Errorf("stale record survived an absent artifact: %v", after.Managed)
	}
	if after.Managed["docs/kept.md"] == nil {
		t.Error("record dropped although its artifact still exists")
	}
	if after.Managed["AGENTS.md"] == nil {
		t.Error("a record with no owning recipe was dropped")
	}
	if got, err := os.ReadFile(filepath.Join(project, "docs/kept.md")); err != nil || string(got) != "user kept\n" {
		t.Errorf("artifact of a retired recipe was touched: %q, %v", got, err)
	}
}

// T5.2: a dep record survives when both dep trees are already absent, so sync
// must prune lock-only dep IDs the manifest does not declare.
func TestSyncPrunesLockDepsWithoutVendoredTree(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'dep-lock-only'\n[brief]\nrender = false\n[agents]\nenabled = ['claude']\n")
	lk := lock.NewLock()
	lock.SetDepSkillHashes(lk, "gone", "gone", map[string]string{"skills/gone/SKILL.md": "hash"})
	lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	runSyncCase(t, home, project)
	after, err := lock.LoadLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Deps["gone"]; ok {
		t.Errorf("lock-only undeclared dep kept its record: %v", after.Deps)
	}
}

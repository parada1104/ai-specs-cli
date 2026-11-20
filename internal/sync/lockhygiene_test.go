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

// T5.1: a shim record whose path is absent loses its authority even when the
// target it named is no longer part of the manifest, so no sync visits it.
func TestSyncDropsHookRecordOfUnvisitedTargetWhenShimIsGone(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'shim-sweep'\n[brief]\nrender = false\n[agents]\nenabled = ['pi']\n")
	lk := lock.NewLock()
	lock.SetHookShim(lk, "gone/.pi/extensions/hookdemo-main.ts", "ai-specs:hooks:hookdemo:main")
	lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	runSyncCase(t, home, project)
	if text := readLockText(t, project); strings.Contains(text, "hookdemo-main") {
		t.Errorf("absent shim of an unvisited target kept its record:\n%s", text)
	}
}

// T6.3 (S1): a refusal must precede the materialize fallback, which prunes
// lock deps. `sync-agent --omp` on a project whose `.omp/skills` is a real
// directory refuses with the lock (and its undeclared [deps.*] record) byte-
// identical.
func TestSyncAgentRefusalDoesNotPruneLockDeps(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, "[project]\nname = 'refusal-order'\n[brief]\nrender = false\n[agents]\nenabled = ['omp']\n")
	spineWrite(t, filepath.Join(project, ".omp/skills/mine/SKILL.md"), "# mine\n", 0o644)
	lk := lock.NewLock()
	lock.SetDepSkillHashes(lk, "gone", "gone", map[string]string{"skills/gone/SKILL.md": "hash"})
	lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
	if err := lock.WriteLock(lockPath, lk); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if rc := RunAgent([]string{"sync-agent", project, "--omp"}, home, strings.NewReader(""), &out, &errOut); rc != 1 {
		t.Fatalf("rc = %d, want 1; stdout=%q stderr=%q", rc, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "✗ refuse to overwrite non-symlink: "+filepath.Join(project, ".omp/skills")) {
		t.Errorf("stderr = %q, want the skills refusal", errOut.String())
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("lock changed despite the refusal:\nbefore=%q\nafter=%q", before, after)
	}
	if got, err := os.ReadFile(filepath.Join(project, ".omp/skills/mine/SKILL.md")); err != nil || string(got) != "# mine\n" {
		t.Errorf("user skill changed: %q (%v)", got, err)
	}
}

// T6b (S1): the hook PRUNE plan follows the same inside-root rule as the agent
// preflight — a symlinked agent parent that fully resolves inside the project
// root is allowed and the retired wiring inside it is still pruned; a link that
// resolves outside is refused before any write.
func TestHookPruneFollowsInsideRootLinkRule(t *testing.T) {
	t.Run("retired claude wiring pruned inside the link", func(t *testing.T) {
		home, _ := spineEnv(t)
		project := spineProject(t, "[project]\nname = 'link-claude'\n[brief]\nrender = false\n[agents]\nenabled = ['claude']\n")
		inside := filepath.Join(project, "config", "claude")
		spineWrite(t, filepath.Join(inside, "settings.json"),
			`{"hooks": {"PreToolUse": [{"_ai_specs_managed": "ai-specs:hooks:hookdemo:gone", "matcher": "x"}]}}`, 0o644)
		mustSymlink(t, inside, filepath.Join(project, ".claude"))

		if stderr := runSyncCase(t, home, project); stderr != "" {
			t.Fatalf("sync stderr = %q", stderr)
		}
		settings, err := os.ReadFile(filepath.Join(inside, "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(settings), "ai-specs:hooks:hookdemo:gone") {
			t.Errorf("retired hook wiring survived inside the link: %s", settings)
		}
		if _, err := os.Lstat(filepath.Join(inside, "skills")); err != nil {
			t.Errorf("skills link not created inside the linked dir: %v", err)
		}
	})

	t.Run("retired cursor shim pruned inside the link", func(t *testing.T) {
		home, _ := spineEnv(t)
		project := spineProject(t, "[project]\nname = 'link-cursor'\n[brief]\nrender = false\n[agents]\nenabled = ['cursor']\n")
		inside := filepath.Join(project, "config", "cursor")
		spineWrite(t, filepath.Join(inside, "hooks", "hookdemo-gone.sh"), "old shim\n", 0o644)
		mustSymlink(t, inside, filepath.Join(project, ".cursor"))
		lk := lock.NewLock()
		lock.SetHookShim(lk, ".cursor/hooks/hookdemo-gone.sh", "ai-specs:hooks:hookdemo:gone")
		lockPath := filepath.Join(project, "ai-specs/.ai-specs.lock")
		if err := lock.WriteLock(lockPath, lk); err != nil {
			t.Fatal(err)
		}

		if stderr := runSyncCase(t, home, project); stderr != "" {
			t.Fatalf("sync stderr = %q", stderr)
		}
		if _, err := os.Lstat(filepath.Join(inside, "hooks", "hookdemo-gone.sh")); !os.IsNotExist(err) {
			t.Errorf("retired shim survived inside the link (err=%v)", err)
		}
		after, err := lock.LoadLock(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := after.Hooks[".cursor/hooks/hookdemo-gone.sh"]; ok {
			t.Errorf("shim record survived: %v", after.Hooks)
		}
	})

	t.Run("outside link still refuses before any write", func(t *testing.T) {
		home, _ := spineEnv(t)
		project := spineProject(t, "[project]\nname = 'link-outside'\n[brief]\nrender = false\n[agents]\nenabled = ['claude']\n")
		outside := t.TempDir()
		settings := filepath.Join(outside, "settings.json")
		spineWrite(t, settings, `{"hooks": {"PreToolUse": [{"_ai_specs_managed": "ai-specs:hooks:hookdemo:gone"}]}}`, 0o644)
		before, err := os.ReadFile(settings)
		if err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, outside, filepath.Join(project, ".claude"))

		var out, errOut bytes.Buffer
		if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &out, &errOut); rc != 1 {
			t.Fatalf("rc = %d, want 1; stderr=%q", rc, errOut.String())
		}
		if !strings.Contains(errOut.String(), "ancestor path is a symlink: ") || !strings.Contains(errOut.String(), "/.claude —") {
			t.Errorf("stderr = %q, want the ancestor refusal", errOut.String())
		}
		after, err := os.ReadFile(settings)
		if err != nil || !bytes.Equal(before, after) {
			t.Errorf("outside settings changed: %q (%v)", after, err)
		}
	})
}

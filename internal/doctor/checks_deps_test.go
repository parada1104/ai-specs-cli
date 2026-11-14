package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

const depsManifest = "[project]\nname = \"p\"\n\n[[deps]]\nid = \"ghost\"\nsource = \"https://example.com/ghost\"\n"

// A registered dep with no copy under ai-specs/.deps/<id> is silent breakage
// (sync has not vendored it yet): doctor must say so and name the fix.
func TestRegisteredDepNotInstalledWarns(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, depsManifest)

	d := New(root, t.TempDir())
	d.checkRegisteredDeps()

	assertChecks(t, d, []Check{
		{WARN, "deps", "dep 'ghost' is registered but not installed",
			"ai-specs sync (vendors registered deps into ai-specs/.deps)"},
	})
}

// An installed dep keeps doctor silent: no extra OK line for projects that are fine.
func TestRegisteredDepInstalledIsSilent(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, depsManifest)
	if err := os.MkdirAll(filepath.Join(root, "ai-specs", ".deps", "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}

	d := New(root, t.TempDir())
	d.checkRegisteredDeps()

	assertChecks(t, d, []Check{})
}

// No [[deps]] at all: nothing to check.
func TestNoRegisteredDepsIsSilent(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"p\"\n")

	d := New(root, t.TempDir())
	d.checkRegisteredDeps()

	assertChecks(t, d, []Check{})
}

// An id that could escape ai-specs/.deps is never joined into a path.
func TestRegisteredDepWithUnsafeIDIsSkipped(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"p\"\n\n[[deps]]\nid = \"../../etc\"\nsource = \"x\"\n")

	d := New(root, t.TempDir())
	d.checkRegisteredDeps()

	assertChecks(t, d, []Check{})
}

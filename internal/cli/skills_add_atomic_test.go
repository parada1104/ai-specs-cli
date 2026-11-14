package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillsAddAtomicProject builds a synced-shaped project plus the shared sources.
func skillsAddAtomicProject(t *testing.T) (proj, home, srcOK, srcNoSkill string) {
	t.Helper()
	root, rerr := locateRoot()
	if rerr != nil {
		t.Skipf("repo root not located: %v", rerr)
	}
	srcOK, srcNoSkill, _ = skillsAddFixtures(t, root)
	proj = filepath.Join(t.TempDir(), "proj")
	buildAddFixture(t, "ok", proj)
	// A full isolated home: the delegated sync needs templates/ and bundled-*.
	return proj, skillsListHome(t, root), srcOK, srcNoSkill
}

func readManifest(t *testing.T, proj string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(proj, "ai-specs", "ai-specs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A source that cannot be vendored must not leave its [[deps]] block behind:
// every later `sync` used to fail on it until `skills remove`.
func TestSkillsAddRestoresTheManifestWhenSyncFails(t *testing.T) {
	proj, home, _, srcNoSkill := skillsAddAtomicProject(t)
	before := readManifest(t, proj)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skills", "add", srcNoSkill, proj}, home, nil, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("exit = 0, want the sync failure's non-zero exit\nstderr: %s", stderr.String())
	}
	if after := readManifest(t, proj); after != before {
		t.Errorf("manifest was not restored after the failed sync:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if _, err := os.Stat(filepath.Join(proj, "ai-specs", ".deps", "src-noskill")); err == nil {
		t.Errorf("a partially vendored ai-specs/.deps/src-noskill was left behind")
	}
	if !strings.Contains(stderr.String(), "restored") {
		t.Errorf("stderr must say the manifest was restored, got:\n%s", stderr.String())
	}
}

// Only what this add created may be removed: a pre-existing .deps/<id> directory
// can hold data the user vendored by hand.
func TestSkillsAddRollbackKeepsAPreexistingDepsDirectory(t *testing.T) {
	proj, home, _, srcNoSkill := skillsAddAtomicProject(t)
	keep := filepath.Join(proj, "ai-specs", ".deps", "src-noskill", "keep.txt")
	skillsWriteFile(t, keep, "user data\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"skills", "add", srcNoSkill, proj}, home, nil, &stdout, &stderr); code == 0 {
		t.Fatalf("exit = 0, want non-zero\nstderr: %s", stderr.String())
	}

	if data, err := os.ReadFile(keep); err != nil || string(data) != "user data\n" {
		t.Errorf("pre-existing .deps data was not preserved: %v %q", err, data)
	}
}

// A successful add keeps the registration (previous behavior).
func TestSkillsAddKeepsTheDepWhenSyncSucceeds(t *testing.T) {
	proj, home, srcOK, _ := skillsAddAtomicProject(t)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"skills", "add", srcOK, proj}, home, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr.String())
	}

	if !strings.Contains(readManifest(t, proj), `id = "src-ok"`) {
		t.Errorf("the registered dep is missing after a successful add:\n%s", readManifest(t, proj))
	}
}

// Package assets embeds the CLI-bundled asset trees (catalog, templates,
// bundled-skills, bundled-commands) into the single binary and exposes a
// filesystem-agnostic accessor over them.
//
// Version isolation guarantee: today bundled assets are version-pinned by
// the per-install-root cache layout ({install-root}/cache/.bundled — each
// CLI version owns its cache). With embedding, the pin becomes the binary
// itself: every build carries exactly the asset tree of its own source
// revision, so per-version isolation holds by construction — there is no
// cache to populate, repair, or share across versions.
//
// User-vendored deps are NOT assets: they stay on disk (card 11).
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// assetRoots mirrors Roots(); the on-disk trees live next to this package at
// the module root (go test runs with the package dir as cwd).
var assetRoots = []string{"catalog", "templates", "bundled-skills", "bundled-commands"}

// TestRootsMatchesEmbeddedTrees pins the root set.
func TestRootsMatchesEmbeddedTrees(t *testing.T) {
	got := Roots()
	if len(got) != len(assetRoots) {
		t.Fatalf("Roots() = %v, want %v", got, assetRoots)
	}
	for i, r := range assetRoots {
		if got[i] != r {
			t.Errorf("Roots()[%d] = %q, want %q", i, got[i], r)
		}
	}
}

// TestEmbeddedDigestsMatchOnDiskSources is the build-time verification
// acceptance criterion: the embedded tree must be byte-identical (sha256 per
// file) to the repo's on-disk asset sources, with the file sets equal in
// both directions — proving the embed patterns neither skip nor add files.
//
// Written justification for the one deliberate skip: go:embed structurally
// excludes nested Go modules, so the disk walk skips any directory holding a
// go.mod (the only one is catalog/recipes/worktree-flow/gate — the
// worktree-gate build-time source, a build-time input for the opt-in local
// build path, never a runtime-read data asset; see assets.go's package doc).
func TestEmbeddedDigestsMatchOnDiskSources(t *testing.T) {
	for _, root := range assetRoots {
		disk := map[string]string{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Nested Go modules are structurally un-embeddable
				// (see the test doc comment); do not compare them.
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return fs.SkipDir
				}
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(raw)
			// The embedded FS is rooted at the asset root (fs.Sub), so
			// compare on root-relative keys in both directions.
			rel := strings.TrimPrefix(path, root+"/")
			disk[rel] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil {
			t.Fatalf("walk disk %s: %v", root, err)
		}
		embed := map[string]string{}
		afs, ferr := FS(root)
		if ferr != nil {
			t.Fatalf("FS(%q): %v", root, ferr)
		}
		werr := fs.WalkDir(afs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			raw, err := fs.ReadFile(afs, path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(raw)
			embed[path] = hex.EncodeToString(sum[:])
			return nil
		})
		if werr != nil {
			t.Fatalf("walk embedded %s: %v", root, werr)
		}
		if len(disk) == 0 {
			t.Errorf("root %s: on-disk walk found no files (broken cwd?)", root)
			continue
		}
		for path, want := range disk {
			got, ok := embed[path]
			if !ok {
				t.Errorf("root %s: embedded tree missing %s", root, path)
				continue
			}
			if got != want {
				t.Errorf("root %s: digest mismatch for %s", root, path)
			}
		}
		for path := range embed {
			if _, ok := disk[path]; !ok {
				t.Errorf("root %s: embedded tree has extra file %s", root, path)
			}
		}
	}
}

// TestResolvesWithoutAI_SPECS_HOME_andWithoutCache is the acceptance
// criterion that the binary resolves every bundled asset with AI_SPECS_HOME
// unset and no cache directory present: the accessor must serve the embedded
// tree without touching either.
func TestResolvesWithoutAI_SPECS_HOME_andWithoutCache(t *testing.T) {
	t.Setenv("AI_SPECS_HOME", "")
	t.Setenv("AI_SPECS_CACHE", "")
	t.Setenv("AI_SPECS_ASSETS_DIR", "")
	total := 0
	for _, root := range assetRoots {
		afs, err := FS(root)
		if err != nil {
			t.Fatalf("FS(%q): %v", root, err)
		}
		n := 0
		err = fs.WalkDir(afs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				n++
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
		if n == 0 {
			t.Errorf("root %s: embedded tree is empty", root)
		}
		total += n
	}
	if total == 0 {
		t.Fatal("no embedded assets resolved")
	}
	// Spot-check real content through Read.
	raw, err := Read("bundled-commands", "rules-audit.md")
	if err != nil || len(raw) == 0 {
		t.Fatalf("Read bundled-commands/rules-audit.md: err=%v len=%d", err, len(raw))
	}
	if !strings.Contains(string(raw), "#") {
		t.Error("bundled command content looks empty")
	}
}

// TestDevModeOverride covers the dev-mode acceptance criterion: pointing
// AI_SPECS_ASSETS_DIR at an on-disk tree serves THAT tree without a rebuild,
// per root; roots absent from the override dir fall back to embedded.
func TestDevModeOverride(t *testing.T) {
	dir := t.TempDir()
	// Override only templates/: a modified template plus an extra file.
	ovr := filepath.Join(dir, "templates")
	if err := os.MkdirAll(ovr, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ovr, "dev-override.txt"), []byte("DEV"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ovr, "ai-specs.toml.tmpl"), []byte("# dev template"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_SPECS_ASSETS_DIR", dir)

	tfs, err := FS("templates")
	if err != nil {
		t.Fatalf("FS(templates): %v", err)
	}
	raw, err := fs.ReadFile(tfs, "dev-override.txt")
	if err != nil || string(raw) != "DEV" {
		t.Fatalf("dev override file: err=%v content=%q", err, raw)
	}
	raw, err = fs.ReadFile(tfs, "ai-specs.toml.tmpl")
	if err != nil || string(raw) != "# dev template" {
		t.Fatalf("overridden template: err=%v content=%q", err, raw)
	}

	// catalog is NOT in the override dir: falls back to embedded.
	cfs, err := FS("catalog")
	if err != nil {
		t.Fatalf("FS(catalog): %v", err)
	}
	if _, err := fs.ReadFile(cfs, "README.md"); err != nil {
		t.Fatalf("catalog fallback to embedded failed: %v", err)
	}

	// Env unset again (subtest without override): embedded template wins.
	t.Setenv("AI_SPECS_ASSETS_DIR", "")
	tfs2, err := FS("templates")
	if err != nil {
		t.Fatalf("FS(templates) after unset: %v", err)
	}
	raw, err = fs.ReadFile(tfs2, "ai-specs.toml.tmpl")
	if err != nil || strings.Contains(string(raw), "# dev template") {
		t.Fatalf("embedded template expected after unset: err=%v content=%q", err, raw)
	}
}

// TestWalkListsSortedPaths sanity-checks the Walk helper used by consumers.
func TestWalkListsSortedPaths(t *testing.T) {
	var got []string
	err := Walk("bundled-commands", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			got = append(got, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(got) != 2 || !sort.StringsAreSorted(got) {
		t.Fatalf("Walk(bundled-commands) = %v, want 2 sorted .md paths", got)
	}
}

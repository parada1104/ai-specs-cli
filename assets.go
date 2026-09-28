// Package assets embeds the CLI-bundled asset trees (catalog, templates,
// bundled-skills, bundled-commands) into the single binary and exposes a
// filesystem-agnostic accessor over them.
//
// Card [Go 06] of the Go single-binary migration epic. go:embed is stdlib;
// zero third-party dependencies; CGO_ENABLED=0 builds.
//
// Version isolation guarantee: before embedding, bundled assets were
// version-pinned by the per-install-root cache layout — each CLI install
// owned its {install-root}/cache/.bundled tier, populated only from that
// install's shipped assets. With embedding the pin becomes the binary
// itself: every build carries exactly the asset tree of its own source
// revision, so per-version isolation holds by construction and there is no
// cache to populate, repair, or share across versions.
//
// Carve-out: go:embed structurally excludes nested Go modules, so the
// worktree-gate source module (catalog/recipes/worktree-flow/gate, own
// go.mod) is NOT embedded. Its runtime-read sibling
// catalog/recipes/worktree-flow/bin/SHA256SUMS (the acquisition trust root)
// IS embedded. The gate source is a build-time input for the opt-in
// local-build path (AI_SPECS_GATE_BUILD=1, lib/_internal/gate_binary.py) and
// stays on disk; the gate module is not restructured by this card.
//
// User-vendored skill/dep content is NOT an asset and never lives here: it
// stays on disk under the project's ai-specs/ tree (card [Go 11]).
package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The asset trees are embedded from the module root, where they live next to
// this package. The all: prefix includes dotfiles (catalog/recipes/.gitkeep).
//
//go:embed all:catalog
//go:embed all:templates
//go:embed all:bundled-skills
//go:embed all:bundled-commands
var embedded embed.FS

// EnvDevAssetsDir is the dev-mode override (card [Go 06]): when set to an
// existing directory, every asset root that exists as a subdirectory of it
// is served from disk instead of the embedded tree. Contributors can point
// at on-disk assets without rebuilding. Roots missing from the override
// directory fall back to the embedded tree.
const EnvDevAssetsDir = "AI_SPECS_ASSETS_DIR"

// roots are the embedded asset trees, in a stable order.
var roots = []string{"catalog", "templates", "bundled-skills", "bundled-commands"}

// Roots returns the embedded asset root names in stable order. Callers must
// not mutate the returned slice.
func Roots() []string { return roots }

// FS returns the filesystem serving the root's asset tree (paths relative to
// the root). It honors EnvDevAssetsDir per root as documented above;
// otherwise the embedded tree is returned.
func FS(root string) (fs.FS, error) {
	valid := false
	for _, r := range roots {
		if r == root {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("assets: unknown asset root %q", root)
	}
	if dir := os.Getenv(EnvDevAssetsDir); dir != "" {
		if info, err := os.Stat(filepath.Join(dir, root)); err == nil && info.IsDir() {
			return os.DirFS(filepath.Join(dir, root)), nil
		}
	}
	return fs.Sub(embedded, root)
}

// Read returns the contents of name within the root's asset tree.
func Read(root, name string) ([]byte, error) {
	afs, err := FS(root)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(afs, name)
}

// Walk walks the root's asset tree, calling fn for each entry (paths
// relative to the root, starting at ".").
func Walk(root string, fn fs.WalkDirFunc) error {
	afs, err := FS(root)
	if err != nil {
		return err
	}
	return fs.WalkDir(afs, ".", fn)
}

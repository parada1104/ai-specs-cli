// Package ledger is the tracker-only Go ledger core: work identity, the durable
// binding witness, the item store, and the checkpoint verdict.
//
// It shares the worktree-gate module and binary (design A1) but no worktree gate
// semantics: nothing here imports the gate's Decide / Event / cleanup types, and
// the dispatcher in package main injects Git facts instead of the ledger
// reaching into gate state. Provider vocabulary stays out of this package (A7).
package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// ReasonIdentityUnavailable is reported when Git cannot name the work:
	// detached HEAD, unborn branch, or no Git common dir. It is never replaced
	// by a heuristic identity (A2).
	ReasonIdentityUnavailable = "identity_unavailable"

	// CollisionChangeAmbiguous is reported when several active change folders
	// exist, so the identity omits the slug instead of guessing one (A11).
	CollisionChangeAmbiguous = "change-ambiguous"
)

// Facts mirrors the worktree gate's git facts reader signature
// (gitfacts.go::gitMemo). The dispatcher passes the memoized reader so the
// ledger reuses the gate's Git-facts layer; a nil reader falls back to the
// built-in one, which is what keeps this package unit-testable on its own.
type Facts func(dir string, args ...string) string

// Identity is the work identity: realpath(git common dir) + short branch name +
// optional change slug (A2, A11).
type Identity struct {
	CommonDir string `json:"common_dir"`
	Branch    string `json:"branch"`
	Change    string `json:"change"`
	// Key is the identity key: common_dir + "\x1f" + branch [+ "\x1f" + change].
	// It is the store's matching key, never a filename.
	Key string `json:"key"`
	// Reason is ReasonIdentityUnavailable when identity could not be derived.
	Reason string `json:"reason"`
	// Collision is CollisionChangeAmbiguous when the slug was omitted because
	// several active change folders exist.
	Collision string `json:"collision"`
}

// Available reports whether Git named the work.
func (i Identity) Available() bool { return i.Reason == "" }

// IdentityOptions are the inputs to identity derivation.
type IdentityOptions struct {
	// Dir is any path inside the owning repository (cwd or --project-root).
	Dir string
	// PlanningRoot holds openspec/; empty means Dir. Identity and store follow
	// the owning repository, the slug follows the planning root (assumption 6).
	PlanningRoot string
	// StoredSlug is the slug already recorded on an open item, if any. A stored
	// slug survives its folder being archived mid-item (A11).
	StoredSlug string
	// Facts overrides the Git facts reader; nil uses the built-in reader.
	Facts Facts
}

// DeriveIdentity derives the work identity for opts. A non-derivable input
// yields an identity carrying ReasonIdentityUnavailable and no key; it never
// guesses (A2).
func DeriveIdentity(opts IdentityOptions) Identity {
	facts := opts.Facts
	if facts == nil {
		facts = localFacts
	}
	common := commonDir(opts.Dir, facts)
	branch := facts(opts.Dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	// Detached HEAD leaves no short branch; an unborn branch names a branch but
	// has no commit yet. Neither can carry a durable item.
	if common == "" || branch == "" || facts(opts.Dir, "rev-parse", "--verify", "--quiet", "HEAD") == "" {
		return Identity{Reason: ReasonIdentityUnavailable}
	}

	ident := Identity{CommonDir: common, Branch: branch}
	switch {
	case opts.StoredSlug != "":
		ident.Change = opts.StoredSlug
	default:
		switch slugs := ActiveChangeSlugs(planningRoot(opts)); len(slugs) {
		case 0:
		case 1:
			ident.Change = slugs[0]
		default:
			ident.Collision = CollisionChangeAmbiguous
		}
	}
	ident.Key = IdentityKey(common, branch, ident.Change)
	return ident
}

// IdentityKey is the store's identity key (A5): the common dir, the branch, and
// the change slug when one is present, joined by the unit separator.
func IdentityKey(commonDir, branch, change string) string {
	key := commonDir + "\x1f" + branch
	if change != "" {
		key += "\x1f" + change
	}
	return key
}

// planningRoot is the openspec/ root to read slugs from.
func planningRoot(opts IdentityOptions) string {
	if opts.PlanningRoot != "" {
		return opts.PlanningRoot
	}
	return opts.Dir
}

// ActiveChangeSlugs lists the active (non-archived) change folders under
// openspec/changes/, name-sorted. A missing openspec/ tree yields no slugs.
func ActiveChangeSlugs(planningRoot string) []string {
	entries, err := os.ReadDir(filepath.Join(planningRoot, "openspec", "changes"))
	if err != nil {
		return nil
	}
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "archive" {
			continue
		}
		slugs = append(slugs, entry.Name())
	}
	// os.ReadDir is already sorted, but the ordering is a contract here (the
	// single-slug enrichment must be deterministic), so make it explicit.
	sort.Strings(slugs)
	return slugs
}

var isoDatePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ResolveChangeDir resolves a known slug to its folder using the archive-aware
// order of tests/_change_paths.py::change_dir: active changes/<slug>/, then the
// latest dated archive entry, then the legacy undated archive entry, then an
// active-shaped fallback (A11).
func ResolveChangeDir(planningRoot, slug string) string {
	active := filepath.Join(planningRoot, "openspec", "changes", slug)
	if isDir(active) {
		return active
	}
	archiveRoot := filepath.Join(planningRoot, "openspec", "changes", "archive")
	// A malformed date must not shadow a real archive entry, so the shape and
	// the calendar date are both checked before a candidate is accepted.
	var dated []string
	if entries, err := os.ReadDir(archiveRoot); err == nil {
		suffix := "-" + slug
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
				continue
			}
			datePrefix := strings.TrimSuffix(entry.Name(), suffix)
			if !isISODate(datePrefix) {
				continue
			}
			dated = append(dated, entry.Name())
		}
	}
	if len(dated) > 0 {
		sort.Strings(dated)
		return filepath.Join(archiveRoot, dated[len(dated)-1])
	}
	legacy := filepath.Join(archiveRoot, slug)
	if isDir(legacy) {
		return legacy
	}
	return active
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isISODate(value string) bool {
	if !isoDatePrefix.MatchString(value) {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

// localFacts is the built-in Git facts reader. The dispatcher passes the gate's
// memoized reader instead, so this only runs in tests and in direct package use.
func localFacts(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// commonDir derives the absolute Git common dir of dir, mirroring
// gitfacts.go::gitCommonWith and then resolving symlinks (A2's realpath).
func commonDir(dir string, facts Facts) string {
	value := facts(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if value == "" {
		value = facts(dir, "rev-parse", "--git-common-dir")
	}
	if value == "" {
		return ""
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(dir, value)
	}
	return realPath(filepath.Clean(value))
}

// realPath resolves symlinks. A leaf that does not exist yet (a .git directory
// about to be created) still resolves through its parent, so a linked worktree
// and its main checkout agree on one common dir.
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	dir, base := filepath.Split(path)
	if dir != "" {
		if resolved, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
			return filepath.Join(resolved, base)
		}
	}
	return filepath.Clean(path)
}

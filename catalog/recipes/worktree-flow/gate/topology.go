package main

import (
	"os"
	"path/filepath"
	"strings"
)

type ownerKind string

const (
	ownerUnproven ownerKind = "unproven"
	ownerSuper    ownerKind = "superrepo"
	ownerSub      ownerKind = "subrepo"
)

// moduleRecord is one proven initialized submodule registration: the module's
// resolved path and its git-common-dir.
type moduleRecord struct {
	module string
	common string
}

// classify mirrors the reference classify() (worktree-gate-legacy.sh:451-477):
// walk every ancestor of repo_root collecting proven module records that match
// this repository; more than one match means nested/ambiguous registrations and
// yields unproven; exactly one yields subrepo; otherwise the repository itself
// being a superrepo (having its own records) yields superrepo; else unproven.
// topology == "standalone" or "monorepo-apps" short-circuits to unproven.
func classify(repoRoot, common, topology string) ownerKind {
	if topology == "standalone" || topology == "monorepo-apps" {
		return ownerUnproven
	}
	repoRoot = RealPath(repoRoot)
	common = RealPath(common)
	if repoRoot == "" || common == "" {
		return ownerUnproven
	}
	var matches []moduleRecord
	for probe := repoRoot; ; probe = filepath.Dir(probe) {
		for _, rec := range moduleRecords(probe) {
			if RealPath(rec.module) == repoRoot && RealPath(rec.common) == common {
				matches = append(matches, rec)
			}
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
	}
	if len(matches) > 1 {
		return ownerUnproven
	}
	if len(matches) == 1 {
		return ownerSub
	}
	if records := moduleRecords(repoRoot); len(records) > 0 {
		return ownerSuper
	}
	return ownerUnproven
}

// moduleRecords mirrors the reference module_records()
// (worktree-gate-legacy.sh:412-449): parse .gitmodules, prove each entry is an
// initialized submodule whose common dir matches .git/modules/<rel> and whose
// owner is the module itself. Any ambiguity — a registration outside the
// superrepo, a duplicate, a nested pair, an uninitialized submodule — makes the
// whole set unproven: nil means "no proven records" (ambiguity and absence are
// indistinguishable downstream, exactly like the reference returning None).
func moduleRecords(superRoot string) []moduleRecord {
	return moduleRecordsWithGit(superRoot, gitMemo)
}

// moduleRecordsFresh bypasses the gate's per-invocation memo cache. Cleanup can
// run after another repository operation in the same Go process, so topology
// discovery must observe the current initialized-module set.
func moduleRecordsFresh(superRoot string) []moduleRecord {
	return moduleRecordsWithGit(superRoot, func(dir string, args ...string) string {
		return git(dir, args...)
	})
}

func moduleRecordsWithGit(superRoot string, gitFact func(string, ...string) string) []moduleRecord {
	superRoot = RealPath(superRoot)
	gm := filepath.Join(superRoot, ".gitmodules")
	dotgit := filepath.Join(superRoot, ".git")
	if !isFile(gm) || !(isDir(dotgit) || isFile(dotgit)) {
		return nil
	}
	status := gitFact(superRoot, "config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if status == "" {
		return nil
	}
	var records []moduleRecord
	seen := map[string]bool{}
	for _, line := range strings.Split(status, "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		rel := strings.TrimSpace(parts[1])
		if rel == "" {
			continue
		}
		module := RealPath(filepath.Join(superRoot, rel))
		if !Inside(module, superRoot) || module == RealPath(superRoot) || seen[module] {
			return nil
		}
		for prior := range seen {
			if Inside(module, prior) || Inside(prior, module) {
				return nil
			}
		}
		seen[module] = true
		subStatus := gitFact(superRoot, "submodule", "status", "--", rel)
		if subStatus == "" {
			continue
		}
		statusLine := subStatus
		if i := strings.IndexByte(subStatus, '\n'); i >= 0 {
			statusLine = subStatus[:i]
		}
		if strings.HasPrefix(statusLine, "-") {
			continue
		}
		common := gitCommonFresh(module, gitFact)
		// Git may normalize the module common dir through /private on macOS,
		// while the expected path is constructed lexically. Canonicalize both
		// sides before comparing the proven module ownership.
		expected := RealPath(filepath.Join(superRoot, ".git", "modules", rel))
		owner := gitFact(module, "rev-parse", "--show-toplevel")
		if owner != "" {
			owner = RealPath(owner)
		}
		if owner != module || common == "" || common != expected {
			continue
		}
		records = append(records, moduleRecord{module: module, common: common})
	}
	return records
}

// centralFromCommon derives the superproject root from an absorbed submodule
// common dir (<super>/.git/modules/<rel>), mirroring the final-/modules/ marker
// parsing in resolve_central_root() (plan-build-gate.sh:238-253). Any other
// layout — including a nested modules prefix — is not a central-root signal.
func centralFromCommon(common string) (string, bool) {
	const marker = "/modules/"
	i := strings.LastIndex(common, marker)
	if i < 0 {
		return "", false
	}
	pre, name := common[:i], common[i+len(marker):]
	// Only an EARLIER /.git/modules/ marker makes the layout nested; a
	// superproject whose own path merely contains a "modules" component is valid
	// (plan-build-gate.sh:245-247 checks the same literal).
	if name == "" || strings.Contains(pre, "/.git/modules/") {
		return "", false
	}
	// pre must end in the superproject's own .git directory.
	if filepath.Base(pre) != ".git" {
		return "", false
	}
	cand := filepath.Dir(pre)
	if cand == "" || cand == string(filepath.Separator) {
		return "", false
	}
	return RealPath(cand), true
}

// resolveCentralRoot is the read-only topology proof behind
// --resolve-central-root: run from the current repository/worktree root, it
// returns the superproject root and the registered relative submodule path only
// when moduleRecords() proves an initialized absorbed module whose common dir is
// this repository's common dir. The linked submodule-worktree path works
// because the shared common dir — not the registered module path — is the
// signal (plan-build-gate.sh:229-231). When the absorbed layout is absent, the
// bounded legacy fallback below applies; an absorbed layout that fails the
// moduleRecords proof never falls through (the reference returns 1 there too).
// Everything else fails closed.
func resolveCentralRoot(cwd string) (string, string, bool) {
	root := RealPath(cwd)
	if top := git(cwd, "rev-parse", "--show-toplevel"); top != "" {
		root = RealPath(top)
	}
	if root == "" {
		return "", "", false
	}
	common := RealPath(gitCommon(root))
	if cand, absorbed := centralFromCommon(common); absorbed {
		if cand == root {
			return "", "", false
		}
		for _, rec := range moduleRecords(cand) {
			if RealPath(rec.common) != common {
				continue
			}
			rel, err := filepath.Rel(cand, RealPath(rec.module))
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			return cand, filepath.ToSlash(rel), true
		}
		// The absorbed layout is authoritative: no legacy fallback here.
		return "", "", false
	}
	return resolveCentralRootLegacy(root)
}

// resolveCentralRootLegacy is the bounded fallback for non-absorbed submodule
// layouts (plan-build-gate.sh:254-286): git's own superproject fact plus the
// superproject's .gitmodules and a live (non-empty, non-"-") submodule status.
// It never guesses — any missing or ambiguous fact fails closed.
func resolveCentralRootLegacy(root string) (string, string, bool) {
	return legacyCentral(root, git)
}

func legacyCentral(root string, fact func(string, ...string) string) (string, string, bool) {
	sup := fact(root, "rev-parse", "--show-superproject-working-tree")
	if sup == "" {
		return "", "", false
	}
	cand := RealPath(sup)
	if cand == root || !Inside(root, cand) {
		return "", "", false
	}
	if !isDir(filepath.Join(cand, ".git")) || !isFile(filepath.Join(cand, ".gitmodules")) {
		return "", "", false
	}
	rel, err := filepath.Rel(cand, root)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	rel = filepath.ToSlash(rel)
	subDir := RealPath(filepath.Join(cand, rel))
	if !Inside(subDir, cand) || !pathExists(filepath.Join(subDir, ".git")) {
		return "", "", false
	}
	status := fact(cand, "submodule", "status", "--", rel)
	if status == "" || strings.HasPrefix(status, "-") {
		return "", "", false
	}
	return cand, rel, true
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

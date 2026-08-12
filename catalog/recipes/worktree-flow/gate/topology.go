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
	superRoot = RealPath(superRoot)
	gm := filepath.Join(superRoot, ".gitmodules")
	dotgit := filepath.Join(superRoot, ".git")
	if !isFile(gm) || !(isDir(dotgit) || isFile(dotgit)) {
		return nil
	}
	status := gitMemo(superRoot, "config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
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
		subStatus := gitMemo(superRoot, "submodule", "status", "--", rel)
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
		common := gitCommon(module)
		expected := RealPath(filepath.Join(superRoot, ".git", "modules", rel))
		owner := gitMemo(module, "rev-parse", "--show-toplevel")
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

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

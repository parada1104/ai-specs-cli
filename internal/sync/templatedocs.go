package sync

// Native root authority for the governed-template actuator and the docs flow
// (GO-07 S12.2).
//
// The template shell is the shared actuator core
// (ai-specs.dev/worktree-gate/shared.MaterializeTemplate) the gate binary also
// runs, so the root single binary and the gate share one decision shell — never
// a second port. The docs flow sits above the S12.1 copy authority
// (ApplyCopyItems) and reproduces Python materialize_doc
// (lib/_internal/recipe-materialize.py:1551-1632) pre-existence policy: a
// user-edited doc is never clobbered by re-sync.
//
// Boundary: no lock or manifest write happens here. Every call returns the
// managed-override record payload for the caller; S14 owns the lock write.

import (
	"fmt"
	"os"
	"strings"

	"ai-specs.dev/worktree-gate/shared"
)

// MaterializeTemplate runs one governed template through the shared actuator
// core: the same dest resolution, render tokens, shared classification and
// write+chmod the gate's --materialize-template surface uses, with the record
// payload returned instead of written. It is the root-side entry point for
// Python materialize_template's per-template call
// (lib/_internal/recipe-materialize.py:1261).
//
// The shared actuator uses ONE spelling for both the record and the
// diagnostics — the Python bridge normalizes the target before it sends it.
// _python_materialize_template instead keys the lock on Path(target).as_posix()
// while every message keeps the raw target, so the root wraps the shared shell
// and normalizes only the record target here, at the root boundary. The gate
// command contract is untouched and as_posix is idempotent.
func MaterializeTemplate(req *shared.TemplateRequest) (shared.TemplateResult, error) {
	res, err := shared.MaterializeTemplate(req)
	if err == nil && res.Record != nil {
		res.Record.Target = posixAsPosix(req.Target)
	}
	return res, err
}

// DocRequest is one docs-flow item: the recipe-relative source, the
// project-relative target, and the lock's [managed."<target>"] entry (nil when
// untracked). ManagedEntry is the doc-specific projection (see DocManagedEntry)
// so an absent policy key stays distinct from an explicit one.
type DocRequest struct {
	ProjectRoot  string
	RecipeDir    string
	RecipeID     string
	Source       string
	Target       string
	ManagedEntry *DocManagedEntry
}

// DocManagedEntry is the lock's [managed."<target>"] entry for one doc target.
// HasPolicy reports whether the policy key EXISTS: Python reads
// (entry or {}).get("policy", "auto"), so a missing entry or a missing policy
// means "auto", but any present non-"auto" value — an explicit "" or a JSON
// null — preserves the doc with the confirm-required warning. An empty SHA256
// means the entry carries no baseline (Python classifies the target untracked).
type DocManagedEntry struct {
	SHA256    string
	Policy    string
	HasPolicy bool
}

// DocResult is the docs-flow outcome. Message holds the exact detail line
// ("✓ doc {target}" or "· doc skipped (exists) {target}"); the directory and
// symlink pre-existence paths have no message because Python returns before
// printing it.
type DocResult struct {
	Dest     string
	Wrote    bool
	Record   *shared.ManagedRecord
	Message  string
	Info     string
	Warnings []string
}

// MaterializeDoc mirrors Python materialize_doc for one doc: source presence,
// the directory/symlink pre-existence refusal, the classify-managed-override
// policy (managed_stale + auto refreshes, user_modified and stale non-auto
// warn, managed_current backfills, untracked seeds or warns), the copy through
// the shared authority, and the record payload. A missing source is a refusal
// with Python's exact message.
func MaterializeDoc(req *DocRequest) (DocResult, error) {
	src := pyPathJoin(req.RecipeDir, req.Source)
	dest := pyPathJoin(req.ProjectRoot, req.Target)
	raw := req.Target
	// The lock key is Path(target).as_posix(); every diagnostic and the copy id
	// keep the RAW target spelling, exactly like Python.
	target := posixAsPosix(raw)
	srcInfo, err := os.Stat(src)
	if err != nil || !srcInfo.Mode().IsRegular() {
		return DocResult{}, shared.Refuse(fmt.Sprintf("doc source not found: %s", src))
	}
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		return DocResult{}, fmt.Errorf("read doc source: %w", err)
	}
	record := func(written []byte) *shared.ManagedRecord {
		return &shared.ManagedRecord{
			Target: target,
			SHA256: shared.Sha256Bytes(written),
			Recipe: req.RecipeID,
			Source: req.Source,
			Kind:   "doc",
			Policy: "auto",
		}
	}

	destInfo, statErr := os.Stat(dest) // follows symlinks, like Python Path.exists()
	if statErr == nil {
		if destInfo.IsDir() {
			return DocResult{Dest: dest, Warnings: []string{
				fmt.Sprintf("override metadata missing for %s; preserving existing directory without assigning ownership.", raw),
			}}, nil
		}
		if linkInfo, linkErr := os.Lstat(dest); linkErr == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
			return DocResult{Dest: dest, Warnings: []string{
				fmt.Sprintf("override metadata missing for %s; preserving existing symlink without assigning ownership.", raw),
			}}, nil
		}
		present, disk, readErr := shared.ReadRegularFile(dest)
		if readErr != nil {
			return DocResult{}, fmt.Errorf("dest %s: %w", dest, readErr)
		}
		managed := (*shared.ClassifyManagedEntry)(nil)
		policyAuto := true
		if entry := req.ManagedEntry; entry != nil {
			managed = &shared.ClassifyManagedEntry{SHA256: entry.SHA256}
			policyAuto = !entry.HasPolicy || entry.Policy == "auto"
		}
		wouldWrite := string(srcBytes)
		state := shared.ClassifyManagedOverride(present, disk, managed, &wouldWrite).State
		out := DocResult{
			Dest:     dest,
			Message:  fmt.Sprintf("· doc skipped (exists) %s", raw),
			Warnings: []string{},
		}
		switch state {
		case shared.ClassifyManagedStale:
			if policyAuto {
				if err := copyDocItem(src, dest, raw); err != nil {
					return DocResult{}, err
				}
				out.Wrote = true
				out.Record = record(srcBytes)
				out.Info = fmt.Sprintf("refreshed managed doc %s", raw)
			} else {
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"override managed-stale (confirm-required): %s was not refreshed. "+
						"Refresh with:\n  rm %s && ai-specs sync", raw, raw))
			}
		case shared.ClassifyUserModified:
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"override user-modified: %s was not refreshed. "+
					"Refresh with:\n  rm %s && ai-specs sync", raw, raw))
		case shared.ClassifyManagedCurrent:
			// Backfill provenance fields without rewriting the target.
			out.Record = record(srcBytes)
		case shared.ClassifyUntracked:
			if string(disk) == string(srcBytes) {
				// Pre-lock install already carrying the exact doc bytes: seed
				// provenance instead of assigning ownership over a rewrite.
				out.Record = record(disk)
			} else {
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"override metadata missing for %s; preserving existing file without assigning ownership. "+
						"To preserve this local file, leave it unchanged. To replace it with the current recipe version, "+
						"remove it and run sync again:\n  rm %s && ai-specs sync", raw, raw))
			}
		case shared.ClassifyMissing:
			// The destination exists but is not a regular file: no branch, but
			// the skip line still prints (Python fall-through).
		}
		return out, nil
	}

	if err := copyDocItem(src, dest, raw); err != nil {
		return DocResult{}, err
	}
	return DocResult{
		Dest:     dest,
		Wrote:    true,
		Record:   record(srcBytes),
		Message:  fmt.Sprintf("✓ doc %s", raw),
		Warnings: []string{},
	}, nil
}

// posixAsPosix reproduces Path(target).as_posix(), the spelling Python uses for
// the managed-override lock key: empty and "." segments are dropped, repeated
// separators collapse, ".." is PRESERVED (path.Clean would remove it), and
// EXACTLY two leading slashes survive while three or more collapse to one (the
// POSIX "//" rule). An all-empty relative path is ".".
func posixAsPosix(p string) string {
	i := 0
	for i < len(p) && p[i] == '/' {
		i++
	}
	prefix, rest := "", p
	switch {
	case i == 1:
		prefix, rest = "/", p[i:]
	case i == 2:
		prefix, rest = "//", p[i:]
	case i > 2:
		prefix, rest = "/", p[i:]
	}
	parts := []string{}
	for _, seg := range strings.Split(rest, "/") {
		if seg == "" || seg == "." {
			continue
		}
		parts = append(parts, seg)
	}
	joined := strings.Join(parts, "/")
	if prefix != "" {
		return prefix + joined
	}
	if joined == "" {
		return "."
	}
	return joined
}

// pyPathJoin reproduces pathlib's `base / part`: an ABSOLUTE part REPLACES
// base (project_root / "/abs" == "/abs"), otherwise the parts concatenate and
// the result is the Path.as_posix() form — which KEEPS ".." so the filesystem
// resolves it during traversal. filepath.Join cannot be used: it CLEANS ".."
// lexically before traversal, diverging when an ancestor is a symlink.
func pyPathJoin(base, part string) string {
	if strings.HasPrefix(part, "/") {
		return posixAsPosix(part)
	}
	return posixAsPosix(base + "/" + part)
}

// copyDocItem copies one doc through the shared S12.1 authority. A
// source-missing result is Python's RuntimeError; any execution failure is a
// Go error.
func copyDocItem(src, dest, target string) error {
	results, err := ApplyCopyItems([]shared.CopyItem{{Kind: "doc", ID: target, Src: src, Dest: dest}})
	if err != nil {
		return err
	}
	if len(results) == 1 && results[0].Status == "source-missing" {
		return shared.Refuse(fmt.Sprintf("doc source not found: %s", src))
	}
	return nil
}

package shared

// Governed-template actuator core, moved out of the gate main package by
// GO-07 S12.2 so the gate binary's --materialize-template surface and the root
// native sync authority share one decision + execution shell instead of two
// ports. The gate main package keeps only the JSON envelope wrapper.
//
// Semantics mirror the Python reference lib/_internal/recipe-materialize.py:
// ResolveTemplateDest / RenderTemplateBytes reproduce resolve_template_dest
// and render_template_bytes, WriteTemplateContent reproduces write_content
// (mkdir -p, rendered bytes, chmod to the source permission bits) with the
// O_NOFOLLOW + ancestor guards, and MaterializeTemplate is the
// envelope-independent shell (_python_materialize_template): the shared
// ClassifyManagedOverride decision, the seed/warn/refresh branches, and the
// managed-override record payload. It never writes a lock — the caller applies
// the returned record.
//
// A decision refusal is returned as *Refusal (the gate emits it as an exit-2
// error envelope, the root authority as a Go error); any other error is an
// infrastructure failure (I/O) that the gate reports on stderr.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The shared render tokens. __WORKTREE_REPO_TOPOLOGY__ is owned by the shared
// Python render_override_bytes contract; the cleanup stamps are the narrow
// worktree-flow additions.
const (
	templateTopologyToken            = "__WORKTREE_REPO_TOPOLOGY__"
	templateWorktreesDirToken        = "__WORKTREE_WORKTREES_DIR__"
	templateIntegrationBranchToken   = "__WORKTREE_INTEGRATION_BRANCH__"
	templatePolicyAuto               = "auto"
	templatePolicyConfirm            = "confirm"
	templatePolicyNeverForce         = "never-force"
	templateConditionNotExists       = "not_exists"
	templateWorktreesDirDefault      = ".worktrees"
	templateIntegrationBranchDefault = "main"
)

// TemplateRequest is the envelope-independent input for MaterializeTemplate.
// Config carries the merged recipe config values (only repo_topology,
// worktrees_dir, and integration_branch are read); nil means render with
// defaults only. ManagedEntry is the lock's managed[target] subset — only
// sha256 is read; nil means untracked. The JSON tags let the gate wrapper
// decode the same shape from its stdin envelope.
type TemplateRequest struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Source       string                `json:"source"`
	Target       string                `json:"target"`
	Condition    string                `json:"condition,omitempty"`
	UpdatePolicy string                `json:"update_policy,omitempty"`
	Config       map[string]any        `json:"config,omitempty"`
	ManagedEntry *ClassifyManagedEntry `json:"managed_entry,omitempty"`
}

// ManagedRecord is the managed-override lock payload one materialization
// produces (Python set_managed_override): target is the posix form of the
// recipe target, sha256 is the normalized (CRLF-folded) sha of the bytes
// actually on disk after the actuation. A nil record means "write nothing to
// the lock". The caller owns the lock write.
type ManagedRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// TemplateResult is the envelope-independent outcome.
type TemplateResult struct {
	Dest     string
	Wrote    bool
	Record   *ManagedRecord
	Message  string
	Info     string
	Warnings []string
}

// Refusal is a decision refusal: the gate emits it as an exit-2 error envelope
// and the Python authority raises RuntimeError. Any other error is an
// infrastructure failure.
type Refusal struct{ Message string }

func (r *Refusal) Error() string { return r.Message }

// Refuse builds a decision refusal.
func Refuse(message string) error { return &Refusal{Message: message} }

// PyConfigString reproduces Python str() over the decoded JSON value domain
// recipe config carries: strings verbatim, True/False capitalized, None for
// null, numbers as their literal text (the decoder preserves it via
// json.Number).
func PyConfigString(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// TemplateConfigOr mirrors Python's `str(cfg.get(key) or default)`: an absent,
// null, empty, or falsy value falls back to the default.
func TemplateConfigOr(config map[string]any, key, fallback string) string {
	v, ok := config[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return fallback
		}
		return t
	case bool:
		if !t {
			return fallback
		}
		return "True"
	case json.Number:
		if t.String() == "0" {
			return fallback
		}
		return t.String()
	default:
		return PyConfigString(v)
	}
}

// RenderTemplateBytes is the pure renderer: the shared topology token
// replacement (missing key or nil config defaults to "auto", mirroring
// util.render_override_bytes), then — only when config is non-nil — the two
// cleanup stamps with `or default` semantics. With a nil config the cleanup
// tokens stay LITERAL (the Python early return fires after the shared render).
// Rendering is byte surgery only: CRLF bytes are never normalized
// (normalization happens only inside Sha256Bytes).
func RenderTemplateBytes(src []byte, config map[string]any) []byte {
	data := src
	token := []byte(templateTopologyToken)
	if bytes.Contains(data, token) {
		topology := "auto"
		if config != nil {
			if v, ok := config["repo_topology"]; ok {
				topology = PyConfigString(v)
			}
		}
		data = bytes.ReplaceAll(data, token, []byte(topology))
	}
	if config == nil {
		return data
	}
	for _, stamp := range [...]struct{ token, key, fallback string }{
		{templateWorktreesDirToken, "worktrees_dir", templateWorktreesDirDefault},
		{templateIntegrationBranchToken, "integration_branch", templateIntegrationBranchDefault},
	} {
		stampToken := []byte(stamp.token)
		if bytes.Contains(data, stampToken) {
			data = bytes.ReplaceAll(data, stampToken, []byte(TemplateConfigOr(config, stamp.key, stamp.fallback)))
		}
	}
	return data
}

// ResolveTemplateDest mirrors Python resolve_template_dest: a `.git/...`
// target (a Git hook) resolves through `git rev-parse --git-path`, because in
// a linked worktree `.git` is a gitfile and hooks land in the SHARED hooks
// directory of the main repository. The second return reports an ACTUAL clean
// git resolution: the exemption from destination containment is keyed on this
// flag, never on the literal ".git/" string prefix — `git rev-parse --git-path`
// honors parent-directory components (an unclean remainder like
// "../../outside/evil" exits 0 emitting an escaping path), and every fallback
// lands on the literal join. The remainder must therefore be a clean,
// non-escaping relative path for git resolution to even be attempted. Every
// fallback returns the literal project-relative join with gitResolved=false,
// keeping such targets subject to the caller's containment check.
func ResolveTemplateDest(projectRoot, target string) (dest string, gitResolved bool) {
	literal := filepath.Join(projectRoot, filepath.FromSlash(target))
	if !strings.HasPrefix(target, ".git/") {
		return literal, false
	}
	remainder := target[len(".git/"):]
	if filepath.IsAbs(remainder) {
		return literal, false
	}
	cleaned := filepath.Clean(filepath.FromSlash(remainder))
	if cleaned == "." || filepath.ToSlash(cleaned) != remainder ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return literal, false
	}
	// --path-format=relative (git >= 2.31) keeps the emitted path LEXICAL: the
	// default absolute emission canonizes symlinks, which on macOS turns the
	// fixture's /var/... project root into /private/var/... and would re-anchor
	// the shared hooks dir through the real path instead of the caller's path.
	out, err := exec.Command("git", "-C", projectRoot, "rev-parse", "--path-format=relative", "--git-path", remainder).Output()
	if err != nil {
		// Older git lacks --path-format; retry with the default emission (an
		// absolute shared-hooks path is still correct, only spelled through the
		// real path).
		out, err = exec.Command("git", "-C", projectRoot, "rev-parse", "--git-path", remainder).Output()
	}
	if err != nil {
		return literal, false
	}
	resolved := strings.TrimSpace(string(out))
	if resolved == "" {
		return literal, false
	}
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(projectRoot, resolved)
	}
	return resolved, true
}

// ErrDestSymlink reports a destination that already exists as a symlink.
// Writing through it would follow the link and rewrite whatever it points at
// (for a dangling link, os.WriteFile would create that target instead), so the
// actuator refuses the write and hands the decision back as a refusal. The
// Python fallback authority mirrors this refusal.
var ErrDestSymlink = errors.New("destination is a symlink")

// ErrAncestorSymlink reports a symlinked ANCESTOR directory of the
// destination (below the containment root). MkdirAll and the open follow
// directory symlinks, so a planted link in the path would silently redirect
// the write outside the project even though the destination path is lexically
// contained.
var ErrAncestorSymlink = errors.New("ancestor path is a symlink")

// TemplateSymlinkRefusal is the actionable refusal for a symlinked
// destination. Both authorities emit it verbatim.
func TemplateSymlinkRefusal(target string) string {
	return fmt.Sprintf(
		"destination %s is a symlink; refusing to write through it. "+
			"Replace it with a regular file and run sync again:\n  rm %s && ai-specs sync",
		target, target)
}

// TemplateAncestorSymlinkRefusal is the actionable refusal for a symlinked
// ancestor directory on the destination path.
func TemplateAncestorSymlinkRefusal(target string) string {
	return fmt.Sprintf(
		"ancestor path of %s is a symlink; refusing to write through it. "+
			"Replace it with a real directory and run sync again", target)
}

// TemplateEscapingTargetRefusal is the actionable refusal for a target whose
// resolved destination lands outside the project root.
func TemplateEscapingTargetRefusal(target string) string {
	return fmt.Sprintf(
		"template target %s escapes the project root; refusing to write outside the project. "+
			"Fix the recipe target and run sync again", target)
}

// writeTemplateRefusal renders a write failure, mapping the symlink guards to
// their actionable refusals instead of the generic write diagnostic.
func writeTemplateRefusal(target, dest string, err error) string {
	if errors.Is(err, ErrAncestorSymlink) {
		return TemplateAncestorSymlinkRefusal(target)
	}
	if errors.Is(err, ErrDestSymlink) {
		return TemplateSymlinkRefusal(target)
	}
	return fmt.Sprintf("write template %s: %v", dest, err)
}

// TemplatePathContained reports whether dest stays inside root in the cleaned
// lexical sense (no `..` escape). Symlink escapes are handled separately by
// firstSymlinkedAncestor and the O_NOFOLLOW open.
func TemplatePathContained(root, dest string) bool {
	rel, err := filepath.Rel(root, dest)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// firstSymlinkedAncestor walks the ancestor directories of dest below the
// containment root, top down, and returns the first one that is a symlink.
// Ancestors at or above root are not inspected: project_root arrives resolved,
// and a git-resolved destination outside the root (the linked-worktree shared
// hooks directory) is trusted to git's own emission. The walk runs before
// MkdirAll so nothing is ever created through a planted link; the window
// between this walk and the open is narrowed, not eliminated (a full fix needs
// an openat chain).
func firstSymlinkedAncestor(root, dest string) (string, bool) {
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return "", false
	}
	cur := root
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		if info, statErr := os.Lstat(cur); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return cur, true
		}
	}
	return "", false
}

// EnsureTemplateAncestorsReal refuses a destination whose ancestor chain below
// the containment root carries a symlink, before MkdirAll can create anything
// through the link. Called once per materialization run, covering both write
// paths (fresh write and managed_stale refresh).
func EnsureTemplateAncestorsReal(root, dest string) error {
	if ancestor, isLink := firstSymlinkedAncestor(root, dest); isLink {
		return fmt.Errorf("%w: %s", ErrAncestorSymlink, ancestor)
	}
	return nil
}

// WriteTemplateContent mirrors Python write_content: mkdir -p the parent,
// write the bytes, then chmod to the source permission bits (os.WriteFile
// applies the mode only at creation, so the explicit chmod is the
// os.chmod(dest, src.st_mode) parity).
//
// The guard is three layers: the caller runs the containment + ancestor-symlink
// checks (EnsureTemplateAncestorsReal) first, the Lstat pre-check here yields
// the early actionable refusal for a symlinked destination itself and covers
// the dangling-link path, and the open itself carries syscall.O_NOFOLLOW so no
// link can be swapped in between the check and the write (ELOOP maps back to
// ErrDestSymlink). The chmod goes through the OPEN FILE HANDLE (file.Chmod)
// instead of the path, so a link swapped in after the write can never be
// chmod'ed through.
func WriteTemplateContent(dest string, content []byte, mode os.FileMode) error {
	if info, lstatErr := os.Lstat(dest); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrDestSymlink
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return ErrDestSymlink
		}
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

// MaterializeTemplate is the envelope-independent shell for one governed
// template: source presence, render, destination resolution + guards, the
// not_exists classification, the write/seed/warn/backfill branches, and the
// record payload. It performs no lock or manifest write.
func MaterializeTemplate(in *TemplateRequest) (TemplateResult, error) {
	policy := in.UpdatePolicy
	if policy == "" {
		policy = templatePolicyAuto
	}
	srcPath := filepath.Join(in.RecipeDir, filepath.FromSlash(in.Source))
	srcInfo, statErr := os.Stat(srcPath)
	if statErr != nil || !srcInfo.Mode().IsRegular() {
		return TemplateResult{}, Refuse(fmt.Sprintf("template source not found: %s", srcPath))
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return TemplateResult{}, fmt.Errorf("read source: %w", err)
	}
	if policy != templatePolicyAuto && policy != templatePolicyConfirm && policy != templatePolicyNeverForce {
		return TemplateResult{}, Refuse(fmt.Sprintf(
			"invalid update policy '%s' for template '%s'; expected auto | confirm | never-force",
			policy, in.Target))
	}
	content := RenderTemplateBytes(srcBytes, in.Config)
	dest, gitResolved := ResolveTemplateDest(in.ProjectRoot, in.Target)
	// The containment exemption is keyed on an ACTUAL clean git resolution,
	// never on the literal ".git/" string prefix. A git-resolved destination
	// (the shared hooks dir of a linked worktree, outside the worktree root)
	// is trusted to git's own clean emission; a ".git/"-prefixed target with
	// an unclean remainder never resolves through git and falls back to the
	// literal join, which is contained like any other literal target.
	if !gitResolved && !TemplatePathContained(in.ProjectRoot, dest) {
		return TemplateResult{}, Refuse(TemplateEscapingTargetRefusal(in.Target))
	}
	if err := EnsureTemplateAncestorsReal(in.ProjectRoot, dest); err != nil {
		return TemplateResult{}, Refuse(writeTemplateRefusal(in.Target, dest, err))
	}
	record := func(sha string) *ManagedRecord {
		return &ManagedRecord{
			Target: in.Target,
			SHA256: sha,
			Recipe: in.RecipeID,
			Source: in.Source,
			Kind:   "template",
			Policy: policy,
		}
	}
	sourceMode := srcInfo.Mode().Perm()

	if in.Condition == templateConditionNotExists {
		if _, existsErr := os.Stat(dest); existsErr == nil {
			present, disk, readErr := ReadRegularFile(dest)
			if readErr != nil {
				return TemplateResult{}, fmt.Errorf("dest %s: %w", dest, readErr)
			}
			wouldWrite := string(content)
			state := ClassifyManagedOverride(present, disk, in.ManagedEntry, &wouldWrite).State
			// The Python reference prints the skip line at the end of the
			// exists branch regardless of the decision (fall-through).
			out := TemplateResult{
				Dest:     dest,
				Message:  fmt.Sprintf("· template skipped (exists) %s", in.Target),
				Warnings: []string{},
			}
			switch state {
			case ClassifyUntracked:
				diskSHA := Sha256Bytes(disk)
				// Existing projects may contain a pre-render placeholder copy
				// still carrying the raw catalog tokens; seed its actual bytes
				// and let the next sync reconcile it.
				if diskSHA == Sha256Bytes(content) || diskSHA == Sha256Bytes(srcBytes) {
					out.Record = record(diskSHA)
				} else {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"override metadata missing for %s; preserving existing file without assigning ownership. "+
							"To preserve this local file, leave it unchanged. To replace it with the current recipe version, "+
							"remove it and run sync again:\n  rm %s && ai-specs sync", in.Target, in.Target))
				}
			case ClassifyManagedStale:
				if policy == templatePolicyAuto {
					if err := WriteTemplateContent(dest, content, sourceMode); err != nil {
						return TemplateResult{}, Refuse(writeTemplateRefusal(in.Target, dest, err))
					}
					out.Wrote = true
					out.Record = record(Sha256Bytes(content))
					out.Info = fmt.Sprintf("refreshed managed template %s", in.Target)
				} else {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"override managed-stale (%s-required): %s was not refreshed. "+
							"Refresh with:\n  rm %s && ai-specs sync", policy, in.Target, in.Target))
				}
			case ClassifyUserModified:
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"override user-modified: %s was not refreshed. "+
						"Refresh with:\n  rm %s && ai-specs sync", in.Target, in.Target))
			case ClassifyManagedCurrent:
				// Backfill provenance fields without rewriting the target.
				out.Record = record(Sha256Bytes(content))
			case ClassifyMissing:
				// The destination exists but is not a regular file: the
				// Python reference matches no branch, records nothing, and
				// still prints the skip line.
			}
			return out, nil
		}
	}

	if err := WriteTemplateContent(dest, content, sourceMode); err != nil {
		return TemplateResult{}, Refuse(writeTemplateRefusal(in.Target, dest, err))
	}
	return TemplateResult{
		Dest:     dest,
		Wrote:    true,
		Record:   record(Sha256Bytes(content)),
		Message:  fmt.Sprintf("✓ template %s", in.Target),
		Warnings: []string{},
	}, nil
}

package shared

// Hook/gate actuator core, moved out of the gate main package by GO-07 S12.3
// so the gate binary's --materialize-hook surface and (in delivery B) the root
// native sync authority share one decision + execution shell instead of two
// ports. The gate main package keeps only the JSON envelope wrapper.
//
// Go owns the hook actuation DECISION + EXECUTION: rendering (the 8 hook
// placeholders), the ownership classification (the shared
// ClassifyManagedOverride core behind --plan-classify — never re-ported), the
// write + chmod 0755, and the refresh backup/rollback. Python keeps the lock
// load/write (set_gate_baseline + write_lock), ALL printing, the refresh
// backup-path precomputation (project-cache ownership) and the gate-version
// resolution (the envelope carries the resolved value), plus the TEMPORARY
// fail-open fallback (GO_HOOK_GATE_BRIDGE_FALLBACK).
//
// The decision core is exposed as MaterializeHook: the gate wrapper decodes the
// stdin envelope into HookActuatorRequest and maps the result / refusal onto
// its stdout contract. A refusal is returned as *Refusal (the gate emits it as
// an exit-2 error envelope, the root authority as a Go error); any other error
// is an infrastructure failure (I/O) that the gate reports on stderr.
//
// Unlike the template actuator there is no seeding path: a destination
// without a lock baseline is NEVER claimed (provenance only from a render).
// Gate files always carry mode 0755 (source mode is never copied).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The 8 hook placeholders, rendered in this order, each only when the token
// is present in the source bytes. The two gate-mode tokens share the
// "gate_mode" config key with different defaults (GATE_MODE_PLACEHOLDERS).
const (
	hookGateModeToken     = "__WORKTREE_GATE_MODE__"
	hookCardGateModeToken = "__TRACKER_CARD_GATE_MODE__"
	hookGateScopeToken    = "__WORKTREE_GATE_SCOPE__"
	hookTopologyToken     = "__WORKTREE_REPO_TOPOLOGY__"
	hookGateImplToken     = "__WORKTREE_GATE_IMPL__"
	hookGateVersionToken  = "__WORKTREE_GATE_VERSION__"
	hookCLIHomeToken      = "__TRACKER_CLI_HOME__"
	hookLibInternalToken  = "__TRACKER_LIB_INTERNAL__"
)

// hookGateModeTokens pairs each gate-mode token with its default: unlike the
// validated tokens below, a configured value is used verbatim (Python's
// str(merged_cfg.get("gate_mode", default)) has no `or default` fallback).
var hookGateModeTokens = [...]struct{ token, fallback string }{
	{hookGateModeToken, "always"},
	{hookCardGateModeToken, "warn"},
}

// hookConfigGet mirrors Python str(merged_cfg.get(key, default)): an absent
// key falls back, a present key is stringified verbatim (None -> "None"),
// with no emptiness check.
func hookConfigGet(config map[string]any, key, fallback string) string {
	if v, ok := config[key]; ok {
		return PyConfigString(v)
	}
	return fallback
}

// HookScriptRelPath is the harness-neutral materialized path for a hook
// script: fixed ai-specs/recipes/{recipe_id}/hooks/ prefix with the script
// BASENAME, so nested recipe scripts flatten into the hook directory.
func HookScriptRelPath(recipeID, script string) string {
	return fmt.Sprintf("ai-specs/recipes/%s/hooks/%s", recipeID, filepath.Base(script))
}

// HookRelPathEscapes reports whether the rel path built from recipe_id +
// script would leave the managed ai-specs/recipes tree: recipe_id is
// interpolated verbatim into the prefix (R1-hook-rel-path-unsanitized), so a
// separator or dot segment in it escapes the recipes directory, and a script
// whose basename is a dot segment joins to a parent directory. Recipe ids are
// catalog slugs, so refusing these inputs never rejects a real recipe.
func HookRelPathEscapes(recipeID, script string) bool {
	if recipeID == "" || recipeID == "." || recipeID == ".." {
		return true
	}
	if strings.ContainsAny(recipeID, `/\`) {
		return true
	}
	switch filepath.Base(script) {
	case ".", "..":
		return true
	}
	return false
}

// RenderHookGateContent is the pure hook renderer: the eight placeholders in
// the documented order, each replaced only when its token is present. The
// three validated tokens (gate_scope, repo_topology, gate_impl) raise the
// exact historical refusal when the configured value is not allowed; an
// absent/empty value falls back to "auto" (Python's `or default`). The gate
// version is Python-resolved and substituted verbatim (Go never invents a
// value); the two tracker-home tokens become empty strings without a CLI
// home, which makes the host skip evidence acquisition (fail open). Rendering
// is string surgery only: CRLF bytes are never normalized (normalization
// happens only inside Sha256Bytes).
func RenderHookGateContent(src []byte, cfg map[string]any, cliHome string, gateVersion string) (string, error) {
	content := string(src)
	replace := func(token, value string) {
		content = strings.ReplaceAll(content, token, value)
	}
	for _, gm := range hookGateModeTokens {
		if strings.Contains(content, gm.token) {
			replace(gm.token, hookConfigGet(cfg, "gate_mode", gm.fallback))
		}
	}
	if strings.Contains(content, hookGateScopeToken) {
		scope := TemplateConfigOr(cfg, "gate_scope", "auto")
		if scope != "auto" && scope != "superrepo" && scope != "subrepo" {
			return "", fmt.Errorf("invalid gate_scope '%s'; allowed: auto | superrepo | subrepo", scope)
		}
		replace(hookGateScopeToken, scope)
	}
	if strings.Contains(content, hookTopologyToken) {
		topology := TemplateConfigOr(cfg, "repo_topology", "auto")
		switch topology {
		case "auto", "standalone", "monorepo-apps", "monorepo-submodules":
		default:
			return "", fmt.Errorf(
				"invalid repo_topology '%s'; allowed: auto | standalone | monorepo-apps | monorepo-submodules", topology)
		}
		replace(hookTopologyToken, topology)
	}
	if strings.Contains(content, hookGateImplToken) {
		impl := TemplateConfigOr(cfg, "gate_impl", "auto")
		if impl != "auto" && impl != "go" {
			return "", fmt.Errorf("invalid gate_impl '%s'; bash has been removed; allowed: auto | go", impl)
		}
		replace(hookGateImplToken, impl)
	}
	if strings.Contains(content, hookGateVersionToken) {
		replace(hookGateVersionToken, gateVersion)
	}
	if strings.Contains(content, hookCLIHomeToken) {
		replace(hookCLIHomeToken, cliHome)
	}
	if strings.Contains(content, hookLibInternalToken) {
		internal := ""
		if cliHome != "" {
			internal = filepath.Join(cliHome, "lib", "_internal")
		}
		replace(hookLibInternalToken, internal)
	}
	return content, nil
}

// hookBackupComplete reports whether the snapshot at path holds exactly the
// bytes its content-hash key names (Sha256Bytes(bytes) == the digest encoded
// in the file name); a missing or partial file is never complete, so a
// truncated write cannot masquerade as the immutable snapshot.
func hookBackupComplete(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	want := strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	return want != "" && Sha256Bytes(data) == want
}

// writeHookBackupSnapshot writes the refresh snapshot atomically: a temp file
// in the SAME directory, written + Sync'd + closed, chmod 0644, then renamed
// onto the final path, so a crash or short write can never leave a partial
// file at the content-hash key. The temp file is removed on every failure
// path (a no-op after a successful rename).
func writeHookBackupSnapshot(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hookgate-backup-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer tmp.Close()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// HookActuatorRequest is the envelope-independent input for MaterializeHook,
// decoded by the gate wrapper from its stdin contract. Config carries the
// merged recipe config (only gate_mode, gate_scope, repo_topology, and
// gate_impl are read); nil means render with defaults only. CLIHome is the
// Python-RESOLVED cli home (Python owns resolution, the same division as
// gate_version). ManagedEntry is the lock's managed[rel] subset — only sha256
// is read; nil means no recorded provenance. BackupPath is the
// Python-precomputed immutable refresh snapshot path (empty when the
// destination does not exist yet).
type HookActuatorRequest struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Script       string                `json:"script"`
	Config       map[string]any        `json:"config,omitempty"`
	CLIHome      string                `json:"cli_home,omitempty"`
	GateVersion  string                `json:"gate_version,omitempty"`
	Refresh      bool                  `json:"refresh,omitempty"`
	ManagedEntry *ClassifyManagedEntry `json:"managed_entry,omitempty"`
	BackupPath   string                `json:"backup_path,omitempty"`
}

// HookActuatorRecord is the lock payload the Python bridge hands to
// lock.set_gate_baseline: target is the project-relative rel path, sha256 is
// the normalized (CRLF-folded) sha of the bytes the actuation produced, kind
// is always "gate", policy always "auto". A nil record means "write nothing
// to the lock".
type HookActuatorRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// HookActuatorResult is the envelope-independent outcome for one hook
// actuation. The gate wrapper maps it onto its stdout contract; the root
// native authority can consume it directly.
type HookActuatorResult struct {
	Rel      string
	Dest     string
	Wrote    bool
	Record   *HookActuatorRecord
	Message  string
	Warnings []string
	Backup   string
}

// hookWriteRefusal maps a hook write failure to its refusal string. The
// symlink guard reuses the template actuator's shared refusal (both
// authorities — Go and the Python fallback — spell it verbatim).
func hookWriteRefusal(rel, dest string, err error) string {
	if errors.Is(err, ErrDestSymlink) {
		return TemplateSymlinkRefusal(rel)
	}
	return fmt.Sprintf("write hook %s: %v", dest, err)
}

// HookRefreshWriteFunc is the refresh write step's signature.
type HookRefreshWriteFunc func(dest string, content []byte, mode os.FileMode) error

// HookGateRefreshWrite is the production refresh write step. It is exported so
// a caller (and the gate wrapper's historical injection seam) can substitute
// the write step; production always uses WriteTemplateContent.
var HookGateRefreshWrite HookRefreshWriteFunc = WriteTemplateContent

// MaterializeHook is the envelope-independent shell for one runtime hook
// gate: the rel-path traversal guard, source presence, render, the shared
// ownership classification, the write/refresh branches, and the record
// payload. It performs no lock or manifest write. The refreshWrite argument is
// the refresh write seam (nil falls back to HookGateRefreshWrite). Decision
// refusals are returned as *Refusal; any other error is an infrastructure
// failure.
func MaterializeHook(in *HookActuatorRequest, refreshWrite HookRefreshWriteFunc) (HookActuatorResult, error) {
	if refreshWrite == nil {
		refreshWrite = HookGateRefreshWrite
	}
	if HookRelPathEscapes(in.RecipeID, in.Script) {
		return HookActuatorResult{}, Refuse(fmt.Sprintf(
			"hook rel path would escape ai-specs/recipes/: recipe_id %q, script %q",
			in.RecipeID, in.Script))
	}

	rel := HookScriptRelPath(in.RecipeID, in.Script)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(rel))
	srcPath := filepath.Join(in.RecipeDir, filepath.FromSlash(in.Script))
	srcInfo, statErr := os.Stat(srcPath)
	if statErr != nil || !srcInfo.Mode().IsRegular() {
		return HookActuatorResult{}, Refuse(fmt.Sprintf("hook script not found: %s", srcPath))
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return HookActuatorResult{}, fmt.Errorf("read source: %w", err)
	}
	content, renderErr := RenderHookGateContent(srcBytes, in.Config, in.CLIHome, in.GateVersion)
	if renderErr != nil {
		return HookActuatorResult{}, Refuse(renderErr.Error())
	}
	contentBytes := []byte(content)
	hookRecord := func(sha string) *HookActuatorRecord {
		return &HookActuatorRecord{
			Target: rel,
			SHA256: sha,
			Recipe: in.RecipeID,
			Source: in.Script,
			Kind:   "gate",
			Policy: "auto",
		}
	}

	if in.Refresh {
		return runHookGateRefresh(hookRefreshCall{
			in:           in,
			rel:          rel,
			dest:         dest,
			contentBytes: contentBytes,
			record:       hookRecord,
			refreshWrite: refreshWrite,
		})
	}

	present, disk, err := ReadRegularFile(dest)
	if err != nil {
		return HookActuatorResult{}, fmt.Errorf("dest %s: %w", dest, err)
	}
	// The SHARED classification port (shared/classify.go): never re-ported in
	// slice 7. The hook path passes the exact rendered content as would-write.
	wouldWrite := content
	state := ClassifyManagedOverride(present, disk, in.ManagedEntry, &wouldWrite).State
	out := HookActuatorResult{Rel: rel, Dest: dest, Warnings: []string{}}
	switch state {
	case ClassifyMissing, ClassifyManagedStale:
		verb := "✓ hook script"
		if state == ClassifyManagedStale {
			// Baseline matches current bytes: the CLI rendered this gate, so an
			// ordinary sync may force-update it and re-record the baseline.
			verb = "✓ hook refreshed (baseline matched)"
		}
		if err := WriteTemplateContent(dest, contentBytes, 0o755); err != nil {
			return HookActuatorResult{}, Refuse(hookWriteRefusal(rel, dest, err))
		}
		out.Wrote = true
		out.Record = hookRecord(Sha256Bytes(contentBytes))
		out.Message = fmt.Sprintf("%s %s", verb, rel)
	case ClassifyManagedCurrent:
		// Backfill provenance without rewriting the target (idempotent pair).
		out.Record = hookRecord(Sha256Bytes(contentBytes))
		out.Message = fmt.Sprintf("· hook skipped (current) %s", rel)
	case ClassifyUserModified:
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"hook %s is user-modified; preserving existing bytes. Refresh with:\n"+
				"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel))
		out.Message = fmt.Sprintf("· hook skipped (user-modified) %s", rel)
	case ClassifyUntracked:
		// No provenance: preserve, NEVER seed (unlike the template actuator).
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"hook %s has no recorded provenance; preserving existing bytes. "+
				"A baseline is recorded only when the CLI renders the gate. Refresh with:\n"+
				"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel))
		out.Message = fmt.Sprintf("· hook skipped (no provenance) %s", rel)
	}
	return out, nil
}

// hookRefreshCall bundles one refresh invocation: the decoded input, the
// derived identity (rel/dest/rendered bytes/record constructor), the refresh
// write seam, and the result shell. The previous eight-parameter signature
// flattened into one readable value (R2-refresh-signature).
type hookRefreshCall struct {
	in           *HookActuatorRequest
	rel, dest    string
	contentBytes []byte
	record       func(sha string) *HookActuatorRecord
	refreshWrite HookRefreshWriteFunc
}

// runHookGateRefresh is the refresh path: cache backup -> gate write,
// all-or-nothing. Classification is skipped — a refresh replaces even a
// user-modified gate after its exact pre-refresh bytes land in the
// Python-precomputed immutable backup. On ANY Go-side failure before success
// the prior bytes are restored (best effort), the backup is removed, and the
// actuation refuses so Python never writes a lock record.
func runHookGateRefresh(c hookRefreshCall) (HookActuatorResult, error) {
	if info, statErr := os.Lstat(c.dest); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return HookActuatorResult{}, Refuse(TemplateSymlinkRefusal(c.rel))
	}
	var prior []byte
	hadPrior := false
	if info, statErr := os.Stat(c.dest); statErr == nil && info.Mode().IsRegular() {
		data, readErr := os.ReadFile(c.dest)
		if readErr != nil {
			return HookActuatorResult{}, fmt.Errorf("dest %s: %w", c.dest, readErr)
		}
		prior = data
		hadPrior = true
	}
	backup := ""
	if hadPrior && c.in.BackupPath != "" {
		// The path is content-hash keyed (immutable): a COMPLETE snapshot is
		// never rewritten. A missing or PARTIAL one (a crash or short write can
		// leave truncated bytes at the key) is (re)written atomically so a
		// partial file never appears at the final path.
		if err := os.MkdirAll(filepath.Dir(c.in.BackupPath), 0o755); err != nil {
			return HookActuatorResult{}, fmt.Errorf("backup %s: %w", c.in.BackupPath, err)
		}
		if !hookBackupComplete(c.in.BackupPath) {
			if err := writeHookBackupSnapshot(c.in.BackupPath, prior); err != nil {
				return HookActuatorResult{}, fmt.Errorf("backup %s: %w", c.in.BackupPath, err)
			}
		}
		backup = c.in.BackupPath
	}
	if err := c.refreshWrite(c.dest, c.contentBytes, 0o755); err != nil {
		if hadPrior {
			// The rollback goes through the SAME guarded writer as the primary
			// path: a destination swapped to a symlink mid-refresh is refused
			// (Lstat + O_NOFOLLOW), never written through (R4-002). Best effort —
			// the refusal below already hands the decision back to Python.
			_ = WriteTemplateContent(c.dest, prior, 0o755)
		}
		if backup != "" {
			_ = os.Remove(backup)
		}
		return HookActuatorResult{}, Refuse(hookWriteRefusal(c.rel, c.dest, err))
	}
	return HookActuatorResult{
		Rel:      c.rel,
		Dest:     c.dest,
		Wrote:    true,
		Record:   c.record(Sha256Bytes(c.contentBytes)),
		Message:  fmt.Sprintf("✓ hook refreshed %s", c.rel),
		Warnings: []string{},
		Backup:   backup,
	}, nil
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Hook/gate actuator for runtime hook scripts (GO-10, strangler slice 7). Go
// owns the hook actuation DECISION + EXECUTION: rendering (the 8 hook
// placeholders), the ownership classification (the shared
// classifyManagedOverride core behind --plan-classify — never re-ported), the
// write + chmod 0755, and the refresh backup/rollback. Python keeps the lock
// load/write (set_gate_baseline + write_lock), ALL printing, the refresh
// backup-path precomputation (project-cache ownership) and the gate-version
// resolution (the envelope carries the resolved value), plus the TEMPORARY
// fail-open fallback (GO_HOOK_GATE_BRIDGE_FALLBACK).
//
// Command surface: --materialize-hook reads one JSON envelope on stdin
//
//	{"project_root": "...", "recipe_dir": "...", "recipe_id": "...",
//	 "script": "hooks/gate.sh", "config": {...}, "cli_home": "...",
//	 "gate_version": "dev", "refresh": false,
//	 "managed_entry": {"sha256": "..."} | null, "backup_path": "..."}
//
// and prints one result envelope on stdout with exit 0:
//
//	{"rel": "...", "dest": "...", "wrote": true|false, "record": {...}|null,
//	 "message": "...", "warnings": [...], "backup": "...", "error": null}
//
// A refusal (missing source, invalid config value, symlinked destination,
// write failure) prints {"error": "<exact reference string>"} on stdout with
// exit 2 and touches no file. The Python bridge fails CLOSED on a delivered
// exit-2 error envelope and falls back to its historical Python body on
// infrastructure failures. The exit-2 taxonomy matches the template actuator:
// INPUT/INFRASTRUCTURE failures print a stderr diagnostic and NO envelope
// (Python fails open); DECISION refusals emit the error envelope (Python
// fails closed).
//
// Unlike the template actuator there is no seeding path: a destination
// without a lock baseline is NEVER claimed (provenance only from a render).
// Gate files always carry mode 0755 (source mode is never copied).

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
		return pyConfigString(v)
	}
	return fallback
}

// hookScriptRelPath is the harness-neutral materialized path for a hook
// script: fixed ai-specs/recipes/{recipe_id}/hooks/ prefix with the script
// BASENAME, so nested recipe scripts flatten into the hook directory.
func hookScriptRelPath(recipeID, script string) string {
	return fmt.Sprintf("ai-specs/recipes/%s/hooks/%s", recipeID, filepath.Base(script))
}

// renderHookGateContent is the pure hook renderer: the eight placeholders in
// the documented order, each replaced only when its token is present. The
// three validated tokens (gate_scope, repo_topology, gate_impl) raise the
// exact historical refusal when the configured value is not allowed; an
// absent/empty value falls back to "auto" (Python's `or default`). The gate
// version is Python-resolved and substituted verbatim (Go never invents a
// value); the two tracker-home tokens become empty strings without a CLI
// home, which makes the host skip evidence acquisition (fail open). Rendering
// is string surgery only: CRLF bytes are never normalized (normalization
// happens only inside sha256Bytes).
func renderHookGateContent(src []byte, cfg map[string]any, cliHome string, gateVersion string) (string, error) {
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
		scope := templateConfigOr(cfg, "gate_scope", "auto")
		if scope != "auto" && scope != "superrepo" && scope != "subrepo" {
			return "", fmt.Errorf("invalid gate_scope '%s'; allowed: auto | superrepo | subrepo", scope)
		}
		replace(hookGateScopeToken, scope)
	}
	if strings.Contains(content, hookTopologyToken) {
		topology := templateConfigOr(cfg, "repo_topology", "auto")
		switch topology {
		case "auto", "standalone", "monorepo-apps", "monorepo-submodules":
		default:
			return "", fmt.Errorf(
				"invalid repo_topology '%s'; allowed: auto | standalone | monorepo-apps | monorepo-submodules", topology)
		}
		replace(hookTopologyToken, topology)
	}
	if strings.Contains(content, hookGateImplToken) {
		impl := templateConfigOr(cfg, "gate_impl", "auto")
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
// bytes its content-hash key names (sha256Bytes(bytes) == the digest encoded
// in the file name); a missing or partial file is never complete, so a
// truncated write cannot masquerade as the immutable snapshot.
func hookBackupComplete(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	want := strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	return want != "" && sha256Bytes(data) == want
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

// hookActuatorRequest is the --materialize-hook stdin contract. Config
// carries the merged recipe config (only gate_mode, gate_scope,
// repo_topology, and gate_impl are read); nil means render with defaults
// only. CLIHome is the Python-RESOLVED cli home (Python owns resolution, the
// same division as gate_version). ManagedEntry is the lock's managed[rel]
// subset — only sha256 is read; nil means no recorded provenance. BackupPath
// is the Python-precomputed immutable refresh snapshot path (empty when the
// destination does not exist yet).
type hookActuatorRequest struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Script       string                `json:"script"`
	Config       map[string]any        `json:"config,omitempty"`
	CLIHome      string                `json:"cli_home,omitempty"`
	GateVersion  string                `json:"gate_version,omitempty"`
	Refresh      bool                  `json:"refresh,omitempty"`
	ManagedEntry *classifyManagedEntry `json:"managed_entry,omitempty"`
	BackupPath   string                `json:"backup_path,omitempty"`
}

// hookActuatorRecord is the lock payload the Python bridge hands to
// lock.set_gate_baseline: target is the project-relative rel path, sha256 is
// the normalized (CRLF-folded) sha of the bytes the actuation produced, kind
// is always "gate", policy always "auto". A nil record means "write nothing
// to the lock".
type hookActuatorRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// hookActuatorOutput is the --materialize-hook stdout contract. Message is
// the detail line WITHOUT the print indentation; warnings carry the exact
// warn() strings (Python adds the "  ! " prefix); Backup is the refresh
// snapshot Go wrote (empty otherwise, omitted from the envelope).
type hookActuatorOutput struct {
	Rel      string              `json:"rel"`
	Dest     string              `json:"dest"`
	Wrote    bool                `json:"wrote"`
	Record   *hookActuatorRecord `json:"record"`
	Message  string              `json:"message"`
	Warnings []string            `json:"warnings"`
	Backup   string              `json:"backup,omitempty"`
	Error    *string             `json:"error"`
}

// hookWriteRefusal maps a hook write failure to its refusal string. The
// symlink guard reuses the template actuator's shared refusal (both
// authorities — Go and the Python fallback — spell it verbatim).
func hookWriteRefusal(rel, dest string, err error) string {
	if errors.Is(err, errDestSymlink) {
		return templateSymlinkRefusal(rel)
	}
	return fmt.Sprintf("write hook %s: %v", dest, err)
}

// runMaterializeHook is the --materialize-hook command: one JSON envelope on
// stdin, one JSON envelope on stdout, exit 0/2.
func runMaterializeHook(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: read stdin: %v\n", err)
		return 2
	}
	var in hookActuatorRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&in); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(in.ProjectRoot) == "" || strings.TrimSpace(in.RecipeID) == "" || strings.TrimSpace(in.Script) == "" {
		fmt.Fprintln(stderr, "worktree-gate: --materialize-hook: missing project_root, recipe_id or script")
		return 2
	}
	emit := func(out hookActuatorOutput) int {
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	refuse := func(message string) int {
		out := hookActuatorOutput{Error: &message}
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return 2
	}

	rel := hookScriptRelPath(in.RecipeID, in.Script)
	dest := filepath.Join(in.ProjectRoot, filepath.FromSlash(rel))
	srcPath := filepath.Join(in.RecipeDir, filepath.FromSlash(in.Script))
	srcInfo, statErr := os.Stat(srcPath)
	if statErr != nil || !srcInfo.Mode().IsRegular() {
		return refuse(fmt.Sprintf("hook script not found: %s", srcPath))
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: read source: %v\n", err)
		return 2
	}
	content, renderErr := renderHookGateContent(srcBytes, in.Config, in.CLIHome, in.GateVersion)
	if renderErr != nil {
		return refuse(renderErr.Error())
	}
	contentBytes := []byte(content)
	hookRecord := func(sha string) *hookActuatorRecord {
		return &hookActuatorRecord{
			Target: rel,
			SHA256: sha,
			Recipe: in.RecipeID,
			Source: in.Script,
			Kind:   "gate",
			Policy: "auto",
		}
	}

	if in.Refresh {
		return runHookGateRefresh(in, rel, dest, contentBytes, hookRecord, emit, refuse, stderr)
	}

	present, disk, err := readRegularFile(dest)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: dest %s: %v\n", dest, err)
		return 2
	}
	// The SHARED classification port (classify.go): never re-ported in
	// slice 7. The hook path passes the exact rendered content as would-write.
	wouldWrite := content
	state := classifyManagedOverride(present, disk, in.ManagedEntry, &wouldWrite).State
	out := hookActuatorOutput{Rel: rel, Dest: dest, Warnings: []string{}}
	switch state {
	case classifyMissing:
		if err := writeTemplateContent(dest, contentBytes, 0o755); err != nil {
			return refuse(hookWriteRefusal(rel, dest, err))
		}
		out.Wrote = true
		out.Record = hookRecord(sha256Bytes(contentBytes))
		out.Message = fmt.Sprintf("✓ hook script %s", rel)
	case classifyManagedCurrent:
		// Backfill provenance without rewriting the target (idempotent pair).
		out.Record = hookRecord(sha256Bytes(contentBytes))
		out.Message = fmt.Sprintf("· hook skipped (current) %s", rel)
	case classifyManagedStale:
		// Baseline matches current bytes: the CLI rendered this gate, so an
		// ordinary sync may force-update it and re-record the baseline.
		if err := writeTemplateContent(dest, contentBytes, 0o755); err != nil {
			return refuse(hookWriteRefusal(rel, dest, err))
		}
		out.Wrote = true
		out.Record = hookRecord(sha256Bytes(contentBytes))
		out.Message = fmt.Sprintf("✓ hook refreshed (baseline matched) %s", rel)
	case classifyUserModified:
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"hook %s is user-modified; preserving existing bytes. Refresh with:\n"+
				"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel))
		out.Message = fmt.Sprintf("· hook skipped (user-modified) %s", rel)
	case classifyUntracked:
		// No provenance: preserve, NEVER seed (unlike the template actuator).
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"hook %s has no recorded provenance; preserving existing bytes. "+
				"A baseline is recorded only when the CLI renders the gate. Refresh with:\n"+
				"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel))
		out.Message = fmt.Sprintf("· hook skipped (no provenance) %s", rel)
	}
	return emit(out)
}

// runHookGateRefresh is the --refresh-gates path: cache backup -> gate write,
// all-or-nothing. Classification is skipped — a refresh replaces even a
// user-modified gate after its exact pre-refresh bytes land in the
// Python-precomputed immutable backup. On ANY Go-side failure before success
// the prior bytes are restored (best effort), the backup is removed, and the
// actuator refuses with exit 2 so Python never writes a lock record.
func runHookGateRefresh(
	in hookActuatorRequest,
	rel, dest string,
	contentBytes []byte,
	hookRecord func(sha string) *hookActuatorRecord,
	emit func(hookActuatorOutput) int,
	refuse func(message string) int,
	stderr io.Writer,
) int {
	if info, statErr := os.Lstat(dest); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return refuse(templateSymlinkRefusal(rel))
	}
	var prior []byte
	hadPrior := false
	if info, statErr := os.Stat(dest); statErr == nil && info.Mode().IsRegular() {
		data, readErr := os.ReadFile(dest)
		if readErr != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: dest %s: %v\n", dest, readErr)
			return 2
		}
		prior = data
		hadPrior = true
	}
	backup := ""
	if hadPrior && in.BackupPath != "" {
		// The path is content-hash keyed (immutable): a COMPLETE snapshot is
		// never rewritten. A missing or PARTIAL one (a crash or short write can
		// leave truncated bytes at the key) is (re)written atomically so a
		// partial file never appears at the final path.
		if err := os.MkdirAll(filepath.Dir(in.BackupPath), 0o755); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: backup %s: %v\n", in.BackupPath, err)
			return 2
		}
		if !hookBackupComplete(in.BackupPath) {
			if err := writeHookBackupSnapshot(in.BackupPath, prior); err != nil {
				fmt.Fprintf(stderr, "worktree-gate: --materialize-hook: backup %s: %v\n", in.BackupPath, err)
				return 2
			}
		}
		backup = in.BackupPath
	}
	if err := writeTemplateContent(dest, contentBytes, 0o755); err != nil {
		if hadPrior {
			if restoreErr := os.WriteFile(dest, prior, 0o755); restoreErr == nil {
				_ = os.Chmod(dest, 0o755)
			}
		}
		if backup != "" {
			_ = os.Remove(backup)
		}
		return refuse(hookWriteRefusal(rel, dest, err))
	}
	return emit(hookActuatorOutput{
		Rel:      rel,
		Dest:     dest,
		Wrote:    true,
		Record:   hookRecord(sha256Bytes(contentBytes)),
		Message:  fmt.Sprintf("✓ hook refreshed %s", rel),
		Warnings: []string{},
		Backup:   backup,
	})
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Template actuator for the governed-template materializer (GO-09, strangler
// slice 6). Go owns the template actuation DECISION + EXECUTION: git-path
// destination resolution, rendering, the ownership classification (the shared
// classifyManagedOverride core behind --plan-classify — never re-ported), the
// write + chmod, and the managed-override record payload. Python keeps the
// lock load/write (set_managed_override + write_lock), all printing, and the
// fail-open fallback (GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK).
//
// Command surface: --materialize-template reads one JSON envelope on stdin
//
//	{"project_root": "...", "recipe_dir": "...", "recipe_id": "...",
//	 "source": "...", "target": "...", "condition": "not_exists",
//	 "update_policy": "auto", "config": {"repo_topology": "...", ...},
//	 "managed_entry": {"sha256": "..."} | null}
//
// and prints one result envelope on stdout with exit 0:
//
//	{"dest": "...", "wrote": true|false, "record": {...}|null,
//	 "message": "...", "info": "...", "warnings": [...], "error": null}
//
// A refusal (invalid update policy, missing source, execution failure) prints
// {"error": "<exact reference string>"} on stdout with exit 2 and touches no
// file. The Python bridge fails CLOSED on a delivered exit-2 error envelope
// (the GO-08 findings fix) and falls back to its historical Python body on
// infrastructure failures (no binary, bad JSON, malformed stdout envelope).

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

// templateActuatorRequest is the stdin envelope. Config carries the merged
// recipe config values (only repo_topology, worktrees_dir, and
// integration_branch are read); nil means render with defaults only.
// ManagedEntry is the lock's managed[target] subset — only sha256 is read;
// nil means untracked.
type templateActuatorRequest struct {
	ProjectRoot  string                `json:"project_root"`
	RecipeDir    string                `json:"recipe_dir"`
	RecipeID     string                `json:"recipe_id"`
	Source       string                `json:"source"`
	Target       string                `json:"target"`
	Condition    string                `json:"condition,omitempty"`
	UpdatePolicy string                `json:"update_policy,omitempty"`
	Config       map[string]any        `json:"config,omitempty"`
	ManagedEntry *classifyManagedEntry `json:"managed_entry,omitempty"`
}

// templateActuatorRecord is the lock payload the Python bridge hands to
// lock.set_managed_override: target is the posix form of tpl.target, sha256
// is the normalized (CRLF-folded) sha of the bytes actually on disk after the
// actuation, kind is always "template". A nil record means "write nothing to
// the lock".
type templateActuatorRecord struct {
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Recipe string `json:"recipe"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Policy string `json:"policy"`
}

// templateActuatorOutput is the stdout contract.
//   - dest: the resolved absolute destination (what resolveTemplateDest
//     produced, anchored when git emitted a repo-relative path).
//   - wrote: Go wrote the file itself (mkdir -p parent, rendered bytes,
//     chmod to the source permission bits). Python never re-writes.
//   - record: the managed-override payload, nil when nothing is recorded.
//   - message: the per-target detail line WITHOUT the print indentation
//     ("✓ template {target}" or "· template skipped (exists) {target}").
//   - info: optional info() line ("refreshed managed template {target}").
//   - warnings: exact warn() strings, in emission order.
//   - error: set with exit 2; no file is touched on refusal paths.
type templateActuatorOutput struct {
	Dest     string                  `json:"dest"`
	Wrote    bool                    `json:"wrote"`
	Record   *templateActuatorRecord `json:"record"`
	Message  string                  `json:"message"`
	Info     string                  `json:"info,omitempty"`
	Warnings []string                `json:"warnings"`
	Error    *string                 `json:"error"`
}

// pyConfigString reproduces Python str() over the decoded JSON value domain
// recipe config carries: strings verbatim, True/False capitalized, None for
// null, numbers as their literal text (the decoder preserves it via
// json.Number).
func pyConfigString(v any) string {
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

// templateConfigOr mirrors Python's `str(cfg.get(key) or default)`: an absent,
// null, empty, or falsy value falls back to the default.
func templateConfigOr(config map[string]any, key, fallback string) string {
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
		return pyConfigString(v)
	}
}

// renderTemplateBytes is the pure renderer: the shared topology token
// replacement (missing key or nil config defaults to "auto", mirroring
// util.render_override_bytes), then — only when config is non-nil — the two
// cleanup stamps with `or default` semantics. With a nil config the cleanup
// tokens stay LITERAL (the Python early return fires after the shared
// render). Rendering is byte surgery only: CRLF bytes are never normalized
// (normalization happens only inside sha256Bytes).
func renderTemplateBytes(src []byte, config map[string]any) []byte {
	data := src
	token := []byte(templateTopologyToken)
	if bytes.Contains(data, token) {
		topology := "auto"
		if config != nil {
			if v, ok := config["repo_topology"]; ok {
				topology = pyConfigString(v)
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
			data = bytes.ReplaceAll(data, stampToken, []byte(templateConfigOr(config, stamp.key, stamp.fallback)))
		}
	}
	return data
}

// resolveTemplateDest mirrors Python resolve_template_dest: a `.git/...`
// target (a Git hook) resolves through `git rev-parse --git-path`, because in
// a linked worktree `.git` is a gitfile and hooks land in the SHARED hooks
// directory of the main repository. Git emits a repo-relative path for the
// main worktree; the invocation anchors it at the project root. Anything else
// stays project-relative, and ANY failure (missing git, nonzero exit, empty
// stdout) fails open to the literal project-relative join.
func resolveTemplateDest(projectRoot, target string) string {
	literal := filepath.Join(projectRoot, filepath.FromSlash(target))
	if !strings.HasPrefix(target, ".git/") {
		return literal
	}
	remainder := target[len(".git/"):]
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
		return literal
	}
	resolved := strings.TrimSpace(string(out))
	if resolved == "" {
		return literal
	}
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(projectRoot, resolved)
	}
	return resolved
}

// writeTemplateContent mirrors Python write_content: mkdir -p the parent,
// write the rendered bytes, then chmod to the source permission bits
// (os.WriteFile applies the mode only at creation, so the explicit chmod is
// the os.chmod(dest, src.st_mode) parity).
func writeTemplateContent(dest string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, content, mode); err != nil {
		return err
	}
	return os.Chmod(dest, mode)
}

// runMaterializeTemplate is the --materialize-template command: one JSON
// envelope on stdin, one JSON envelope on stdout, exit 0/2.
func runMaterializeTemplate(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: read stdin: %v\n", err)
		return 2
	}
	var in templateActuatorRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&in); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: invalid input JSON: %v\n", err)
		return 2
	}
	if strings.TrimSpace(in.ProjectRoot) == "" || strings.TrimSpace(in.Target) == "" {
		fmt.Fprintln(stderr, "worktree-gate: --materialize-template: missing project_root or target")
		return 2
	}
	emit := func(out templateActuatorOutput) int {
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-template: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	refuse := func(message string) int {
		out := templateActuatorOutput{Error: &message}
		payload, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --materialize-template: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(payload))
		return 2
	}

	policy := in.UpdatePolicy
	if policy == "" {
		policy = templatePolicyAuto
	}
	srcPath := filepath.Join(in.RecipeDir, filepath.FromSlash(in.Source))
	srcInfo, statErr := os.Stat(srcPath)
	if statErr != nil || !srcInfo.Mode().IsRegular() {
		return refuse(fmt.Sprintf("template source not found: %s", srcPath))
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --materialize-template: read source: %v\n", err)
		return 2
	}
	if policy != templatePolicyAuto && policy != templatePolicyConfirm && policy != templatePolicyNeverForce {
		return refuse(fmt.Sprintf(
			"invalid update policy '%s' for template '%s'; expected auto | confirm | never-force",
			policy, in.Target))
	}
	content := renderTemplateBytes(srcBytes, in.Config)
	dest := resolveTemplateDest(in.ProjectRoot, in.Target)
	record := func(sha string) *templateActuatorRecord {
		return &templateActuatorRecord{
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
			present, disk, readErr := readRegularFile(dest)
			if readErr != nil {
				fmt.Fprintf(stderr, "worktree-gate: --materialize-template: dest %s: %v\n", dest, readErr)
				return 2
			}
			wouldWrite := string(content)
			state := classifyManagedOverride(present, disk, in.ManagedEntry, &wouldWrite).State
			// The Python reference prints the skip line at the end of the
			// exists branch regardless of the decision (fall-through).
			out := templateActuatorOutput{
				Dest:     dest,
				Message:  fmt.Sprintf("· template skipped (exists) %s", in.Target),
				Warnings: []string{},
			}
			switch state {
			case classifyUntracked:
				diskSHA := sha256Bytes(disk)
				// Existing projects may contain a pre-render placeholder copy
				// still carrying the raw catalog tokens; seed its actual bytes
				// and let the next sync reconcile it.
				if diskSHA == sha256Bytes(content) || diskSHA == sha256Bytes(srcBytes) {
					out.Record = record(diskSHA)
				} else {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"override metadata missing for %s; preserving existing file without assigning ownership. "+
							"To preserve this local file, leave it unchanged. To replace it with the current recipe version, "+
							"remove it and run sync again:\n  rm %s && ai-specs sync", in.Target, in.Target))
				}
			case classifyManagedStale:
				if policy == templatePolicyAuto {
					if err := writeTemplateContent(dest, content, sourceMode); err != nil {
						return refuse(fmt.Sprintf("write template %s: %v", dest, err))
					}
					out.Wrote = true
					out.Record = record(sha256Bytes(content))
					out.Info = fmt.Sprintf("refreshed managed template %s", in.Target)
				} else {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"override managed-stale (%s-required): %s was not refreshed. "+
							"Refresh with:\n  rm %s && ai-specs sync", policy, in.Target, in.Target))
				}
			case classifyUserModified:
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"override user-modified: %s was not refreshed. "+
						"Refresh with:\n  rm %s && ai-specs sync", in.Target, in.Target))
			case classifyManagedCurrent:
				// Backfill provenance fields without rewriting the target.
				out.Record = record(sha256Bytes(content))
			case classifyMissing:
				// The destination exists but is not a regular file: the
				// Python reference matches no branch, records nothing, and
				// still prints the skip line.
			}
			return emit(out)
		}
	}

	if err := writeTemplateContent(dest, content, sourceMode); err != nil {
		return refuse(fmt.Sprintf("write template %s: %v", dest, err))
	}
	return emit(templateActuatorOutput{
		Dest:     dest,
		Wrote:    true,
		Record:   record(sha256Bytes(content)),
		Message:  fmt.Sprintf("✓ template %s", in.Target),
		Warnings: []string{},
	})
}

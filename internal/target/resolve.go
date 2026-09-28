package target

// Go port of lib/_internal/target-resolve.py: resolve root + declared subrepo
// sync targets with byte-identical JSON output and identical error/exit
// surfaces. Manifest access reuses internal/config.LoadManifest (the port of
// toml-read.load_toml); project.subrepos normalization reuses the read_project
// semantics (strings only, stripped, non-empty).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/toml"
)

// DERIVED_ARTIFACTS and VERSION_POLICY, verbatim from target-resolve.py.
var DerivedArtifacts = []string{
	"AGENTS.md",
	"ai-specs/.gitignore",
	"ai-specs/skills/**",
	"ai-specs/commands/**",
	"agent-configs",
}

const VersionPolicy = "derived-only: overwrite from the root manifest on every root sync; " +
	"subrepo copies are advisory outputs and must not be hand-edited"

// programName is the usage string the Python CLI prints when driven as
// `python3 lib/_internal/target-resolve.py` (sys.argv[0]).
const programName = "lib/_internal/target-resolve.py"

// ResolutionError mirrors target-resolve.ResolutionError.
type ResolutionError struct {
	Rel    string
	Reason string
}

func (e *ResolutionError) Error() string { return e.Rel + ": " + e.Reason }

// AsDict returns the payload in the exact Python key order (path, reason).
func (e *ResolutionError) AsDict() *obj {
	out := newObj()
	out.set("path", e.Rel)
	out.set("reason", e.Reason)
	return out
}

// posixNormpath mirrors posixpath.normpath (including the POSIX double-slash
// special case) for forward-slash paths.
func posixNormpath(path string) string {
	if path == "" {
		return "."
	}
	initialSlashes := 0
	if strings.HasPrefix(path, "/") {
		initialSlashes = 1
		if strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "///") {
			initialSlashes = 2
		}
	}
	var newComps []string
	for _, comp := range strings.Split(path, "/") {
		if comp == "" || comp == "." {
			continue
		}
		if comp != ".." ||
			(initialSlashes == 0 && len(newComps) == 0) ||
			(len(newComps) > 0 && newComps[len(newComps)-1] == "..") {
			newComps = append(newComps, comp)
		} else if len(newComps) > 0 {
			newComps = newComps[:len(newComps)-1]
		}
		// else: leading slashes and no comps — drop the "..".
	}
	out := strings.Repeat("/", initialSlashes) + strings.Join(newComps, "/")
	if out == "" {
		return "."
	}
	return out
}

// normalizeDeclaredRelpath mirrors _normalize_declared_relpath. The error
// carries the PRE-normalization candidate; the return value is normalized.
func normalizeDeclaredRelpath(raw any) (string, error) {
	s, isStr := raw.(string)
	if !isStr {
		return "", &ResolutionError{Rel: pyRepr(raw), Reason: "must be a string"}
	}

	candidate := strings.TrimSpace(s)
	if candidate == "" {
		return "", nil
	}
	if strings.HasPrefix(candidate, "/") {
		return "", &ResolutionError{Rel: candidate, Reason: "must be relative to the root"}
	}

	candidate = strings.ReplaceAll(candidate, "\\", "/")
	normalized := posixNormpath(candidate)
	if normalized == "." || normalized == "" {
		return ".", nil
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", &ResolutionError{Rel: candidate, Reason: "escapes the root"}
	}
	return normalized, nil
}

// resolveNonStrict mirrors pathlib.Path.resolve(strict=False): resolve the
// existing prefix, keep the tail. Dangling symlinks resolve to their target
// path (os.path.realpath semantics), which the containment check then sees.
func resolveNonStrict(p string) string {
	// ponytail: depth cap 40 mirrors kernel MAXSYMLINKS; deeper chains
	// degrade to the un-resolved tail instead of looping.
	return resolveNonStrictDepth(p, 40)
}

func resolveNonStrictDepth(p string, depth int) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if depth > 0 {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if tgt, err := os.Readlink(p); err == nil {
				if !filepath.IsAbs(tgt) {
					tgt = filepath.Join(filepath.Dir(p), tgt)
				}
				return resolveNonStrictDepth(tgt, depth-1)
			}
		}
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolveNonStrictDepth(parent, depth-1), filepath.Base(p))
}

// validateTarget mirrors _validate_target: containment vs the resolved root
// first, then existence, then directory-ness. Returns the resolved path.
func validateTarget(root, rel string) (string, error) {
	candidate := resolveNonStrict(filepath.Join(root, rel))
	if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return "", &ResolutionError{Rel: rel, Reason: "escapes the root after resolution"}
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", &ResolutionError{Rel: rel, Reason: "directory does not exist"}
	}
	if !info.IsDir() {
		return "", &ResolutionError{Rel: rel, Reason: "path is not a directory"}
	}
	return candidate, nil
}

// worktreesDir mirrors _worktree_flow_config + the worktrees_dir fallback:
// [recipes.worktree-flow.config].worktrees_dir when config is a dict, else
// flat-style keys under [recipes.worktree-flow]; falsy → ".worktrees".
func worktreesDir(data *toml.Table) string {
	var v any
	var ok bool
	if recipes := tableOf(data, "recipes"); recipes != nil {
		if wf := tableOf(recipes, "worktree-flow"); wf != nil {
			if cfg := tableOf(wf, "config"); cfg != nil {
				v, ok = cfg.Get("worktrees_dir")
			} else {
				// Flat style: wf minus enabled/version — worktrees_dir is
				// the only key we read, so fetch it directly.
				v, ok = wf.Get("worktrees_dir")
			}
		}
	}
	if ok && pythonTruthy(v) {
		return pyStr(v)
	}
	return ".worktrees"
}

// pythonTruthy mirrors Python truthiness for tomllib value types.
func pythonTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case *toml.Table:
		return x != nil && len(x.Keys()) > 0
	default:
		return true
	}
}

// projectSubrepos mirrors toml-read.read_project's _normalize_subrepos:
// keep only stripped non-empty strings (everything else filtered out).
func projectSubrepos(data *toml.Table) []string {
	out := []string{}
	project := tableOf(data, "project")
	if project == nil {
		return out
	}
	v, ok := project.Get("subrepos")
	if !ok {
		return out
	}
	switch items := v.(type) {
	case []any:
		for _, item := range items {
			if s, isStr := item.(string); isStr {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
	case []*toml.Table:
		// Array of tables: entries are dicts, filtered out by Python.
	}
	return out
}

// ResolveTargetPlan mirrors resolve_target_plan: it returns the ordered plan
// object ready for json.dumps(plan, indent=2) emission.
func ResolveTargetPlan(projectRoot string) (*obj, error) {
	root := resolveNonStrict(absClean(projectRoot))
	tomlPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	data, err := config.LoadManifest(tomlPath)
	if err != nil {
		return nil, err
	}

	targets := []*obj{}
	fanout := []any{}
	seen := map[string]bool{".": true}

	appendTarget := func(name, kind, rel, path string) {
		target := newObj()
		target.set("name", name)
		target.set("kind", kind)
		target.set("path", path)
		target.set("rel", rel)
		target.set("planning_root", root)
		target.set("derived_ai_specs", filepath.Join(path, "ai-specs"))
		target.set("manifest_source", tomlPath)
		target.set("derived_artifacts", DerivedArtifacts)
		target.set("version_policy", VersionPolicy)
		targets = append(targets, target)
	}

	appendTarget("root", "root", ".", root)

	for _, raw := range projectSubrepos(data) {
		rel, err := normalizeDeclaredRelpath(raw)
		if err != nil {
			return nil, err
		}
		if rel == "" || seen[rel] {
			continue
		}
		path, err := validateTarget(root, rel)
		if err != nil {
			return nil, err
		}
		seen[rel] = true
		appendTarget(rel, "subrepo", rel, path)
		fanout = append(fanout, rel)
	}

	topology := projectRepoTopology(root, data)
	topologyObj := newObj()
	topologyObj.set("resolved", topology.Resolved)
	topologyObj.set("configured", topology.Configured)
	topologyObj.set("via", topology.Via)
	topologyObj.set("source", topology.Source)

	gitmodulesPath := filepath.Join(root, ".gitmodules")
	gitmodules := newObj()
	gitmodules.set("path", gitmodulesPath)
	gitmodules.set("mode", "advisory-only")
	gitmodules.set("present", isFile(gitmodulesPath))

	plan := newObj()
	plan.set("root", root)
	plan.set("manifest", tomlPath)
	plan.set("planning_root", root)
	plan.set("topology", topologyObj)
	plan.set("declared_only", true)
	plan.set("fanout_targets", fanout)
	plan.set("worktrees_dir", worktreesDir(data))
	plan.set("gitmodules", gitmodules)
	plan.set("targets", targets)
	return plan, nil
}

// absClean turns projectRoot into an absolute, cleaned path (the input side
// of Path(project_root).resolve()).
func absClean(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

// Run mirrors main(): the exit code and stdout/stderr bytes of
// `python3 lib/_internal/target-resolve.py <args...>`.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stderr, "Usage: %s <project_root>\n", programName)
		return 2
	}

	plan, err := ResolveTargetPlan(args[0])
	if err != nil {
		var resErr *ResolutionError
		// FileNotFoundError and the defensive top-level guard both print
		// "error: {exc}" in the Python original; only ResolutionError gets
		// a JSON envelope.
		if errors.As(err, &resErr) {
			envelope := newObj()
			envelope.set("error", resErr.AsDict())
			fmt.Fprintln(stderr, encodeCompact(envelope))
			return 1
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, encodeIndent(plan))
	return 0
}

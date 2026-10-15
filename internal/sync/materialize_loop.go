package sync

// Per-recipe artifact actuation (GO-07 S14a WU2).
//
// materialize.go owns the orchestration order and the verbatim prints up to the
// per-recipe frame; this file owns everything the Python authority performs
// inside that frame after the config merge: the project-owned config pass, the
// bundled/dep skills, the commands, the governed templates and docs, the
// sync-time hooks, the runtime-hook collection with the resolved-hooks entries,
// and the worktree-flow gate-binary acquisition.
//
// Every decision stays in the merged native authority (shared actuators, the
// copy authority, lock mutation helpers, RunRecipeHooks, MaterializeHook); this
// file only sequences those calls, prints their messages verbatim and writes
// the lock through ONE shared helper (applyLockRecord). One step is the
// deliberate, documented, single non-native delegation (the gate-binary
// acquisition runs natively through internal/gatebinary):
//
//   - delegateDepSkillToPython: dependency-skill vendoring stays in
//     vendor-skills.py's sync_dep_target, closed with the deps-layer card.
//
// The Python authority is lib/_internal/recipe-materialize.py
// materialize_recipes (:4013-4096 for the loop, :4139-4147 for the dump).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/cliversion"
	"ai-specs.dev/ai-specs/internal/gatebinary"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
	shared "ai-specs.dev/worktree-gate/shared"
)

// materializeArtifactRequest is the root boundary input for one recipe's
// artifact actuation. Merge is the config decision materialize.go already made
// (its warnings were printed there); the project-owned pass is applied here,
// exactly as the Python loop does after merge_config.
type materializeArtifactRequest struct {
	ProjectRoot  string
	AISpecsHome  string
	CatalogDir   string
	Recipe       materializeRecipe
	Merge        shared.MergeConfigResult
	RefreshGates bool
	Stdout       io.Writer
	Stderr       io.Writer
}

// materializeRecipeArtifacts runs the Python loop body for one enabled recipe
// and returns the resolved-hooks entries it collected (empty when the recipe
// declares no runtime hooks). Every failure is returned as an error; the caller
// turns it into the frozen fatal line.
func materializeRecipeArtifacts(req materializeArtifactRequest) ([]any, error) {
	recipeDir := filepath.Join(req.CatalogDir, req.Recipe.ID)
	recipePath := filepath.Join(recipeDir, "recipe.toml")
	recipe, err := schema.LoadRecipeToml(recipePath)
	if err != nil {
		return nil, err
	}
	mergedCfg := materializeProjectOwnedConfig(req.ProjectRoot, req.Recipe.ID, req.Merge.Config)
	lockPath := filepath.Join(req.ProjectRoot, "ai-specs", ".ai-specs.lock")
	roots := CopyRoots{
		RecipeSkillsRoot: projectcache.RecipeSkillsRoot(req.ProjectRoot, req.AISpecsHome),
		CommandsDir:      projectcache.CommandsDir(req.ProjectRoot, req.AISpecsHome),
		ProjectRoot:      req.ProjectRoot,
	}

	for _, skill := range recipe.Skills {
		switch skill.Source {
		case "bundled":
			if err := materializeBundledSkill(req, recipeDir, lockPath, roots, skill); err != nil {
				return nil, err
			}
		case "dep":
			if err := delegateDepSkillToPython(req.AISpecsHome, req.ProjectRoot, skill, req.Stdout, req.Stderr); err != nil {
				return nil, err
			}
			fmt.Fprintf(req.Stdout, "    ✓ dep skill %s\n", skill.ID)
		default:
			return nil, fmt.Errorf("unknown skill source '%s' for skill '%s'", skill.Source, skill.ID)
		}
	}

	for _, cmd := range recipe.Commands {
		if err := materializeCommandArtifact(req, recipeDir, roots, cmd); err != nil {
			return nil, err
		}
	}

	for _, tpl := range recipe.Templates {
		if err := materializeTemplateArtifact(req, recipeDir, lockPath, tpl, mergedCfg); err != nil {
			return nil, err
		}
	}

	for _, doc := range recipe.Docs {
		if err := materializeDocArtifact(req, recipeDir, lockPath, doc); err != nil {
			return nil, err
		}
	}

	if err := RunRecipeHooks(HooksRequest{
		RecipePath:   recipePath,
		MergedConfig: mergedCfg,
		ProjectRoot:  req.ProjectRoot,
		CLIHome:      req.AISpecsHome,
	}, req.Stdout, req.Stderr); err != nil {
		return nil, err
	}

	resolvedHooks := []any{}
	for _, rhook := range recipe.RuntimeHooks {
		entry, err := materializeRuntimeHook(req, recipeDir, lockPath, rhook, mergedCfg)
		if err != nil {
			return nil, err
		}
		resolvedHooks = append(resolvedHooks, entry)
	}

	if err := materializeWorktreeGateAcquisition(req, mergedCfg); err != nil {
		return nil, err
	}
	return resolvedHooks, nil
}

// materializeProjectOwnedConfig adapts the merge decision's ordered object onto
// the promoted project-owned grader. The Python loop passes project_root and a
// None data, so the grader re-reads the manifest (nil data).
func materializeProjectOwnedConfig(projectRoot, recipeID string, config *shared.OrderedMap) map[string]any {
	var raw map[string]any
	if config != nil {
		raw = config.Values()
	}
	return ProjectOwnedRecipeConfig(projectRoot, nil, recipeID, raw)
}

// applyLockRecord is the ONE shared lock-write helper (decision 5): load the
// lock, apply the mutation, write it back. Python loads and writes once per
// artifact record, so each call here is one load + one write, preserving the
// reference's per-record write count.
func applyLockRecord(lockPath string, mutate func(*lock.Lock)) error {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	mutate(lk)
	return lock.WriteLock(lockPath, lk)
}

// materializeManagedSHA loads the [managed."<target>"] sha256 the actuators
// read (nil when the target is untracked).
func materializeManagedSHA(lockPath, target string) (*shared.ClassifyManagedEntry, error) {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	raw, ok := lk.Managed[target]
	if !ok {
		return nil, nil
	}
	sha, _ := raw["sha256"].(string)
	return &shared.ClassifyManagedEntry{SHA256: strings.TrimSpace(sha)}, nil
}

// materializeDocManagedEntry loads the docs-flow projection of the
// [managed."<target>"] entry: sha256 plus whether a policy key is present.
func materializeDocManagedEntry(lockPath, target string) (*DocManagedEntry, error) {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	raw, ok := lk.Managed[target]
	if !ok {
		return nil, nil
	}
	entry := &DocManagedEntry{}
	if sha, ok := raw["sha256"].(string); ok {
		entry.SHA256 = strings.TrimSpace(sha)
	}
	if policy, present := raw["policy"]; present {
		entry.HasPolicy = true
		if text, ok := policy.(string); ok {
			entry.Policy = text
		}
	}
	return entry, nil
}

// materializeBundledSkill mirrors materialize_bundled_skill (:938): one copy
// item through the copy authority, a missing source is fatal with Python's
// message, the confirmation prints, then every file under the destination is
// hashed (posix relative keys) and registered as the recipe skill hashes.
func materializeBundledSkill(req materializeArtifactRequest, recipeDir, lockPath string, roots CopyRoots, skill *schema.SkillRef) error {
	src := filepath.Join(recipeDir, "skills", skill.ID)
	dest := filepath.Join(roots.RecipeSkillsRoot, req.Recipe.ID, "skills", skill.ID)
	results, err := ApplyCopyItems([]shared.CopyItem{{
		Kind: "bundled-skill",
		ID:   skill.ID,
		Src:  src,
		Dest: dest,
	}})
	if err != nil {
		return err
	}
	if len(results) == 1 && results[0].Status == "source-missing" {
		return fmt.Errorf("bundled skill not found: %s", src)
	}
	fmt.Fprintf(req.Stdout, "    ✓ bundled skill %s\n", skill.ID)
	hashes, err := materializeSkillFileHashes(dest)
	if err != nil {
		return err
	}
	return applyLockRecord(lockPath, func(lk *lock.Lock) {
		lock.SetRecipeSkillHashes(lk, req.Recipe.ID, skill.ID, hashes)
	})
}

// materializeSkillFileHashes mirrors `{str(p.relative_to(dest)): sha256_of(p)
// for p in dest.rglob("*") if p.is_file()}`: every regular file below dest,
// keyed by its posix relative path, hashed through lock.Sha256OfFile (normalized
// bytes). A missing destination yields an empty map.
func materializeSkillFileHashes(dest string) (map[string]string, error) {
	hashes := map[string]string{}
	if _, err := os.Stat(dest); err != nil {
		return hashes, nil
	}
	err := filepath.WalkDir(dest, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
		}
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		sum, err := lock.Sha256OfFile(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

// materializeCommandArtifact mirrors materialize_command (:989): one copy item,
// a missing source is fatal with Python's message, an overwrite prints the
// verbatim warning, then the confirmation.
func materializeCommandArtifact(req materializeArtifactRequest, recipeDir string, roots CopyRoots, cmd *schema.CommandRef) error {
	src := filepath.Join(recipeDir, cmd.Path)
	dest := filepath.Join(roots.CommandsDir, cmd.ID+".md")
	results, err := ApplyCopyItems([]shared.CopyItem{{
		Kind:        "command",
		ID:          cmd.ID,
		Src:         src,
		Dest:        dest,
		CommandsDir: roots.CommandsDir,
	}})
	if err != nil {
		return err
	}
	if len(results) == 1 {
		if results[0].Status == "source-missing" {
			return fmt.Errorf("command source not found: %s", src)
		}
		if results[0].Overwrite {
			warnf(req.Stderr, "recipe command '%s' overwrites existing managed command at %s", cmd.ID, dest)
		}
	}
	fmt.Fprintf(req.Stdout, "    ✓ command %s\n", cmd.ID)
	return nil
}

// materializeTemplateArtifact mirrors materialize_template (:1261): the shared
// template actuator decides and writes, then the record is applied to the lock,
// the info line prints, and the message prints with the four-space indent.
func materializeTemplateArtifact(req materializeArtifactRequest, recipeDir, lockPath string, tpl *schema.TemplateRef, mergedCfg map[string]any) error {
	target := posixAsPosix(tpl.Target)
	entry, err := materializeManagedSHA(lockPath, target)
	if err != nil {
		return err
	}
	res, err := MaterializeTemplate(&shared.TemplateRequest{
		ProjectRoot:  req.ProjectRoot,
		RecipeDir:    recipeDir,
		RecipeID:     req.Recipe.ID,
		Source:       tpl.Source,
		Target:       target,
		Condition:    tpl.Condition,
		UpdatePolicy: tpl.UpdatePolicy,
		Config:       mergedCfg,
		ManagedEntry: entry,
	})
	if err != nil {
		return err
	}
	for _, line := range res.Warnings {
		warnf(req.Stderr, "%s", line)
	}
	if res.Record != nil {
		rec := res.Record
		if err := applyLockRecord(lockPath, func(lk *lock.Lock) {
			lock.SetManagedOverride(lk, rec.Target, rec.SHA256, &rec.Recipe, &rec.Source, &rec.Kind, &rec.Policy)
		}); err != nil {
			return err
		}
	}
	if res.Info != "" {
		fmt.Fprintf(req.Stdout, "  \u2139 %s\n", res.Info)
	}
	fmt.Fprintf(req.Stdout, "    %s\n", res.Message)
	return nil
}

// materializeDocArtifact mirrors materialize_doc (:1551): the docs-flow
// authority decides and copies, then the record is applied to the lock, the
// info line prints and the message prints (the directory/symlink preserve path
// returns an empty message and prints nothing).
func materializeDocArtifact(req materializeArtifactRequest, recipeDir, lockPath string, doc *schema.DocRef) error {
	entry, err := materializeDocManagedEntry(lockPath, posixAsPosix(doc.Target))
	if err != nil {
		return err
	}
	res, err := MaterializeDoc(&DocRequest{
		ProjectRoot:  req.ProjectRoot,
		RecipeDir:    recipeDir,
		RecipeID:     req.Recipe.ID,
		Source:       doc.Source,
		Target:       doc.Target,
		ManagedEntry: entry,
	})
	if err != nil {
		return err
	}
	for _, line := range res.Warnings {
		warnf(req.Stderr, "%s", line)
	}
	if res.Record != nil {
		rec := res.Record
		if err := applyLockRecord(lockPath, func(lk *lock.Lock) {
			lock.SetManagedOverride(lk, rec.Target, rec.SHA256, &rec.Recipe, &rec.Source, &rec.Kind, &rec.Policy)
		}); err != nil {
			return err
		}
	}
	if res.Info != "" {
		fmt.Fprintf(req.Stdout, "  \u2139 %s\n", res.Info)
	}
	if res.Message != "" {
		fmt.Fprintf(req.Stdout, "    %s\n", res.Message)
	}
	return nil
}

// materializeRuntimeHook mirrors the runtime-hook branch of the loop
// (:4066-4085): materialize_hook_script through the shared hook actuator, then
// the gate baseline is recorded, the message prints and a resolved-hooks entry
// is collected.
func materializeRuntimeHook(req materializeArtifactRequest, recipeDir, lockPath string, rhook *schema.RuntimeHook, mergedCfg map[string]any) (*mcpObj, error) {
	rel := shared.HookScriptRelPath(req.Recipe.ID, rhook.Script)
	// The Python authority refuses a symlinked destination before it reads the
	// lock or calls the actuator; the shared actuator's write guard alone would
	// instead classify a symlink whose bytes match the baseline as current.
	if info, err := os.Lstat(filepath.Join(req.ProjectRoot, filepath.FromSlash(rel))); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, shared.Refuse(shared.TemplateSymlinkRefusal(rel))
	}
	entry, err := materializeManagedSHA(lockPath, rel)
	if err != nil {
		return nil, err
	}
	backupPath, err := materializeGateBackupPath(req, rel)
	if err != nil {
		return nil, err
	}
	res, err := MaterializeHook(&HookRequest{
		ProjectRoot:  req.ProjectRoot,
		RecipeDir:    recipeDir,
		RecipeID:     req.Recipe.ID,
		Script:       rhook.Script,
		Config:       mergedCfg,
		CLIHome:      projectcache.ResolvePath(req.AISpecsHome),
		GateVersion:  cliversion.ReadInstalledVersion(req.AISpecsHome),
		Refresh:      req.RefreshGates,
		ManagedEntry: entry,
		BackupPath:   backupPath,
	})
	if err != nil {
		return nil, err
	}
	for _, line := range res.Warnings {
		warnf(req.Stderr, "%s", line)
	}
	if res.Record != nil {
		rec := res.Record
		if err := applyLockRecord(lockPath, func(lk *lock.Lock) {
			lock.SetGateBaseline(lk, rec.Target, rec.SHA256, &rec.Recipe, &rec.Source)
		}); err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(req.Stdout, "    %s\n", res.Message)
	return materializeResolvedHookEntry(req.Recipe.ID, rhook, res.Rel, mergedCfg), nil
}

// materializeGateBackupPath mirrors the Python refresh precomputation: only
// when --refresh-gates is set and the destination is a regular file is the
// immutable content-hash backup path computed (project-cache ownership).
func materializeGateBackupPath(req materializeArtifactRequest, rel string) (string, error) {
	if !req.RefreshGates {
		return "", nil
	}
	dest := filepath.Join(req.ProjectRoot, filepath.FromSlash(rel))
	info, err := os.Stat(dest)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		return "", err
	}
	return projectcache.GateBackupPath(req.ProjectRoot, rel, lock.Sha256Bytes(data), req.AISpecsHome), nil
}

// materializeResolvedHookEntry builds the dict hooks-render.py consumes:
// {recipe, id, event, matcher, blocking, script_path, env}. env carries the
// merged-config keys that look like environment names: str.isupper() AND no
// hyphen AND the underscore-stripped key is alphanumeric. Values are Python
// str().
func materializeResolvedHookEntry(recipeID string, rhook *schema.RuntimeHook, scriptPath string, mergedCfg map[string]any) *mcpObj {
	entry := newMCPObj()
	entry.set("recipe", recipeID)
	entry.set("id", rhook.ID)
	entry.set("event", rhook.Event)
	entry.set("matcher", rhook.Matcher)
	entry.set("blocking", rhook.Blocking)
	entry.set("script_path", scriptPath)
	env := newMCPObj()
	for _, key := range sortedStrings(materializeEnvKeys(mergedCfg)) {
		env.set(key, materializePyStr(mergedCfg[key]))
	}
	entry.set("env", env)
	return entry
}

// materializeEnvKeys returns the merged-config keys Python exports to a runtime
// hook: k.isupper() and "-" not in k and k.replace("_", "").isalnum().
func materializeEnvKeys(mergedCfg map[string]any) []string {
	out := make([]string, 0, len(mergedCfg))
	for key := range mergedCfg {
		if materializeEnvKeyExported(key) {
			out = append(out, key)
		}
	}
	return out
}

// materializeEnvKeyExported is the Python predicate over one config key.
func materializeEnvKeyExported(key string) bool {
	return materializePyIsUpper(key) &&
		!strings.Contains(key, "-") &&
		hooksIsAlnum(strings.ReplaceAll(key, "_", ""))
}

// materializePyIsUpper mirrors str.isupper(): True when at least one character
// is cased and no cased character is lowercase or titlecase. Cased covers
// letters with the Unicode Lower/Upper/Titlecase properties; every other
// character (digits, "_", "-", symbols) is ignored.
func materializePyIsUpper(value string) bool {
	cased := false
	for _, r := range value {
		if unicode.IsLower(r) || unicode.IsTitle(r) {
			return false
		}
		if unicode.IsUpper(r) {
			cased = true
		}
	}
	return cased
}

// materializePyStr renders Python str() for the merged-config value domain:
// scalars through the existing str() parity helper, ordered objects as their
// Python dict repr. json.Number keeps its literal text, which is Python's
// str(int) / str(float) shortest form.
func materializePyStr(value any) string {
	switch typed := value.(type) {
	case json.Number:
		return typed.String()
	case *shared.OrderedMap:
		if typed == nil {
			return "None"
		}
		parts := make([]string, 0, len(typed.Keys()))
		for _, key := range typed.Keys() {
			item, _ := typed.Get(key)
			parts = append(parts, materializePyStr(key)+": "+materializePyStr(item))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return hooksPyStr(value)
	}
}

// materializeWorktreeGateAcquisition mirrors the Phase 3 distribution branch of
// the loop (:4087-4103): only worktree-flow with gate_impl auto|go acquires,
// acquisition never fails the sync, and a status warning prints verbatim.
func materializeWorktreeGateAcquisition(req materializeArtifactRequest, mergedCfg map[string]any) error {
	if req.Recipe.ID != legacyTopologyRecipeID {
		return nil
	}
	impl := "auto"
	if value, ok := mergedCfg["gate_impl"]; ok && materializeConfigTruthy(value) {
		impl = materializePyStr(value)
	}
	if impl != "auto" && impl != "go" {
		return nil
	}
	warning, err := acquireGateBinary(req.AISpecsHome, impl)
	if err != nil {
		warnf(req.Stderr, "worktree-flow: gate binary acquisition failed (%s); gate degrades per gate_impl (see 'ai-specs doctor')", err.Error())
		return nil
	}
	if warning != "" {
		warnf(req.Stderr, "worktree-flow: %s", warning)
	}
	return nil
}

// materializeConfigTruthy mirrors Python truthiness over the merged-config
// value domain for the `merged_cfg.get("gate_impl") or "auto"` fallback: an
// absent, null, empty, false or zero value falls back.
func materializeConfigTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		return err != nil || number != 0
	case []any:
		return len(typed) > 0
	case []*toml.Table:
		return len(typed) > 0
	case *shared.OrderedMap:
		return typed != nil && len(typed.Keys()) > 0
	}
	return true
}

// --- the two deliberate delegations -----------------------------------------

// depSkillDelegationScript loads vendor-skills.py the same way
// materialize_dep_skill does (sibling import with lib/_internal on sys.path)
// and calls sync_dep_target(dep, project_root, cli_home=...). The dep dict is
// JSON on argv, and in_project stays False (recipe-deps stage under the CLI
// cache), exactly as the reference call. A raised exception is printed as its
// plain message on stderr with exit 1 so the root emits ONE fatal line; the
// vendor module's own fail() keeps its already-printed "  ✗ <msg>" line.
const depSkillDelegationScript = `import importlib.util, json, sys
from pathlib import Path
try:
    vendor_path = Path(sys.argv[1])
    internal_dir = str(vendor_path.parent)
    if internal_dir not in sys.path:
        sys.path.insert(0, internal_dir)
    spec = importlib.util.spec_from_file_location("vendor_skills_internal", vendor_path)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load vendor-skills.py at %s" % vendor_path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    mod.sync_dep_target(json.loads(sys.argv[4]), Path(sys.argv[2]), cli_home=Path(sys.argv[3]))
except SystemExit:
    raise
except BaseException as exc:
    sys.stderr.write(str(exc) + "\n")
    sys.exit(1)
`

// delegateDepSkillToPython is the single deliberate non-native step of the
// per-recipe loop: dependency-skill vendoring lives in vendor-skills.py's
// sync_dep_target (clone + SKILL.md render + ancillary copy) and is not ported
// yet. It runs that function through python3 with the same arguments
// materialize_dep_skill passes, forwarding the vendoring progress line on
// stdout and mapping a failure onto the exact fatal message. It is closed with
// the deps-layer card, not with S14.
func delegateDepSkillToPython(aiSpecsHome, projectRoot string, skill *schema.SkillRef, stdout, stderr io.Writer) error {
	vendorPath := filepath.Join(aiSpecsHome, "lib", "_internal", "vendor-skills.py")
	if _, err := os.Stat(vendorPath); err != nil {
		return fmt.Errorf("unable to load vendor-skills.py at %s", vendorPath)
	}
	dep := map[string]any{"id": skill.ID, "source": skill.URL}
	if skill.Path != "" {
		dep["path"] = skill.Path
	}
	payload, err := json.Marshal(dep)
	if err != nil {
		return err
	}
	stdoutText, stderrText, runErr := materializeRunPython(depSkillDelegationScript, []string{
		vendorPath, projectRoot, aiSpecsHome, string(payload),
	})
	if stdoutText != "" {
		fmt.Fprint(stdout, stdoutText)
	}
	if runErr != nil {
		message := strings.TrimRight(stderrText, "\n")
		message = strings.TrimPrefix(message, "  ✗ ")
		if message == "" {
			message = runErr.Error()
		}
		return errors.New(message)
	}
	if stderrText != "" {
		fmt.Fprint(stderr, stderrText)
	}
	return nil
}

// acquireGateBinary runs the native gatebinary.Acquire with the arguments the
// Python loop passes (gate_impl, the CLI home, AI_SPECS_GATE_OFFLINE) and
// returns the status warning ("" when none). Acquire never errors; a panic is
// the only failure left, reported as "<Type>: <message>" so the caller prints
// the frozen degradation warning instead of failing the sync.
func acquireGateBinary(aiSpecsHome, impl string) (warning string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("RuntimeError: %v", r)
		}
	}()
	status := gatebinary.Acquire(gatebinary.AcquireOptions{
		GateImpl: impl,
		Home:     aiSpecsHome,
		Offline:  os.Getenv("AI_SPECS_GATE_OFFLINE") == "1",
	})
	if status.Warn != nil {
		return *status.Warn, nil
	}
	return "", nil
}

// materializeRunPython runs one inline python3 helper with PYTHONDONTWRITEBYTECODE
// set so the delegated import never drops __pycache__ into the installation.
func materializeRunPython(script string, args []string) (stdout, stderr string, err error) {
	cmd := exec.Command("python3", append([]string{"-c", script}, args...)...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

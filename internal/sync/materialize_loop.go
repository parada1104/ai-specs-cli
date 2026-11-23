package sync

// Per-recipe artifact actuation (GO-07 S14a WU2).
//
// materialize.go owns the orchestration order and the verbatim prints up to the
// per-recipe frame; this file owns everything the Python authority performs
// inside that frame after the config merge: the project-owned config pass, the
// bundled/dep skills, the commands, the governed templates and docs, the
// sync-time hooks and the runtime-hook collection with the resolved-hooks
// entries.
//
// Every decision stays in the merged native authority (shared actuators, the
// copy authority, lock mutation helpers, RunRecipeHooks, MaterializeHook); this
// file only sequences those calls, prints their messages verbatim and writes
// the lock through ONE shared helper (applyLockRecord). The deliberate
// delegation is dependency-skill vendoring:
//
//   - materializeDepSkill: dependency-skill vendoring runs natively through
//     internal/sync/vendorskills.go (the vendor-skills.py port).
//
// The Python authority is lib/_internal/recipe-materialize.py
// materialize_recipes (:4013-4096 for the loop, :4139-4147 for the dump).

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"ai-specs.dev/ai-specs/internal/cliversion"
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
	// SnapshotOut receives the resolved snapshot. The spine publishes it
	// only on its successful return, so a failed run leaves the old one live.
	SnapshotOut *pendingRuntimeSnapshot
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
	// A CLI-served hook (T2/T6) is served from the snapshot the CLI reads, so
	// sync records the resolved runtime config it just merged. Each recipe owns
	// its own half of one per-project snapshot.
	if recipeContributesRuntimeSnapshot(req.Recipe.ID) {
		snapshot := projectcache.RuntimeSnapshot{}
		if req.SnapshotOut != nil {
			snapshot = req.SnapshotOut.Snapshot
		}
		if snapErr := applyRecipeRuntimeSnapshot(req.Recipe.ID, mergedCfg, &snapshot); snapErr != nil {
			return nil, snapErr
		}
		// Staged, not written: the snapshot may only go live once the whole run
		// succeeded, so the spine publishes it (R1-002/R3-001).
		if req.SnapshotOut != nil {
			*req.SnapshotOut = pendingRuntimeSnapshot{Snapshot: snapshot, Found: true}
		}
	}
	lockPath := filepath.Join(req.ProjectRoot, "ai-specs", ".ai-specs.lock")
	roots := CopyRoots{
		RecipeSkillsRoot: projectcache.RecipeSkillsRoot(req.ProjectRoot, req.AISpecsHome),
		CommandsDir:      projectcache.CommandsDir(req.ProjectRoot, req.AISpecsHome),
		ProjectRoot:      req.ProjectRoot,
	}

	ownedSkills := []string{}
	for _, skill := range recipe.Skills {
		switch skill.Source {
		case "bundled":
			if err := materializeBundledSkill(req, recipeDir, lockPath, roots, skill); err != nil {
				return nil, err
			}
			ownedSkills = append(ownedSkills, skill.ID)
		case "dep":
			if err := materializeDepSkill(req.ProjectRoot, req.AISpecsHome, skill, req.Stdout, req.Stderr); err != nil {
				return nil, err
			}
			fmt.Fprintf(req.Stdout, "    ✓ dep skill %s\n", skill.ID)
		default:
			return nil, fmt.Errorf("unknown skill source '%s' for skill '%s'", skill.Source, skill.ID)
		}
	}

	// Record the skill copies this run materialized so the per-skill orphan
	// prune only ever deletes ai-specs' own output (T7, S3).
	if err := recordOwnedRecipeSkills(req.AISpecsHome, roots.RecipeSkillsRoot, req.Recipe.ID, ownedSkills); err != nil {
		return nil, err
	}

	for _, cmd := range recipe.Commands {
		if err := materializeCommandArtifact(req, recipeDir, roots, cmd); err != nil {
			return nil, err
		}
	}

	postMerge := false
	for _, tpl := range recipe.Templates {
		if worktreeTemplateServesCLI(req.Recipe.ID, tpl.Target) {
			// The CLI serves this target itself (T3/S4): the cleanup launcher is
			// retired and the post-merge hook is generated after this loop.
			if posixAsPosix(tpl.Target) == worktreePostMergeTarget {
				postMerge = true
			}
			continue
		}
		if vaultMCPTemplateServesCLI(req.Recipe.ID, tpl.Target) {
			// `ai-specs mcp vault-fs` serves the vault MCP server (T4/S5); the
			// wrapper script is retired by the prune, never materialized.
			continue
		}
		if err := materializeTemplateArtifact(req, recipeDir, lockPath, tpl, mergedCfg); err != nil {
			return nil, err
		}
	}
	// Always regenerated when declared: the hook holds no policy, only the verb.
	if postMerge {
		if err := materializeWorktreePostMergeHook(req.ProjectRoot); err != nil {
			return nil, err
		}
	}

	for _, doc := range recipe.Docs {
		if recipeREADMEServesCLI(req.Recipe.ID, doc.Target) {
			// The README stays catalog documentation (T5/S1): the project keeps
			// no copy, and the prune retires an older sync's one.
			continue
		}
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
		if cliServedHookCommand(req.Recipe.ID, rhook) != "" {
			// CLI-served: no launcher bytes to materialize; the resolved entry
			// carries the CLI command every renderer wires.
			resolvedHooks = append(resolvedHooks, materializeResolvedHookEntry(req.Recipe.ID, rhook, "", mergedCfg))
			continue
		}
		entry, err := materializeRuntimeHook(req, recipeDir, lockPath, rhook, mergedCfg)
		if err != nil {
			return nil, err
		}
		resolvedHooks = append(resolvedHooks, entry)
	}

	return resolvedHooks, nil
}

// applyRecipeRuntimeSnapshot merges one CLI-served recipe's contribution into
// the pending per-project snapshot: worktree-flow owns the worktree gate policy,
// trello-mcp-workflow the tracker gate's legacy hint. The merge is additive so a
// project with only one of the two recipes still gets a complete snapshot.
func applyRecipeRuntimeSnapshot(recipeID string, mergedCfg map[string]any, snapshot *projectcache.RuntimeSnapshot) error {
	switch recipeID {
	case "worktree-flow":
		resolved, err := runtimeSnapshotFromConfig(mergedCfg)
		if err != nil {
			return err
		}
		resolved.TrackerGateMode = snapshot.TrackerGateMode
		*snapshot = resolved
	case "trello-mcp-workflow":
		snapshot.TrackerGateMode = trackerRuntimeGateMode(mergedCfg)
	}
	return nil
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
	// The copier RemoveAll's the destination: a redirected skills directory
	// below the CLI home would delete and replace the link target (JD-A-002).
	if err := cachePathGuard(req.AISpecsHome, dest); err != nil {
		return err
	}
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
	// The shared copier writes and chmods through a destination symlink: a
	// redirected live command entry below the CLI home must refuse (JD-A-001).
	if err := cachePathGuard(req.AISpecsHome, dest); err != nil {
		return err
	}
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
	backup := projectcache.GateBackupPath(req.ProjectRoot, rel, lock.Sha256Bytes(data), req.AISpecsHome)
	// The actuator mkdirs and writes the snapshot: a redirected backups entry
	// below the CLI home must refuse instead of writing outside it (JD-A-003).
	if err := cachePathGuard(req.AISpecsHome, backup); err != nil {
		return "", err
	}
	return backup, nil
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
	if command := cliServedHookCommand(recipeID, rhook); command != "" {
		entry.set("command", command)
	} else {
		entry.set("script_path", scriptPath)
	}
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

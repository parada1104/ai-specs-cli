package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// cleanupConfig is intentionally independent from the pre-tool-use gate config.
// The cleanup command is destructive and therefore defaults to a fail-closed
// implementation when its verified binary is unavailable at the launcher.
type cleanupConfig struct {
	worktreesDir      string
	baseBranch        string
	integrationBranch string
	protected         map[string]struct{}
	dryRun            bool
	topology          string
	scopes            []string
}

type repoPass struct {
	root       string
	modulePath string
}

type worktreeRecord struct {
	path   string
	sha    string
	branch string
}

// protectedBranchSet returns the hard-coded names plus all configured branch
// names. A map makes membership checks cheap and removes duplicate aliases.
func protectedBranchSet(baseBranch, integrationBranch string) map[string]struct{} {
	protected := map[string]struct{}{
		"main":        {},
		"master":      {},
		"development": {},
		"staging":     {},
	}
	for _, name := range []string{baseBranch, integrationBranch} {
		if name = strings.TrimSpace(name); name != "" {
			protected[name] = struct{}{}
		}
	}
	return protected
}

func newCleanupConfig(superRoot string, worktreesDir, base, integration, topology string, dryRun bool, scopes []string) cleanupConfig {
	if worktreesDir == "" {
		worktreesDir = ".worktrees"
	}
	if topology == "" {
		topology = "auto"
	}
	if base == "" {
		base = integration
	}
	if integration == "" {
		integration = base
	}
	return cleanupConfig{
		worktreesDir:      filepath.Clean(worktreesDir),
		baseBranch:        base,
		integrationBranch: integration,
		protected:         protectedBranchSet(base, integration),
		dryRun:            dryRun,
		topology:          topology,
		scopes:            append([]string(nil), scopes...),
	}
}

func isProtectedBranch(protected map[string]struct{}, branch string) bool {
	_, ok := protected[strings.TrimSpace(branch)]
	return ok
}

func refuseProtected(kind, branch string) error {
	return fmt.Errorf("worktree-cleanup: refusing destructive cleanup of protected branch %q before %s", branch, kind)
}

// assertDeletable is called immediately before each destructive wrapper. Do
// not move this check to classification: callers can observe repository state
// between classification and deletion, and protected heads must stay protected
// even under that race.
func assertDeletable(kind, branch string, protected map[string]struct{}) error {
	if isProtectedBranch(protected, branch) {
		return refuseProtected(kind, branch)
	}
	return nil
}

func formatCleanupStatus(w io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(w, format+"\n", args...)
}

func cleanupPath(root, dir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Join(root, filepath.Clean(dir))
}

// parseWorktreePorcelain preserves branch names and paths exactly as Git emits
// them. Git worktree porcelain records are line-oriented for metadata; paths
// are only used after Git has already delimited the record.
func parseWorktreePorcelain(text string) []worktreeRecord {
	var records []worktreeRecord
	var current worktreeRecord
	flush := func() {
		if current.path != "" {
			records = append(records, current)
		}
		current = worktreeRecord{}
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.sha = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			current.branch = ""
		}
	}
	flush()
	return records
}

func worktreeRecords(root string) []worktreeRecord {
	return parseWorktreePorcelain(git(root, "worktree", "list", "--porcelain"))
}

func resolveBaseCandidatesCleanup(repoRoot, base string) []string {
	var candidates []string
	seen := map[string]struct{}{}
	emit := func(ref string) {
		if ref == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		if gitMemo(repoRoot, "rev-parse", "--verify", "--quiet", ref) == "" {
			return
		}
		seen[ref] = struct{}{}
		candidates = append(candidates, ref)
	}

	emit(base)
	upstream := git(repoRoot, "rev-parse", "--verify", "--quiet", "--symbolic-full-name", base+"@{u}")
	emit(upstream)
	configuredRemote := git(repoRoot, "config", "--get", "branch."+base+".remote")
	configuredResolved := false
	if configuredRemote != "" {
		ref := "refs/remotes/" + configuredRemote + "/" + base
		if gitMemo(repoRoot, "rev-parse", "--verify", "--quiet", ref) != "" {
			configuredResolved = true
			emit(ref)
		}
	}
	if !configuredResolved && git(repoRoot, "config", "--get", "remote.origin.url") != "" {
		emit("refs/remotes/origin/" + base)
	}
	return candidates
}

func candidateHasMergedTipCleanup(repoRoot, sha, candidate string) bool {
	return runGit(repoRoot, "merge-base", "--is-ancestor", sha, candidate) == nil
}

func candidateHasPatchEquivalenceCleanup(repoRoot, sha, candidate string) bool {
	if strings.TrimSpace(git(repoRoot, "rev-list", candidate+".."+sha)) == "" {
		return false
	}
	cherry := git(repoRoot, "cherry", candidate, sha)
	if cherry == "" {
		return false
	}
	for _, line := range strings.Split(cherry, "\n") {
		if strings.HasPrefix(line, "+") {
			return false
		}
	}
	// Matches the Bash reference exactly: the per-commit `git cherry` proof
	// stands on its own here. A reverted squash is already rejected at the
	// cherry step, because the individual commits' patch-ids no longer match
	// anything reachable. Standalone tree equivalence remains the third,
	// independent term of isMergedCleanup's chain, as in the reference.
	return true
}

func candidateHasCombinedPatchEquivalenceCleanup(repoRoot, sha, candidate string) bool {
	common := git(repoRoot, "merge-base", candidate, sha)
	if common == "" {
		return false
	}
	branchPatch := directPatchID(repoRoot, common, sha)
	candidatePatch := directPatchID(repoRoot, common, candidate)
	return branchPatch != "" && branchPatch == candidatePatch
}

func directPatchID(repoRoot, from, to string) string {
	// Use an OS pipe rather than a shell so repository paths and Git output are
	// never re-parsed by a command interpreter.
	cmd := newGitCommand(repoRoot, "diff", "--no-ext-diff", "--binary", from, to)
	patch, err := cmd.StdoutPipe()
	if err != nil {
		return ""
	}
	if err := cmd.Start(); err != nil {
		return ""
	}
	patchBytes, readErr := io.ReadAll(patch)
	waitErr := cmd.Wait()
	if readErr != nil || waitErr != nil {
		return ""
	}
	pid := execPatchID(patchBytes)
	return pid
}

func candidateHasCombinedTreeEquivalenceCleanup(repoRoot, sha, candidate string) bool {
	common := git(repoRoot, "merge-base", candidate, sha)
	if common == "" {
		return false
	}
	paths := gitRawBytes(repoRoot, "diff", "--no-ext-diff", "--name-only", "-z", "--no-renames", common, sha)
	if len(paths) == 0 {
		return false
	}
	count := 0
	for _, path := range bytes.Split(paths, []byte{0}) {
		if len(path) == 0 {
			continue
		}
		count++
		branchEntry, ok := treeEntry(repoRoot, sha, string(path))
		if !ok {
			return false
		}
		candidateEntry, ok := treeEntry(repoRoot, candidate, string(path))
		if !ok || branchEntry != candidateEntry {
			return false
		}
	}
	return count > 0
}

func isMergedCleanup(repoRoot, sha, base string) bool {
	candidates := resolveBaseCandidatesCleanup(repoRoot, base)
	for _, candidate := range candidates {
		if candidateHasMergedTipCleanup(repoRoot, sha, candidate) {
			return true
		}
	}
	for _, candidate := range candidates {
		if candidateHasPatchEquivalenceCleanup(repoRoot, sha, candidate) ||
			candidateHasCombinedPatchEquivalenceCleanup(repoRoot, sha, candidate) ||
			candidateHasCombinedTreeEquivalenceCleanup(repoRoot, sha, candidate) {
			return true
		}
	}
	return false
}

func treeEntry(repoRoot, revision, path string) (string, bool) {
	cmd := newGitCommand(repoRoot, "ls-tree", revision, "--", path)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	entry := bytes.TrimSpace(out)
	if len(entry) == 0 {
		return "", false
	}
	return string(entry), true
}

func branchHeldByWorktree(records []worktreeRecord, branch, except string) bool {
	for _, record := range records {
		if record.branch == branch && filepath.Clean(record.path) != filepath.Clean(except) {
			return true
		}
	}
	return false
}

func removeWorktreeCleanup(repoRoot string, record worktreeRecord, displayName string, cfg cleanupConfig, all []worktreeRecord, out io.Writer) error {
	if err := assertDeletable("worktree removal", record.branch, cfg.protected); err != nil {
		return err
	}
	if branchHeldByWorktree(all, record.branch, record.path) {
		return fmt.Errorf("worktree-cleanup: refusing worktree removal for branch %q: another worktree still holds it", record.branch)
	}
	if cfg.dryRun {
		formatCleanupStatus(out, "would remove %s", displayName)
		return nil
	}
	if err := runGit(repoRoot, "worktree", "remove", record.path); err != nil {
		return fmt.Errorf("worktree-cleanup: worktree remove %q failed: %w", record.path, err)
	}
	return nil
}

func removeLocalBranchCleanup(repoRoot string, record worktreeRecord, cfg cleanupConfig, out io.Writer) error {
	if err := assertDeletable("local branch deletion", record.branch, cfg.protected); err != nil {
		return err
	}
	if branchHeldByWorktree(worktreeRecords(repoRoot), record.branch, "") {
		return fmt.Errorf("worktree-cleanup: refusing local deletion for branch %q: a worktree still holds it", record.branch)
	}
	if cfg.dryRun {
		return nil
	}
	if err := assertDeletable("local branch deletion", record.branch, cfg.protected); err == nil {
		if err := runGit(repoRoot, "branch", "-d", record.branch); err == nil {
			return nil
		}
	} else {
		return err
	}
	if err := assertDeletable("forced local branch deletion", record.branch, cfg.protected); err != nil {
		return err
	}
	if err := runGit(repoRoot, "branch", "-D", record.branch); err != nil {
		return fmt.Errorf("worktree-cleanup: local branch deletion %q failed: %w", record.branch, err)
	}
	return nil
}

func remoteForBranch(repoRoot, branch string) string {
	if remote := git(repoRoot, "config", "--get", "branch."+branch+".remote"); remote != "" && remote != "." {
		return remote
	}
	if git(repoRoot, "config", "--get", "remote.origin.url") != "" {
		return "origin"
	}
	return ""
}

func requirePrimaryCleanupCheckout(root string) error {
	if root == "" {
		return fmt.Errorf("worktree-cleanup: current repository root is unavailable")
	}
	gitDir := git(root, "rev-parse", "--absolute-git-dir")
	common := gitCommon(root)
	if gitDir == "" || common == "" || RealPath(gitDir) != common {
		return fmt.Errorf("worktree-cleanup: run cleanup from the main repository worktree; linked worktrees are not allowed")
	}
	return nil
}

func remoteRefExists(repoRoot, remote, branch string) (bool, error) {
	cmd := newGitCommand(repoRoot, "ls-remote", "--heads", remote, branch)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

type localBranchRecord struct {
	branch string
	sha    string
}

// localBranchRecordsCleanup enumerates every local branch explicitly. The NUL
// delimiters keep branch names and object ids independent of line splitting.
func localBranchRecordsCleanup(repoRoot string) []localBranchRecord {
	raw := gitRawBytes(repoRoot, "for-each-ref", "--format=%(refname:short)%00%(objectname)%00", "refs/heads/")
	fields := bytes.Split(raw, []byte{0})
	var records []localBranchRecord
	for i := 0; i+1 < len(fields); i += 2 {
		branch := strings.TrimSpace(string(fields[i]))
		sha := strings.TrimSpace(string(fields[i+1]))
		if branch == "" || sha == "" {
			continue
		}
		records = append(records, localBranchRecord{branch: branch, sha: sha})
	}
	return records
}

// ghPRRecord is the minimal shape read from `gh pr list --json mergeCommit`.
type ghPRRecord struct {
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

// staleBranchEvidenceCleanup decides whether a local branch with no worktree is
// provably merged. Missing, open, or malformed pull-request evidence preserves
// the branch rather than guessing.
func staleBranchEvidenceCleanup(repoRoot string, record localBranchRecord, base string) (bool, string) {
	if isMergedCleanup(repoRoot, record.sha, base) {
		return true, "merged"
	}
	cmd := exec.Command("gh", "pr", "list", "--head", record.branch, "--state", "all", "--json", "mergeCommit")
	output, err := cmd.Output()
	if err != nil {
		return false, "unmerged"
	}
	var prs []ghPRRecord
	if err := json.Unmarshal(output, &prs); err != nil {
		return false, "unmerged"
	}
	// Scan EVERY pull request for this head. Returning on the first entry that
	// lacks a merge commit misses the case of a branch closed unmerged once and
	// later reused for a pull request that did merge.
	for _, pr := range prs {
		if pr.MergeCommit == nil || pr.MergeCommit.OID == "" {
			continue
		}
		if runGit(repoRoot, "merge-base", "--is-ancestor", pr.MergeCommit.OID, base) == nil {
			return true, "merged"
		}
	}
	// No path-presence fallback. Proving that a same-named file exists on the
	// base is not proof that this branch's work landed: two commits can touch
	// the same path with entirely different content and never meet.
	//
	// Making that check sound would mean comparing blob content — which is
	// exactly candidateHasCombinedTreeEquivalenceCleanup, already run above by
	// isMergedCleanup. So the fallback was either unsound or redundant. A
	// branch whose merge cannot be proven is preserved, per the contract:
	// refuse rather than guess.
	return false, "unmerged"
}

func removeRemoteBranchCleanup(repoRoot string, record worktreeRecord, remote string, cfg cleanupConfig, out io.Writer) error {
	if err := assertDeletable("remote branch deletion", record.branch, cfg.protected); err != nil {
		return err
	}
	if branchHeldByWorktree(worktreeRecords(repoRoot), record.branch, "") {
		return fmt.Errorf("worktree-cleanup: refusing remote deletion for branch %q: a worktree still holds it", record.branch)
	}
	if remote == "" {
		formatCleanupStatus(out, "skipped %s (no remote)", record.branch)
		return nil
	}
	if cfg.dryRun {
		formatCleanupStatus(out, "would delete remote %s/%s", remote, record.branch)
		return nil
	}
	if err := assertDeletable("remote branch deletion", record.branch, cfg.protected); err != nil {
		return err
	}
	// A remote branch that is already gone is the goal state, not a failure.
	// Someone deleting it through the host's UI is ordinary, and treating that
	// as an error used to abort the entire cleanup pass.
	if exists, err := remoteRefExists(repoRoot, remote, record.branch); err == nil && !exists {
		formatCleanupStatus(out, "remote %s/%s already absent", remote, record.branch)
		return nil
	}
	if err := runGit(repoRoot, "push", remote, "--delete", record.branch); err != nil {
		return fmt.Errorf("worktree-cleanup: remote deletion %s/%s failed: %w", remote, record.branch, err)
	}
	if err := assertDeletable("remote deletion verification", record.branch, cfg.protected); err != nil {
		return err
	}
	exists, err := remoteRefExists(repoRoot, remote, record.branch)
	if err != nil {
		return fmt.Errorf("worktree-cleanup: remote verification %s/%s failed: %w", remote, record.branch, err)
	}
	if exists {
		return fmt.Errorf("worktree-cleanup: remote verification found surviving ref %s/%s", remote, record.branch)
	}
	formatCleanupStatus(out, "verified remote %s/%s absent", remote, record.branch)
	return nil
}

func cleanupOnePass(repoRoot, superRoot string, cfg cleanupConfig, out io.Writer) error {
	prefix := filepath.Clean(cleanupPath(superRoot, cfg.worktreesDir)) + string(filepath.Separator)
	records := worktreeRecords(repoRoot)
	base := cfg.baseBranch
	if base == "" {
		base = git(repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	}
	var failures []error
	for _, record := range records {
		recordPath := filepath.Clean(record.path)
		if !strings.HasPrefix(recordPath+string(filepath.Separator), prefix) {
			continue
		}
		name := strings.TrimPrefix(recordPath, prefix)
		if record.branch == "" {
			formatCleanupStatus(out, "skipped %s (detached)", name)
			continue
		}
		if strings.TrimSpace(git(record.path, "status", "--porcelain")) != "" {
			formatCleanupStatus(out, "skipped %s (dirty)", name)
			continue
		}
		if !isMergedCleanup(repoRoot, record.sha, base) {
			formatCleanupStatus(out, "skipped %s (unmerged)", name)
			continue
		}
		remote := remoteForBranch(repoRoot, record.branch)
		// One candidate's failure must never abandon the rest of the pass.
		// spec.md: "it MUST not stop after the first candidate". Returning here
		// left every remaining eligible worktree unattempted and unreported —
		// no output line, no status, nothing to tell the caller they were
		// skipped. Record the failure, say so, and move on.
		if err := removeWorktreeCleanup(repoRoot, record, name, cfg, records, out); err != nil {
			formatCleanupStatus(out, "failed %s (%v)", name, err)
			failures = append(failures, err)
			continue
		}
		if cfg.dryRun {
			continue
		}
		if err := removeLocalBranchCleanup(repoRoot, record, cfg, out); err != nil {
			formatCleanupStatus(out, "failed %s (%v)", name, err)
			failures = append(failures, err)
			continue
		}
		if err := removeRemoteBranchCleanup(repoRoot, record, remote, cfg, out); err != nil {
			formatCleanupStatus(out, "failed %s (%v)", name, err)
			failures = append(failures, err)
			continue
		}
		formatCleanupStatus(out, "removed %s", name)
	}
	// A local branch can outlive its linked worktree (for example after a
	// manually removed worktree). Reconcile those branches in the same pass,
	// but only after the linked candidates have been handled.
	held := worktreeRecords(repoRoot)
	for _, local := range localBranchRecordsCleanup(repoRoot) {
		// The base itself is never a stale feature candidate, even when the
		// configured protected set is incomplete.
		if local.branch == base {
			continue
		}
		if isProtectedBranch(cfg.protected, local.branch) || branchHeldByWorktree(held, local.branch, "") {
			continue
		}
		merged, reason := staleBranchEvidenceCleanup(repoRoot, local, base)
		if !merged {
			formatCleanupStatus(out, "skipped %s (%s)", local.branch, reason)
			continue
		}
		if cfg.dryRun {
			formatCleanupStatus(out, "would remove %s", local.branch)
			continue
		}
		record := worktreeRecord{branch: local.branch, sha: local.sha}
		remote := remoteForBranch(repoRoot, local.branch)
		if err := removeLocalBranchCleanup(repoRoot, record, cfg, out); err != nil {
			formatCleanupStatus(out, "failed %s (%v)", local.branch, err)
			failures = append(failures, err)
			continue
		}
		if err := removeRemoteBranchCleanup(repoRoot, record, remote, cfg, out); err != nil {
			formatCleanupStatus(out, "failed %s (%v)", local.branch, err)
			failures = append(failures, err)
			continue
		}
		formatCleanupStatus(out, "removed %s", local.branch)
	}
	if len(failures) > 0 {
		msgs := make([]string, 0, len(failures))
		for _, err := range failures {
			msgs = append(msgs, err.Error())
		}
		return fmt.Errorf("%s", strings.Join(msgs, "; "))
	}
	return nil
}

// syncBaseCleanup is intentionally separate from candidate deletion. It is
// called only after every pass completes successfully, making base sync the
// final operation in the post-merge sequence.
func syncBaseCleanup(repoRoot string, cfg cleanupConfig, out io.Writer) error {
	if cfg.dryRun {
		return nil
	}
	base := cfg.baseBranch
	if base == "" {
		base = git(repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	}
	if base == "" {
		return fmt.Errorf("worktree-cleanup: base branch is unavailable; refusing final sync")
	}
	current := git(repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if current != base {
		return fmt.Errorf("worktree-cleanup: base branch %q is not checked out in the primary worktree; refusing final sync", base)
	}
	remote := remoteForBranch(repoRoot, base)
	if remote == "" {
		formatCleanupStatus(out, "skipped base sync %s (no remote)", base)
		return nil
	}
	exists, err := remoteRefExists(repoRoot, remote, base)
	if err != nil {
		return fmt.Errorf("worktree-cleanup: base remote verification %s/%s failed: %w", remote, base, err)
	}
	if !exists {
		formatCleanupStatus(out, "skipped base sync %s (remote ref absent)", base)
		return nil
	}
	if err := runGit(repoRoot, "pull", "--ff-only", remote, base); err != nil {
		return fmt.Errorf("worktree-cleanup: final base sync %s/%s failed: %w", remote, base, err)
	}
	formatCleanupStatus(out, "synced base %s/%s", remote, base)
	return nil
}

// cleanupModulePaths mirrors the cleanup script's submodule enumeration. It
// intentionally does not reuse the gate's stricter moduleRecords proof: cleanup
// must scan initialized submodules with custom gitmodule names too (for example
// name=web at path=apps/web), while the gate's ownership proof correctly uses a
// different `.git/modules/<registration>` invariant.
func cleanupModulePaths(superRoot string, scopes []string) []string {
	raw := strings.TrimRight(gitRaw(superRoot, "submodule", "status"), "\n")
	var paths []string
	for _, line := range strings.Split(raw, "\n") {
		if line == "" || line[0] == '-' {
			continue
		}
		fields := strings.Fields(line[1:])
		if len(fields) < 2 {
			continue
		}
		rel := fields[1]
		module := RealPath(filepath.Join(superRoot, rel))
		if len(scopes) > 0 && !containsString(scopes, rel) && !containsString(scopes, module) {
			continue
		}
		paths = append(paths, module)
	}
	return paths
}

// enumerateCleanupPasses mirrors the Bash topology contract, but returns a
// slice so every module is necessarily visited by a Go range loop.
func enumerateCleanupPasses(superRoot string, topology string, scopes []string) []repoPass {
	if topology == "standalone" || topology == "monorepo-apps" {
		return []repoPass{{root: superRoot}}
	}
	modules := cleanupModulePaths(superRoot, scopes)
	if len(modules) == 0 {
		return []repoPass{{root: superRoot}}
	}
	var passes []repoPass
	for _, module := range modules {
		rel, err := filepath.Rel(superRoot, module)
		if err != nil {
			continue
		}
		passes = append(passes, repoPass{root: module, modulePath: rel})
	}
	return passes
}

func runCleanup(superRoot string, cfg cleanupConfig, out, errOut io.Writer) int {
	if err := requirePrimaryCleanupCheckout(superRoot); err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if superRoot != "" {
		superRoot = RealPath(superRoot)
	}
	if superRoot == "" {
		fmt.Fprintln(errOut, "worktree-cleanup: repository root could not be resolved; no destructive action taken")
		return 2
	}
	passes := enumerateCleanupPasses(superRoot, cfg.topology, cfg.scopes)
	if len(passes) == 0 {
		return 0
	}
	var failures []string
	for _, pass := range passes {
		if err := cleanupOnePass(pass.root, superRoot, cfg, out); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) == 0 && !cfg.dryRun {
		for _, pass := range passes {
			if err := syncBaseCleanup(pass.root, cfg, out); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(errOut, failure)
		}
		return 2
	}
	return 0
}

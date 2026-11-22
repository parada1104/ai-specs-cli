package sync

// agentprune.go — T7 (S3 "borrar siempre lo gestionado"): when an agent leaves
// [agents].enabled, sync removes what it generated for that agent and the lock
// records that licensed it. No new mechanism: the instruction/skills link is
// proven by the exact shape ai-specs writes, command copies by [managed.*],
// MCP servers by [mcp."<rel>"], and hook wiring/shims by the existing
// planRuntimeHookPrune (a retired agent's runtimes simply have no live hook).
// Anything without a record is kept and, when it looks generated, noted.

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
)

// retiredAgents lists the known agents [agents].enabled no longer names, in
// agentPlatformMatrix order. An absent or empty list declares no agent at all —
// such a run writes nothing for any agent and says so — so it retires nothing:
// clearing the list is not evidence that every agent's files are leftovers.
func retiredAgents(root string) ([]string, error) {
	enabled, rc := readEnabledAgents(filepath.Join(root, "ai-specs", "ai-specs.toml"), io.Discard)
	if rc != 0 {
		return nil, fmt.Errorf("cannot read [agents].enabled")
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	live := materializeStringSet(enabled)
	var retired []string
	for _, agent := range slices.Sorted(maps.Keys(agentPlatformMatrix)) {
		if !live[agent] {
			retired = append(retired, agent)
		}
	}
	return retired, nil
}

// retiredLink is a symlink ai-specs writes for an agent. want is the EXACT path
// it must still resolve to: the instruction link's target `AGENTS.md`, a subrepo
// target's mirrored `ai-specs/skills`, or the root target's
// `<CLI home>/cache/projects/<project key>/resolved-skills` — the same path sync
// itself links to, so a link repointed at another install's cache is kept.
type retiredLink struct {
	root, path, rel, want string
	info                  os.FileInfo
}

// retiredCommand is a command copy recorded in [managed.*] as a recipe command.
type retiredCommand struct {
	path, rel string
	info      os.FileInfo
	hash      string
}

// retiredMCP is one retired agent's recorded MCP target.
type retiredMCP struct {
	agent, key, target, rel string
	owned                   []string
	blockSHA                string
	prunable                bool
	// done marks a record whose servers were actually pruned, so a render that
	// fails mid-apply never drops a record it did not rewrite.
	done bool
}

type agentLeftoverPlan struct {
	root, home         string
	tomlPath, lockPath string
	links              []retiredLink
	commands           []retiredCommand
	mcp                []retiredMCP
	kept               []string
	// dropLinks holds recorded links that are no longer the link ai-specs
	// wrote: dropping a record only removes deletion authority.
	dropLinks []string
	// legacyLinks holds one notice per retired agent naming the links that
	// carry no record (created before 0.26): they are never auto-deleted.
	legacyLinks map[string][]string
	// unrecorded holds the per-agent "not recorded as ai-specs-managed"
	// notices; planned holds the agents this plan still has something to prune
	// for. The notice belongs to the retirement in progress, so it is only
	// printed while that agent is still losing artifacts — otherwise the same
	// line would repeat on every later sync.
	unrecorded map[string][]string
	planned    map[string]bool
	// dropManaged holds records whose artifact is already gone: dropping a
	// record only removes deletion authority, so a later handwritten file at
	// that path is never deleted.
	dropManaged map[string]bool
}

// planRetiredAgentLeftovers decides, before the first delete, everything the
// retired agents leave behind. It returns the retired agent names (the hook
// plan prunes their runtimes) and the apply step.
//
// Every agent [agents].enabled no longer names is retired for the hook plan: its
// managed wiring and shims are recognisable by their own ai-specs markers and
// [hooks] records, so a retired runtime loses them even when the lock holds no
// link record for it. The link, command and MCP leftovers need the stricter
// evidence check below, because an unrecorded user link must stay silent.
func planRetiredAgentLeftovers(root, home string, paths []string) ([]string, func(io.Writer, io.Writer) error, error) {
	noop := func(io.Writer, io.Writer) error { return nil }
	retired, err := retiredAgents(root)
	if err != nil {
		return nil, noop, err
	}
	if len(retired) == 0 {
		return nil, noop, nil
	}
	lockPath := filepath.Join(root, bundledLockRel)
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return nil, noop, err
	}
	p := &agentLeftoverPlan{
		root:        root,
		home:        home,
		tomlPath:    filepath.Join(root, "ai-specs", "ai-specs.toml"),
		lockPath:    lockPath,
		unrecorded:  map[string][]string{},
		planned:     map[string]bool{},
		dropManaged: map[string]bool{},
		legacyLinks: map[string][]string{},
	}
	for _, agent := range retired {
		if !p.agentEvidence(lk, paths, agent) {
			// A project that never enabled this agent has nothing of ours to
			// retire: its user-made links stay, silently.
			continue
		}
		for _, target := range paths {
			if err := p.addAgent(lk, target, agent); err != nil {
				return nil, noop, err
			}
		}
	}
	return retired, p.apply, nil
}

// agentEvidence reports whether ai-specs demonstrably managed agent: a link,
// command copy, MCP target or hook shim sync recorded for it, or — for a lock
// written before 0.26 recorded links — the managed marker still present in its
// runtime hook file. Without any of these, an agent missing from
// [agents].enabled is indistinguishable from one the project never enabled, so
// its user links stay silent (S4).
func (p *agentLeftoverPlan) agentEvidence(lk *lock.Lock, paths []string, agent string) bool {
	under := func(rel, dir string) bool {
		return rel == dir || strings.HasPrefix(rel, dir+"/")
	}
	for _, target := range paths {
		for _, field := range []string{"instructions_path", "skills_dir"} {
			rel, _ := platformGet(agent, field)
			if rel == "" {
				continue
			}
			if _, ok := lk.Links[p.rel(filepath.Join(target, rel))]; ok {
				return true
			}
		}
		if rel, _ := platformGet(agent, "mcp_config_path"); rel != "" {
			if _, ok := lk.MCP[p.rel(filepath.Join(target, rel))]; ok {
				return true
			}
		}
		if p.hookMarker(target, agent) {
			return true
		}
		for _, field := range []string{"commands_dir", "runtime_hooks_target"} {
			rel, _ := platformGet(agent, field)
			if rel == "" {
				continue
			}
			dir := p.rel(filepath.Join(target, rel))
			for key, rec := range lk.Managed {
				if under(key, dir) && rec["kind"] == RecipeCommandKind {
					return true
				}
			}
			for key := range lk.Hooks {
				if under(key, dir) {
					return true
				}
			}
		}
	}
	return false
}

// hookMarker reports whether the agent's runtime hook file still carries a
// managed hook id: proof ai-specs wired this agent before the lock recorded
// links. It is best-effort, so an unreadable or unusual file is no evidence.
func (p *agentLeftoverPlan) hookMarker(target, agent string) bool {
	rel, _ := platformGet(agent, "runtime_hooks_target")
	if rel == "" {
		return false
	}
	path := filepath.Join(target, rel)
	if exists, err := pruneHookFile(p.root, target, path); err != nil || !exists {
		return false
	}
	content, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(content), hooksManagedPrefix)
}

// addAgent plans one retired agent's artifacts under one target.
func (p *agentLeftoverPlan) addAgent(lk *lock.Lock, target, agent string) error {
	for _, field := range []string{"instructions_path", "skills_dir"} {
		rel, _ := platformGet(agent, field)
		if rel == "" {
			continue
		}
		path := filepath.Join(target, rel)
		linkRel := p.rel(path)
		recorded := lk.Links[linkRel] == lock.LinkMarker
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if recorded {
				p.dropLinks = append(p.dropLinks, linkRel)
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// A real file or directory here is the user's: ai-specs never writes
			// one at these paths (it refuses at the skills path, T6). A stale
			// record must not license deleting a link later written there.
			if recorded {
				p.dropLinks = append(p.dropLinks, linkRel)
			}
			continue
		}
		link := retiredLink{root: p.root, path: path, rel: linkRel, info: info}
		if field == "instructions_path" {
			link.want = filepath.Join(target, "AGENTS.md")
		} else if target == p.root {
			link.want = projectcache.ResolvedSkillsDir(p.root, p.home)
		} else {
			link.want = filepath.Join(target, "ai-specs", "skills")
		}
		// Only a link the lock records as sync's own may be retired (S4); one
		// that predates 0.26 is kept and named once.
		if lk.Links[link.rel] != lock.LinkMarker {
			p.legacyLinks[agent] = append(p.legacyLinks[agent], link.rel)
			continue
		}
		if !link.ours() {
			// Repointed or replaced since sync wrote it: the user's now.
			p.dropLinks = append(p.dropLinks, link.rel)
			continue
		}
		if err := projectWriteGuard(p.root, path, false); err != nil {
			return err
		}
		p.links = append(p.links, link)
		p.planned[agent] = true
	}
	if err := p.addCommands(lk, target, agent); err != nil {
		return err
	}
	return p.addMCP(lk, target, agent)
}

// addCommands plans the recorded command copies of one retired agent and notes
// the unrecorded ones. A copy with no ownership record is not provably
// ai-specs' (T2 decision), so it stays.
func (p *agentLeftoverPlan) addCommands(lk *lock.Lock, target, agent string) error {
	rel, _ := platformGet(agent, "commands_dir")
	if rel == "" {
		return nil
	}
	dir := filepath.Join(target, rel)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	unrecorded := 0
	present := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || filepath.Ext(name) != ".md" {
			continue
		}
		path := filepath.Join(dir, name)
		rec := lk.Managed[p.rel(path)]
		if rec["kind"] != RecipeCommandKind {
			unrecorded++
			continue
		}
		present[p.rel(path)] = true
		// pruneHookFile holds the inside-root ancestor rule for agent paths (T6b)
		// and refuses a symlink or directory leaf, so only a real copy is planned.
		exists, err := pruneHookFile(p.root, target, path)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		hash, err := lock.Sha256OfFile(path)
		if err != nil {
			return err
		}
		// Ownership is the recorded hash of the last generated copy: an edited
		// copy is the user's now, so the file and its record both stay (T2, L11).
		if recorded, _ := rec["sha256"].(string); recorded == "" || hash != recorded {
			continue
		}
		p.commands = append(p.commands, retiredCommand{path: path, rel: p.rel(path), info: info, hash: hash})
		p.planned[agent] = true
	}
	// A record whose copy is already gone loses its deletion authority now: it
	// proves nothing about a file later written at that path.
	for relPath, rec := range lk.Managed {
		if rec["kind"] != RecipeCommandKind || present[relPath] {
			continue
		}
		if filepath.Dir(filepath.Join(p.root, filepath.FromSlash(relPath))) == dir {
			p.dropManaged[relPath] = true
		}
	}
	if unrecorded > 0 {
		p.unrecorded[agent] = append(p.unrecorded[agent], fmt.Sprintf("%s: %d command copy(ies) not recorded as ai-specs-managed (kept; delete them if ai-specs did not write them)", p.rel(dir), unrecorded))
	}
	return nil
}

// addMCP plans the recorded servers of one retired agent's MCP target. Without
// a lock record nothing proves ownership, so the target is left alone; a
// target the renderer refuses (a hand-edited managed block) is kept with the
// renderer's reason instead of failing the run.
func (p *agentLeftoverPlan) addMCP(lk *lock.Lock, target, agent string) error {
	rel, _ := platformGet(agent, "mcp_config_path")
	key, _ := platformGet(agent, "mcp_key")
	if rel == "" || key == "" {
		return nil
	}
	path := filepath.Join(target, rel)
	projRel := p.rel(path)
	record, recorded := lk.MCP[projRel]
	if !recorded {
		return nil
	}
	item := retiredMCP{agent: agent, key: key, target: path, rel: projRel, blockSHA: lk.MCPBlock[projRel]}
	for _, name := range slices.Sorted(maps.Keys(record)) {
		item.owned = append(item.owned, name)
	}
	if !mcpIsFile(path) {
		// The target is already gone: only its record is left behind, and the
		// apply step's render is skipped for it.
		item.prunable = true
		p.mcp = append(p.mcp, item)
		return nil
	}
	if err := agentWriteGuard(p.root, path, true); err != nil {
		return err
	}
	var reason bytes.Buffer
	if rc := RenderMCPFile(p.tomlPath, agent, path, key, RenderMCPOptions{
		PruneOnly: true, DryRun: true, Owned: item.owned, Recorded: true, BlockSHA: item.blockSHA,
	}, io.Discard, &reason); rc != 0 {
		p.kept = append(p.kept, fmt.Sprintf("%s: %s", projRel, strings.TrimSpace(reason.String())))
		return nil
	}
	item.prunable = true
	p.mcp = append(p.mcp, item)
	p.planned[agent] = true
	return nil
}

func (p *agentLeftoverPlan) rel(path string) string {
	rel, err := filepath.Rel(p.root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// ours reports whether the link at path is still the one ai-specs wrote.
func (l retiredLink) ours() bool {
	info, err := os.Lstat(l.path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	text, err := os.Readlink(l.path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(text) {
		text = filepath.Join(filepath.Dir(l.path), text)
	}
	return projectcache.ResolvePath(text) == projectcache.ResolvePath(l.want)
}

// renderMCP rewrites one planned MCP target, or — with dryRun — re-validates it
// against the current file, so a target edited since the plan is refused before
// the first delete instead of halfway through the apply.
func (p *agentLeftoverPlan) renderMCP(item retiredMCP, dryRun bool, out, errW io.Writer) error {
	opts := RenderMCPOptions{PruneOnly: true, DryRun: dryRun, Owned: item.owned, Recorded: true, BlockSHA: item.blockSHA}
	if !dryRun {
		if rc := RenderMCPFile(p.tomlPath, item.agent, item.target, item.key, opts, out, errW); rc != 0 {
			return fmt.Errorf("cannot prune %s MCP servers", item.rel)
		}
		return nil
	}
	var reason bytes.Buffer
	if rc := RenderMCPFile(p.tomlPath, item.agent, item.target, item.key, opts, io.Discard, &reason); rc != 0 {
		return fmt.Errorf("cannot prune %s MCP servers: %s", item.rel, strings.TrimSpace(reason.String()))
	}
	return nil
}

func (p *agentLeftoverPlan) apply(out, errW io.Writer) error {
	for _, kept := range p.kept {
		fmt.Fprintf(errW, "  ! %s\n", kept)
	}
	for _, agent := range slices.Sorted(maps.Keys(p.unrecorded)) {
		if !p.planned[agent] {
			continue
		}
		for _, kept := range p.unrecorded[agent] {
			fmt.Fprintf(errW, "  ! %s\n", kept)
		}
	}
	// Links without a record predate 0.26: sync never owned them, so they stay
	// and one notice names them.
	for _, agent := range slices.Sorted(maps.Keys(p.legacyLinks)) {
		fmt.Fprintf(errW, "  ! %s: retired agent link(s) not recorded as ai-specs-managed (kept; delete them if ai-specs did not write them)\n", strings.Join(p.legacyLinks[agent], ", "))
	}
	// Every MCP target is re-validated first: its render may still refuse (a
	// target edited since the plan) and a refusal must precede every delete.
	for _, item := range p.mcp {
		if item.prunable && mcpIsFile(item.target) {
			if err := p.renderMCP(item, true, io.Discard, errW); err != nil {
				return err
			}
		}
	}
	// Preflight every planned unlink before the first mutation, so an unlink
	// that cannot succeed (a read-only parent directory) refuses here rather
	// than after other artifacts are already gone (L53).
	if err := p.preflightDeletes(); err != nil {
		return err
	}
	// Deletes run before the MCP rewrites of the user's config files: a refused
	// or failed unlink must not leave those files already rewritten.
	applyErr := p.deleteArtifacts(out)
	if applyErr == nil {
		applyErr = p.rewriteMCP(out, errW)
	}
	// A delete that still fails mid-apply despite the preflight (a race) removed
	// the artifacts before it, so the lock records exactly what is gone instead
	// of keeping stale authority over paths that no longer hold a managed file.
	lockErr := applyLockRecord(p.lockPath, func(lk *lock.Lock) {
		for rel := range p.dropManaged {
			delete(lk.Managed, rel)
		}
		for _, rel := range p.dropLinks {
			lock.SetLink(lk, rel, "")
		}
		for _, link := range p.links {
			lock.SetLink(lk, link.rel, "")
		}
		for _, item := range p.mcp {
			if !item.done {
				continue
			}
			delete(lk.MCP, item.rel)
			delete(lk.MCPBlock, item.rel)
		}
	})
	if applyErr != nil {
		return applyErr
	}
	return lockErr
}

// preflightDeletes proves, read-only, that every planned unlink can succeed: the
// entry is still the planned one and its parent directory is writable and
// searchable by this process, which is what unlink(2) requires.
func (p *agentLeftoverPlan) preflightDeletes() error {
	for _, c := range p.commands {
		info, err := os.Lstat(c.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !os.SameFile(info, c.info) {
			continue // Replaced since the plan: kept.
		}
		hash, err := lock.Sha256OfFile(c.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if hash != c.hash {
			continue // Edited since the plan: kept.
		}
		if err := unlinkPreflight(p.root, c.path); err != nil {
			return err
		}
	}
	for _, link := range p.links {
		info, err := os.Lstat(link.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !os.SameFile(info, link.info) || !link.ours() {
			continue // Relinked or replaced since the plan: kept.
		}
		if err := unlinkPreflight(p.root, link.path); err != nil {
			return err
		}
	}
	return nil
}

// deleteArtifacts unlinks the planned command copies and links, re-checking each
// one so a copy edited or a link repointed since the plan stays.
func (p *agentLeftoverPlan) deleteArtifacts(out io.Writer) error {
	for _, c := range p.commands {
		exists, err := pruneHookFile(p.root, p.root, c.path)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		info, err := os.Lstat(c.path)
		if err != nil {
			return err
		}
		hash, err := lock.Sha256OfFile(c.path)
		if err != nil {
			return err
		}
		if !os.SameFile(info, c.info) || hash != c.hash {
			continue // Edited since preflight: keep the copy and its record.
		}
		if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		p.dropManaged[c.rel] = true
		fmt.Fprintf(out, "  ✓ removed retired agent command %s\n", c.rel)
	}
	for _, link := range p.links {
		if !link.ours() {
			continue
		}
		if err := projectWriteGuard(p.root, link.path, false); err != nil {
			return err
		}
		if err := os.Remove(link.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintf(out, "  ✓ removed retired agent link %s\n", link.rel)
	}
	return nil
}

// rewriteMCP prunes the recorded servers out of each planned target and marks the
// record droppable only once its render succeeded.
func (p *agentLeftoverPlan) rewriteMCP(out, errW io.Writer) error {
	for i := range p.mcp {
		item := &p.mcp[i]
		if !item.prunable {
			continue
		}
		if mcpIsFile(item.target) {
			if err := p.renderMCP(*item, false, out, errW); err != nil {
				return err
			}
		}
		item.done = true
		fmt.Fprintf(out, "  ✓ removed retired agent %s MCP record %s\n", item.agent, item.rel)
	}
	return nil
}

// accessW_OK and accessX_OK are the access(2) modes from <unistd.h>: the Go
// syscall package exports Access but not these constants.
const (
	accessW_OK = 0x2
	accessX_OK = 0x1
)

// unlinkPreflight fails when the entry's parent directory cannot be written and
// searched by this process. access(2) only inspects permissions, so nothing is
// mutated.
func unlinkPreflight(root, path string) error {
	if err := syscall.Access(filepath.Dir(path), accessW_OK|accessX_OK); err != nil {
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		return fmt.Errorf("cannot unlink %s: %v", filepath.ToSlash(rel), err)
	}
	return nil
}

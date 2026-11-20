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

// retiredLink is a symlink ai-specs writes for an agent. want is the file it
// must still point at (the instruction link); "" accepts any symlink at the
// path (the skills link, where T6 guarantees ai-specs is the only writer that
// puts a symlink).
type retiredLink struct {
	root, path, rel, want string
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
}

type agentLeftoverPlan struct {
	root, tomlPath, lockPath string
	links                    []retiredLink
	commands                 []retiredCommand
	mcp                      []retiredMCP
	kept                     []string
}

// planRetiredAgentLeftovers decides, before the first delete, everything the
// retired agents leave behind. It returns the retired agent names (the hook
// plan prunes their runtimes) and the apply step.
func planRetiredAgentLeftovers(root string, paths []string) ([]string, func(io.Writer, io.Writer) error, error) {
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
		root:     root,
		tomlPath: filepath.Join(root, "ai-specs", "ai-specs.toml"),
		lockPath: lockPath,
	}
	for _, target := range paths {
		for _, agent := range retired {
			if err := p.addAgent(lk, target, agent); err != nil {
				return nil, noop, err
			}
		}
	}
	return retired, p.apply, nil
}

// addAgent plans one retired agent's artifacts under one target.
func (p *agentLeftoverPlan) addAgent(lk *lock.Lock, target, agent string) error {
	for _, field := range []string{"instructions_path", "skills_dir"} {
		rel, _ := platformGet(agent, field)
		if rel == "" {
			continue
		}
		path := filepath.Join(target, rel)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// A real file or directory here is the user's: ai-specs never writes
			// one at these paths (it refuses at the skills path, T6).
			continue
		}
		link := retiredLink{root: p.root, path: path, rel: p.rel(path)}
		if field == "instructions_path" {
			link.want = filepath.Join(target, "AGENTS.md")
		}
		if !link.ours() {
			continue
		}
		if err := projectWriteGuard(p.root, path, false); err != nil {
			return err
		}
		p.links = append(p.links, link)
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
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	unrecorded := 0
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
		p.commands = append(p.commands, retiredCommand{path: path, rel: p.rel(path), info: info, hash: hash})
	}
	if unrecorded > 0 {
		p.kept = append(p.kept, fmt.Sprintf("%s: %d command copy(ies) not recorded as ai-specs-managed (kept; delete them if ai-specs did not write them)", p.rel(dir), unrecorded))
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
	if l.want == "" {
		return true
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

func (p *agentLeftoverPlan) apply(out, errW io.Writer) error {
	for _, kept := range p.kept {
		fmt.Fprintf(errW, "  ! %s\n", kept)
	}
	// MCP first: its render can still refuse (a target edited since preflight),
	// and a refusal must precede the other deletes of this plan where possible.
	for _, item := range p.mcp {
		if item.prunable && mcpIsFile(item.target) {
			if rc := RenderMCPFile(p.tomlPath, item.agent, item.target, item.key, RenderMCPOptions{
				PruneOnly: true, Owned: item.owned, Recorded: true, BlockSHA: item.blockSHA,
			}, out, errW); rc != 0 {
				return fmt.Errorf("cannot prune %s MCP servers", item.rel)
			}
		}
		if item.prunable {
			fmt.Fprintf(out, "  ✓ removed retired agent %s MCP record %s\n", item.agent, item.rel)
		}
	}
	dropManaged := map[string]bool{}
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
		dropManaged[c.rel] = true
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
	return applyLockRecord(p.lockPath, func(lk *lock.Lock) {
		for rel := range dropManaged {
			delete(lk.Managed, rel)
		}
		for _, item := range p.mcp {
			if !item.prunable {
				continue
			}
			delete(lk.MCP, item.rel)
			delete(lk.MCPBlock, item.rel)
		}
	})
}

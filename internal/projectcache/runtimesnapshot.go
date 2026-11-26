package projectcache

// Per-project runtime snapshot (cli-served-recipes T1, S2).
//
// `ai-specs sync` writes the recipe runtime config it resolved into the project
// cache, next to meta.toml and sync.lock. The CLI verbs a hook invokes later
// (`worktree-gate --hook`, `worktree-cleanup`) read THIS file and never
// ai-specs.toml: the manifest may have changed since the last sync, and the
// snapshot is the config that sync actually applied — the lock's companion.
//
// The file lives inside the cache root EnsureCache owns and is written
// atomically, behind the same containment guard.

import (
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// RuntimeSnapshotFile is the snapshot's leaf name inside the project cache.
const RuntimeSnapshotFile = "runtime.toml"

// runtimeSnapshotTable is the worktree-flow table; runtimeSnapshotTrackerTable
// carries the tracker ledger's legacy gate_mode hint (T6).
const (
	runtimeSnapshotTable        = "worktree_gate"
	runtimeSnapshotTrackerTable = "tracker_gate"
)

// RuntimeSnapshot is the resolved recipe runtime config of the last sync. Every
// worktree-flow field is the exact value the retired launcher was stamped with;
// TrackerGateMode is the hint tracker-card-gate.sh was stamped with.
type RuntimeSnapshot struct {
	GateMode          string
	GateScope         string
	RepoTopology      string
	Protected         string
	WorktreesDir      string
	IntegrationBranch string
	TrackerGateMode   string
}

// RuntimeSnapshotPath is the snapshot location for a project. It does not
// create the cache directory.
func RuntimeSnapshotPath(projectRoot, cliHome string) string {
	return filepath.Join(CacheRoot(projectRoot, cliHome), RuntimeSnapshotFile)
}

// WriteRuntimeSnapshot persists the snapshot atomically. It mirrors the cache
// writes EnsureCache performs: the cache root is created first and the
// destination is refused when it would resolve outside the CLI home.
func WriteRuntimeSnapshot(projectRoot, cliHome string, snap RuntimeSnapshot) (string, error) {
	cache, err := EnsureCache(projectRoot, cliHome)
	if err != nil {
		return "", err
	}
	path := filepath.Join(cache, RuntimeSnapshotFile)
	home := AISpecsHome(cliHome)
	if err := ContainWrite(home, path, true, true, false); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(cache, ".runtime-*.toml")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(renderRuntimeSnapshot(snap)); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}

// renderRuntimeSnapshot emits the TOML document in declaration order. Values
// are TOML strings, so the serializer owns the quoting.
func renderRuntimeSnapshot(snap RuntimeSnapshot) string {
	var sb strings.Builder
	sb.WriteString("# ai-specs runtime snapshot — written by `ai-specs sync`; do not edit.\n")
	sb.WriteString("# The CLI verbs read this file instead of ai-specs.toml.\n\n")
	sb.WriteString("[" + runtimeSnapshotTable + "]\n")
	for _, field := range []struct{ key, value string }{
		{"gate_mode", snap.GateMode},
		{"gate_scope", snap.GateScope},
		{"repo_topology", snap.RepoTopology},
		{"protected", snap.Protected},
		{"worktrees_dir", snap.WorktreesDir},
		{"integration_branch", snap.IntegrationBranch},
	} {
		quoted, err := toml.TOMLValue(field.value)
		if err != nil {
			quoted = `""`
		}
		sb.WriteString(field.key + " = " + quoted + "\n")
	}
	sb.WriteString("\n[" + runtimeSnapshotTrackerTable + "]\n")
	quoted, err := toml.TOMLValue(snap.TrackerGateMode)
	if err != nil {
		quoted = `""`
	}
	sb.WriteString("gate_mode = " + quoted + "\n")
	return sb.String()
}

// ReadRuntimeSnapshot loads the snapshot. found is false when the project has
// no snapshot yet (the fail-open S6 path) and when the file is unparseable or
// incomplete; error is reserved for a read failure that is not "absent".
func ReadRuntimeSnapshot(projectRoot, cliHome string) (RuntimeSnapshot, bool, error) {
	data, err := os.ReadFile(RuntimeSnapshotPath(projectRoot, cliHome))
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeSnapshot{}, false, nil
		}
		return RuntimeSnapshot{}, false, err
	}
	root, err := toml.Parse(data)
	if err != nil {
		return RuntimeSnapshot{}, false, nil
	}
	tableVal, ok := root.Get(runtimeSnapshotTable)
	if !ok {
		return RuntimeSnapshot{}, false, nil
	}
	table, ok := tableVal.(*toml.Table)
	if !ok {
		return RuntimeSnapshot{}, false, nil
	}
	snap := RuntimeSnapshot{}
	fields := []struct {
		key  string
		dest *string
	}{
		{"gate_mode", &snap.GateMode},
		{"gate_scope", &snap.GateScope},
		{"repo_topology", &snap.RepoTopology},
		{"protected", &snap.Protected},
		{"worktrees_dir", &snap.WorktreesDir},
		{"integration_branch", &snap.IntegrationBranch},
	}
	for _, field := range fields {
		value, present := table.Get(field.key)
		text, isString := value.(string)
		if !present || !isString {
			return RuntimeSnapshot{}, false, nil
		}
		*field.dest = text
	}
	// The tracker table is optional: a snapshot written before T6 has no hint,
	// and the ledger then resolves from env and the manifest as it always did.
	if trackerVal, ok := root.Get(runtimeSnapshotTrackerTable); ok {
		if trackerTable, ok := trackerVal.(*toml.Table); ok {
			if text, isString := trackerTable.Get("gate_mode"); isString {
				snap.TrackerGateMode, _ = text.(string)
			}
		}
	}
	return snap, true, nil
}

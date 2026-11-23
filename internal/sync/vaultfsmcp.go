package sync

// CLI-served vault filesystem MCP (cli-served-recipes T4, S5).
//
// The vault-canonical-store recipe used to materialize a wrapper script,
// ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh, whose only job was
// to validate CANONICAL_VAULT_PATH and exec the pinned npx. `ai-specs mcp
// vault-fs` does that now, so:
//
//   - the template target is no longer materialized;
//   - the declared [[provides.mcp]] preset renders with command `ai-specs` and
//     args `mcp vault-fs`, keeping the env (and timeout) it already carried: the
//     verb reads the vault path from that env at run time;
//   - an older sync's copy is retired, including a user-edited one (one
//     notice), because the project serves this from the CLI.
//
// The pin itself (filesystem 2025.7.1 plus zod 3) lives with the verb: the MCP
// entry is static per project, so sync resolves nothing at run time.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/lock"
)

const (
	vaultMCPRecipeID   = "vault-canonical-store"
	vaultMCPServerID   = "vault-canonical"
	vaultMCPScriptPath = "ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh"
)

// vaultMCPTemplateServesCLI reports the recipe template target the CLI writes
// itself instead of the governed-template actuator.
func vaultMCPTemplateServesCLI(recipeID, target string) bool {
	return recipeID == vaultMCPRecipeID && posixAsPosix(target) == vaultMCPScriptPath
}

// vaultMCPPresetConfig renders the recipe's declared preset as the CLI command.
// Everything else stays the preset's own: the entry carries the env variable
// name, never a resolved path.
func vaultMCPPresetConfig(recipeID, presetID string, config *mcpObj) *mcpObj {
	if recipeID != vaultMCPRecipeID || presetID != vaultMCPServerID {
		return config
	}
	out := mcpShallowCopy(config)
	out.set("command", "ai-specs")
	out.set("args", []any{"mcp", "vault-fs"})
	return out
}

// retireVaultMCPScript removes the wrapper an older sync materialized. It is
// lock-driven like the recipe README retirement: only a copy a record proves
// ai-specs wrote is removed — with one notice when the bytes are no longer the
// recorded ones — and an unrecorded copy is left to the user.
func retireVaultMCPScript(root, lockPath string, out io.Writer) error {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	record, recorded := lk.Managed[vaultMCPScriptPath]
	if !recorded {
		return nil
	}
	path := filepath.Join(root, filepath.FromSlash(vaultMCPScriptPath))
	if err := projectWriteGuard(root, path, false); err != nil {
		return err
	}
	recordedSHA, _ := record["sha256"].(string)
	current, hashErr := lock.Sha256OfFile(path)
	if recordedSHA == "" || hashErr == nil && current != recordedSHA {
		fmt.Fprintf(out, "  ✓ removed user-modified vault-fs-mcp.sh %s (ai-specs now serves the vault MCP from the CLI)\n",
			vaultMCPScriptPath)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := removeEmptiedParents(filepath.Join(root, "ai-specs", "recipes", vaultMCPRecipeID), path); err != nil {
		return err
	}
	delete(lk.Managed, vaultMCPScriptPath)
	return lock.WriteLock(lockPath, lk)
}

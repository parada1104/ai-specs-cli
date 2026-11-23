package cli

// `ai-specs mcp vault-fs` — the CLI serves the canonical vault MCP server
// (cli-served-recipes T4, S5).
//
// It replaces ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh, whose
// only job was to validate CANONICAL_VAULT_PATH and exec one pinned npx. Every
// refusal keeps the script's message and exit code, and nothing is written to
// stdout before the server starts, so the MCP stdio handshake is unaffected.
//
// The filesystem package stays pinned to 2025.7.1: 2025.7.29+ replaces argv
// directories with MCP client roots whenever the client advertises them, which
// would re-scope or widen the vault. zod is pinned to 3 as a second -p package
// because 2025.7.1 does not depend on it directly: the SDK resolves zod 4, and
// the server's own zod-to-json-schema@^3 then emits empty tool schemas.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	vaultFSFilesystemPin = "@modelcontextprotocol/server-filesystem@2025.7.1"
	vaultFSZodPin        = "zod@3"
	vaultFSServerBinary  = "mcp-server-filesystem"
)

// runVaultFSMCP validates the vault scope, then runs the pinned server with the
// caller's streams. The verb takes no arguments: like the retired script, it
// ignores any it receives.
func runVaultFSMCP(stdin io.Reader, stdout, stderr io.Writer) int {
	root, ok := vaultFSScope(os.Getenv("CANONICAL_VAULT_PATH"), os.Getenv("HOME"), stderr)
	if !ok {
		return 1
	}
	cmd := exec.Command("npx", "-y",
		"-p", vaultFSFilesystemPin,
		"-p", vaultFSZodPin,
		vaultFSServerBinary, root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err == nil {
		return 0
	}
	// The server's own status is the verb's status, so an agent host observes
	// the same exit code a direct exec produced.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		return exitErr.ExitCode()
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		fmt.Fprint(stderr, "vault-fs-mcp: npx not found on PATH; install Node.js so the vault MCP server can start.\n")
		return 127
	}
	fmt.Fprintf(stderr, "vault-fs-mcp: %v\n", err)
	return 1
}

// vaultFSScope reproduces the retired script's checks in order: unset or empty,
// a leading ~/ expanded against HOME, an unexpanded shell reference, then the
// directory test. A false return has already written the script's message; the
// caller exits 1 for every refusal, exactly like the script.
func vaultFSScope(raw, home string, stderr io.Writer) (string, bool) {
	root := raw
	if root == "" {
		fmt.Fprint(stderr, "vault-fs-mcp: CANONICAL_VAULT_PATH is unset or empty.\n")
		fmt.Fprint(stderr, "Set it in .envrc to an absolute vault scope path, then restart the agent.\n")
		return "", false
	}
	if strings.HasPrefix(root, "~") {
		root = home + root[1:]
	}
	if strings.Contains(root, "$") {
		fmt.Fprintf(stderr, "vault-fs-mcp: CANONICAL_VAULT_PATH still contains '$' (%s).\n", root)
		fmt.Fprint(stderr, "Use an absolute path, or set it in .envrc so the shell expands nested vars.\n")
		return "", false
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "vault-fs-mcp: not a directory: %s\n", root)
		return "", false
	}
	return root, true
}

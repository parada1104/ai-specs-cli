package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ledgerStoreAPIs are the only ways to reach the ledger store files. Nothing
// outside the store package, the gate that calls it, the plan port, and this
// entrypoint may name them (S14): the on-disk layout is an implementation detail,
// so a consumer that opens those paths itself either breaks on the next layout or
// resurrects rows the CLI already migrated. The last three carry no path in their
// name, so a caller that builds a location in memory and only loads or saves it
// used to be invisible to the path literals below.
var ledgerStoreAPIs = []string{
	"TrackerLocation(", "StoreAt(",
	"TrackerStorePath(", "StorePath(",
	"LoadStore(", "SaveStore(",
	"LedgerAt(", ".Load()", ".Save(",
}

// ledgerStoreImports are the import paths that reach the store API itself. A new
// caller can skip every string above by importing the package and calling
// ledger.LedgerAt(root).Load(), so an importer outside the owned prefixes is a
// finding on its own; the quoted form keeps the base import apart from a
// subpackage, and both appear once per offender.
var ledgerStoreImports = []string{
	`"ai-specs.dev/worktree-gate/ledger"`,
	`"ai-specs.dev/worktree-gate/ledger/`,
}

// ledgerStorePathLiterals are the on-disk names a script would need to open the
// store directly. The witness file shares the pre-split directory and is not a
// store, so only the store names are forbidden.
var ledgerStorePathLiterals = []string{"ledger/state.json", "ledgers/tracker.json", "ledgers/"}

// entrypointOwnedPrefixes are the packages allowed to name the store surface:
// the store itself, the gate that runs inside the CLI process, the CLI, and the
// plan port. internal/planledger is the CLI's port to the plan section: the one
// place outside the gate that may reach the ledger, and it is reachable only from
// internal/cli. gate/shared is no exception to the contract but a neighbour of it:
// it is compiled into the gate that the CLI runs in-process and imports the ledger
// package for the witness schema and the witness path, never for a store call.
var entrypointOwnedPrefixes = []string{
	filepath.Join("catalog", "recipes", "worktree-flow", "gate", "ledger") + string(filepath.Separator),
	filepath.Join("catalog", "recipes", "worktree-flow", "gate", "runner") + string(filepath.Separator),
	filepath.Join("catalog", "recipes", "worktree-flow", "gate", "shared") + string(filepath.Separator),
	filepath.Join("internal", "cli") + string(filepath.Separator),
	filepath.Join("internal", "planledger") + string(filepath.Separator),
}

// TestOnlyTheEntrypointReachesTheLedgerStore is the S14 contract: the ledger store
// paths are constructed and called only from the entrypoint and the store it
// calls. It fails when a package adds its own writer, which is how the retired
// split-brain readers happened.
func TestOnlyTheEntrypointReachesTheLedgerStore(t *testing.T) {
	root := repositoryRoot(t)
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "node_modules", "testdata", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !isShippedSource(rel) {
			return nil
		}
		for _, owned := range entrypointOwnedPrefixes {
			if strings.HasPrefix(rel, owned) {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(data)
		for _, imp := range ledgerStoreImports {
			if strings.Contains(body, imp) {
				offenders = append(offenders, rel+" imports "+strings.Trim(imp, `"`))
			}
		}
		for _, api := range ledgerStoreAPIs {
			if strings.Contains(body, api) {
				offenders = append(offenders, rel+" names "+api)
			}
		}
		for _, literal := range ledgerStorePathLiterals {
			if strings.Contains(body, literal) {
				offenders = append(offenders, rel+" opens "+literal)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(offenders) > 0 {
		t.Fatalf("the ledger store is reached only through the CLI; these read or write it directly:\n%s",
			strings.Join(offenders, "\n"))
	}
}

// isShippedSource reports whether rel is shipped production code: Go source
// outside tests, and the shell and Python that recipes install. Documentation,
// fixtures and the test suites may name the paths to test them.
func isShippedSource(rel string) bool {
	switch filepath.Ext(rel) {
	case ".go":
		return !strings.HasSuffix(rel, "_test.go")
	case ".sh", ".py":
		return strings.HasPrefix(rel, "catalog"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, "bin"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, "scripts"+string(filepath.Separator))
	}
	return false
}

// repositoryRoot resolves this package's checkout root without depending on the
// process working directory.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the test source path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

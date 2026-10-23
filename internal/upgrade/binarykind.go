package upgrade

// The binary install kind (GO-20 U1b-2a).
//
// A binary install keeps the same home as a legacy one — the checkout is still
// the data home and the trust anchor, and it still carries the committed
// digests — but the ENTRY is a real compiled file in $INSTALL_BIN instead of a
// symlink into that checkout. Upgrading therefore has two moves: refresh the
// checkout with git when it is present (that is where VERSION and the trust
// root come from), then download the platform asset, verify it against the
// committed digest BEFORE anything is replaced, and swap it in atomically.
//
// No committed digest means no install: the current binary is left untouched
// and the message names the asset. That is what makes the download path safe to
// ship before the release channel for this binary exists.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// releaseBaseURL is where this version's platform assets live. The env override
// exists for tests and mirrors.
func releaseBaseURL(env Env, version string) string {
	if env.ReleaseBase != "" {
		return env.ReleaseBase
	}
	const repo = "https://github.com/parada1104/ai-specs-cli"
	if version == "" {
		return repo + "/releases/latest/download"
	}
	return fmt.Sprintf("%s/releases/download/v%s", repo, version)
}

// binaryUpgrade refreshes the checkout (best effort, git is optional for a data
// home) and installs the verified asset for this platform.
func binaryUpgrade(inst Install, o Options, env Env, stdout, stderr io.Writer) int {
	git := env.git()
	current := readVersion(inst.Home)

	if _, err := os.Stat(filepath.Join(inst.Home, ".git")); err == nil {
		if _, err := git(inst.Home, "rev-parse", "--verify", "origin/main"); err == nil {
			if _, err := git(inst.Home, "merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
				return abort(stderr, "Local branch has diverged from origin/main. Resolve manually or re-run install.sh.", ExitPreflight)
			}
		}
		if !o.DryRun {
			if code := runStep(o, git, inst.Home, stdout, stderr, "fetching origin/main", "fetch", "origin", "main"); code != 0 {
				return abort(stderr, "Failed to fetch from origin. Check your network connection.", ExitFetch)
			}
			if _, err := git(inst.Home, "merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
				return abort(stderr, "Local branch has diverged from origin/main after fetch. Resolve manually or re-run install.sh.", ExitPreflight)
			}
			head, _ := git(inst.Home, "rev-parse", "HEAD")
			origin, _ := git(inst.Home, "rev-parse", "origin/main")
			if strings.TrimSpace(head) != strings.TrimSpace(origin) {
				target := gitShowVersion(git, inst.Home, "origin/main")
				label := "fast-forwarding to origin/main"
				if current != "" && target != "" {
					label = fmt.Sprintf("fast-forwarding %s -> %s", current, target)
				}
				if code := runStep(o, git, inst.Home, stdout, stderr, label, "merge", "--ff-only", "origin/main"); code != 0 {
					return abort(stderr, "Fast-forward merge failed. The local branch may have diverged. Resolve manually or re-run install.sh.", ExitFetch)
				}
			}
		}
	}

	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	root, err := LoadTrustRoot(filepath.Join(inst.Home, "bin", "SHA256SUMS"))
	if err != nil {
		return abort(stderr, fmt.Sprintf("could not read the committed digests: %v", err), ExitBroken)
	}
	if _, ok := root[asset]; !ok {
		return abort(stderr, fmt.Sprintf("no committed digest for %s, so the download cannot be verified. Re-run install.sh to repair.", asset), ExitBroken)
	}

	targetVersion := readVersion(inst.Home)
	if o.DryRun {
		fmt.Fprintln(stdout, "Dry-run: no changes will be made.")
		fmt.Fprintf(stdout, "Current version: %s\n", current)
		if targetVersion != "" {
			fmt.Fprintf(stdout, "Asset:           %s\n", asset)
		}
		return ExitOK
	}

	if _, err := Acquire(AcquireOptions{
		TrustRoot: root,
		BaseURL:   releaseBaseURL(env, targetVersion),
		Asset:     asset,
		Target:    inst.LocalBin,
	}); err != nil {
		return abort(stderr, fmt.Sprintf("could not install %s: %v", asset, err), ExitFetch)
	}

	if current == targetVersion {
		fmt.Fprintf(stdout, "Already up to date (version %s).\n", current)
		return ExitOK
	}
	fmt.Fprintf(stdout, "Upgraded: %s -> %s\n", current, targetVersion)
	printReleaseReport(inst.Home, current, targetVersion, stdout)
	return ExitOK
}

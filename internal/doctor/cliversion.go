package doctor

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/toml"
)

// Native port of lib/_internal/cli_version.py, restricted to the surface
// doctor uses: read_installed_version, read_lock_meta and
// evaluate_cli_version. The sync-side helpers (stamp_lock_meta, check-sync)
// belong to the sync port.

var versionRe = regexp.MustCompile("^([0-9]+)\\.([0-9]+)\\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\\+([0-9A-Za-z.-]+))?$")

// toolPolicy mirrors cli_version.ToolPolicy.
type toolPolicy struct {
	kind    string // "exact" | "min"
	version string
}

// readInstalledVersion mirrors cli_version.read_installed_version.
func readInstalledVersion(cliHome string) string {
	path := filepath.Join(cliHome, "VERSION")
	if !isFile(path) {
		return "unknown"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "unknown"
	}
	return text
}

// versionTuple mirrors _parse_version_tuple; ok=false is the None branch.
type versionTuple struct {
	major      int
	minor      int
	patch      int
	prerelease *string
}

func parseVersionTuple(version string) (versionTuple, bool) {
	if version == "unknown" {
		return versionTuple{}, false
	}
	m := versionRe.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return versionTuple{}, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return versionTuple{}, false
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return versionTuple{}, false
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return versionTuple{}, false
	}
	tuple := versionTuple{major: major, minor: minor, patch: patch}
	if m[4] != "" {
		pre := m[4]
		tuple.prerelease = &pre
	}
	return tuple, true
}

func preEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// compareVersions mirrors cli_version.compare_versions: -1, 0 or 1.
func compareVersions(left, right string) int {
	leftT, leftOK := parseVersionTuple(left)
	rightT, rightOK := parseVersionTuple(right)
	if !leftOK || !rightOK {
		if left == right {
			return 0
		}
		if !leftOK {
			return -1
		}
		if !rightOK {
			return 1
		}
		return 0
	}
	if leftT.major != rightT.major {
		if leftT.major < rightT.major {
			return -1
		}
		return 1
	}
	if leftT.minor != rightT.minor {
		if leftT.minor < rightT.minor {
			return -1
		}
		return 1
	}
	if leftT.patch != rightT.patch {
		if leftT.patch < rightT.patch {
			return -1
		}
		return 1
	}
	switch {
	case preEqual(leftT.prerelease, rightT.prerelease):
		return 0
	case leftT.prerelease == nil:
		return 1
	case rightT.prerelease == nil:
		return -1
	case *leftT.prerelease < *rightT.prerelease:
		return -1
	default:
		return 1
	}
}

// policyMatches mirrors `effective_policy != "exact"`: the raw policy value
// must be that same string.
func policyMatches(value any, want string) bool {
	s, isStr := value.(string)
	return isStr && s == want
}

// parseToolPolicy mirrors cli_version.parse_tool_policy; the second result is
// the diagnostic message ("" when the policy is valid or absent).
func parseToolPolicy(manifest *toml.Table) (*toolPolicy, string) {
	if manifest == nil {
		return nil, ""
	}
	raw, present := manifest.Get("tool")
	if !present || !truthy(raw) {
		return nil, ""
	}
	tool, isTable := raw.(*toml.Table)
	if !isTable {
		return nil, "invalid [tool] table"
	}
	version, hasVersion := tool.Get("version")
	minVersion, hasMin := tool.Get("min_version")
	policy, hasPolicy := tool.Get("policy")
	_ = hasPolicy

	if hasVersion && hasMin {
		return nil, "cannot set both [tool].version and [tool].min_version"
	}
	if hasVersion {
		s, isStr := version.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			return nil, "[tool].version must be a non-empty string"
		}
		if policy != nil && !policyMatches(policy, "exact") {
			return nil, "unknown [tool].policy: " + pythonRepr(policy)
		}
		return &toolPolicy{kind: "exact", version: strings.TrimSpace(s)}, ""
	}
	if hasMin {
		s, isStr := minVersion.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			return nil, "[tool].min_version must be a non-empty string"
		}
		if policy != nil && !policyMatches(policy, "min") {
			return nil, "unknown [tool].policy: " + pythonRepr(policy)
		}
		return &toolPolicy{kind: "min", version: strings.TrimSpace(s)}, ""
	}
	if policy != nil {
		return nil, "[tool].policy requires [tool].version or [tool].min_version"
	}
	return nil, ""
}

// checkPolicy mirrors cli_version.check_policy.
func checkPolicy(installed string, policy *toolPolicy) (bool, string) {
	if installed == "unknown" {
		return false, "installed CLI version is unknown"
	}
	if policy.kind == "exact" {
		if compareVersions(installed, policy.version) == 0 {
			return true, ""
		}
		return false, "installed CLI " + installed + " does not match pinned " + policy.version
	}
	if compareVersions(installed, policy.version) >= 0 {
		return true, ""
	}
	return false, "installed CLI " + installed + " is below minimum " + policy.version
}

// readLockMeta mirrors cli_version.read_lock_meta through the ported lock
// loader, which keeps exactly cli_version/synced_at as stripped non-blank
// strings. A malformed lock file raises in Python (uncaught: doctor aborts with
// a traceback); the port degrades to "no metadata" instead of fabricating one.
func readLockMeta(lockPath string) map[string]string {
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return map[string]string{}
	}
	return lk.Meta
}

// evaluateCLIVersion mirrors cli_version.evaluate_cli_version, returning the
// doctor severity, the check name and the message.
func evaluateCLIVersion(installed string, manifest *toml.Table, lockMeta map[string]string) (Severity, string, string) {
	policy, errText := parseToolPolicy(manifest)
	if errText != "" {
		return ERROR, "cli-version", errText
	}
	lastSynced := lockMeta["cli_version"]
	if policy != nil {
		ok, reason := checkPolicy(installed, policy)
		if !ok {
			return ERROR, "cli-version",
				reason + " (run ai-specs upgrade or adjust [tool] in ai-specs.toml)"
		}
		if lastSynced != "" && lastSynced != installed {
			return WARN, "cli-version",
				"installed " + installed + ", pinned " + policy.version + ", last sync " + lastSynced
		}
		if lastSynced == installed {
			return OK, "cli-version",
				"installed " + installed + ", pinned " + policy.version + ", last sync " + lastSynced
		}
		return OK, "cli-version",
			"installed " + installed + ", pinned " + policy.version + ", last sync unknown"
	}
	if lastSynced == "" {
		return INFO, "cli-version",
			"installed " + installed + ", no [tool] pin, last sync unknown — run ai-specs sync"
	}
	if lastSynced == installed {
		return OK, "cli-version",
			"installed " + installed + ", no [tool] pin, last sync " + lastSynced
	}
	return WARN, "cli-version",
		"installed " + installed + ", no [tool] pin, last sync " + lastSynced + " — run ai-specs sync"
}

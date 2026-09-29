// Package cliversion ports the pure version-resolution and policy core of
// lib/_internal/cli_version.py (card [Go 12]).
//
// It covers reading the installed version, semver parsing/comparison, the
// [tool] policy parse, the policy check, lock-meta extraction, and the
// doctor-facing evaluator. The thin `check-sync`/`stamp-meta` CLI shims stay
// in Python for now (stamp-meta is a lock write owned by the lock surface);
// this package is the Go authority the strangler bridge will call.
//
// Python semantics are preserved where they are observable: the "unknown"
// sentinel, the None-tuple comparison fallbacks, the exact policy error
// strings, and Python repr() of a rejected [tool].policy value.
package cliversion

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// versionRE mirrors VERSION_RE: MAJOR.MINOR.PATCH with optional -prerelease
// and +build metadata.
var versionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

// Version is the parsed semver tuple (Python's (major, minor, patch, pre)).
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // "" means Python None
}

// ReadInstalledVersion mirrors read_installed_version: the trimmed VERSION
// file, or "unknown" when absent or blank.
func ReadInstalledVersion(cliHome string) string {
	data, err := os.ReadFile(filepath.Join(cliHome, "VERSION"))
	if err != nil {
		return "unknown"
	}
	text := stripSpace(string(data))
	if text == "" {
		return "unknown"
	}
	return text
}

// ParseVersionTuple mirrors _parse_version_tuple: ok=false for "unknown" or
// any string that is not a full semver match.
func ParseVersionTuple(version string) (Version, bool) {
	if version == "unknown" {
		return Version{}, false
	}
	m := versionRE.FindStringSubmatch(stripSpace(version))
	if m == nil {
		return Version{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: major, Minor: minor, Patch: patch, Pre: m[4]}, true
}

// compareCore returns -1, 0 or 1 for the (major, minor, patch) tuple order.
func (v Version) compareCore(o Version) int {
	switch {
	case v.Major != o.Major:
		if v.Major < o.Major {
			return -1
		}
		return 1
	case v.Minor != o.Minor:
		if v.Minor < o.Minor {
			return -1
		}
		return 1
	case v.Patch != o.Patch:
		if v.Patch < o.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// CompareVersions mirrors compare_versions: -1 if left < right, 0 if equal,
// 1 if left > right. Unparseable operands fall back to the Python
// string-equality rule.
func CompareVersions(left, right string) int {
	lt, lok := ParseVersionTuple(left)
	rt, rok := ParseVersionTuple(right)
	if !lok || !rok {
		if left == right {
			return 0
		}
		if !lok {
			return -1
		}
		if !rok {
			return 1
		}
		return 0
	}

	if c := lt.compareCore(rt); c != 0 {
		return c
	}
	// Build metadata is ignored (Python groups()[:4]).
	if lt.Pre == rt.Pre {
		return 0
	}
	if lt.Pre == "" { // a release outranks its prerelease
		return 1
	}
	if rt.Pre == "" {
		return -1
	}
	if lt.Pre < rt.Pre {
		return -1
	}
	return 1
}

// ToolPolicy mirrors ToolPolicy: kind "exact" or "min".
type ToolPolicy struct {
	Kind    string
	Version string
}

// ParseToolPolicy mirrors parse_tool_policy. The second return is the exact
// Python error message, "" when there is none.
func ParseToolPolicy(manifest *toml.Table) (*ToolPolicy, string) {
	if manifest == nil {
		return nil, ""
	}
	toolVal, ok := manifest.Get("tool")
	if !ok {
		return nil, ""
	}
	// Python checks falsiness (empty table/dict counts as absent) BEFORE the
	// type check, so an empty [tool] table means "no policy".
	if !pyTruthy(toolVal) {
		return nil, ""
	}
	tool, isTable := toolVal.(*toml.Table)
	if !isTable {
		return nil, "invalid [tool] table"
	}

	version, hasVersion := tool.Get("version")
	minVersion, hasMin := tool.Get("min_version")
	policy, hasPolicy := tool.Get("policy")

	if hasVersion && hasMin {
		return nil, "cannot set both [tool].version and [tool].min_version"
	}

	policyRejects := func(want string) (bool, string) {
		if !hasPolicy || policy == nil {
			return false, ""
		}
		if s, ok := policy.(string); ok && s == want {
			return false, ""
		}
		return true, "unknown [tool].policy: " + pyRepr(policy)
	}

	if hasVersion {
		s, ok := version.(string)
		if !ok || stripSpace(s) == "" {
			return nil, "[tool].version must be a non-empty string"
		}
		if bad, msg := policyRejects("exact"); bad {
			return nil, msg
		}
		return &ToolPolicy{Kind: "exact", Version: stripSpace(s)}, ""
	}

	if hasMin {
		s, ok := minVersion.(string)
		if !ok || stripSpace(s) == "" {
			return nil, "[tool].min_version must be a non-empty string"
		}
		if bad, msg := policyRejects("min"); bad {
			return nil, msg
		}
		return &ToolPolicy{Kind: "min", Version: stripSpace(s)}, ""
	}

	if hasPolicy && policy != nil {
		return nil, "[tool].policy requires [tool].version or [tool].min_version"
	}
	return nil, ""
}

// CheckPolicy mirrors check_policy.
func CheckPolicy(installed string, policy *ToolPolicy) (bool, string) {
	if installed == "unknown" {
		return false, "installed CLI version is unknown"
	}
	if policy.Kind == "exact" {
		if CompareVersions(installed, policy.Version) == 0 {
			return true, ""
		}
		return false, fmt.Sprintf("installed CLI %s does not match pinned %s", installed, policy.Version)
	}
	if CompareVersions(installed, policy.Version) >= 0 {
		return true, ""
	}
	return false, fmt.Sprintf("installed CLI %s is below minimum %s", installed, policy.Version)
}

// ReadLockMeta mirrors read_lock_meta: the trimmed cli_version/synced_at
// string values under [meta], or an empty map.
func ReadLockMeta(lockPath string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return out
	}
	root, err := toml.Parse(data)
	if err != nil {
		return out
	}
	meta, ok := root.Table("meta")
	if !ok {
		return out
	}
	for _, key := range []string{"cli_version", "synced_at"} {
		if s, ok := meta.String(key); ok && stripSpace(s) != "" {
			out[key] = stripSpace(s)
		}
	}
	return out
}

// EvaluateCLIVersion mirrors evaluate_cli_version, returning (severity,
// check-name, message) for doctor.
func EvaluateCLIVersion(installed string, manifest *toml.Table, lockMeta map[string]string) (string, string, string) {
	policy, err := ParseToolPolicy(manifest)
	if err != "" {
		return "ERROR", "cli-version", err
	}
	lastSynced := lockMeta["cli_version"]

	if policy != nil {
		ok, reason := CheckPolicy(installed, policy)
		if !ok {
			guidance := "run ai-specs upgrade or adjust [tool] in ai-specs.toml"
			return "ERROR", "cli-version", fmt.Sprintf("%s (%s)", reason, guidance)
		}
		if lastSynced != "" && lastSynced != installed {
			return "WARN", "cli-version", fmt.Sprintf("installed %s, pinned %s, last sync %s", installed, policy.Version, lastSynced)
		}
		if lastSynced == installed {
			return "OK", "cli-version", fmt.Sprintf("installed %s, pinned %s, last sync %s", installed, policy.Version, lastSynced)
		}
		return "OK", "cli-version", fmt.Sprintf("installed %s, pinned %s, last sync unknown", installed, policy.Version)
	}

	if lastSynced == "" {
		return "INFO", "cli-version", fmt.Sprintf("installed %s, no [tool] pin, last sync unknown \u2014 run ai-specs sync", installed)
	}
	if lastSynced == installed {
		return "OK", "cli-version", fmt.Sprintf("installed %s, no [tool] pin, last sync %s", installed, lastSynced)
	}
	return "WARN", "cli-version", fmt.Sprintf("installed %s, no [tool] pin, last sync %s \u2014 run ai-specs sync", installed, lastSynced)
}

// --- Python-compat helpers -------------------------------------------------

// stripSpace reproduces str.strip() for the ASCII version/policy strings.
func stripSpace(s string) string { return strings.TrimSpace(s) }

// pyTruthy reproduces Python truthiness for the TOML value shapes.
func pyTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int64:
		return t != 0
	case float64:
		return t != 0
	case []any:
		return len(t) > 0
	case []*toml.Table:
		return len(t) > 0
	case *toml.Table:
		return len(t.Keys()) > 0
	}
	return true
}

// pyRepr reproduces repr() for the value shapes a [tool].policy can hold,
// which is what the error message embeds.
func pyRepr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return pyReprString(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'r', -1, 64)
	}
	return fmt.Sprint(v)
}

// pyReprString reproduces CPython str.__repr__ quote selection for the
// common single-line cases.
func pyReprString(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case rune(quote):
			b.WriteByte('\\')
			b.WriteByte(quote)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// SortedStrings returns the keys sorted (test/debug helper for lock meta).
func SortedStrings(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

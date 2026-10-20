package deps

import (
	"fmt"
	"runtime"
	"strings"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
)

// Plan kinds, InstallPlan.kind's values.
const (
	KindBrew          = "brew"
	KindApt           = "apt"
	KindGuidance      = "guidance"
	KindGitHubRelease = "github-release"
)

// PlanRequest is resolve_install_plan's argument list: the binary plus the
// dep-side hints the caller may or may not carry (direnv's future offer has no
// schema.CliDep, hence the loose shape — the legacy signature's own).
type PlanRequest struct {
	Binary        string
	InstallURL    string
	Installer     string
	Repository    string
	ReleasePolicy string
	MinVersion    string
}

// Plan mirrors dep_install.InstallPlan: the command to run, the display line,
// and — for the provider kind — the release plan the install half consumes.
type Plan struct {
	Binary        string
	Command       []string
	Display       string
	GuidanceURL   string
	Kind          string
	Installer     string
	Repository    string
	ReleasePolicy string
	MinVersion    string
	ProviderPlan  *provider.ReleasePlan
}

// packageMap is dep_install._PACKAGE_MAP: binary -> (brew formula, apt
// package). An empty side means guidance-only for that package manager.
var packageMap = map[string][2]string{
	"gh":     {"gh", "gh"},
	"glab":   {"glab", "glab"},
	"jq":     {"jq", "jq"},
	"direnv": {"direnv", "direnv"},
	"git":    {"git", "git"},
	"bb":     {"bb-cli", ""},
}

// guidanceOnly is dep_install._GUIDANCE_ONLY: never a blind package install.
var guidanceOnly = map[string]bool{"npx": true}

// hostSystem is platform.system()'s answer in Go's own vocabulary
// ("darwin"/"linux"/"windows"). It is a variable so tests can script the matrix
// (and the differential can mirror the legacy patching).
var hostSystem = func() string { return runtime.GOOS }

// ResolvePlan is dep_install.resolve_install_plan: a constrained install plan
// for one binary, never an install itself. The branch order is the legacy
// module's and is load-bearing: the provider kind first, then brew on Darwin AND
// Linux, then apt on Linux only, then guidance.
func ResolvePlan(req PlanRequest, home string) (Plan, error) {
	if req.Installer == KindGitHubRelease {
		releasePlan, err := provider.BuildReleasePlan(&schema.CliDep{
			Binary:        req.Binary,
			InstallURL:    req.InstallURL,
			Installer:     req.Installer,
			Repository:    req.Repository,
			ReleasePolicy: req.ReleasePolicy,
			MinVersion:    req.MinVersion,
		}, installHome(home))
		if err != nil {
			return Plan{}, err
		}
		return Plan{
			Binary:        req.Binary,
			Command:       []string{},
			Display:       "GitHub Release " + releasePlan.Repository + " (" + releasePlan.ReleasePolicy + ") for " + releasePlan.Goos + "/" + releasePlan.Goarch,
			GuidanceURL:   req.InstallURL,
			Kind:          KindGitHubRelease,
			Installer:     req.Installer,
			Repository:    req.Repository,
			ReleasePolicy: req.ReleasePolicy,
			MinVersion:    req.MinVersion,
			ProviderPlan:  &releasePlan,
		}, nil
	}

	if packages, mapped := packageMap[req.Binary]; mapped && !guidanceOnly[req.Binary] {
		formula, aptPackage := packages[0], packages[1]
		system := hostSystem()
		if _, err := lookPath("brew"); err == nil && formula != "" && (system == "darwin" || system == "linux") {
			return packagePlan(req, KindBrew, []string{"brew", "install", formula}), nil
		}
		if _, err := lookPath("apt-get"); err == nil && aptPackage != "" && system == "linux" {
			return packagePlan(req, KindApt, []string{"sudo", "apt-get", "install", "-y", aptPackage}), nil
		}
	}

	return guidancePlan(req), nil
}

func packagePlan(req PlanRequest, kind string, command []string) Plan {
	return Plan{
		Binary:      req.Binary,
		Command:     command,
		Display:     strings.Join(command, " "),
		GuidanceURL: req.InstallURL,
		Kind:        kind,
	}
}

func guidancePlan(req PlanRequest) Plan {
	display := req.InstallURL
	if display == "" {
		display = "install '" + req.Binary + "' manually"
	}
	return Plan{
		Binary:      req.Binary,
		Command:     []string{},
		Display:     display,
		GuidanceURL: req.InstallURL,
		Kind:        KindGuidance,
	}
}

// Describe is dep_install.describe_install_plan: the consent preview shown
// before an install, with no side effects. Only the provider kind expands; every
// other kind's whole text is its display line.
func Describe(plan Plan) string {
	if plan.Kind != KindGitHubRelease || plan.ProviderPlan == nil {
		return plan.Display
	}
	release := plan.ProviderPlan
	suffix := ".tar.gz"
	if release.Goos == "windows" {
		suffix = ".zip"
	}
	minimum := release.MinVersion
	if minimum == "" {
		minimum = "not declared"
	}
	lines := []string{
		"  GitHub Release installation plan for '" + plan.Binary + "':",
		"    repository:      " + plan.Repository,
		"    release policy:  " + plan.ReleasePolicy + " (latest non-draft, non-prerelease tag)",
		"    target:          " + release.Goos + "/" + release.Goarch,
		"    expected asset:  jinna_<version>_" + release.Goos + "_" + release.Goarch + suffix,
		"    destination:     " + release.CacheRoot,
		"    checksum source: SHA256SUMS from the same release",
		"    minimum version: " + minimum,
		"    replacement:     an existing PATH provider is never modified; " +
			"a verified managed version is reused",
	}
	if plan.GuidanceURL != "" {
		lines = append(lines, "    manual fallback: "+plan.GuidanceURL)
	}
	return strings.Join(lines, "\n")
}

// PanelLines is config_wizard._render_dep_panel's line, one per result:
//
//	f"{status:4s}  {r.binary:12s}  {r.purpose}{detail}{hint}"
//
// The trailing hint appears only for a dep that is NOT ok and declares an
// install URL; the detail shows whenever the probe recorded one, ok or not.
func PanelLines(results []Result) []string {
	lines := make([]string, 0, len(results))
	for _, r := range results {
		status := "INFO"
		switch {
		case r.OK:
			status = "OK"
		case r.Required:
			status = "WARN"
		}
		detail := ""
		if r.Detail != "" {
			detail = " (" + r.Detail + ")"
		}
		hint := ""
		if r.InstallURL != "" && !r.OK {
			hint = " → " + r.InstallURL
		}
		lines = append(lines, fmt.Sprintf("%-4s  %-12s  %s%s%s", status, r.Binary, r.Purpose, detail, hint))
	}
	return lines
}

// MissingRequired is _dep_gate's filter: required AND not ok. These are the
// deps the install offer plans for.
func MissingRequired(results []Result) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.Required && !r.OK {
			out = append(out, r)
		}
	}
	return out
}

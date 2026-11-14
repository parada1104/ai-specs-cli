package provider

import (
	"strings"

	"ai-specs.dev/ai-specs/internal/schema"
)

// ReleasePlan mirrors provider_install.GithubReleasePlan field for field.
// CacheRoot is the managed cache directory the install writes into; the
// remaining fields are the consent text describe_install_plan renders.
type ReleasePlan struct {
	Binary        string
	Repository    string
	ReleasePolicy string
	MinVersion    string
	InstallURL    string
	Goos          string
	Goarch        string
	CacheRoot     string
}

// supportedPlatformList is provider_install.SUPPORTED_PLATFORMS in its declared
// order: the no-artifact message lists the targets in exactly this order.
var supportedPlatformList = [][2]string{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "arm64"},
	{"linux", "amd64"},
	{"windows", "amd64"},
}

// BuildReleasePlan is provider_install.build_release_plan: the allowlist gate
// (only this package's repository, only the latest-stable policy), the host's
// Go target, and the managed cache root. Its three rejections are InstallError's
// messages verbatim, because the gate surfaces them inside the legacy
// "install offer skipped: <exc>" line.
func BuildReleasePlan(dep *schema.CliDep, aiSpecsHome *string) (ReleasePlan, error) {
	if dep.Installer != "github-release" || dep.Repository != Repository {
		return ReleasePlan{}, &InstallError{
			Message: "provider dependency is not the allowlisted GitHub Release installer",
		}
	}
	if dep.ReleasePolicy != "latest-stable" {
		return ReleasePlan{}, &InstallError{
			Message: "provider release policy must be latest-stable",
		}
	}
	goos, goarch := DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		targets := make([]string, 0, len(supportedPlatformList))
		for _, target := range supportedPlatformList {
			targets = append(targets, target[0]+"/"+target[1])
		}
		guidance := ""
		if dep.InstallURL != "" {
			guidance = " see " + dep.InstallURL + " for manual installation"
		}
		return ReleasePlan{}, &InstallError{
			Message: "provider has no artifact for this platform; supported targets: " +
				strings.Join(targets, ", ") + "." + guidance,
		}
	}
	return ReleasePlan{
		Binary:        dep.Binary,
		Repository:    dep.Repository,
		ReleasePolicy: dep.ReleasePolicy,
		MinVersion:    dep.MinVersion,
		InstallURL:    dep.InstallURL,
		Goos:          goos,
		Goarch:        goarch,
		CacheRoot:     CacheRoot(aiSpecsHome),
	}, nil
}

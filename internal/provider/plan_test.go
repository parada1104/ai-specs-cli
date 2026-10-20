package provider

import (
	"errors"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/schema"
)

func providerDep() *schema.CliDep {
	return &schema.CliDep{
		Binary:        "jinna",
		Purpose:       "OpenProject provider",
		Required:      true,
		InstallURL:    "https://example.com/jinna",
		Installer:     "github-release",
		Repository:    Repository,
		ReleasePolicy: "latest-stable",
		MinVersion:    "0.4.0",
	}
}

// TestBuildReleasePlan pins provider_install.build_release_plan's accepted
// shape: the allowlisted repository, the latest-stable policy, the host's Go
// target and the managed cache root.
func TestBuildReleasePlan(t *testing.T) {
	goos, goarch := DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		t.Skipf("unsupported provider platform %q/%q", goos, goarch)
	}
	home := t.TempDir()
	plan, err := BuildReleasePlan(providerDep(), &home)
	if err != nil {
		t.Fatalf("BuildReleasePlan: %v", err)
	}
	if plan.Binary != "jinna" || plan.Repository != Repository || plan.ReleasePolicy != "latest-stable" {
		t.Errorf("plan = %+v", plan)
	}
	if plan.MinVersion != "0.4.0" || plan.InstallURL != "https://example.com/jinna" {
		t.Errorf("plan lost the request fields: %+v", plan)
	}
	if plan.Goos != goos || plan.Goarch != goarch {
		t.Errorf("target = %s/%s, want %s/%s", plan.Goos, plan.Goarch, goos, goarch)
	}
	if want := CacheRoot(&home); plan.CacheRoot != want {
		t.Errorf("CacheRoot = %q, want %q", plan.CacheRoot, want)
	}
}

// TestBuildReleasePlanRejects pins the three validation errors verbatim.
func TestBuildReleasePlanRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.CliDep)
		want   string
	}{
		{
			name:   "other installer",
			mutate: func(d *schema.CliDep) { d.Installer = "curl-bash" },
			want:   "provider dependency is not the allowlisted GitHub Release installer",
		},
		{
			name:   "other repository",
			mutate: func(d *schema.CliDep) { d.Repository = "someone/else" },
			want:   "provider dependency is not the allowlisted GitHub Release installer",
		},
		{
			name:   "other release policy",
			mutate: func(d *schema.CliDep) { d.ReleasePolicy = "latest" },
			want:   "provider release policy must be latest-stable",
		},
	}
	for _, tc := range cases {
		dep := providerDep()
		tc.mutate(dep)
		_, err := BuildReleasePlan(dep, nil)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
		var installErr *InstallError
		if !errors.As(err, &installErr) {
			t.Errorf("%s: error %v is not a *InstallError", tc.name, err)
		}
	}
}

// TestBuildReleasePlanUnsupportedPlatform pins the platform message, including
// its ordered target list and its guidance tail. It only fires on a host with
// no published artifact, so it skips where the host IS supported — the same
// shape the legacy detect_platform imposes.
func TestBuildReleasePlanUnsupportedPlatform(t *testing.T) {
	if goos, goarch := DetectPlatform(nil, nil); goos != "" && goarch != "" {
		t.Skipf("host %s/%s is a supported provider target", goos, goarch)
	}
	_, err := BuildReleasePlan(providerDep(), nil)
	if err == nil {
		t.Fatal("BuildReleasePlan on an unsupported host returned no error")
	}
	want := "provider has no artifact for this platform; supported targets: " +
		"darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/amd64." +
		" see https://example.com/jinna for manual installation"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), "..") {
		t.Errorf("guidance tail produced a doubled period: %q", err.Error())
	}
}

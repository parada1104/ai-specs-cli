package provider

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- archive and release fixtures -------------------------------------------

// stageMember describes one archive entry a fixture builds.
type stageMember struct {
	name    string
	data    []byte
	dir     bool
	symlink bool
	mode    int64
}

func buildTarGz(t *testing.T, members []stageMember) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, member := range members {
		header := &tar.Header{Name: member.name, Mode: 0o644}
		switch {
		case member.dir:
			header.Typeflag = tar.TypeDir
			header.Mode = 0o755
		case member.symlink:
			header.Typeflag = tar.TypeSymlink
			header.Linkname = "target"
			header.Mode = 0o777
		default:
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(member.data))
		}
		if member.mode != 0 {
			header.Mode = member.mode
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if !member.dir && !member.symlink {
			if _, err := tarWriter.Write(member.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildZip(t *testing.T, members []stageMember) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for _, member := range members {
		header := &zip.FileHeader{Name: member.name, Method: zip.Deflate}
		mode := os.FileMode(0o644)
		switch {
		case member.dir:
			mode = os.ModeDir | 0o755
		case member.symlink:
			mode = os.ModeSymlink | 0o777
		case member.mode != 0:
			mode = os.FileMode(member.mode)
		}
		header.SetMode(mode)
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !member.dir {
			if _, err := writer.Write(member.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func digestOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// stageFixture is the full required member set for one archive name.
func stageRequiredMembers(root, binary string) []stageMember {
	return []stageMember{
		{name: root + "/", dir: true},
		{name: root + "/" + binary, data: []byte("#!/bin/sh\necho v1.2.3\n"), mode: 0o755},
		{name: root + "/LICENSE", data: []byte("license")},
		{name: root + "/THIRD_PARTY_NOTICES.md", data: []byte("notices")},
		{name: root + "/README.md", data: []byte("readme")},
		{name: root + "/docs/", dir: true},
		{name: root + "/docs/mcp.md", data: []byte("mcp")},
		{name: root + "/docs/migration.md", data: []byte("migration")},
		{name: root + "/docs/operations.md", data: []byte("operations")},
	}
}

const stageTag = "v1.2.3"

func stageArchiveName(goos, goarch string) string {
	name, _ := ReleaseAssetName(stageTag, goos, goarch)
	return name
}

// extractStage writes the archive bytes to a temp file and extracts them.
func extractStage(t *testing.T, archiveName, goos string, raw []byte) (string, error) {
	t.Helper()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, archiveName)
	if err := os.WriteFile(archivePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return ExtractReleaseArchive(archivePath, filepath.Join(dir, "out"), archiveName, goos)
}

func requireInstallError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	var installErr *InstallError
	if !errors.As(err, &installErr) {
		t.Fatalf("expected *InstallError, got %T: %v", err, err)
	}
	if installErr.Message != want {
		t.Fatalf("error = %q, want %q", installErr.Message, want)
	}
}

// --- ReleaseAssetName --------------------------------------------------------

func TestReleaseAssetName(t *testing.T) {
	cases := []struct {
		tag, goos, goarch, want string
	}{
		{"v1.2.3", "linux", "amd64", "jinna_v1.2.3_linux_amd64.tar.gz"},
		{"v1.2.3", "darwin", "arm64", "jinna_v1.2.3_darwin_arm64.tar.gz"},
		{"v1.2.3", "windows", "amd64", "jinna_v1.2.3_windows_amd64.zip"},
	}
	for _, c := range cases {
		got, err := ReleaseAssetName(c.tag, c.goos, c.goarch)
		if err != nil {
			t.Fatalf("ReleaseAssetName(%q, %q, %q): %v", c.tag, c.goos, c.goarch, err)
		}
		if got != c.want {
			t.Errorf("ReleaseAssetName(%q, %q, %q) = %q, want %q", c.tag, c.goos, c.goarch, got, c.want)
		}
	}
	_, err := ReleaseAssetName("v1.2.3", "linux", "386")
	requireInstallError(t, err, "unsupported provider target: linux/386")
	_, err = ReleaseAssetName("v1.2.3", "", "")
	requireInstallError(t, err, "unsupported provider target: ?/?")
	_, err = ReleaseAssetName("v1.2.3", "", "amd64")
	requireInstallError(t, err, "unsupported provider target: ?/amd64")
}

// --- RepositoryFromURL -------------------------------------------------------

func TestRepositoryFromURL(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://github.com/parada1104/jinna-provider/releases/tag/v1.2.3", Repository},
		{"https://api.github.com/repos/parada1104/jinna-provider/releases/1", Repository},
		{"https://github.com/parada1104/jinna-provider", Repository},
	}
	for _, c := range cases {
		got, err := RepositoryFromURL(c.raw)
		if err != nil || got != c.want {
			t.Errorf("RepositoryFromURL(%q) = %q, %v; want %q", c.raw, got, err, c.want)
		}
	}
	_, err := RepositoryFromURL("http://github.com/parada1104/jinna-provider")
	requireInstallError(t, err, "provider release URL must use the allowlisted GitHub HTTPS host")
	_, err = RepositoryFromURL("https://example.com/parada1104/jinna-provider")
	requireInstallError(t, err, "provider release URL must use the allowlisted GitHub HTTPS host")
	_, err = RepositoryFromURL("https://github.com/")
	requireInstallError(t, err, "provider release URL has no repository")
}

// --- ParseRelease ------------------------------------------------------------

func stageReleasePayload() []byte {
	archive := stageArchiveName("linux", "amd64")
	base := "https://github.com/" + Repository + "/releases/download/" + stageTag + "/"
	payload := map[string]any{
		"html_url":   "https://github.com/" + Repository + "/releases/tag/" + stageTag,
		"tag_name":   stageTag,
		"draft":      false,
		"prerelease": false,
		"assets": []any{
			map[string]any{"name": archive, "browser_download_url": base + archive},
			map[string]any{"name": "SHA256SUMS", "browser_download_url": base + "SHA256SUMS"},
			map[string]any{"name": "RELEASE.json", "browser_download_url": base + "RELEASE.json"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestParseReleaseAccepts(t *testing.T) {
	release, err := ParseRelease(stageReleasePayload())
	if err != nil {
		t.Fatalf("ParseRelease: %v", err)
	}
	if release.Repository != Repository || release.Tag != stageTag {
		t.Errorf("release = %+v", release)
	}
	if len(release.Assets) != 3 {
		t.Errorf("assets = %+v", release.Assets)
	}
	if asset, ok := release.Assets["SHA256SUMS"]; !ok || asset.Name != "SHA256SUMS" {
		t.Errorf("SHA256SUMS asset = %+v", asset)
	}
}

func TestParseReleaseRejections(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    string
	}{
		{
			name:    "not json",
			payload: []byte("{"),
			want:    "provider release metadata is not valid JSON",
		},
		{
			name:    "not an object",
			payload: []byte(`[]`),
			want:    "GitHub release metadata must be an object",
		},
		{
			name:    "missing html_url",
			payload: []byte(`{"tag_name":"v1.2.3"}`),
			want:    "GitHub release belongs to an unexpected repository",
		},
		{
			name:    "wrong repository",
			payload: []byte(`{"html_url":"https://github.com/other/provider/releases/tag/v1.2.3","tag_name":"v1.2.3"}`),
			want:    "GitHub release belongs to an unexpected repository",
		},
		{
			name:    "bad tag",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider/releases/tag/latest","tag_name":"latest"}`),
			want:    "GitHub release has an invalid stable version tag",
		},
		{
			name:    "tag without v",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"1.2.3"}`),
			want:    "GitHub release has an invalid stable version tag",
		},
		{
			name:    "draft",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","draft":true}`),
			want:    "provider release is draft or prerelease",
		},
		{
			name:    "prerelease",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","prerelease":true}`),
			want:    "provider release is draft or prerelease",
		},
		{
			name:    "assets not array",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":null}`),
			want:    "GitHub release assets must be an array",
		},
		{
			name:    "asset not object",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":["x"]}`),
			want:    "GitHub release asset is not an object",
		},
		{
			name:    "asset missing url",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":[{"name":"x"}]}`),
			want:    "GitHub release asset is missing name or download URL",
		},
		{
			name:    "asset empty name",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":[{"name":"","browser_download_url":"https://github.com/parada1104/jinna-provider/x"}]}`),
			want:    "GitHub release asset is missing name or download URL",
		},
		{
			name:    "duplicate asset",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":[{"name":"SHA256SUMS","browser_download_url":"https://github.com/parada1104/jinna-provider/a"},{"name":"SHA256SUMS","browser_download_url":"https://github.com/parada1104/jinna-provider/b"}]}`),
			want:    "duplicate GitHub release asset: SHA256SUMS",
		},
		{
			name:    "asset outside repo",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":[{"name":"x","browser_download_url":"https://github.com/other/provider/x"}]}`),
			want:    "asset is outside the allowlisted repository: x",
		},
		{
			name:    "asset not allowlisted host",
			payload: []byte(`{"html_url":"https://github.com/parada1104/jinna-provider","tag_name":"v1.2.3","assets":[{"name":"x","browser_download_url":"https://example.com/parada1104/jinna-provider/x"}]}`),
			want:    "provider release URL must use the allowlisted GitHub HTTPS host",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseRelease(c.payload)
			requireInstallError(t, err, c.want)
		})
	}
}

// --- ParseReleaseMetadata ----------------------------------------------------

func TestParseReleaseMetadata(t *testing.T) {
	archive := stageArchiveName("linux", "amd64")
	good := []byte(`{"version":"` + stageTag + `","artifacts":["` + archive + `"]}`)
	if err := ParseReleaseMetadata(good, stageTag, archive); err != nil {
		t.Fatalf("ParseReleaseMetadata: %v", err)
	}

	cases := []struct {
		name    string
		payload []byte
		want    string
	}{
		{
			name:    "invalid utf8",
			payload: []byte{0xff, 0xfe},
			want:    "provider RELEASE.json is not valid UTF-8",
		},
		{
			name:    "invalid json",
			payload: []byte(`{`),
			want:    "provider RELEASE.json is not valid JSON",
		},
		{
			name:    "not an object",
			payload: []byte(`[]`),
			want:    "RELEASE.json must be a JSON object",
		},
		{
			name:    "version mismatch",
			payload: []byte(`{"version":"v9.9.9","artifacts":[]}`),
			want:    "RELEASE.json version 'v9.9.9' does not match release " + stageTag,
		},
		{
			name:    "version missing",
			payload: []byte(`{"artifacts":[]}`),
			want:    "RELEASE.json version None does not match release " + stageTag,
		},
		{
			name:    "artifacts not array",
			payload: []byte(`{"version":"` + stageTag + `","artifacts":{}}`),
			want:    "RELEASE.json artifacts must be an array of names",
		},
		{
			name:    "artifact not a name",
			payload: []byte(`{"version":"` + stageTag + `","artifacts":[1]}`),
			want:    "RELEASE.json artifacts must be an array of names",
		},
		{
			name:    "duplicate artifact",
			payload: []byte(`{"version":"` + stageTag + `","artifacts":["` + archive + `","` + archive + `"]}`),
			want:    "RELEASE.json lists a duplicate artifact",
		},
		{
			name:    "missing artifact",
			payload: []byte(`{"version":"` + stageTag + `","artifacts":["other"]}`),
			want:    "RELEASE.json does not list " + archive,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			requireInstallError(t, ParseReleaseMetadata(c.payload, stageTag, archive), c.want)
		})
	}
}

// --- ParseSHA256Sums ---------------------------------------------------------

func TestParseSHA256Sums(t *testing.T) {
	upper := strings.Repeat("A", 64)
	text := "# comment\n\n" + upper + "  jinna.tar.gz\n" + strings.Repeat("b", 64) + "  other.zip\n"
	digest, err := ParseSHA256Sums(text, "jinna.tar.gz")
	if err != nil {
		t.Fatalf("ParseSHA256Sums: %v", err)
	}
	if digest != strings.Repeat("a", 64) {
		t.Errorf("digest = %q, want lowercased", digest)
	}

	requireInstallError(t, func() error {
		_, err := ParseSHA256Sums("not-a-digest  file\n", "file")
		return err
	}(), "malformed SHA256SUMS entry")
	requireInstallError(t, func() error {
		_, err := ParseSHA256Sums(upper+"  a\n"+upper+"  a\n", "a")
		return err
	}(), "duplicate SHA256SUMS entry: a")
	requireInstallError(t, func() error {
		_, err := ParseSHA256Sums(upper+"  a\n", "b")
		return err
	}(), "SHA256SUMS has no entry for b")
}

// --- ExtractReleaseArchive ---------------------------------------------------

func TestExtractReleaseArchiveTarGz(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	binary, err := extractStage(t, archiveName, "linux",
		buildTarGz(t, stageRequiredMembers(root, "jinna")))
	if err != nil {
		t.Fatalf("ExtractReleaseArchive: %v", err)
	}
	if want := filepath.Join(root, "jinna"); !strings.HasSuffix(binary, want) {
		t.Errorf("binary = %q, want suffix %q", binary, want)
	}
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("binary mode = %v, want 0755", info.Mode().Perm())
	}
	for _, name := range []string{
		"LICENSE", "THIRD_PARTY_NOTICES.md", "README.md",
		"docs/mcp.md", "docs/migration.md", "docs/operations.md",
	} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(binary), filepath.FromSlash(name))); err != nil {
			t.Errorf("missing extracted member %s: %v", name, err)
		}
	}
}

func TestExtractReleaseArchiveZip(t *testing.T) {
	archiveName := stageArchiveName("windows", "amd64")
	root := trimArchiveSuffix(archiveName)
	binary, err := extractStage(t, archiveName, "windows",
		buildZip(t, stageRequiredMembers(root, "jinna.exe")))
	if err != nil {
		t.Fatalf("ExtractReleaseArchive: %v", err)
	}
	if want := filepath.Join(root, "jinna.exe"); !strings.HasSuffix(binary, want) {
		t.Errorf("binary = %q, want suffix %q", binary, want)
	}
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("binary mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestExtractReleaseArchiveMemberRejections(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	members := func(extra ...stageMember) []stageMember {
		return append(stageRequiredMembers(root, "jinna"), extra...)
	}
	cases := []struct {
		name    string
		members []stageMember
		want    string
	}{
		{
			name:    "absolute",
			members: members(stageMember{name: "/etc/passwd", data: []byte("x")}),
			want:    "archive contains an absolute member path",
		},
		{
			name:    "traversal",
			members: members(stageMember{name: root + "/../evil", data: []byte("x")}),
			want:    "archive contains a traversal member path",
		},
		{
			name:    "backslash",
			members: members(stageMember{name: root + `\evil`, data: []byte("x")}),
			want:    "archive contains an invalid member path",
		},
		{
			name:    "wrong root",
			members: members(stageMember{name: "other/README.md", data: []byte("x")}),
			want:    "archive contains an unexpected root: other/README.md",
		},
		{
			name:    "unexpected member",
			members: members(stageMember{name: root + "/extra.txt", data: []byte("x")}),
			want:    "archive contains an unexpected member: " + root + "/extra.txt",
		},
		{
			name: "duplicate member",
			members: append(stageRequiredMembers(root, "jinna"),
				stageMember{name: root + "/README.md", data: []byte("again")}),
			want: "archive contains a duplicate member: " + root + "/README.md",
		},
		{
			name: "symlink",
			members: append(stageRequiredMembers(root, "jinna"),
				stageMember{name: root + "/LICENSE", symlink: true}),
			want: "archive contains a duplicate member: " + root + "/LICENSE",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := extractStage(t, archiveName, "linux", buildTarGz(t, c.members))
			requireInstallError(t, err, c.want)
		})
	}
}

func TestExtractReleaseArchiveSymlinkRejected(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	members := []stageMember{
		{name: root + "/", dir: true},
		{name: root + "/jinna", symlink: true},
		{name: root + "/LICENSE", data: []byte("l")},
		{name: root + "/THIRD_PARTY_NOTICES.md", data: []byte("n")},
		{name: root + "/README.md", data: []byte("r")},
		{name: root + "/docs/mcp.md", data: []byte("m")},
		{name: root + "/docs/migration.md", data: []byte("m")},
		{name: root + "/docs/operations.md", data: []byte("o")},
	}
	_, err := extractStage(t, archiveName, "linux", buildTarGz(t, members))
	requireInstallError(t, err, "archive member is not a regular file: "+root+"/jinna")
}

func TestExtractReleaseArchiveZipSymlinkRejected(t *testing.T) {
	archiveName := stageArchiveName("windows", "amd64")
	root := trimArchiveSuffix(archiveName)
	members := []stageMember{
		{name: root + "/", dir: true},
		{name: root + "/jinna.exe", symlink: true},
		{name: root + "/LICENSE", data: []byte("l")},
		{name: root + "/THIRD_PARTY_NOTICES.md", data: []byte("n")},
		{name: root + "/README.md", data: []byte("r")},
		{name: root + "/docs/mcp.md", data: []byte("m")},
		{name: root + "/docs/migration.md", data: []byte("m")},
		{name: root + "/docs/operations.md", data: []byte("o")},
	}
	_, err := extractStage(t, archiveName, "windows", buildZip(t, members))
	requireInstallError(t, err, "archive member is a symbolic link: "+root+"/jinna.exe")
}

func TestExtractReleaseArchiveMissingMember(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	var members []stageMember
	for _, member := range stageRequiredMembers(root, "jinna") {
		if member.name == root+"/docs/operations.md" {
			continue
		}
		members = append(members, member)
	}
	_, err := extractStage(t, archiveName, "linux", buildTarGz(t, members))
	requireInstallError(t, err, "provider archive is missing: "+root+"/docs/operations.md")
}

func TestExtractReleaseArchiveOversizeMember(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	members := []stageMember{
		{name: root + "/", dir: true},
		{name: root + "/jinna", data: []byte("bin"), mode: 0o755},
		{name: root + "/LICENSE", data: []byte("l")},
		{name: root + "/THIRD_PARTY_NOTICES.md", data: []byte("n")},
		{name: root + "/README.md", data: make([]byte, maxMemberBytes+1)},
		{name: root + "/docs/mcp.md", data: []byte("m")},
		{name: root + "/docs/migration.md", data: []byte("m")},
		{name: root + "/docs/operations.md", data: []byte("o")},
	}
	_, err := extractStage(t, archiveName, "linux", buildTarGz(t, members))
	requireInstallError(t, err, "archive member is too large")
}

func TestExtractReleaseArchiveUnsupported(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "jinna.tar")
	if err := os.WriteFile(archivePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractReleaseArchive(archivePath, filepath.Join(dir, "out"), "jinna.tar", "linux")
	requireInstallError(t, err, "unsupported provider archive: jinna.tar")
}

func TestExtractReleaseArchiveMissingBinary(t *testing.T) {
	archiveName := stageArchiveName("linux", "amd64")
	root := trimArchiveSuffix(archiveName)
	// Every other required member is present, so the missing set is exactly the
	// binary the archive was supposed to publish.
	members := []stageMember{
		{name: root + "/", dir: true},
		{name: root + "/LICENSE", data: []byte("l")},
		{name: root + "/THIRD_PARTY_NOTICES.md", data: []byte("n")},
		{name: root + "/README.md", data: []byte("r")},
		{name: root + "/docs/mcp.md", data: []byte("m")},
		{name: root + "/docs/migration.md", data: []byte("m")},
		{name: root + "/docs/operations.md", data: []byte("o")},
	}
	_, err := extractStage(t, archiveName, "linux", buildTarGz(t, members))
	requireInstallError(t, err, "provider archive is missing: "+root+"/jinna")
}

// --- InstallRelease ----------------------------------------------------------

// installFixture serves one fully consistent release over an injected Fetcher.
type installFixture struct {
	plan       ReleasePlan
	archive    []byte
	archiveURL string
	payloads   map[string][]byte
	version    string
}

func newInstallFixture(t *testing.T, goos, goarch string) *installFixture {
	t.Helper()
	archiveName := stageArchiveName(goos, goarch)
	root := trimArchiveSuffix(archiveName)
	var archive []byte
	if strings.HasSuffix(archiveName, ".tar.gz") {
		archive = buildTarGz(t, stageRequiredMembers(root, ExpectedBinaryName(goos)))
	} else {
		archive = buildZip(t, stageRequiredMembers(root, ExpectedBinaryName(goos)))
	}
	base := "https://github.com/" + Repository + "/releases/download/" + stageTag + "/"
	assets := []any{
		map[string]any{"name": archiveName, "browser_download_url": base + archiveName},
		map[string]any{"name": "SHA256SUMS", "browser_download_url": base + "SHA256SUMS"},
		map[string]any{"name": "RELEASE.json", "browser_download_url": base + "RELEASE.json"},
	}
	releasePayload := map[string]any{
		"html_url":   "https://github.com/" + Repository + "/releases/tag/" + stageTag,
		"tag_name":   stageTag,
		"draft":      false,
		"prerelease": false,
		"assets":     assets,
	}
	releaseRaw, err := json.Marshal(releasePayload)
	if err != nil {
		t.Fatal(err)
	}
	metadataRaw, err := json.Marshal(map[string]any{
		"version":   stageTag,
		"artifacts": []string{archiveName},
	})
	if err != nil {
		t.Fatal(err)
	}
	sums := digestOf(archive) + "  " + archiveName + "\n"
	return &installFixture{
		plan: ReleasePlan{
			Binary:        "jinna",
			Repository:    Repository,
			ReleasePolicy: "latest-stable",
			MinVersion:    "0.4.0",
			InstallURL:    "https://example.com/jinna",
			Goos:          goos,
			Goarch:        goarch,
			CacheRoot:     filepath.Join(t.TempDir(), "cache", "bin", "jinna"),
		},
		archive:    archive,
		archiveURL: base + archiveName,
		payloads: map[string][]byte{
			GithubAPIBase + "/releases/latest": releaseRaw,
			base + "RELEASE.json":              metadataRaw,
			base + "SHA256SUMS":                []byte(sums),
			base + archiveName:                 archive,
		},
		version: "1.2.3",
	}
}

func (f *installFixture) fetch(t *testing.T) Fetcher {
	t.Helper()
	return func(rawurl string) ([]byte, error) {
		payload, ok := f.payloads[rawurl]
		if !ok {
			t.Fatalf("unexpected fetch %q", rawurl)
		}
		return payload, nil
	}
}

func (f *installFixture) runVersion(t *testing.T) VersionRunner {
	t.Helper()
	return func(path string) (string, error) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("version runner got %q: %v", path, err)
		}
		return f.version, nil
	}
}

func TestInstallReleaseHappyPath(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	resolution, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	if err != nil {
		t.Fatalf("InstallRelease: %v", err)
	}
	targetDir := filepath.Join(fixture.plan.CacheRoot, stageTag, "linux-amd64")
	wantPath := filepath.Join(targetDir, "jinna")
	if resolution.Source != SourceManaged || !resolution.Verified {
		t.Errorf("resolution = %+v", resolution)
	}
	if resolution.Version != "1.2.3" || resolution.ReleaseTag != stageTag {
		t.Errorf("resolution = %+v", resolution)
	}
	if resolution.Repository != Repository || resolution.Target != [2]string{"linux", "amd64"} {
		t.Errorf("resolution = %+v", resolution)
	}
	if resolution.Binary != "jinna" || resolution.Command != wantPath {
		t.Errorf("resolution = %+v", resolution)
	}
	if resolution.Path == nil || *resolution.Path != wantPath {
		t.Errorf("resolution path = %v, want %q", resolution.Path, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("installed binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "docs", "mcp.md")); err != nil {
		t.Errorf("installed docs: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(targetDir, "install.json"))
	if err != nil {
		t.Fatalf("install.json: %v", err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatalf("install.json: %v", err)
	}
	wantReceipt := map[string]any{
		"status":         "verified",
		"repository":     Repository,
		"release_tag":    stageTag,
		"target":         "linux-amd64",
		"archive":        stageArchiveName("linux", "amd64"),
		"archive_sha256": digestOf(fixture.archive),
		"binary_sha256":  digestOf([]byte("#!/bin/sh\necho v1.2.3\n")),
	}
	for key, want := range wantReceipt {
		if got := receipt[key]; got != want {
			t.Errorf("receipt[%q] = %v, want %v", key, got, want)
		}
	}
	if len(receipt) != len(wantReceipt) {
		t.Errorf("receipt keys = %v", receipt)
	}
}

func TestInstallReleaseChecksumMismatch(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	sumsURL := "https://github.com/" + Repository + "/releases/download/" + stageTag + "/SHA256SUMS"
	fixture.payloads[sumsURL] = []byte(strings.Repeat("0", 64) + "  " + stageArchiveName("linux", "amd64") + "\n")
	_, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	requireInstallError(t, err,
		"provider checksum mismatch: expected "+strings.Repeat("0", 64)+", got "+digestOf(fixture.archive))
}

func TestInstallReleaseVersionMismatch(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	fixture.version = "1.2.4"
	_, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	requireInstallError(t, err, "provider version 1.2.4 does not match release "+stageTag)
}

func TestInstallReleaseBelowMinVersion(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	fixture.plan.MinVersion = "9.0.0"
	_, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	requireInstallError(t, err, "provider version 1.2.3 is below required 9.0.0")
}

func TestInstallReleaseExistingTarget(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	targetDir := filepath.Join(fixture.plan.CacheRoot, stageTag, "linux-amd64")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	requireInstallError(t, err, "managed provider target already exists: "+targetDir)
}

func TestInstallReleaseFetchFailure(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	sentinel := errors.New("network down")
	fetch := func(rawurl string) ([]byte, error) {
		if rawurl == GithubAPIBase+"/releases/latest" {
			return nil, sentinel
		}
		return fixture.payloads[rawurl], nil
	}
	_, err := InstallRelease(fixture.plan, fetch, fixture.runVersion(t))
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the fetcher's error", err)
	}
}

func TestInstallReleaseMissingAssets(t *testing.T) {
	fixture := newInstallFixture(t, "linux", "amd64")
	delete(fixture.payloads, "https://github.com/"+Repository+"/releases/download/"+stageTag+"/RELEASE.json")
	releaseRaw := fixture.payloads[GithubAPIBase+"/releases/latest"]
	var payload map[string]any
	if err := json.Unmarshal(releaseRaw, &payload); err != nil {
		t.Fatal(err)
	}
	assets := payload["assets"].([]any)
	payload["assets"] = assets[:2]
	trimmed, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	fixture.payloads[GithubAPIBase+"/releases/latest"] = trimmed
	_, installErr := InstallRelease(fixture.plan,
		func(rawurl string) ([]byte, error) { return fixture.payloads[rawurl], nil },
		fixture.runVersion(t))
	requireInstallError(t, installErr,
		"release "+stageTag+" lacks "+stageArchiveName("linux", "amd64")+", SHA256SUMS, or RELEASE.json")
}

func TestInstallReleaseZipTarget(t *testing.T) {
	fixture := newInstallFixture(t, "windows", "amd64")
	resolution, err := InstallRelease(fixture.plan, fixture.fetch(t), fixture.runVersion(t))
	if err != nil {
		t.Fatalf("InstallRelease: %v", err)
	}
	wantPath := filepath.Join(fixture.plan.CacheRoot, stageTag, "windows-amd64", "jinna.exe")
	if resolution.Path == nil || *resolution.Path != wantPath {
		t.Errorf("resolution path = %v, want %q", resolution.Path, wantPath)
	}
	if resolution.Target != [2]string{"windows", "amd64"} {
		t.Errorf("resolution target = %v", resolution.Target)
	}
}

// TestInstallReleaseDefaultFetcherRejectsNonAllowlisted pins the default
// Fetcher's allowlist without touching the network.
func TestInstallReleaseDefaultFetcherRejectsNonAllowlisted(t *testing.T) {
	_, err := fetchBytes("http://api.github.com/repos/x/y")
	requireInstallError(t, err, "provider download URL is not an allowlisted HTTPS URL")
	_, err = fetchBytes("https://example.com/repos/x/y")
	requireInstallError(t, err, "provider download URL is not an allowlisted HTTPS URL")
}

// TestInstallReleaseDefaultFetcherPlumbed pins that a nil fetch selects the
// allowlisted fetcher rather than panicking, and that a nil version runner does
// not distract from a fetch failure.
func TestInstallReleaseDefaultFetcherPlumbed(t *testing.T) {
	plan := ReleasePlan{
		Binary:    "jinna",
		Goos:      "linux",
		Goarch:    "amd64",
		CacheRoot: filepath.Join(t.TempDir(), "cache"),
	}
	// The default fetcher would reach api.github.com, so this case injects a
	// fetcher that stops first and leaves the version runner nil.
	sentinel := errors.New("stop before network")
	_, err := InstallRelease(plan, func(string) ([]byte, error) { return nil, sentinel }, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the fetcher's error", err)
	}
}

// Install half of the native port of lib/_internal/provider_install.py:
// release_asset_name (:99), _repository_from_url (:108), parse_release (:120),
// parse_release_metadata (:153), parse_sha256sums (:176), _sha256 (:192,
// already ported as sha256File), _safe_member_name (:201), _expected_members
// (:219), _write_member (:234), extract_release_archive (:251),
// _fetch_bytes (:471) and install_github_release (:499).
//
// Faithfulness notes (divergences pinned by the tests, never claimed as parity):
//   - _fetch_bytes: the allowlist, headers, timeout, size cap and the three
//     messages are reproduced; a transport failure surfaces Go's error text
//     where Python wraps urllib's, and a non-2xx status reproduces urllib's
//     "HTTP Error <code>: <reason>" shape.
//   - archive inspection errors are Python's "cannot inspect provider archive:"
//     prefix over Go's tar/gzip/zip error text.
//   - the install receipt keeps Python's key order via a struct, so its JSON
//     body is byte-identical for the same values.
package provider

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// GithubAPIBase is provider_install.GITHUB_API_BASE.
const GithubAPIBase = "https://api.github.com/repos/" + Repository

const (
	// maxHTTPBytes is provider_install.MAX_HTTP_BYTES.
	maxHTTPBytes = 16 << 20
	// maxMemberBytes is provider_install.MAX_MEMBER_BYTES: the released
	// darwin/arm64 `jinna` executable is ~7.9 MiB, so 32 MiB keeps a wide
	// margin without accepting unreasonable archive members.
	maxMemberBytes = 32 << 20
	// maxMetadataBytes is provider_install.MAX_METADATA_BYTES.
	maxMetadataBytes = 64 << 10
	// fetchTimeout is _fetch_bytes' default timeout.
	fetchTimeout = 15 * time.Second
)

// providerTagRe is parse_release's `v[0-9]+(?:\.[0-9]+)+` full match.
var providerTagRe = regexp.MustCompile(`^v[0-9]+(?:\.[0-9]+)+$`)

// checksumRe is CHECKSUM_RE: a 64 hex digit digest, two spaces, a non-empty
// whitespace-free name.
var checksumRe = regexp.MustCompile(`^([0-9a-fA-F]{64})  ([^\s]+)$`)

// ReleaseAsset mirrors provider_install.ReleaseAsset.
type ReleaseAsset struct {
	Name string
	URL  string
}

// ReleaseInfo mirrors provider_install.ReleaseInfo.
type ReleaseInfo struct {
	Repository string
	Tag        string
	Assets     map[string]ReleaseAsset
}

// ReleaseAssetName is provider_install.release_asset_name. An empty goos or
// goarch renders as "?" exactly as Python's `goos or '?'`.
func ReleaseAssetName(tag, goos, goarch string) (string, error) {
	if !supportedPlatforms[[2]string{goos, goarch}] {
		shownOS, shownArch := goos, goarch
		if shownOS == "" {
			shownOS = "?"
		}
		if shownArch == "" {
			shownArch = "?"
		}
		return "", &InstallError{
			Message: "unsupported provider target: " + shownOS + "/" + shownArch,
		}
	}
	suffix := ".tar.gz"
	if goos == "windows" {
		suffix = ".zip"
	}
	return "jinna_" + tag + "_" + goos + "_" + goarch + suffix, nil
}

// RepositoryFromURL is provider_install._repository_from_url.
func RepositoryFromURL(rawurl string) (string, error) {
	parsed, err := url.Parse(rawurl)
	if err != nil || parsed.Scheme != "https" ||
		(parsed.Host != "github.com" && parsed.Host != "api.github.com") {
		return "", &InstallError{
			Message: "provider release URL must use the allowlisted GitHub HTTPS host",
		}
	}
	var parts []string
	for _, part := range strings.Split(parsed.Path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) >= 2 && parts[0] == "repos" {
		end := 3
		if len(parts) < end {
			end = len(parts)
		}
		return strings.Join(parts[1:end], "/"), nil
	}
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "/"), nil
	}
	return "", &InstallError{Message: "provider release URL has no repository"}
}

// ParseRelease is provider_install.parse_release. The Python function takes the
// already-decoded payload; this port decodes the bytes first with the message
// install_github_release raises for an undecodable release.
func ParseRelease(payload []byte) (ReleaseInfo, error) {
	object, err := decodeJSONObject(payload,
		"provider release metadata is not valid JSON",
		"GitHub release metadata must be an object")
	if err != nil {
		return ReleaseInfo{}, err
	}
	rawHTMLURL, ok := object["html_url"].(string)
	if !ok {
		return ReleaseInfo{}, &InstallError{
			Message: "GitHub release belongs to an unexpected repository",
		}
	}
	repository, err := RepositoryFromURL(rawHTMLURL)
	if err != nil {
		return ReleaseInfo{}, err
	}
	if repository != Repository {
		return ReleaseInfo{}, &InstallError{
			Message: "GitHub release belongs to an unexpected repository",
		}
	}
	tag, ok := object["tag_name"].(string)
	if !ok || !providerTagRe.MatchString(tag) {
		return ReleaseInfo{}, &InstallError{
			Message: "GitHub release has an invalid stable version tag",
		}
	}
	if draft, _ := object["draft"].(bool); draft {
		return ReleaseInfo{}, &InstallError{
			Message: "provider release is draft or prerelease",
		}
	}
	if prerelease, _ := object["prerelease"].(bool); prerelease {
		return ReleaseInfo{}, &InstallError{
			Message: "provider release is draft or prerelease",
		}
	}
	rawAssets, present := object["assets"]
	if !present {
		rawAssets = []any{}
	}
	assetList, ok := rawAssets.([]any)
	if !ok {
		return ReleaseInfo{}, &InstallError{
			Message: "GitHub release assets must be an array",
		}
	}
	assets := make(map[string]ReleaseAsset, len(assetList))
	for _, raw := range assetList {
		assetObject, ok := raw.(map[string]any)
		if !ok {
			return ReleaseInfo{}, &InstallError{
				Message: "GitHub release asset is not an object",
			}
		}
		name, nameOK := assetObject["name"].(string)
		assetURL, urlOK := assetObject["browser_download_url"].(string)
		if !nameOK || name == "" || !urlOK {
			return ReleaseInfo{}, &InstallError{
				Message: "GitHub release asset is missing name or download URL",
			}
		}
		if _, duplicate := assets[name]; duplicate {
			return ReleaseInfo{}, &InstallError{
				Message: "duplicate GitHub release asset: " + name,
			}
		}
		assetRepository, err := RepositoryFromURL(assetURL)
		if err != nil {
			return ReleaseInfo{}, err
		}
		if assetRepository != Repository {
			return ReleaseInfo{}, &InstallError{
				Message: "asset is outside the allowlisted repository: " + name,
			}
		}
		assets[name] = ReleaseAsset{Name: name, URL: assetURL}
	}
	return ReleaseInfo{Repository: Repository, Tag: tag, Assets: assets}, nil
}

// ParseReleaseMetadata is provider_install.parse_release_metadata. The metadata
// is provenance, not a trust substitute: it must agree with the selected
// release tag and list the exact archive about to be installed.
func ParseReleaseMetadata(payload []byte, tag, archiveName string) error {
	if !utf8.Valid(payload) {
		return &InstallError{
			Message: "provider RELEASE.json is not valid UTF-8",
		}
	}
	object, err := decodeJSONObject(payload,
		"provider RELEASE.json is not valid JSON",
		"RELEASE.json must be a JSON object")
	if err != nil {
		return err
	}
	rawVersion, present := object["version"]
	if !present {
		return &InstallError{
			Message: "RELEASE.json version None does not match release " + tag,
		}
	}
	if version, isString := rawVersion.(string); !isString || version != tag {
		return &InstallError{
			Message: "RELEASE.json version " + pyReprValue(rawVersion) +
				" does not match release " + tag,
		}
	}
	rawArtifacts, ok := object["artifacts"].([]any)
	if !ok {
		return &InstallError{
			Message: "RELEASE.json artifacts must be an array of names",
		}
	}
	seen := make(map[string]bool, len(rawArtifacts))
	for _, raw := range rawArtifacts {
		name, isString := raw.(string)
		if !isString {
			return &InstallError{
				Message: "RELEASE.json artifacts must be an array of names",
			}
		}
		if seen[name] {
			return &InstallError{Message: "RELEASE.json lists a duplicate artifact"}
		}
		seen[name] = true
	}
	if !seen[archiveName] {
		return &InstallError{
			Message: "RELEASE.json does not list " + archiveName,
		}
	}
	return nil
}

// ParseSHA256Sums is provider_install.parse_sha256sums: blank lines and
// `#` comments are skipped, every other line must be a well-formed unique entry,
// and the digest is returned lowercased.
func ParseSHA256Sums(text, assetName string) (string, error) {
	found := make(map[string]string)
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := checksumRe.FindStringSubmatch(line)
		if match == nil {
			return "", &InstallError{Message: "malformed SHA256SUMS entry"}
		}
		digest, name := match[1], match[2]
		if _, duplicate := found[name]; duplicate {
			return "", &InstallError{
				Message: "duplicate SHA256SUMS entry: " + name,
			}
		}
		found[name] = strings.ToLower(digest)
	}
	digest, ok := found[assetName]
	if !ok {
		return "", &InstallError{
			Message: "SHA256SUMS has no entry for " + assetName,
		}
	}
	return digest, nil
}

// ExtractReleaseArchive is provider_install.extract_release_archive: it safely
// extracts the exact provider file set for one archive (tar.gz or zip) and
// returns the extracted binary path.
func ExtractReleaseArchive(archivePath, destination, archiveName, goos string) (string, error) {
	root := trimArchiveSuffix(archiveName)
	binaryName := ExpectedBinaryName(goos)
	binaryMember := root + "/" + binaryName
	required := map[string]bool{
		binaryMember:                     true,
		root + "/LICENSE":                true,
		root + "/THIRD_PARTY_NOTICES.md": true,
		root + "/README.md":              true,
		root + "/docs/mcp.md":            true,
		root + "/docs/migration.md":      true,
		root + "/docs/operations.md":     true,
	}
	container := map[string]bool{root: true, root + "/docs": true}
	seen := make(map[string]bool, len(required))
	validate := func(name string) (string, error) {
		clean, err := safeMemberName(name, root)
		if err != nil {
			return "", err
		}
		if seen[clean] {
			return "", &InstallError{
				Message: "archive contains a duplicate member: " + clean,
			}
		}
		seen[clean] = true
		if !required[clean] && !container[clean] {
			return "", &InstallError{
				Message: "archive contains an unexpected member: " + clean,
			}
		}
		return clean, nil
	}

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", &InstallError{
			Message: "cannot inspect provider archive: " + err.Error(),
		}
	}

	var err error
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		err = extractTarGz(archivePath, destination, binaryMember, validate)
	case strings.HasSuffix(archiveName, ".zip"):
		err = extractZip(archivePath, destination, binaryMember, validate)
	default:
		return "", &InstallError{
			Message: "unsupported provider archive: " + archiveName,
		}
	}
	if err != nil {
		return "", wrapArchiveError(err)
	}

	var missing []string
	for name := range required {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", &InstallError{
			Message: "provider archive is missing: " + strings.Join(missing, ", "),
		}
	}
	binary := filepath.Join(destination, root, binaryName)
	if info, statErr := os.Stat(binary); statErr != nil || info.IsDir() {
		return "", &InstallError{
			Message: "provider archive did not produce an executable",
		}
	}
	return binary, nil
}

// safeMemberName is provider_install._safe_member_name: no empty or backslash
// names, no absolute or traversal paths, and the member must live under the
// archive's root directory.
func safeMemberName(name, expectedRoot string) (string, error) {
	if name == "" || strings.Contains(name, "\\") {
		return "", &InstallError{
			Message: "archive contains an invalid member path",
		}
	}
	clean := strings.TrimSuffix(name, "/")
	if strings.HasPrefix(clean, "/") {
		return "", &InstallError{
			Message: "archive contains an absolute member path",
		}
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", &InstallError{
				Message: "archive contains a traversal member path",
			}
		}
	}
	if clean != expectedRoot && !strings.HasPrefix(clean, expectedRoot+"/") {
		return "", &InstallError{
			Message: "archive contains an unexpected root: " + clean,
		}
	}
	return clean, nil
}

// trimArchiveSuffix is archive_name.removesuffix(".tar.gz").removesuffix(".zip").
func trimArchiveSuffix(archiveName string) string {
	return strings.TrimSuffix(strings.TrimSuffix(archiveName, ".tar.gz"), ".zip")
}

// writeMember is provider_install._write_member: the size cap, parent creation,
// O_EXCL creation and the executable mode for the binary member.
func writeMember(destination, relative string, data []byte, executable bool) error {
	if len(data) > maxMemberBytes {
		return &InstallError{Message: "archive member is too large"}
	}
	target := filepath.Join(destination, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	fh, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	if _, err := fh.Write(data); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	if executable {
		return os.Chmod(target, 0o755)
	}
	return nil
}

// memberValidator is the per-entry name gate extract_release_archive installs.
type memberValidator func(string) (string, error)

// extractTarGz walks a .tar.gz archive exactly as tarfile.getmembers does:
// every header is validated in order and only regular files are written.
func extractTarGz(archivePath, destination, binaryMember string, validate memberValidator) error {
	fh, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	gzipReader, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	archive := tar.NewReader(gzipReader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		clean, err := validate(header.Name)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return &InstallError{
				Message: "archive member is not a regular file: " + clean,
			}
		}
		data, err := io.ReadAll(io.LimitReader(archive, maxMemberBytes+1))
		if err != nil {
			return err
		}
		if err := writeMember(destination, clean, data, clean == binaryMember); err != nil {
			return err
		}
	}
}

// extractZip walks a .zip archive as zipfile.infolist does: every entry is
// validated in order, symbolic links are rejected, and only files are written.
func extractZip(archivePath, destination, binaryMember string, validate memberValidator) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, member := range archive.File {
		clean, err := validate(member.Name)
		if err != nil {
			return err
		}
		if member.Mode()&os.ModeSymlink != 0 {
			return &InstallError{
				Message: "archive member is a symbolic link: " + clean,
			}
		}
		if member.FileInfo().IsDir() {
			continue
		}
		if member.FileInfo().Size() > int64(maxMemberBytes) {
			return &InstallError{
				Message: "archive member is too large: " + clean,
			}
		}
		reader, err := member.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, maxMemberBytes+1))
		reader.Close()
		if readErr != nil {
			return readErr
		}
		if err := writeMember(destination, clean, data, clean == binaryMember); err != nil {
			return err
		}
	}
	return nil
}

// wrapArchiveError keeps an InstallError's message and gives any raw tar, gzip,
// zip or filesystem error the provider_install inspection prefix.
func wrapArchiveError(err error) error {
	var installErr *InstallError
	if errors.As(err, &installErr) {
		return err
	}
	return &InstallError{
		Message: "cannot inspect provider archive: " + err.Error(),
	}
}

// Fetcher is the injectable _fetch_bytes seam of install_github_release.
type Fetcher func(string) ([]byte, error)

// VersionRunner is the injectable _run_version seam of install_github_release.
type VersionRunner func(string) (string, error)

// installReceipt is the install.json receipt; the field order matches Python's
// insertion order so the serialized body is byte-identical.
type installReceipt struct {
	Status        string `json:"status"`
	Repository    string `json:"repository"`
	ReleaseTag    string `json:"release_tag"`
	Target        string `json:"target"`
	Archive       string `json:"archive"`
	ArchiveSHA256 string `json:"archive_sha256"`
	BinarySHA256  string `json:"binary_sha256"`
}

// InstallRelease is provider_install.install_github_release: acquire, verify,
// self-test, and atomically publish one provider target. A nil fetch uses the
// allowlisted HTTPS fetcher and a nil runVersion uses the package's
// _run_version port.
func InstallRelease(plan ReleasePlan, fetch Fetcher, runVersion VersionRunner) (Resolution, error) {
	if fetch == nil {
		fetch = fetchBytes
	}
	if runVersion == nil {
		runVersion = func(path string) (string, error) {
			return runVersionPath(path, versionTimeout)
		}
	}

	releasePayload, err := fetch(GithubAPIBase + "/releases/latest")
	if err != nil {
		return Resolution{}, err
	}
	release, err := ParseRelease(releasePayload)
	if err != nil {
		return Resolution{}, err
	}
	expectedVersion := joinVersion(VersionTuple(release.Tag))
	if expectedVersion == "" {
		return Resolution{}, &InstallError{
			Message: "provider release tag is not a version: " + release.Tag,
		}
	}
	archiveName, err := ReleaseAssetName(release.Tag, plan.Goos, plan.Goarch)
	if err != nil {
		return Resolution{}, err
	}
	archiveAsset, haveArchive := release.Assets[archiveName]
	sumsAsset, haveSums := release.Assets["SHA256SUMS"]
	metadataAsset, haveMetadata := release.Assets["RELEASE.json"]
	if !haveArchive || !haveSums || !haveMetadata {
		return Resolution{}, &InstallError{
			Message: "release " + release.Tag + " lacks " + archiveName +
				", SHA256SUMS, or RELEASE.json",
		}
	}

	metadataRaw, err := fetch(metadataAsset.URL)
	if err != nil {
		return Resolution{}, err
	}
	if len(metadataRaw) > maxMetadataBytes {
		return Resolution{}, &InstallError{
			Message: "provider RELEASE.json exceeds the size limit",
		}
	}
	if err := ParseReleaseMetadata(metadataRaw, release.Tag, archiveName); err != nil {
		return Resolution{}, err
	}

	sumsRaw, err := fetch(sumsAsset.URL)
	if err != nil {
		return Resolution{}, err
	}
	if !utf8.Valid(sumsRaw) {
		return Resolution{}, &InstallError{
			Message: "provider SHA256SUMS manifest is not valid UTF-8",
		}
	}
	expectedDigest, err := ParseSHA256Sums(string(sumsRaw), archiveName)
	if err != nil {
		return Resolution{}, err
	}

	targetDir := filepath.Join(plan.CacheRoot, release.Tag, plan.Goos+"-"+plan.Goarch)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	tempParent, err := os.MkdirTemp(filepath.Dir(targetDir), ".jinna-")
	if err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	defer os.RemoveAll(tempParent)

	archivePath := filepath.Join(tempParent, archiveName)
	archiveBytes, err := fetch(archiveAsset.URL)
	if err != nil {
		return Resolution{}, err
	}
	if err := os.WriteFile(archivePath, archiveBytes, 0o666); err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	observedDigest, err := sha256File(archivePath)
	if err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	if observedDigest != expectedDigest {
		return Resolution{}, &InstallError{
			Message: "provider checksum mismatch: expected " + expectedDigest +
				", got " + observedDigest,
		}
	}

	extractedDir := filepath.Join(tempParent, "extracted")
	extracted, err := ExtractReleaseArchive(archivePath, extractedDir, archiveName, plan.Goos)
	if err != nil {
		return Resolution{}, err
	}
	version, err := runVersion(extracted)
	if err != nil {
		return Resolution{}, err
	}
	if version != expectedVersion {
		return Resolution{}, &InstallError{
			Message: "provider version " + version + " does not match release " + release.Tag,
		}
	}
	if !versionAtLeast(version, plan.MinVersion) {
		return Resolution{}, &InstallError{
			Message: "provider version " + version + " is below required " + plan.MinVersion,
		}
	}

	root := trimArchiveSuffix(archiveName)
	extractedRoot := filepath.Join(extractedDir, root)
	finalBinary := filepath.Join(extractedRoot, ExpectedBinaryName(plan.Goos))
	binaryDigest, err := sha256File(finalBinary)
	if err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	receipt := installReceipt{
		Status:        "verified",
		Repository:    Repository,
		ReleaseTag:    release.Tag,
		Target:        plan.Goos + "-" + plan.Goarch,
		Archive:       archiveName,
		ArchiveSHA256: observedDigest,
		BinarySHA256:  binaryDigest,
	}
	receiptRaw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}
	receiptRaw = append(receiptRaw, '\n')
	receiptPath := filepath.Join(extractedRoot, "install.json")
	if err := os.WriteFile(receiptPath, receiptRaw, 0o666); err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}

	if _, err := os.Stat(targetDir); err == nil {
		return Resolution{}, &InstallError{
			Message: "managed provider target already exists: " + targetDir,
		}
	}
	if err := os.Rename(extractedRoot, targetDir); err != nil {
		return Resolution{}, &InstallError{
			Message: "provider installation failed: " + err.Error(),
		}
	}

	installed := filepath.Join(targetDir, ExpectedBinaryName(plan.Goos))
	return Resolution{
		Binary:     plan.Binary,
		Command:    installed,
		Version:    version,
		Source:     SourceManaged,
		Path:       &installed,
		Repository: Repository,
		ReleaseTag: release.Tag,
		Target:     [2]string{plan.Goos, plan.Goarch},
		Verified:   true,
	}, nil
}

// runVersionPath keeps the package's _run_version port reachable while the
// InstallRelease parameter named runVersion is in scope.
func runVersionPath(path string, timeout time.Duration) (string, error) {
	return runVersion(path, timeout)
}

// fetchBytes is provider_install._fetch_bytes: HTTPS only, the two allowlisted
// GitHub hosts, GitHub's JSON Accept header, a 15s timeout and a hard size cap.
func fetchBytes(rawurl string) ([]byte, error) {
	parsed, err := url.Parse(rawurl)
	if err != nil || parsed.Scheme != "https" ||
		(parsed.Host != "api.github.com" && parsed.Host != "github.com") {
		return nil, &InstallError{
			Message: "provider download URL is not an allowlisted HTTPS URL",
		}
	}
	request, err := http.NewRequest(http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, &InstallError{
			Message: "provider download URL is not an allowlisted HTTPS URL",
		}
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "ai-specs-jinna-mcp-recipe")
	client := &http.Client{Timeout: fetchTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, &InstallError{
			Message: "provider download failed: " + err.Error(),
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, &InstallError{
			Message: fmt.Sprintf("provider download failed: HTTP Error %d: %s",
				response.StatusCode, http.StatusText(response.StatusCode)),
		}
	}
	chunks := make([]byte, 0, 64<<10)
	buffer := make([]byte, 1024*1024)
	for {
		read, err := response.Body.Read(buffer)
		if read > 0 {
			chunks = append(chunks, buffer[:read]...)
			if len(chunks) > maxHTTPBytes {
				return nil, &InstallError{
					Message: "provider response exceeds the size limit",
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return chunks, nil
		}
		if err != nil {
			return nil, &InstallError{
				Message: "provider download failed: " + err.Error(),
			}
		}
	}
}

// decodeJSONObject is the shared json.loads + object check of parse_release and
// parse_release_metadata, with each caller's two messages.
func decodeJSONObject(payload []byte, invalidJSON, notAnObject string) (map[string]any, error) {
	if !utf8.Valid(payload) {
		return nil, &InstallError{Message: invalidJSON}
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, &InstallError{Message: invalidJSON}
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, &InstallError{Message: notAnObject}
	}
	return object, nil
}

// pyReprValue renders the JSON values parse_release_metadata's `{version!r}`
// interpolation can see: strings via pyRepr, None, booleans and numbers.
func pyReprValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case string:
		return pyRepr(typed)
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case float64:
		return strconvFloat(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// strconvFloat renders a JSON number the way Python reprs an int or float.
func strconvFloat(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}
	return fmt.Sprint(value)
}

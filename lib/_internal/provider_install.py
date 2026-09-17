"""Verified acquisition and resolution of the jinna OpenProject provider."""
from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import shutil
import stat
import subprocess
import tarfile
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Iterable


PROVIDER_REPOSITORY = "parada1104/jinna-provider"
GITHUB_API_BASE = "https://api.github.com/repos/" + PROVIDER_REPOSITORY
GITHUB_DOWNLOAD_BASE = "https://github.com/" + PROVIDER_REPOSITORY
SUPPORTED_PLATFORMS = (
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("linux", "arm64"),
    ("linux", "amd64"),
    ("windows", "amd64"),
)
MAX_HTTP_BYTES = 16 * 1024 * 1024
# The released darwin/arm64 `jinna` executable is ~7.9 MiB, so the old 8 MiB
# cap left almost no headroom. 32 MiB keeps a wide margin without accepting
# unreasonable archive members.
MAX_MEMBER_BYTES = 32 * 1024 * 1024
MAX_METADATA_BYTES = 64 * 1024
VERSION_RE = re.compile(r"(?<!\d)v?(\d+(?:\.\d+)+)(?!\d)")
CHECKSUM_RE = re.compile(r"^([0-9a-fA-F]{64})  ([^\s]+)$")


class InstallError(RuntimeError):
    """A provider release was not safe or usable for installation."""


@dataclass(frozen=True)
class ReleaseAsset:
    name: str
    url: str


@dataclass(frozen=True)
class ReleaseInfo:
    repository: str
    tag: str
    assets: dict[str, ReleaseAsset]


@dataclass(frozen=True)
class ProviderResolution:
    binary: str
    command: str
    version: str
    source: str  # path | managed | unresolved
    path: Path | None
    repository: str = PROVIDER_REPOSITORY
    release_tag: str = ""
    target: tuple[str, str] = ("", "")
    verified: bool = False


@dataclass(frozen=True)
class GithubReleasePlan:
    binary: str
    repository: str
    release_policy: str
    min_version: str
    install_url: str
    goos: str
    goarch: str
    cache_root: Path


def detect_platform(system: str | None = None, machine: str | None = None) -> tuple[str, str]:
    """Return the provider's Go target for an OS/machine pair."""
    system = system if system is not None else platform.system()
    machine = machine if machine is not None else platform.machine()
    systems = {"darwin": "darwin", "macos": "darwin", "linux": "linux", "windows": "windows"}
    goos = systems.get(str(system).strip().lower(), "")
    machines = {"arm64": "arm64", "aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64", "x64": "amd64"}
    goarch = machines.get(str(machine).strip().lower(), "")
    target = (goos, goarch)
    if target not in SUPPORTED_PLATFORMS:
        return ("", "")
    return target


def release_asset_name(tag: str, goos: str, goarch: str) -> str:
    if (goos, goarch) not in SUPPORTED_PLATFORMS:
        raise InstallError(f"unsupported provider target: {goos or '?'}/{goarch or '?'}")
    suffix = ".zip" if goos == "windows" else ".tar.gz"
    return f"jinna_{tag}_{goos}_{goarch}{suffix}"


def expected_binary_name(goos: str) -> str:
    return "jinna.exe" if goos == "windows" else "jinna"


def _repository_from_url(url: str) -> str:
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https" or parsed.netloc not in {"github.com", "api.github.com"}:
        raise InstallError("provider release URL must use the allowlisted GitHub HTTPS host")
    parts = [part for part in parsed.path.split("/") if part]
    if len(parts) >= 2 and parts[0] == "repos":
        return "/".join(parts[1:3])
    if len(parts) >= 2:
        return "/".join(parts[:2])
    raise InstallError("provider release URL has no repository")


def parse_release(payload: dict) -> ReleaseInfo:
    if not isinstance(payload, dict):
        raise InstallError("GitHub release metadata must be an object")
    html_url = payload.get("html_url")
    tag = payload.get("tag_name")
    if not isinstance(html_url, str) or _repository_from_url(html_url) != PROVIDER_REPOSITORY:
        raise InstallError("GitHub release belongs to an unexpected repository")
    if not isinstance(tag, str) or not re.fullmatch(r"v[0-9]+(?:\.[0-9]+)+", tag):
        raise InstallError("GitHub release has an invalid stable version tag")
    if payload.get("draft") is True or payload.get("prerelease") is True:
        raise InstallError("provider release is draft or prerelease")
    raw_assets = payload.get("assets", [])
    if not isinstance(raw_assets, list):
        raise InstallError("GitHub release assets must be an array")
    assets: dict[str, ReleaseAsset] = {}
    for raw in raw_assets:
        if not isinstance(raw, dict):
            raise InstallError("GitHub release asset is not an object")
        name = raw.get("name")
        url = raw.get("browser_download_url")
        if not isinstance(name, str) or not name or not isinstance(url, str):
            raise InstallError("GitHub release asset is missing name or download URL")
        if name in assets:
            raise InstallError(f"duplicate GitHub release asset: {name}")
        if _repository_from_url(url) != PROVIDER_REPOSITORY:
            raise InstallError(f"asset is outside the allowlisted repository: {name}")
        assets[name] = ReleaseAsset(name=name, url=url)
    return ReleaseInfo(repository=PROVIDER_REPOSITORY, tag=tag, assets=assets)


def parse_release_metadata(payload: object, tag: str, archive_name: str) -> dict:
    """Validate the RELEASE.json contract for one selected artifact.

    The metadata is provenance, not a trust substitute: it must agree with the
    selected release tag and list the exact archive about to be installed.
    """
    if not isinstance(payload, dict):
        raise InstallError("RELEASE.json must be a JSON object")
    version = payload.get("version")
    if version != tag:
        raise InstallError(f"RELEASE.json version {version!r} does not match release {tag}")
    artifacts = payload.get("artifacts")
    if not isinstance(artifacts, list) or not all(isinstance(name, str) for name in artifacts):
        raise InstallError("RELEASE.json artifacts must be an array of names")
    if len(artifacts) != len(set(artifacts)):
        raise InstallError("RELEASE.json lists a duplicate artifact")
    if archive_name not in artifacts:
        raise InstallError(f"RELEASE.json does not list {archive_name}")
    return payload


def parse_sha256sums(text: str, asset_name: str) -> str:
    found: dict[str, str] = {}
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        match = CHECKSUM_RE.fullmatch(line)
        if not match:
            raise InstallError("malformed SHA256SUMS entry")
        digest, name = match.groups()
        if name in found:
            raise InstallError(f"duplicate SHA256SUMS entry: {name}")
        found[name] = digest.lower()
    digest = found.get(asset_name)
    if digest is None:
        raise InstallError(f"SHA256SUMS has no entry for {asset_name}")
    return digest


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _safe_member_name(name: str, expected_root: str) -> str:
    if not name or "\\" in name:
        raise InstallError("archive contains an invalid member path")
    clean = name[:-1] if name.endswith("/") else name
    if clean.startswith("/"):
        raise InstallError("archive contains an absolute member path")
    parts = clean.split("/")
    if any(part in {"", ".", ".."} for part in parts):
        raise InstallError("archive contains a traversal member path")
    if not clean.startswith(expected_root + "/") and clean != expected_root:
        raise InstallError(f"archive contains an unexpected root: {clean}")
    return clean


def _expected_members(archive_name: str, goos: str) -> tuple[str, set[str]]:
    root = archive_name.removesuffix(".tar.gz").removesuffix(".zip")
    binary = expected_binary_name(goos)
    required = {
        f"{root}/{binary}",
        f"{root}/LICENSE",
        f"{root}/THIRD_PARTY_NOTICES.md",
        f"{root}/README.md",
        f"{root}/docs/mcp.md",
        f"{root}/docs/migration.md",
        f"{root}/docs/operations.md",
    }
    return root, required


def _write_member(destination: Path, relative: str, data: bytes, executable: bool) -> None:
    if len(data) > MAX_MEMBER_BYTES:
        raise InstallError("archive member is too large")
    target = destination / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("xb") as fh:
        fh.write(data)
    if executable:
        target.chmod(0o755)


def extract_release_archive(
    archive_path: Path,
    destination: Path,
    archive_name: str,
    goos: str,
) -> Path:
    """Safely extract the expected provider files and return its binary path."""
    root, required = _expected_members(archive_name, goos)
    seen: set[str] = set()
    destination.mkdir(parents=True, exist_ok=True)

    def validate(name: str) -> str:
        clean = _safe_member_name(name, root)
        if clean in seen:
            raise InstallError(f"archive contains a duplicate member: {clean}")
        seen.add(clean)
        if clean not in required and clean not in {root, f"{root}/docs"}:
            raise InstallError(f"archive contains an unexpected member: {clean}")
        return clean

    try:
        if archive_name.endswith(".tar.gz"):
            with tarfile.open(archive_path, "r:gz") as archive:
                for member in archive.getmembers():
                    clean = validate(member.name)
                    if member.isdir():
                        continue
                    if member.issym() or member.islnk() or not member.isfile():
                        raise InstallError(f"archive member is not a regular file: {clean}")
                    source = archive.extractfile(member)
                    if source is None:
                        raise InstallError(f"archive member cannot be read: {clean}")
                    data = source.read(MAX_MEMBER_BYTES + 1)
                    _write_member(
                        destination,
                        clean,
                        data,
                        executable=clean == f"{root}/{expected_binary_name(goos)}",
                    )
        elif archive_name.endswith(".zip"):
            with zipfile.ZipFile(archive_path) as archive:
                for member in archive.infolist():
                    clean = validate(member.filename)
                    mode = (member.external_attr >> 16) & 0o170000
                    if mode == stat.S_IFLNK:
                        raise InstallError(f"archive member is a symbolic link: {clean}")
                    if member.is_dir():
                        continue
                    if member.file_size > MAX_MEMBER_BYTES:
                        raise InstallError(f"archive member is too large: {clean}")
                    data = archive.read(member)
                    _write_member(
                        destination,
                        clean,
                        data,
                        executable=clean == f"{root}/{expected_binary_name(goos)}",
                    )
        else:
            raise InstallError(f"unsupported provider archive: {archive_name}")
    except (OSError, tarfile.TarError, zipfile.BadZipFile) as exc:
        raise InstallError(f"cannot inspect provider archive: {exc}") from exc

    missing = sorted(required - seen)
    if missing:
        raise InstallError("provider archive is missing: " + ", ".join(missing))
    binary = destination / root / expected_binary_name(goos)
    if not binary.is_file():
        raise InstallError("provider archive did not produce an executable")
    return binary


def _version_tuple(text: str) -> tuple[int, ...]:
    match = VERSION_RE.search(text or "")
    return tuple(int(part) for part in match.group(1).split(".")) if match else ()


def _version_at_least(have: str, minimum: str) -> bool:
    want = _version_tuple(minimum)
    current = _version_tuple(have)
    return not want or (bool(current) and current >= want)


def _run_version(path: Path, timeout: float = 5.0) -> str:
    try:
        proc = subprocess.run(
            [str(path), "version"],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise InstallError(f"provider version check failed: {exc}") from exc
    output = (proc.stdout or "") + (proc.stderr or "")
    version = _version_tuple(output)
    if proc.returncode != 0 or not version:
        raise InstallError("provider version check returned no usable version")
    return ".".join(str(part) for part in version)


def cache_root(ai_specs_home: Path | None = None) -> Path:
    if ai_specs_home is None:
        configured = os.environ.get("AI_SPECS_HOME")
        if configured:
            ai_specs_home = Path(configured)
        else:
            ai_specs_home = Path.home() / ".cache" / "ai-specs"
    return Path(ai_specs_home).expanduser().resolve() / "cache" / "bin" / "jinna"


def managed_binary_path(root: Path, version: str, goos: str, goarch: str) -> Path:
    return root / version / f"{goos}-{goarch}" / expected_binary_name(goos)


def _read_receipt(path: Path) -> dict[str, object]:
    receipt = path.parent / "install.json"
    try:
        data = json.loads(receipt.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def _managed_candidates(root: Path, goos: str, goarch: str) -> Iterable[Path]:
    target = f"{goos}-{goarch}"
    try:
        if not root.is_dir():
            return ()
        version_dirs = list(root.iterdir())
    except OSError:
        return ()
    paths: list[Path] = []
    for version_dir in version_dirs:
        candidate = version_dir / target / expected_binary_name(goos)
        try:
            if candidate.is_file():
                paths.append(candidate)
        except OSError:
            continue
    return paths


def resolve_provider(dep, ai_specs_home: Path | None = None) -> ProviderResolution:
    """Resolve a GitHub-release provider without network or filesystem mutation."""
    goos, goarch = detect_platform()
    binary = str(getattr(dep, "binary", "jinna"))
    minimum = str(getattr(dep, "min_version", ""))
    path_candidate = shutil.which(binary)
    if path_candidate:
        try:
            version = _run_version(Path(path_candidate))
            if _version_at_least(version, minimum):
                return ProviderResolution(binary, binary, version, "path", Path(path_candidate), target=(goos, goarch), verified=True)
        except InstallError:
            pass

    root = cache_root(ai_specs_home)
    target = f"{goos}-{goarch}"
    try:
        managed_candidates = list(_managed_candidates(root, goos, goarch))
    except OSError:
        # An unreadable cache must degrade to "unresolved", never abort a sync.
        managed_candidates = []
    candidates: list[tuple[tuple[int, ...], Path, str]] = []
    for candidate in managed_candidates:
        receipt = _read_receipt(candidate)
        if receipt.get("status") != "verified":
            continue
        if receipt.get("repository") != PROVIDER_REPOSITORY:
            continue
        if receipt.get("target") != target:
            continue
        tag = receipt.get("release_tag")
        if not isinstance(tag, str) or not _version_tuple(tag):
            continue
        expected_digest = receipt.get("binary_sha256")
        if not isinstance(expected_digest, str):
            continue
        try:
            if _sha256(candidate) != expected_digest:
                continue
            version = _run_version(candidate)
        except (InstallError, OSError):
            continue
        if not _version_at_least(version, minimum):
            continue
        if _version_tuple(version) != _version_tuple(tag):
            # A receipt that disagrees with the executable is not a trust anchor.
            continue
        candidates.append((_version_tuple(version), candidate, tag))
    if candidates:
        _, candidate, tag = max(candidates, key=lambda item: item[0])
        version = _run_version(candidate)
        return ProviderResolution(binary, str(candidate), version, "managed", candidate, release_tag=tag, target=(goos, goarch), verified=True)
    return ProviderResolution(binary, binary, "", "unresolved", None, target=(goos, goarch), verified=False)


def build_release_plan(dep, ai_specs_home: Path | None = None) -> GithubReleasePlan:
    repository = str(getattr(dep, "repository", ""))
    policy = str(getattr(dep, "release_policy", ""))
    installer = str(getattr(dep, "installer", ""))
    if installer != "github-release" or repository != PROVIDER_REPOSITORY:
        raise InstallError("provider dependency is not the allowlisted GitHub Release installer")
    if policy != "latest-stable":
        raise InstallError("provider release policy must be latest-stable")
    goos, goarch = detect_platform()
    install_url = str(getattr(dep, "install_url", ""))
    if not goos or not goarch:
        supported = ", ".join(
            f"{supported_goos}/{supported_goarch}"
            for supported_goos, supported_goarch in SUPPORTED_PLATFORMS
        )
        guidance = f" see {install_url} for manual installation" if install_url else ""
        raise InstallError(
            f"provider has no artifact for this platform; supported targets: {supported}.{guidance}"
        )
    return GithubReleasePlan(
        binary=str(getattr(dep, "binary", "jinna")),
        repository=repository,
        release_policy=policy,
        min_version=str(getattr(dep, "min_version", "")),
        install_url=install_url,
        goos=goos,
        goarch=goarch,
        cache_root=cache_root(ai_specs_home),
    )


def _fetch_bytes(url: str, *, max_bytes: int = MAX_HTTP_BYTES, timeout: float = 15.0) -> bytes:
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != "https" or parsed.netloc not in {"api.github.com", "github.com"}:
        raise InstallError("provider download URL is not an allowlisted HTTPS URL")
    request = urllib.request.Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "User-Agent": "ai-specs-jinna-mcp-recipe",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = response.read(min(1024 * 1024, max_bytes - total + 1))
                if not chunk:
                    break
                chunks.append(chunk)
                total += len(chunk)
                if total > max_bytes:
                    raise InstallError("provider response exceeds the size limit")
            return b"".join(chunks)
    except InstallError:
        raise
    except (OSError, urllib.error.URLError) as exc:
        raise InstallError(f"provider download failed: {exc}") from exc


def install_github_release(
    plan: GithubReleasePlan,
    *,
    fetch: Callable[[str], bytes] = _fetch_bytes,
    run_version: Callable[[Path], str] = _run_version,
) -> ProviderResolution:
    """Acquire, verify, self-test, and atomically publish one provider target."""
    try:
        release_payload = json.loads(
            fetch(GITHUB_API_BASE + "/releases/latest").decode("utf-8")
        )
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise InstallError("provider release metadata is not valid JSON") from exc
    release = parse_release(release_payload)
    expected_version = ".".join(str(part) for part in _version_tuple(release.tag))
    if not expected_version:
        raise InstallError(f"provider release tag is not a version: {release.tag}")
    archive_name = release_asset_name(release.tag, plan.goos, plan.goarch)
    archive_asset = release.assets.get(archive_name)
    sums_asset = release.assets.get("SHA256SUMS")
    metadata_asset = release.assets.get("RELEASE.json")
    if archive_asset is None or sums_asset is None or metadata_asset is None:
        raise InstallError(
            f"release {release.tag} lacks {archive_name}, SHA256SUMS, or RELEASE.json"
        )
    metadata_raw = fetch(metadata_asset.url)
    if len(metadata_raw) > MAX_METADATA_BYTES:
        raise InstallError("provider RELEASE.json exceeds the size limit")
    try:
        metadata_text = metadata_raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise InstallError("provider RELEASE.json is not valid UTF-8") from exc
    try:
        metadata_payload = json.loads(metadata_text)
    except json.JSONDecodeError as exc:
        raise InstallError("provider RELEASE.json is not valid JSON") from exc
    parse_release_metadata(metadata_payload, release.tag, archive_name)
    try:
        sums_text = fetch(sums_asset.url).decode("utf-8")
    except UnicodeDecodeError as exc:
        raise InstallError("provider SHA256SUMS manifest is not valid UTF-8") from exc
    expected_digest = parse_sha256sums(sums_text, archive_name)

    target_dir = plan.cache_root / release.tag / f"{plan.goos}-{plan.goarch}"
    target_dir.parent.mkdir(parents=True, exist_ok=True)
    temp_parent = Path(tempfile.mkdtemp(prefix=".jinna-", dir=str(target_dir.parent)))
    try:
        archive_path = temp_parent / archive_name
        archive_path.write_bytes(fetch(archive_asset.url))
        observed_digest = _sha256(archive_path)
        if observed_digest != expected_digest:
            raise InstallError(
                f"provider checksum mismatch: expected {expected_digest}, got {observed_digest}"
            )
        extracted = extract_release_archive(
            archive_path,
            temp_parent / "extracted",
            archive_name,
            plan.goos,
        )
        version = run_version(extracted)
        if version != expected_version:
            raise InstallError(
                f"provider version {version} does not match release {release.tag}"
            )
        if not _version_at_least(version, plan.min_version):
            raise InstallError(f"provider version {version} is below required {plan.min_version}")
        final_binary = temp_parent / "extracted" / archive_name.removesuffix(".tar.gz").removesuffix(".zip") / expected_binary_name(plan.goos)
        receipt = {
            "status": "verified",
            "repository": PROVIDER_REPOSITORY,
            "release_tag": release.tag,
            "target": f"{plan.goos}-{plan.goarch}",
            "archive": archive_name,
            "archive_sha256": observed_digest,
            "binary_sha256": _sha256(final_binary),
        }
        receipt_path = temp_parent / "extracted" / archive_name.removesuffix(".tar.gz").removesuffix(".zip") / "install.json"
        receipt_path.write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")
        if target_dir.exists():
            raise InstallError(f"managed provider target already exists: {target_dir}")
        os.replace(
            temp_parent / "extracted" / archive_name.removesuffix(".tar.gz").removesuffix(".zip"),
            target_dir,
        )
        installed = target_dir / expected_binary_name(plan.goos)
        return ProviderResolution(
            plan.binary,
            str(installed),
            version,
            "managed",
            installed,
            release_tag=release.tag,
            target=(plan.goos, plan.goarch),
            verified=True,
        )
    except (OSError, json.JSONDecodeError) as exc:
        raise InstallError(f"provider installation failed: {exc}") from exc
    finally:
        shutil.rmtree(temp_parent, ignore_errors=True)

#!/usr/bin/env python3
"""Gate binary acquisition, verification and cache layout (design §6).

`ai-specs sync` calls this when the worktree-flow recipe is enabled and
`gate_impl` is `auto` or `go`. The acquired binary lives at

    $AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate

so each CLI version resolves its own binary and an upgrade naturally
acquires a new one while an older CLI keeps working against its own.

Invariants (spec "Binary acquisition, verification and cache layout"):

- Digest before execution, always. The expected SHA-256 comes from the
  committed `catalog/recipes/worktree-flow/bin/SHA256SUMS` — the trust root
  (D5). A mismatched asset is quarantined (or deleted when no prior candidate
  exists), warned about, never executed, and the mismatch is recorded for
  `doctor`.
- Atomic install. Download to a temp file in the destination directory,
  verify, `chmod 0755`, then `os.replace` into place. A partial download can
  never be executed.
- Never selects unverified bytes. Every failure warns and degrades; the gate
  falls back to the frozen Bash implementation (`gate_impl=auto`) or fails
  open (`gate_impl=go`) with a recorded `doctor` ERROR.
- Opt-in local build. `AI_SPECS_GATE_BUILD=1`, or offline with a Go toolchain
  present, builds from the in-repo Go source into the same cache layout.
  A Go toolchain is NEVER a user prerequisite for install or use.

This module is imported by recipe-materialize.py (same directory), so it uses
only stdlib and sibling-loading helpers.
"""
from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

# Version-keyed cache root beneath the AI_SPECS_HOME cache.
CACHE_REL = Path("cache") / "bin" / "worktree-gate"

# Supported release matrix (spec "Multi-arch build matrix"). A platform
# outside this matrix gets no binary target (launcher resolution step 3 is a
# no-op) and acquisition warns naming the alternatives.
SUPPORTED_PLATFORMS = (
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
)

# Mismatch records live under the same version-keyed cache directory so a
# doctor run on the same CLI version reads them.
MISMATCH_FILENAME = "last-digest-mismatch.txt"
VERIFICATION_SUFFIX = ".verified"

# Canonical GitHub repository owner of ai-specs-cli. This is the owner used
# everywhere else in the product (the git remote, install.sh, bin/ai-specs,
# catalog/README.md, and the release workflow); the released worktree-gate
# assets are attached to this repository's GitHub Releases, so the download
# URL must use it or every sync acquisition 404s.
REPO_OWNER = "parada1104"
REPO_NAME = "ai-specs-cli"


def _ai_specs_home() -> Path:
    env = os.environ.get("AI_SPECS_HOME")
    if env:
        return Path(env).resolve()
    return Path(__file__).resolve().parents[2]


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = __import__("importlib.util").util.spec_from_file_location(
        name.replace("-", "_"), path
    )
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = __import__("importlib.util").util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def detect_platform(uname_s: str | None = None, uname_m: str | None = None) -> tuple[str, str]:
    """Map `uname -s` / `uname -m` to a (goos, goarch) pair.

    `Darwin`→`darwin`, `Linux`→`linux`; `arm64|aarch64`→`arm64`,
    `x86_64|amd64`→`amd64`. Anything else → `("", "")`. A Rosetta-translated
    shell on Apple Silicon reports `x86_64`, which selects darwin-amd64 —
    correct, merely slower (design §5.1).
    """
    import platform as _platform

    uname_s = uname_s if uname_s is not None else _platform.system()
    uname_m = uname_m if uname_m is not None else _platform.machine()
    goos = ""
    if uname_s == "Darwin":
        goos = "darwin"
    elif uname_s == "Linux":
        goos = "linux"
    goarch = ""
    if uname_m in ("arm64", "aarch64"):
        goarch = "arm64"
    elif uname_m in ("x86_64", "amd64"):
        goarch = "amd64"
    return goos, goarch


def cli_version(cli_home: Path | None = None) -> str:
    """Installed CLI version (the version key for the cache layout)."""
    home = cli_home if cli_home is not None else _ai_specs_home()
    version_path = home / "VERSION"
    if not version_path.is_file():
        return "dev"
    text = version_path.read_text(encoding="utf-8").strip()
    return text or "dev"


def cache_bin_path(
    cli_home: Path | None = None,
    version: str | None = None,
    goos: str | None = None,
    goarch: str | None = None,
) -> Path:
    """Version-keyed cache path for the host platform binary."""
    home = cli_home if cli_home is not None else _ai_specs_home()
    ver = version if version is not None else cli_version(home)
    goos = goos if goos is not None else detect_platform()[0]
    goarch = goarch if goarch is not None else detect_platform()[1]
    return home / CACHE_REL / ver / f"{goos}-{goarch}" / "worktree-gate"


def digest_mismatch_record_path(
    cli_home: Path | None = None,
    version: str | None = None,
) -> Path:
    home = cli_home if cli_home is not None else _ai_specs_home()
    ver = version if version is not None else cli_version(home)
    return home / CACHE_REL / ver / MISMATCH_FILENAME


def record_digest_mismatch(message: str, cli_home: Path | None = None, version: str | None = None) -> None:
    """Persist the last acquisition mismatch for the doctor check."""
    try:
        path = digest_mismatch_record_path(cli_home, version)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(message + "\n", encoding="utf-8")
    except OSError:
        pass


def clear_digest_mismatch(cli_home: Path | None = None, version: str | None = None) -> None:
    """Clear a prior rejection once a replacement is fully verified."""
    try:
        digest_mismatch_record_path(cli_home, version).unlink()
    except FileNotFoundError:
        pass
    except OSError:
        # A stale diagnostic must not make a verified asset executable failure.
        pass


def load_expected_digests(ai_specs_home: Path) -> dict[str, str]:
    """Parse the committed SHA256SUMS trust root.

    Returns {asset_name: hex_digest} for lines matching
    `<64-hex>  worktree-gate-<goos>-<goarch>`. Comment/blank lines ignored.
    """
    sums_path = ai_specs_home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS"
    digests: dict[str, str] = {}
    if not sums_path.is_file():
        return digests
    for line in sums_path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            continue
        digest, name = parts
        if len(digest) == 64 and name.startswith("worktree-gate-"):
            digests[name] = digest.lower()
    return digests


def _sha256_of(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verification_record_path(binary_path: Path) -> Path:
    """Sidecar carrying the latest acquisition verification receipt."""
    return binary_path.with_name(binary_path.name + VERIFICATION_SUFFIX)


def _write_verification_record(binary_path: Path, version: str, digest: str) -> None:
    """Atomically write the evidence consumed by the launcher hot path."""
    path = verification_record_path(binary_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(
        dir=str(path.parent), prefix=f".{path.name}.", suffix=".tmp"
    )
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write("status=verified\n")
            fh.write(f"version={version}\n")
            fh.write(f"digest={digest}\n")
            fh.write("selftest=passed\n")
        os.replace(tmp, path)
    except BaseException:
        try:
            tmp.unlink()
        except OSError:
            pass
        raise


def read_verification_record(binary_path: Path) -> dict[str, str]:
    """Read a sidecar receipt without treating malformed data as evidence."""
    path = verification_record_path(binary_path)
    if not path.is_file():
        return {}
    result: dict[str, str] = {}
    try:
        for line in path.read_text(encoding="utf-8").splitlines():
            key, separator, value = line.partition("=")
            if separator and key and value:
                result[key] = value
    except OSError:
        return {}
    return result


def verify_cached_binary(
    binary_path: Path,
    ai_specs_home: Path,
    version: str,
    goos: str,
    goarch: str,
    *,
    require_receipt: bool = False,
    allow_missing_digest: bool = False,
) -> dict[str, object]:
    """Revalidate cache bytes against digest, version, self-test, and receipt."""
    asset_name = f"worktree-gate-{goos}-{goarch}"
    expected = load_expected_digests(ai_specs_home).get(asset_name)
    evidence: dict[str, object] = {
        "verified": False,
        "expected_digest": expected or "",
        "observed_digest": "",
        "version": "",
        "selftest": "not_run",
        "receipt": str(verification_record_path(binary_path)),
        "reason": "",
    }
    reasons: list[str] = []
    if not binary_path.is_file() or not os.access(binary_path, os.X_OK):
        evidence["reason"] = "cache candidate is missing or not executable"
        return evidence
    try:
        observed = _sha256_of(binary_path)
    except OSError as exc:
        evidence["reason"] = f"cache digest read failed: {exc}"
        return evidence
    evidence["observed_digest"] = observed
    if expected is None and not allow_missing_digest:
        evidence["reason"] = f"no committed digest for {asset_name}"
        return evidence
    if expected is not None and observed != expected:
        evidence["reason"] = f"digest mismatch: expected {expected}, got {observed}"
        return evidence

    # Only execute version/self-test after the trust-root digest matches. A
    # stale or unknown executable must never be run merely to inspect it.
    reported_version = binary_version(binary_path)
    selftest = _run_selftest(binary_path)
    evidence["version"] = reported_version
    evidence["selftest"] = "passed" if selftest is None else selftest
    if expected is None and not allow_missing_digest:
        reasons.append(f"no committed digest for {asset_name}")
    elif expected is not None and observed != expected:
        reasons.append(f"digest mismatch: expected {expected}, got {observed}")
    if reported_version != version:
        reasons.append(
            f"version mismatch: expected {version}, got {reported_version or 'unknown'}"
        )
    if selftest is not None:
        reasons.append(f"self-test failed: {selftest}")
    receipt = read_verification_record(binary_path)
    if require_receipt:
        if receipt.get("status") != "verified":
            reasons.append("verification receipt is missing a verified status")
        if receipt.get("version") != version:
            reasons.append("verification receipt version is missing or stale")
        if receipt.get("digest") != observed:
            reasons.append("verification receipt digest is missing or stale")
        if receipt.get("selftest") != "passed":
            reasons.append("verification receipt does not record a passing self-test")
    evidence["reason"] = "; ".join(reasons) if reasons else "digest, version, and self-test verified"
    evidence["verified"] = not reasons
    return evidence


def _quarantine_cached_candidate(binary_path: Path) -> Path | None:
    """Move rejected cache bytes aside so the executable path is never selected."""
    if not binary_path.exists():
        return None
    try:
        digest = _sha256_of(binary_path)
    except OSError:
        digest = "unknown"
    rejected_dir = binary_path.parent / "rejected"
    rejected_dir.mkdir(parents=True, exist_ok=True)
    rejected = rejected_dir / f"{binary_path.name}-{digest}"
    os.replace(binary_path, rejected)
    receipt = verification_record_path(binary_path)
    if receipt.exists():
        os.replace(receipt, verification_record_path(rejected))
    return rejected


def _local_build(binary_path: Path, ai_specs_home: Path, version: str, goos: str, goarch: str) -> tuple[bool, str]:
    """Build the in-repo Go source into the destination.

    Returns (ok, detail). Never touches network. The module has zero
    third-party dependencies, so `go build` is hermetic.
    """
    gate_dir = ai_specs_home / "catalog" / "recipes" / "worktree-flow" / "gate"
    if not (gate_dir / "go.mod").is_file():
        return False, f"gate Go source not found at {gate_dir}"
    if shutil.which("go") is None:
        return False, "go toolchain not found on PATH"
    binary_path.parent.mkdir(parents=True, exist_ok=True)
    tmp = binary_path.with_name(f".{binary_path.name}.build-tmp")
    try:
        env = dict(os.environ)
        env["CGO_ENABLED"] = "0"
        env["GOOS"] = goos
        env["GOARCH"] = goarch
        proc = subprocess.run(
            [
                "go", "build", "-trimpath", "-buildvcs=false",
                "-ldflags", f"-s -w -X main.version={version}",
                "-o", str(tmp), ".",
            ],
            cwd=str(gate_dir),
            env=env,
            capture_output=True,
            text=True,
            timeout=300,
        )
        if proc.returncode != 0:
            return False, (proc.stderr or proc.stdout or "go build failed").strip()
        os.chmod(tmp, 0o755)
        os.replace(tmp, binary_path)
        return True, ""
    except Exception as exc:  # noqa: BLE001
        try:
            if tmp.exists():
                tmp.unlink()
        except OSError:
            pass
        return False, f"{type(exc).__name__}: {exc}"


def acquire(
    *,
    gate_impl: str,
    ai_specs_home: Path | None = None,
    offline: bool = False,
) -> dict:
    """Acquire the gate binary for the host platform (design §6.2).

    Returns a status dict consumed by sync and doctor:

        {
          "attempted": bool,      # platform in matrix and impl != bash
          "installed": bool,      # usable binary now present at cache path
          "platform": (goos, goarch),
          "cache_path": str,
          "warn": str | None,     # single warning line for sync (degradation)
          "mismatch": str | None, # recorded digest mismatch (doctor ERROR)
        }

    Never raises. Every failure warns and degrades: `gate_impl=auto` falls
    back to the legacy Bash implementation (the launcher decides); `go` fails
    open with a recorded doctor ERROR.
    """
    home = ai_specs_home if ai_specs_home is not None else _ai_specs_home()
    version = cli_version(home)
    goos, goarch = detect_platform()
    out: dict = {
        "attempted": False,
        "installed": False,
        "platform": (goos, goarch),
        "cache_path": "",
        "warn": None,
        "mismatch": None,
    }

    if gate_impl == "bash":
        return out

    # Platform outside the published matrix (or unknown) → no binary target.
    if not goos or not goarch or (goos, goarch) not in SUPPORTED_PLATFORMS:
        out["attempted"] = True
        out["warn"] = (
            f"worktree-gate: unsupported platform ({goos or '?'}/{goarch or '?'}); "
            "no binary acquired — available: darwin/arm64, darwin/amd64, "
            "linux/amd64, linux/arm64"
        )
        return out

    binary_path = cache_bin_path(home, version, goos, goarch)
    out["cache_path"] = str(binary_path)
    out["attempted"] = True

    asset_name = f"worktree-gate-{goos}-{goarch}"
    expected = load_expected_digests(home).get(asset_name)
    cache_rejection: str | None = None

    # Cache hits are evidence candidates, not acceptance. Revalidate every
    # current digest/version/self-test before writing a launcher receipt.
    if binary_path.is_file() and os.access(binary_path, os.X_OK):
        evidence = verify_cached_binary(
            binary_path, home, version, goos, goarch, require_receipt=False
        )
        out["verification"] = evidence
        if evidence["verified"]:
            try:
                _write_verification_record(
                    binary_path, version, str(evidence["observed_digest"])
                )
            except Exception as exc:  # noqa: BLE001
                mismatch = (
                    f"worktree-gate: verification receipt write failed for {binary_path}: "
                    f"{type(exc).__name__}: {exc}; candidate never executed"
                )
                record_digest_mismatch(mismatch, home, version)
                out["mismatch"] = mismatch
                out["warn"] = mismatch
                return out
            clear_digest_mismatch(home, version)
            out["mismatch"] = None
            out["installed"] = True
            return out

        reason = str(evidence.get("reason") or "verification failed")
        recovery = _quarantine_cached_candidate(binary_path)
        recovery_text = str(recovery) if recovery is not None else "none"
        cache_rejection = (
            f"worktree-gate: rejected cached candidate at {binary_path}: {reason}; "
            f"recovery={recovery_text}; candidate never executed"
        )
        record_digest_mismatch(cache_rejection, home, version)
        out["mismatch"] = cache_rejection

    # Opt-in local build: AI_SPECS_GATE_BUILD=1, or offline with go present.
    want_build = os.environ.get("AI_SPECS_GATE_BUILD") == "1"
    if want_build or (offline and shutil.which("go") is not None):
        ok, detail = _local_build(binary_path, home, version, goos, goarch)
        if ok:
            evidence = verify_cached_binary(
                binary_path,
                home,
                version,
                goos,
                goarch,
                require_receipt=False,
                allow_missing_digest=True,
            )
            local_digest_ok = expected is None or evidence["observed_digest"] == expected
            local_version_ok = evidence["version"] == version
            local_selftest_ok = evidence["selftest"] == "passed"
            if local_digest_ok and local_version_ok and local_selftest_ok:
                try:
                    _write_verification_record(
                        binary_path, version, str(evidence["observed_digest"])
                    )
                except Exception as exc:  # noqa: BLE001
                    mismatch = (
                        f"worktree-gate: local verification receipt write failed at {binary_path}: "
                        f"{type(exc).__name__}: {exc}; candidate never executed"
                    )
                    record_digest_mismatch(mismatch, home, version)
                    out["mismatch"] = mismatch
                    out["warn"] = mismatch
                    return out
                clear_digest_mismatch(home, version)
                out["mismatch"] = None
                out["installed"] = True
                out["verification"] = evidence
                return out
            reason = str(evidence.get("reason") or "local build verification failed")
            recovery = _quarantine_cached_candidate(binary_path)
            mismatch = (
                f"worktree-gate: locally built binary rejected at {binary_path}: {reason}; "
                f"recovery={recovery or 'none'}; candidate never executed"
            )
            record_digest_mismatch(mismatch, home, version)
            out["mismatch"] = mismatch
            out["warn"] = mismatch
            return out
        out["warn"] = (
            f"worktree-gate: local build failed ({detail}); "
            + _degradation_hint(gate_impl)
        )
        return out

    if offline:
        out["warn"] = (
            (cache_rejection + "; " if cache_rejection else "")
            + "worktree-gate: offline with no cached binary; "
            + _degradation_hint(gate_impl)
        )
        return out

    if expected is None:
        out["warn"] = (
            f"worktree-gate: no committed digest for {asset_name} in "
            "catalog/recipes/worktree-flow/bin/SHA256SUMS; "
            + _degradation_hint(gate_impl)
        )
        return out

    url = _asset_url(version, asset_name)
    binary_path.parent.mkdir(parents=True, exist_ok=True)
    tmp = binary_path.with_name(f".{binary_path.name}.download")
    try:
        try:
            urllib.request.urlretrieve(url, tmp)
        except (urllib.error.URLError, OSError) as exc:
            out["warn"] = (
                f"worktree-gate: download failed from {url} ({exc}); "
                + _degradation_hint(gate_impl)
            )
            return out
        actual = _sha256_of(tmp)
        if actual != expected:
            try:
                tmp.unlink()
            except OSError:
                pass
            mismatch = (
                f"worktree-gate: digest mismatch for {asset_name}: expected "
                f"{expected}, got {actual}; artifact deleted and never executed"
            )
            record_digest_mismatch(mismatch, home, version)
            out["mismatch"] = mismatch
            out["warn"] = mismatch
            return out
        os.chmod(tmp, 0o755)
        selftest = _run_selftest(tmp)
        reported_version = binary_version(tmp)
        if selftest is not None or reported_version != version:
            mismatch = (
                f"worktree-gate: downloaded binary rejected for {asset_name}: "
                f"version={reported_version or 'unknown'} expected_version={version}; "
                f"selftest={'passed' if selftest is None else selftest}; "
                "artifact never installed"
            )
            record_digest_mismatch(mismatch, home, version)
            out["mismatch"] = mismatch
            out["warn"] = mismatch
            out["verification"] = {
                "verified": False,
                "expected_digest": expected,
                "observed_digest": actual,
                "version": reported_version,
                "selftest": "passed" if selftest is None else selftest,
                "reason": mismatch,
            }
            return out
        os.replace(tmp, binary_path)
        try:
            _write_verification_record(binary_path, version, actual)
        except Exception as exc:  # noqa: BLE001
            try:
                binary_path.unlink()
            except OSError:
                pass
            mismatch = (
                f"worktree-gate: verification receipt write failed for {binary_path}: "
                f"{type(exc).__name__}: {exc}; candidate never executed"
            )
            record_digest_mismatch(mismatch, home, version)
            out["mismatch"] = mismatch
            out["warn"] = mismatch
            return out
        out["installed"] = True
        clear_digest_mismatch(home, version)
        out["mismatch"] = None
        out["verification"] = {
            "verified": True,
            "expected_digest": expected,
            "observed_digest": actual,
            "version": reported_version,
            "selftest": "passed",
            "receipt": str(verification_record_path(binary_path)),
            "reason": (
                "re-acquired after " + cache_rejection
                if cache_rejection else "acquired and verified"
            ),
        }
        return out
    finally:
        try:
            if tmp.exists():
                tmp.unlink()
        except OSError:
            pass


def _degradation_hint(gate_impl: str) -> str:
    if gate_impl == "go":
        return "gate_impl=go: gate is failing open (run 'ai-specs doctor')"
    return "gate_impl=auto: falling back to the Bash implementation"


def _asset_url(version: str, asset_name: str) -> str:
    return (
        f"https://github.com/{REPO_OWNER}/{REPO_NAME}/releases/download/"
        f"v{version}/{asset_name}"
    )


def _run_selftest(binary_path: Path) -> str | None:
    """Run `--selftest`; returns None on success, else the failure text."""
    try:
        proc = subprocess.run(
            [str(binary_path), "--selftest"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except Exception as exc:  # noqa: BLE001
        return f"{type(exc).__name__}: {exc}"
    if proc.returncode != 0:
        return (proc.stderr or proc.stdout or "selftest failed").strip()
    return None


def cache_size(cli_home: Path | None = None) -> int:
    """Total bytes under the version-keyed cache root (doctor reporting)."""
    home = cli_home if cli_home is not None else _ai_specs_home()
    root = home / CACHE_REL
    if not root.is_dir():
        return 0
    total = 0
    for p in root.rglob("*"):
        if p.is_file():
            try:
                total += p.stat().st_size
            except OSError:
                pass
    return total


def binary_version(binary_path: Path) -> str:
    """Read the binary's `--version` output; "" on any failure."""
    try:
        proc = subprocess.run(
            [str(binary_path), "--version"],
            capture_output=True,
            text=True,
            timeout=10,
        )
    except Exception:
        return ""
    if proc.returncode != 0:
        return ""
    return (proc.stdout or "").strip()

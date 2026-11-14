"""GO-20: the repo launcher self-heals a missing native binary.

An `ai-specs upgrade` run from an old checkout only fast-forwards the git tree:
the entry point that is already installed keeps running, and the new tree
arrives without `dist/ai-specs`. A launcher that only execs that file leaves
every verb dying with "native binary not found". So `bin/ai-specs` acquires the
binary itself, once: a SHA-256-verified release asset through the same trust
root and release base `install.sh` uses, or a local `go build`.

Every case is hermetic. A local HTTP server stands in for the release
download, the trust root is a file inside the fixture home, and PATH is a
curated symlink farm with no Go toolchain (unless the case stubs `go`), so no
test can reach the network or a real build.
"""
from __future__ import annotations
import hashlib
import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

from test_install_binary_entry import _Server, _platform_asset


ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "bin" / "ai-specs"
BASH = shutil.which("bash") or "/bin/bash"
INSTALL_URL = (
    "https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh"
)

# Every external command the launcher may reach for.
_TOOLS = (
    "awk", "bash", "cat", "chmod", "cp", "curl", "dirname", "grep", "head",
    "ls", "mkdir", "mktemp", "mv", "readlink", "rm", "sed", "sha256sum",
    "shasum", "sh", "tr", "uname",
)

# A stand-in native binary: it proves dispatch and verbose output separation.
PAYLOAD = b'#!/bin/sh\necho "native $*"\necho "native-stderr" >&2\nexit 3\n'


def _tool_path(farm: Path) -> str:
    """A PATH holding only the launcher's tools: no `go`, no surprises."""
    farm.mkdir(parents=True, exist_ok=True)
    for name in _TOOLS:
        real = shutil.which(name)
        if real is not None and not (farm / name).exists():
            (farm / name).symlink_to(real)
    return str(farm)


class _CountingServer(_Server):
    """`_Server` that records every request path it serves."""

    def __init__(self, directory: Path):
        self.requests: list[str] = []
        requests = self.requests

        def make(directory: Path):
            class Handler(_Server._handler(directory)):
                def do_GET(self) -> None:
                    requests.append(self.path)
                    super().do_GET()

            return Handler

        self._handler = make  # type: ignore[method-assign]
        super().__init__(directory)


def _go_shim(farm: Path, fake_output: Path) -> None:
    """A `go` that ignores its work and drops a known file at `-o`."""
    shim = farm / "go"
    shim.write_text(
        "#!/bin/sh\n"
        'out=""\n'
        'prev=""\n'
        'for a in "$@"; do\n'
        '    if [ "$prev" = "-o" ]; then out="$a"; fi\n'
        '    prev="$a"\n'
        "done\n"
        '[ -n "$out" ] || { echo "go shim: no -o" >&2; exit 9; }\n'
        f'cp "{fake_output}" "$out" || exit 9\n',
        encoding="utf-8",
    )
    shim.chmod(0o755)


class LauncherSelfHealTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory(prefix="ai-specs-heal-")
        self.addCleanup(self._tmp.cleanup)
        # Resolved: the launcher canonicalizes the home directory, so an
        # expectation built from a symlinked temp path would not compare.
        self.base = Path(self._tmp.name).resolve()
        self.home = self.base / "home"
        (self.home / "bin").mkdir(parents=True)
        (self.home / "VERSION").write_text("0.25.0\n", encoding="utf-8")
        (self.base / "userhome").mkdir()
        self.asset = _platform_asset()
        self.assets = self.base / "assets"
        self.assets.mkdir()
        self.server = _CountingServer(self.assets)
        self.addCleanup(self.server.close)
        self.dist = self.home / "dist" / "ai-specs"
        self.tool_path = _tool_path(self.base / "path")

    # -- fixtures ---------------------------------------------------------
    def _serve(self) -> None:
        (self.assets / self.asset).write_bytes(PAYLOAD)

    def _trust(self, digest: str, home: Path | None = None,
               asset: str | None = None) -> None:
        home = home or self.home
        (home / "bin" / "SHA256SUMS").write_text(
            f"{digest}  {asset or self.asset}\n", encoding="utf-8")

    def _run(self, *args: str, env_extra: dict | None = None,
             path: str | None = None):
        env = {
            "PATH": path or self.tool_path,
            "HOME": str(self.base / "userhome"),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_RELEASE_BASE": self.server.base_url,
            "LC_ALL": "C",
            "LANG": "C",
        }
        if env_extra:
            env.update(env_extra)
        return subprocess.run([BASH, str(LAUNCHER), *args], env=env,
                              capture_output=True, text=True, check=False,
                              timeout=120)

    def _fetch_line(self, home: Path | None = None) -> str:
        home = home or self.home
        return (f"ai-specs: no native binary at {home}/dist/ai-specs; "
                f"fetching {self.asset} from {self.server.base_url}")

    def _failure_lines(self, home: Path | None = None) -> list[str]:
        home = home or self.home
        return [
            f"ai-specs: native binary not found at {home}/dist/ai-specs",
            "  No verified release asset for this platform and no usable Go toolchain.",
            f"  Re-run the installer: curl -fsSL {INSTALL_URL} | bash",
            "  Or build it with scripts/build-cli.sh, point AI_SPECS_RELEASE_BASE"
            " at a mirror, or set AI_SPECS_GO_BIN.",
        ]

    # -- cases ------------------------------------------------------------
    def test_a_verified_asset_is_installed_and_the_verb_runs(self):
        self._serve()
        self._trust(hashlib.sha256(PAYLOAD).hexdigest())

        proc = self._run("version")

        self.assertEqual(proc.returncode, 3, proc.stderr)
        self.assertEqual(proc.stdout, "native version\n")
        self.assertEqual(
            proc.stderr.splitlines(),
            [
                self._fetch_line(),
                f"ai-specs: installed {self.asset} at {self.dist}",
                "native-stderr",
            ],
        )
        self.assertEqual(self.dist.read_bytes(), PAYLOAD,
                         "the verified bytes are what gets installed")
        self.assertEqual(stat.S_IMODE(self.dist.stat().st_mode), 0o755)
        self.assertEqual([p.name for p in self.dist.parent.iterdir()],
                         ["ai-specs"], "no temp file may survive a success")
        self.assertEqual(len(self.server.requests), 1)

    def test_a_digest_mismatch_installs_nothing(self):
        self._serve()
        self._trust(hashlib.sha256(b"some other build").hexdigest())

        proc = self._run("version")

        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stdout, "")
        self.assertEqual(
            proc.stderr.splitlines(),
            [
                self._fetch_line(),
                f"ai-specs: warning: {self.asset} did not match the committed"
                " digest; discarding the download",
                *self._failure_lines(),
            ],
        )
        self.assertFalse(self.dist.exists(),
                         "unverified bytes are never installed")
        self.assertEqual(list(self.dist.parent.iterdir()), [],
                         "the discarded download must not be left behind")

    def test_no_committed_digest_means_no_download_and_a_go_build(self):
        # The trust root names another platform: no digest for this one.
        self._trust(hashlib.sha256(b"another platform").hexdigest(),
                    asset="ai-specs-plan9-vax")
        farm = self.base / "path-with-go"
        farm.mkdir()
        fake_build = self.base / "fake-build"
        fake_build.write_bytes(PAYLOAD)
        _go_shim(farm, fake_build)
        path = os.pathsep.join([str(farm), self.tool_path])

        proc = self._run("version", path=path)

        self.assertEqual(proc.returncode, 3, proc.stderr)
        self.assertEqual(proc.stdout, "native version\n")
        self.assertEqual(
            proc.stderr.splitlines(),
            [
                "ai-specs: no verified asset for this platform;"
                " building locally with go",
                f"ai-specs: built and installed {self.dist}",
                "native-stderr",
            ],
        )
        self.assertEqual(self.dist.read_bytes(), PAYLOAD)
        self.assertEqual(stat.S_IMODE(self.dist.stat().st_mode), 0o755)
        self.assertEqual(self.server.requests, [],
                         "no committed digest means no download is attempted")

    def test_a_pinned_missing_binary_keeps_the_legacy_failure(self):
        self._serve()
        self._trust(hashlib.sha256(PAYLOAD).hexdigest())
        pinned = self.base / "pinned" / "ai-specs"

        proc = self._run("version", env_extra={"AI_SPECS_GO_BIN": str(pinned)})

        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stdout, "")
        self.assertEqual(
            proc.stderr.splitlines(),
            [
                f"ai-specs: native binary not found at {pinned}",
                "Build it with scripts/build-cli.sh, or set AI_SPECS_GO_BIN.",
            ],
            "an explicitly pinned binary is never replaced",
        )
        self.assertEqual(self.server.requests, [])
        self.assertFalse((self.home / "dist").exists())

    def test_an_existing_dist_binary_is_exec_directly(self):
        (self.home / "dist").mkdir()
        self.dist.write_bytes(PAYLOAD)
        self.dist.chmod(0o755)
        self._serve()
        self._trust(hashlib.sha256(PAYLOAD).hexdigest())

        proc = self._run("version")

        self.assertEqual(proc.returncode, 3, proc.stderr)
        self.assertEqual(proc.stdout, "native version\n")
        self.assertEqual(proc.stderr, "native-stderr\n",
                         "an existing binary means no acquisition at all")
        self.assertEqual(self.server.requests, [])

    def test_a_symlinked_home_receives_dist_under_its_real_directory(self):
        real = self.base / "real-home"
        (real / "bin").mkdir(parents=True)
        (real / "VERSION").write_text("0.25.0\n", encoding="utf-8")
        self._serve()
        self._trust(hashlib.sha256(PAYLOAD).hexdigest(), home=real)
        link = self.base / "linked-home"
        link.symlink_to(real, target_is_directory=True)

        proc = self._run("version", env_extra={"AI_SPECS_HOME": str(link)})

        self.assertEqual(proc.returncode, 3, proc.stderr)
        self.assertEqual(proc.stdout, "native version\n")
        self.assertEqual(
            proc.stderr.splitlines(),
            [
                self._fetch_line(home=real),
                f"ai-specs: installed {self.asset} at {real}/dist/ai-specs",
                "native-stderr",
            ],
        )
        self.assertEqual((real / "dist" / "ai-specs").read_bytes(), PAYLOAD,
                         "the canonical home directory receives dist/")


if __name__ == "__main__":
    unittest.main()

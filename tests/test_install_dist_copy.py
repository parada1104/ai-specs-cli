"""GO-20 U2e: the installed home carries a copy of the verified binary.

After the flip the repo launcher bin/ai-specs execs $AI_SPECS_HOME/dist/ai-specs.
install.sh therefore copies the SAME verified bytes it installed at
$INSTALL_BIN/ai-specs into the home's dist/ directory. This test drives the
installer exactly like tests/test_install_binary_entry.py does (hermetic fixture
repo + local HTTP release asset), so no network and no Go toolchain.

PR-B (the launcher flip) owns the assertion that "$AI_SPECS_HOME/bin/ai-specs"
(a BASH launcher today) execs the dist copy; this suite only proves the dist
copy exists, is executable, is byte-equal, and runs.
"""
from __future__ import annotations

import hashlib
import http.server
import os
import platform
import shutil
import socketserver
import subprocess
import tempfile
import threading
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"
LAUNCHER = ROOT / "bin" / "ai-specs"
# macOS ships Bash 3.2 as /bin/bash: the shell the installer must survive.
SYSTEM_BASH = Path("/bin/bash")

# Every external command bin/ai-specs may reach for on its cached path.
_LAUNCHER_TOOLS = (
    "awk", "bash", "cat", "curl", "dirname", "grep", "head", "ls",
    "mkdir", "mktemp", "mv", "readlink", "rm", "sed", "sha256sum",
    "shasum", "sh", "stat", "tr", "uname",
)


def _calling_tool_path(farm: Path, log: Path) -> str:
    """A PATH where every hasher and curl appends its arguments to `log`.

    The launcher must reach neither on a stamp that is already bound.
    """
    farm.mkdir(parents=True, exist_ok=True)
    for name in _LAUNCHER_TOOLS:
        real = shutil.which(name)
        if real is not None and not (farm / name).exists():
            (farm / name).symlink_to(real)
    for name in ("shasum", "sha256sum", "curl"):
        real = shutil.which(name)
        if real is None:
            continue
        wrapper = farm / name
        if wrapper.is_symlink() or wrapper.exists():
            wrapper.unlink()
        wrapper.write_text(
            "#!/bin/sh\n"
            f'printf "{name} %s\\n" "$*" >> "{log}"\n'
            f'exec "{real}" "$@"\n',
            encoding="utf-8",
        )
        wrapper.chmod(0o755)
    return str(farm)


def _platform_asset() -> str:
    system = platform.system()
    os_name = {"Darwin": "darwin", "Linux": "linux"}.get(system)
    machine = platform.machine().lower()
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(machine)
    if os_name is None or arch is None:
        raise unittest.SkipTest(f"unsupported platform {system}/{machine}")
    return f"ai-specs-{os_name}-{arch}"


def _git(repo: Path, *args: str) -> None:
    env = {
        **os.environ,
        "GIT_AUTHOR_NAME": "t",
        "GIT_AUTHOR_EMAIL": "t@example.com",
        "GIT_COMMITTER_NAME": "t",
        "GIT_COMMITTER_EMAIL": "t@example.com",
    }
    subprocess.run(["git", *args], cwd=str(repo), env=env, check=True,
                   capture_output=True, text=True)


class _Server:
    """A minimal HTTP server over a directory, on a free port."""

    def __init__(self, directory: Path):
        handler = self._handler(directory)
        self.httpd = socketserver.TCPServer(("127.0.0.1", 0), handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    @staticmethod
    def _handler(directory: Path):
        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(directory), **kwargs)

            def log_message(self, format, *args):
                pass

        return Handler

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def close(self) -> None:
        self.httpd.shutdown()
        self.httpd.server_close()


class InstallDistCopyTests(unittest.TestCase):
    def _fixture_repo(self, base: Path, asset: str, digest: str,
                      launcher: bytes | None = None) -> Path:
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        (source / "bin").mkdir()
        (source / "bin" / "ai-specs").write_bytes(
            launcher if launcher is not None else b"#!/bin/sh\nexit 0\n")
        (source / "lib").mkdir()
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source

    def _run_install(self, base: Path, source: Path, server: _Server, home: Path,
                     install_bin: Path | None = None, shell: str = "bash"):
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "userhome"),
            "AI_SPECS_HOME": str(home),
            "INSTALL_BIN": str(install_bin or (base / "bin")),
            "AI_SPECS_REPO": str(source),
            "AI_SPECS_REF": "main",
            "AI_SPECS_RELEASE_BASE": server.base_url,
            "AI_SPECS_TRUST_ROOT_FILE": str(home / "bin" / "SHA256SUMS"),
            "LC_ALL": "C",
            "LANG": "C",
        }
        (base / "userhome").mkdir(exist_ok=True)
        return subprocess.run([shell, str(INSTALL_SH)], cwd=str(ROOT), env=env,
                              capture_output=True, text=True, check=False, timeout=300)

    def test_installer_copies_the_installed_binary_into_the_home_dist(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 'ai-specs test-version 1.0.0'\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-dist-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"

            proc = self._run_install(base, source, server, home)

            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            entry = base / "bin" / "ai-specs"
            dist = home / "dist" / "ai-specs"
            self.assertTrue(dist.is_file(), proc.stdout + proc.stderr)
            self.assertTrue(os.access(dist, os.X_OK), "the dist copy must stay executable")
            self.assertEqual(dist.read_bytes(), entry.read_bytes(),
                             "dist must be a byte-for-byte copy of the installed entry")
            self.assertEqual(dist.read_bytes(), payload,
                             "dist must carry the VERIFIED bytes, not a rebuild")
            # JD-A-005: the stamp binds the version to those exact bytes, so the
            # launcher's first run needs neither a download nor a hash.
            stamp = (home / "dist" / "VERSION").read_text().split()
            self.assertEqual(len(stamp), 3, stamp)
            self.assertEqual(stamp[0], "1.0.0", stamp)
            self.assertEqual(stamp[1], hashlib.sha256(payload).hexdigest(), stamp)

            # PR-B owns the "$AI_SPECS_HOME/bin/ai-specs" launcher assertion;
            # the flip has not happened yet, so only the dist copy runs here.
            run = subprocess.run([str(dist), "version"], capture_output=True,
                                 text=True, check=False, timeout=60,
                                 env={k: v for k, v in os.environ.items()
                                      if k != "AI_SPECS_GO_BIN"})
            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
            self.assertTrue(run.stdout.strip(), "the dist binary must print a version")

    def test_a_failed_dist_copy_fails_the_install_loudly(self):
        if os.geteuid() == 0:
            self.skipTest("root ignores directory permissions; the read-only case is unenforceable")
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-dist-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"

            first = self._run_install(base, source, server, home)
            self.assertEqual(first.returncode, 0, first.stdout + first.stderr)

            # Re-run against a home whose dist/ cannot accept the copy. The
            # whole directory goes: the stamp inside it is untracked content the
            # fixture repo does not ignore, and a dirty tree is a different
            # (earlier) failure than the one under test.
            second_home = base / "home2"
            shutil.copytree(home, second_home, symlinks=True)
            shutil.rmtree(second_home / "dist")
            (second_home / "dist").mkdir()
            (second_home / "dist").chmod(0o555)

            proc = self._run_install(base, source, server, second_home,
                                     install_bin=base / "bin2")

            self.assertNotEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            combined = proc.stdout + proc.stderr
            self.assertIn("dist/ai-specs", combined,
                          "the failure must name the dist path")

    def test_the_system_bash_stamps_a_cache_key_that_needs_no_hash(self):
        """JD-A-005: install.sh must run under macOS's Bash 3.2 (/bin/bash).

        A `case` inside "$(...)" is mis-parsed there: the install still exits 0
        and prints its success banner, while the stamp receives shell source
        instead of the cache key and the first launcher run hashes and fetches
        once. `bash -n` does not catch it; only an actual run of the system
        bash does.
        """
        if not SYSTEM_BASH.exists():
            self.skipTest("no /bin/bash on this platform")
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 'ai-specs 1.0.0'\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-bash32-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest,
                                        launcher=LAUNCHER.read_bytes())
            home = base / "home"

            proc = self._run_install(base, source, server, home,
                                     shell=str(SYSTEM_BASH))

            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            self.assertEqual(proc.stderr, "",
                             "the system bash must install without a single "
                             "stderr line")
            self.assertIn("Installed ai-specs 1.0.0", proc.stdout)

            dist = home / "dist" / "ai-specs"
            st = dist.stat()
            key = f"{st.st_size}:{int(st.st_mtime)}:{int(st.st_ctime)}:{st.st_ino}"
            self.assertEqual(
                (home / "dist" / "VERSION").read_text(),
                f"1.0.0 {digest} {key}\n",
                "the stamp must be '<version> <sha256> <size>:<mtime>:<ctime>:<inode>'")

            log = base / "calls.log"
            path = _calling_tool_path(base / "path", log)
            run = subprocess.run(
                [str(SYSTEM_BASH), str(home / "bin" / "ai-specs"), "version"],
                env={
                    "PATH": path,
                    "HOME": str(base / "userhome"),
                    "AI_SPECS_HOME": str(home),
                    "AI_SPECS_RELEASE_BASE": server.base_url,
                    "LC_ALL": "C",
                    "LANG": "C",
                },
                capture_output=True, text=True, check=False, timeout=120)

            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
            self.assertEqual(run.stdout, "ai-specs 1.0.0\n")
            self.assertEqual(run.stderr, "")
            self.assertFalse(log.exists(),
                             "a bound stamp must need no hash and no download: "
                             f"{log.read_text() if log.exists() else ''}")


if __name__ == "__main__":
    unittest.main()

// Differential between the native Acquire and Python's gate_binary.acquire
// (T7/C2c). Both graders run against the same temp home — one Go leg, then the
// cache wiped and one Python leg — and the status projection plus the cache
// tree must match exactly.
//
// The decided same-user divergences are excluded on purpose and covered by the
// package's own tests instead:
//
//   - no committed digest: the native acquisition fails closed (76541ae) while
//     Python attempts the local build;
//   - cache ownership/permissions: the native acquisition refuses a cache that
//     breaks the same-user boundary; Python proceeds;
//   - cache directory modes: the native acquisition creates 0700 directories,
//     Python uses the umask default.
package gatebinary

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// differentialDriver loads gate_binary.py, pins detect_platform and _asset_url
// to the arguments the Go leg used, runs acquire and prints the shared status
// keys as JSON.
const differentialDriver = `import importlib.util, json, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("gate_binary_differential", sys.argv[1])
if spec is None or spec.loader is None:
    raise RuntimeError("unable to load gate_binary.py at %s" % sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)
mod.detect_platform = lambda: (sys.argv[2], sys.argv[3])
mod._asset_url = lambda version, asset_name: sys.argv[4] + "/v" + version + "/" + asset_name
status = mod.acquire(
    gate_impl="auto",
    ai_specs_home=Path(sys.argv[5]),
    offline=sys.argv[6] == "1",
)
print(json.dumps({
    "attempted": status.get("attempted"),
    "installed": status.get("installed"),
    "platform": list(status.get("platform") or ("", "")),
    "cache_path": status.get("cache_path"),
    "warn": status.get("warn"),
    "mismatch": status.get("mismatch"),
}))
`

// differentialStatus is the status projection both legs are compared on: the
// keys Python's acquire documents (the Go Verification evidence and the Go-only
// boundary behavior are out of scope).
type differentialStatus struct {
	Attempted bool      `json:"attempted"`
	Installed bool      `json:"installed"`
	Platform  [2]string `json:"platform"`
	CachePath string    `json:"cache_path"`
	Warn      *string   `json:"warn"`
	Mismatch  *string   `json:"mismatch"`
}

func projectAcquireStatus(status AcquireStatus) map[string]any {
	return map[string]any{
		"attempted":  status.Attempted,
		"installed":  status.Installed,
		"platform":   status.Platform,
		"cache_path": status.CachePath,
		"warn":       status.Warn,
		"mismatch":   status.Mismatch,
	}
}

func normalizedJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// diffTree maps every regular file under <home>/cache to its bytes, so the
// installed binary, the receipt and the mismatch record compare byte for byte
// (directory modes are excluded: they are part of the decided divergence).
func diffTree(t *testing.T, home string) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := filepath.Join(home, "cache")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk cache: %v", err)
	}
	return files
}

func runDifferentialPython(t *testing.T, python3, module, home, goos, goarch, baseURL string, offline bool, pathDir string) map[string]any {
	t.Helper()
	offlineFlag := "0"
	if offline {
		offlineFlag = "1"
	}
	cmd := exec.Command(python3, append([]string{"-c", differentialDriver},
		module, goos, goarch, baseURL, home, offlineFlag)...)
	cmd.Env = append(os.Environ(), "PATH="+pathDir, "PYTHONDONTWRITEBYTECODE=1")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("python acquire: %v\n%s", err, errBuf.String())
	}
	var status map[string]any
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("python status: %v\n%s", err, out.String())
	}
	return status
}

func TestAcquireDifferentialAgainstPython(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	module, err := filepath.Abs(filepath.Join("..", "..", "lib", "_internal", "gate_binary.py"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(module); err != nil {
		t.Skipf("gate_binary.py unavailable: %v", err)
	}
	t.Setenv("AI_SPECS_GATE_BUILD", "")
	emptyPath := t.TempDir()
	fakeGoDir := t.TempDir()
	writeExec(t, filepath.Join(fakeGoDir, "go"), "#!/bin/sh\nexit 1\n")
	hostGoos, hostGoarch := DetectHostPlatform()
	srv, _ := assetServer(t, gateScript())

	// run executes the Go leg, wipes the cache, executes the Python leg and
	// compares the status projection and the cache tree byte for byte.
	run := func(name, digest string, opts AcquireOptions, pythonPath string, platform [2]string) {
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			if digest != "" {
				writeSums(t, home, digest, AssetName(platform[0], platform[1]))
			}
			goOpts := opts
			goOpts.GateImpl = "auto"
			goOpts.Home = home
			goOpts.GOOS, goOpts.GOARCH = platform[0], platform[1]
			goStatus := normalizedJSON(t, projectAcquireStatus(Acquire(goOpts)))
			goTree := diffTree(t, home)

			if err := os.RemoveAll(filepath.Join(home, "cache")); err != nil {
				t.Fatalf("reset cache: %v", err)
			}
			pyStatus := runDifferentialPython(t, python3, module, home,
				platform[0], platform[1], opts.BaseURL, opts.Offline, pythonPath)

			if !reflect.DeepEqual(goStatus, pyStatus) {
				t.Fatalf("status:\n--- go ---\n%v\n--- python ---\n%v", goStatus, pyStatus)
			}
			goTreeAfter := diffTree(t, home)
			if !reflect.DeepEqual(goTree, goTreeAfter) {
				t.Fatalf("cache tree:\n--- go ---\n%v\n--- python ---\n%v", goTree, goTreeAfter)
			}
		})
	}

	// Shared branch: offline, no cached binary, no local-build tool — the
	// missing-digest branch is never reached.
	run("offline with no cached binary", "",
		AcquireOptions{Offline: true, LookPath: func(string) (string, error) {
			return "", errors.New("no go")
		}},
		emptyPath, [2]string{hostGoos, hostGoarch})

	// Shared branch: offline with a toolchain and a committed digest, but no
	// gate Go source in the home — both graders fail the build the same way.
	run("offline local build without gate source", strings.Repeat("a", 64),
		AcquireOptions{Offline: true, LookPath: func(string) (string, error) {
			return filepath.Join(fakeGoDir, "go"), nil
		}},
		fakeGoDir, [2]string{hostGoos, hostGoarch})

	// Shared branch: the download installs the digest-verified binary plus its
	// receipt, byte for byte.
	run("download installs the verified binary", sha256Hex(gateScript()),
		AcquireOptions{BaseURL: srv.URL}, os.Getenv("PATH"),
		[2]string{hostGoos, hostGoarch})

	// Shared branch: a downloaded artifact that misses the committed digest is
	// deleted, recorded and never executed.
	run("download digest mismatch", strings.Repeat("a", 64),
		AcquireOptions{BaseURL: srv.URL}, os.Getenv("PATH"),
		[2]string{hostGoos, hostGoarch})

	// Shared branch: a platform outside the published matrix warns and
	// acquires nothing.
	run("unsupported platform", "",
		AcquireOptions{}, os.Getenv("PATH"), [2]string{"solaris", "riscv"})
}

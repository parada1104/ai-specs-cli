// Package hook holds the native event helpers of the gate hooks (GO-20 U2e,
// U-F2a). NormalizeEvent is the port of plan-build-gate.sh's embedded python
// normalization: it resolves the hook's target path the way os.path.realpath
// does (non-strict, symlink-aware), so the boundary decision is made on the
// canonical path.
package hook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Target is the normalized write target of one hook event.
type Target struct {
	Tool  string // tool_name, "" when absent
	Path  string // canonical target path
	Probe string // canonical nearest existing directory of Path
}

// NormalizeEvent decodes one hook event (JSON on stdin) and returns its target.
// ok is false whenever the python original exits without output: invalid JSON,
// a non-object event, no string file path, or no existing ancestor directory.
func NormalizeEvent(stdin []byte, processCwd string) (Target, bool) {
	var event any
	if err := json.Unmarshal(stdin, &event); err != nil {
		return Target{}, false
	}
	obj, ok := event.(map[string]any)
	if !ok {
		return Target{}, false
	}
	toolInput, _ := obj["tool_input"].(map[string]any)
	fileValue := firstTruthy(toolInput["file_path"], toolInput["notebook_path"])
	filePath, ok := fileValue.(string)
	if !ok || filePath == "" {
		return Target{}, false
	}
	cwd := processCwd
	if cwdValue := obj["cwd"]; truthy(cwdValue) {
		text, ok := cwdValue.(string)
		if !ok {
			return Target{}, false
		}
		cwd = text
	}
	base, err := filepath.Abs(cwd)
	if err != nil {
		return Target{}, false
	}
	candidate := filePath
	if !filepath.IsAbs(filePath) {
		candidate = filepath.Join(base, filePath)
	}
	target, err := realpath(candidate)
	if err != nil {
		return Target{}, false
	}
	probe := filepath.Dir(target)
	for probe != "" && !isDir(probe) && probe != filepath.Dir(probe) {
		probe = filepath.Dir(probe)
	}
	if !isDir(probe) {
		return Target{}, false
	}
	probe, err = realpath(probe)
	if err != nil {
		return Target{}, false
	}
	tool, ok := toolName(obj["tool_name"])
	if !ok {
		return Target{}, false
	}
	return Target{Tool: tool, Path: target, Probe: probe}, true
}

// toolName mirrors `event.get("tool_name", "") or ""` followed by a str
// concatenation: a truthy non-string tool name makes the python raise.
func toolName(value any) (string, bool) {
	if !truthy(value) {
		return "", true
	}
	text, ok := value.(string)
	return text, ok
}

func firstTruthy(values ...any) any {
	for _, value := range values {
		if truthy(value) {
			return value
		}
	}
	return nil
}

// truthy follows python truthiness for the JSON values a hook can carry.
func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// realpath is a port of CPython's posixpath.realpath(strict=False): symlinks
// are resolved component by component, missing components are kept as written,
// and a symlink loop stops resolution instead of failing.
func realpath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	resolved, _, err := joinRealpath("", path, map[string]*string{})
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return ".", nil
	}
	return resolved, nil
}

// joinRealpath follows CPython's _joinrealpath. ok is false when a loop was
// found; the returned path then keeps the unresolved remainder, as python does.
func joinRealpath(path, rest string, seen map[string]*string) (string, bool, error) {
	if strings.HasPrefix(rest, "/") {
		rest = rest[1:]
		path = "/"
	}
	for rest != "" {
		name := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			if i := strings.LastIndexByte(path, '/'); i > 0 {
				path = path[:i]
			} else {
				path = "/"
			}
			continue
		}
		newpath := strings.TrimSuffix(path, "/") + "/" + name
		info, err := os.Lstat(newpath)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			path = newpath
			continue
		}
		if known, ok := seen[newpath]; ok {
			if known != nil {
				path = *known
				continue
			}
			return newpath + tail(rest), false, nil
		}
		seen[newpath] = nil
		link, err := os.Readlink(newpath)
		if err != nil {
			return "", false, errors.New("readlink failed")
		}
		resolvedLink, ok, err := joinRealpath(path, link, seen)
		if err != nil {
			return "", false, err
		}
		if !ok {
			return resolvedLink + tail(rest), false, nil
		}
		seen[newpath] = &resolvedLink
		path = resolvedLink
	}
	return path, true, nil
}

func tail(rest string) string {
	if rest == "" {
		return ""
	}
	return "/" + rest
}

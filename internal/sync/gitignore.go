package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/toml"
)

// gitignoreHeader and gitignoreFooter are the byte-exact HEADER/FOOTER
// constants of lib/_internal/gitignore-render.py. Both strings carry their own
// trailing newline, exactly like the triple-quoted Python literals.
const (
	gitignoreHeader = "# --- ai-specs: managed by `ai-specs init`/`sync` — do not edit ---\n"
	gitignoreFooter = "# --- end ai-specs ---\n"
)

// renderAiSpecsGitignoreBody is the fixed render() output of
// lib/_internal/gitignore-render.py: HEADER, the ignored/committed entries,
// and FOOTER joined with "\n". The join keeps FOOTER's trailing newline, so
// the emitted file ends with one newline.
func renderAiSpecsGitignoreBody() string {
	return strings.Join([]string{
		gitignoreHeader,
		".internal/",
		".deps/",
		"",
		"# Recipe materialization is CLI-owned; only declared overrides are committed.",
		"recipes/**",
		"!recipes/*/",
		"!recipes/*/overrides/",
		"!recipes/*/overrides/**",
		"",
		gitignoreFooter,
	}, "\n")
}

// RenderAiSpecsGitignore is the Go port of lib/_internal/gitignore-render.py:
// parse tomlPath for the [[deps]] array (its count feeds only the status
// line), create the output's parent directories, write the fixed body, print
// the Python status line to stdout, and return the process exit code.
func RenderAiSpecsGitignore(tomlPath, outputPath string, stdout io.Writer) int {
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return 1
	}
	root, err := toml.Parse(data)
	if err != nil {
		return 1
	}
	deps, _ := root.Tables("deps")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return 1
	}
	if err := os.WriteFile(outputPath, []byte(renderAiSpecsGitignoreBody()), 0o644); err != nil {
		return 1
	}
	fmt.Fprintf(stdout, "  ✓ wrote %s (%d dep(s))\n", outputPath, len(deps))
	return 0
}

// gitignoreRootMarkerBegin / gitignoreRootMarkerEnd are MARKER_BEGIN /
// MARKER_END from lib/_internal/gitignore-root-refresh.py.
const (
	gitignoreRootMarkerBegin = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
	gitignoreRootMarkerEnd   = "# --- end ai-specs ---"
)

// RefreshRootGitignore is the Go port of lib/_internal/gitignore-root-refresh.py:
// replace the managed agent block in <root>/.gitignore when both markers are
// present, otherwise append it, printing the Python status line and returning
// the process exit code.
//
// Deviation: the "begin marker without end marker" branch is TOLERANT, not
// parity-gated. The Python authority raises an unhandled ValueError (traceback
// on stderr); this port prints a one-line error and returns 1.
func RefreshRootGitignore(root, templatePath string, stdout, stderr io.Writer) int {
	if fi, err := os.Stat(templatePath); err != nil || !fi.Mode().IsRegular() {
		fmt.Fprintf(stderr, "ERROR: template not found: %s\n", templatePath)
		return 1
	}
	template, err := readTextUniversal(templatePath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot read %s: %v\n", templatePath, err)
		return 1
	}
	if !strings.HasSuffix(template, "\n") {
		template += "\n"
	}

	gitignore := filepath.Join(root, ".gitignore")
	text := ""
	if fi, err := os.Stat(gitignore); err == nil && fi.Mode().IsRegular() {
		text, err = readTextUniversal(gitignore)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: cannot read %s: %v\n", gitignore, err)
			return 1
		}
	}

	begin := strings.Index(text, gitignoreRootMarkerBegin)
	if begin != -1 {
		end := strings.Index(text[begin:], gitignoreRootMarkerEnd)
		if end == -1 {
			fmt.Fprintf(stderr, "ERROR: %s: found begin marker without matching end marker\n", gitignore)
			return 1
		}
		end = begin + end + len(gitignoreRootMarkerEnd)
		after := text[end:]
		if strings.HasPrefix(after, "\n") {
			after = after[1:]
		}
		before := text[:begin]
		if before != "" && !strings.HasSuffix(before, "\n") {
			before += "\n"
		}
		if err := os.WriteFile(gitignore, []byte(before+template+after), 0o644); err != nil {
			return 1
		}
		fmt.Fprintln(stdout, "  ✓ refreshed root .gitignore (agent block)")
		return 0
	}

	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(gitignore), 0o755); err != nil {
		return 1
	}
	if err := os.WriteFile(gitignore, []byte(text+template), 0o644); err != nil {
		return 1
	}
	fmt.Fprintln(stdout, "  ✓ appended root .gitignore (agent block)")
	return 0
}

// readTextUniversal reads path the way Python's Path.read_text() does: bytes
// decoded as text with universal-newline translation (\r\n and lone \r become
// \n). A read error is returned, never swallowed: Python raises on a failed
// read, and treating an unreadable existing file as empty would let the
// caller overwrite content it failed to load.
func readTextUniversal(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), nil
}

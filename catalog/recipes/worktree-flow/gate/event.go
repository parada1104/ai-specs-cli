package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// WriteCandidate is one extracted write target plus the cwd used to resolve it.
type WriteCandidate struct {
	Path   string
	Base   string
	Source cwdSource
}

type Event struct {
	Mode       string
	Tool       string
	Cwd        string
	CwdTrusted bool
	Candidates []WriteCandidate
}

func ParseEvent(r io.Reader, processCwd string) Event {
	var raw map[string]interface{}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}
	}
	ti, _ := raw["tool_input"].(map[string]interface{})
	tool, _ := raw["tool_name"].(string)
	if ti == nil {
		ti = map[string]interface{}{}
	}
	cwd, trusted := eventCwd(raw, processCwd)
	for _, key := range []string{"file_path", "notebook_path"} {
		if p, ok := ti[key].(string); ok && p != "" {
			src := cwdSourceNone
			base := ""
			if trusted {
				src = cwdSourceEvent
				base = cwd
			}
			return Event{Mode: "path", Tool: tool, Cwd: cwd, CwdTrusted: trusted,
				Candidates: []WriteCandidate{{Path: p, Base: base, Source: src}}}
		}
	}
	command := ""
	for _, key := range []string{"command", "script", "cmd"} {
		if v, ok := ti[key].(string); ok && v != "" {
			command = v
			break
		}
	}
	if command == "" {
		for _, key := range []string{"command", "script"} {
			if v, ok := raw[key].(string); ok && v != "" {
				command = v
				break
			}
		}
	}
	if command == "" {
		return Event{}
	}
	tokens, ok := splitPOSIX(command)
	if !ok {
		return Event{}
	}
	segs := splitSegmentsWithSep(tokens)
	overlays, finalS, finalSrc := recoverCwdWalk(segs, cwd, trusted)
	seen := map[string]bool{}
	var candidates []WriteCandidate
	add := func(path, base string, src cwdSource) {
		path = scrub(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		candidates = append(candidates, WriteCandidate{Path: path, Base: base, Source: src})
	}
	for i, seg := range segs {
		base, src := "", cwdSourceNone
		if i < len(overlays) {
			base, src = overlays[i].Base, overlays[i].Source
		}
		for _, p := range extractPass1(seg.tokens) {
			add(p, base, src)
		}
	}
	for _, p := range extractPass2(command) {
		add(p, finalS, finalSrc)
	}
	return Event{Mode: "shell", Tool: tool, Cwd: cwd, CwdTrusted: trusted, Candidates: candidates}
}

func eventCwd(raw map[string]interface{}, fallback string) (string, bool) {
	if c, ok := raw["cwd"].(string); ok {
		// Trim outer whitespace only, then require an absolute existing
		// directory. Internal path bytes are preserved (parity with the
		// legacy Bash c.strip() reference).
		if trimmed := strings.TrimSpace(c); IsExistingDirectory(trimmed) {
			return trimmed, true
		}
	}
	return fallback, false
}
func splitSegments(tokens []string) [][]string {
	segs := splitSegmentsWithSep(tokens)
	var out [][]string
	for _, s := range segs {
		if len(s.tokens) > 0 {
			out = append(out, s.tokens)
		}
	}
	return out
}
func processCwd() string {
	c, err := os.Getwd()
	if err != nil {
		return ""
	}
	return c
}

package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

type Event struct {
	Mode       string
	Tool       string
	Cwd        string
	Candidates []string
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
	for _, key := range []string{"file_path", "notebook_path"} {
		if p, ok := ti[key].(string); ok && p != "" {
			return Event{Mode: "path", Tool: tool, Cwd: eventCwd(raw, processCwd), Candidates: []string{p}}
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
	var candidates []string
	for _, segment := range splitSegments(tokens) {
		candidates = append(candidates, extractPass1(segment)...)
		candidates = append(candidates, extractPass2(command)...)
	}
	return Event{Mode: "shell", Tool: tool, Cwd: eventCwd(raw, processCwd), Candidates: dedupeStrings(candidates)}
}

func eventCwd(raw map[string]interface{}, fallback string) string {
	if c, ok := raw["cwd"].(string); ok {
		// Trim outer whitespace only, then require an absolute existing
		// directory. Internal path bytes are preserved (parity with the
		// legacy Bash c.strip() reference).
		if trimmed := strings.TrimSpace(c); IsExistingDirectory(trimmed) {
			return trimmed
		}
	}
	return fallback
}
func splitSegments(tokens []string) [][]string {
	var out [][]string
	start := 0
	for i, t := range tokens {
		if t == "|" || t == "||" || t == "&&" || t == ";" {
			if i > start {
				out = append(out, tokens[start:i])
			}
			start = i + 1
		}
	}
	if start < len(tokens) {
		out = append(out, tokens[start:])
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

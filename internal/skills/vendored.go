package skills

import (
	"regexp"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/toml"
)

var (
	nameRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:[-+][A-Za-z0-9.-]+)?$`)
)

// VendoredSkillMarkdown mirrors
// skill_contract.render_skill_markdown(skill_contract.from_dep(dep, upstream)),
// the only skill_contract surface on the sync path (vendor-skills.py). dep is
// one [[deps]] table. Errors are *SkillContractError with the Python message.
func VendoredSkillMarkdown(dep *toml.Table, upstreamText string) (string, error) {
	get := func(k string) any { v, _ := dep.Get(k); return v }

	depID, err := requireString(get("id"), "id")
	if err != nil {
		return "", err
	}
	if !nameRE.MatchString(depID) {
		return "", &SkillContractError{Field: "name", Message: "must be lowercase kebab-case"}
	}
	source, err := requireString(get("source"), "source")
	if err != nil {
		return "", err
	}

	frontmatter, body := SplitFrontmatter(upstreamText)
	var upstreamDesc any
	if frontmatter != "" {
		raw, err := ParseFrontmatter(frontmatter)
		if err != nil {
			return "", err
		}
		upstreamDesc = raw["description"]
		if list, ok := upstreamDesc.([]string); ok { // str(list) is its repr
			items := make([]any, len(list))
			for i, s := range list {
				items[i] = s
			}
			upstreamDesc = items
		}
	}
	upstream := canonicalDescription(orStr(upstreamDesc, ""))

	attribution := pyStrip(orStr(get("vendor_attribution"), ""))
	var parts []string
	if upstream != "" {
		parts = append(parts, strings.TrimRight(upstream, ". "))
	}
	if attribution != "" {
		parts = append(parts, "Vendored from "+attribution+" (see metadata.source)")
	}
	description := strings.Join(parts, ". ")
	if description == "" {
		description = "Vendored skill: " + depID
	}

	author := attribution
	if author == "" {
		author = "upstream"
	}
	version := orStr(get("version"), "1.0")
	if !versionRE.MatchString(version) {
		return "", &SkillContractError{Field: "metadata.version", Message: "must look like a semantic version (for example `1.0` or `1.0.0`)"}
	}
	license := orStr(get("license"), "Unknown")

	scope := get("scope")
	if !config.Truthy(scope) {
		scope = []any{"root"}
	}
	scopeList, err := normalizeStringList(scope, "metadata.scope")
	if err != nil {
		return "", err
	}
	autoInvoke := get("auto_invoke")
	if !config.Truthy(autoInvoke) {
		autoInvoke = nil
	}
	autoList, err := normalizeStringList(autoInvoke, "metadata.auto_invoke")
	if err != nil {
		return "", err
	}

	out := []string{
		"---",
		"name: " + depID,
		// Frozen parity (S8 review R3-multiline-description-yaml): a newline in the
		// description (only reachable via a TOML vendor_attribution; frontmatter
		// block scalars are folded) breaks the folded scalar exactly as in Python.
		"description: >",
		"  " + pyStrip(description),
		"license: " + license,
		"metadata:",
		"  author: " + yamlQuote(author),
		"  version: " + yamlQuote(version),
		"  source: " + yamlQuote(source),
	}
	if attribution != "" {
		out = append(out, "  vendor_attribution: "+yamlQuote(attribution))
	}
	if len(scopeList) > 0 {
		out = append(out, "  scope: ["+strings.Join(scopeList, ", ")+"]")
	}
	if len(autoList) > 0 {
		out = append(out, "  auto_invoke:")
		for _, phrase := range autoList {
			out = append(out, "    - "+yamlQuote(phrase))
		}
	}
	out = append(out, "---")
	rendered := strings.Join(out, "\n") + "\n"
	if body != "" {
		return rendered + "\n" + strings.TrimLeft(body, "\n"), nil
	}
	return rendered, nil
}

// orStr mirrors str(v or fallback).
func orStr(v any, fallback string) string {
	if !config.Truthy(v) {
		return fallback
	}
	return config.PyStr(v)
}

// requireString mirrors _require_string with no default (path=None).
func requireString(v any, field string) (string, error) {
	s, isStr := v.(string)
	if v == nil || (isStr && pyStrip(s) == "") {
		return "", &SkillContractError{Field: field, Message: "is required"}
	}
	if !isStr {
		return "", &SkillContractError{Field: field, Message: "must be a string"}
	}
	return pyStrip(s), nil
}

// normalizeStringList mirrors _normalize_string_list with compatibility set
// to isinstance(value, str), as from_dep calls it.
func normalizeStringList(v any, field string) ([]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if s := pyStrip(x); s != "" {
			return []string{s}, nil
		}
		return nil, nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, &SkillContractError{Field: field, Message: "must contain only strings"}
			}
			if s = pyStrip(s); s == "" {
				return nil, &SkillContractError{Field: field, Message: "must not contain empty items"}
			}
			out = append(out, s)
		}
		return out, nil
	case []*toml.Table:
		if len(x) == 0 {
			return nil, nil
		}
		return nil, &SkillContractError{Field: field, Message: "must contain only strings"}
	}
	return nil, &SkillContractError{Field: field, Message: "must be a YAML list of strings"}
}

// canonicalDescription mirrors skill_contract.canonical_description.
func canonicalDescription(description string) string {
	trimmed := pyStrip(description)
	if idx := strings.Index(trimmed, " Trigger:"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimRight(pyStrip(trimmed), ".")
}

// yamlQuote mirrors skill_contract._yaml_quote.
func yamlQuote(v string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
}

// pyIsSpace matches Python str.isspace() for the characters str.strip() trims.
func pyIsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		0x1c, 0x1d, 0x1e, 0x1f, 0x85, 0xa0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007,
		0x2008, 0x2009, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return false
}

func pyStrip(s string) string { return strings.TrimFunc(s, pyIsSpace) }

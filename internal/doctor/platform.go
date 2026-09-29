package doctor

// Native port of doctor.PLATFORM (mirrors platform.sh for doctor use only).
// Field names, values and the agent key order are frozen by the parity
// contract; tests lock this to the expected generated paths. Do not mutate.

// platformInfo is one PLATFORM row. skillsCopy mirrors the legacy
// `skills_copy: True` marker (OpenCode copies skill directories instead of
// symlinking them).
type platformInfo struct {
	instructionsPath string
	skillsDir        string
	mcpConfigPath    string
	mcpKey           string
	commandsDir      string
	skillsCopy       bool
}

// platform is doctor.PLATFORM, in the legacy declaration order.
var platform = map[string]platformInfo{
	"claude": {
		instructionsPath: "CLAUDE.md",
		skillsDir:        ".claude/skills",
		mcpConfigPath:    ".mcp.json",
		mcpKey:           "mcpServers",
		commandsDir:      ".claude/commands",
	},
	"cursor": {
		instructionsPath: "",
		skillsDir:        ".cursor/skills",
		mcpConfigPath:    ".cursor/mcp.json",
		mcpKey:           "mcpServers",
		commandsDir:      ".cursor/commands",
	},
	"opencode": {
		instructionsPath: "",
		skillsDir:        ".opencode/skills",
		mcpConfigPath:    "opencode.json",
		mcpKey:           "mcp",
		commandsDir:      ".opencode/commands",
		skillsCopy:       true,
	},
	"pi": {
		instructionsPath: "",
		skillsDir:        ".pi/skills",
		mcpConfigPath:    ".mcp.json",
		mcpKey:           "mcpServers",
		commandsDir:      "",
	},
	"omp": {
		instructionsPath: ".omp/AGENTS.md",
		skillsDir:        ".omp/skills",
		mcpConfigPath:    ".omp/mcp.json",
		mcpKey:           "mcpServers",
		commandsDir:      ".omp/commands",
	},
	"codex": {
		instructionsPath: "",
		skillsDir:        "",
		mcpConfigPath:    ".codex/config.toml",
		mcpKey:           "mcp_servers",
		commandsDir:      "",
	},
	"copilot": {
		instructionsPath: ".github/copilot-instructions.md",
		skillsDir:        "",
		mcpConfigPath:    "",
		mcpKey:           "",
		commandsDir:      "",
	},
	"gemini": {
		instructionsPath: "GEMINI.md",
		skillsDir:        ".gemini/skills",
		mcpConfigPath:    ".gemini/settings.json",
		mcpKey:           "mcpServers",
		commandsDir:      "",
	},
}

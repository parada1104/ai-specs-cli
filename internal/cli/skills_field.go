package cli

// Native `ai-specs skills field <path>` (GO-20 U2e, T5): reads one JSON object
// on stdin and prints the value at a dotted path, replacing the three python3
// JSON one-liners in bundled-skills/skill-sync/assets/sync.sh. The text rules
// are the shared runJSONField ones that `tracker field` and `hook field` pin.

const skillsFieldUsage = `Usage: ai-specs skills field <path>

Read one JSON object from stdin and print the value at <path>. Dotted paths
descend objects. A string prints as is, true as 1, false as 0, null or a missing
key as an empty line, a list as its items joined with ", ", and an object as
compact JSON. Invalid JSON, or a top-level value that is not an object, exits 1
with nothing printed.

Arguments:
  <path>              Key or dotted path to print

Options:
  --help, -h          Show this help
`

package cli

// Shared JSON field reader for `tracker field` (GO-20 U2e U-F) and `hook field`
// (GO-20 U2e U-F2a). Both print the value at a dotted path of the object on
// stdin with the same text rules; `hook field --string` additionally refuses
// any value that is not a JSON string.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func runJSONField(args []string, stdin io.Reader, stdout, stderr io.Writer, invocation, usage string, stringFlag bool) int {
	usageError := func(message string) int {
		fmt.Fprintln(stderr, message)
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", invocation)
		return 2
	}
	var positional []string
	requireString := false
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, usage)
			return 0
		case stringFlag && arg == "--string":
			requireString = true
		case strings.HasPrefix(arg, "-"):
			return usageError("ERROR: unknown flag: " + arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usageError("ERROR: missing required argument: <path>")
	}
	if len(positional) > 1 {
		return usageError("ERROR: unexpected positional argument: " + positional[1])
	}
	decoder := json.NewDecoder(stdin)
	decoder.UseNumber()
	var doc any
	if err := decoder.Decode(&doc); err != nil {
		fmt.Fprintln(stderr, "ERROR: stdin is not valid JSON")
		return 1
	}
	if _, err := decoder.Token(); err != io.EOF {
		fmt.Fprintln(stderr, "ERROR: stdin is not valid JSON")
		return 1
	}
	if _, ok := doc.(map[string]any); !ok {
		fmt.Fprintln(stderr, "ERROR: stdin JSON is not an object")
		return 1
	}
	var current any = doc
	for _, key := range strings.Split(positional[0], ".") {
		object, ok := current.(map[string]any)
		if !ok {
			current = nil
			break
		}
		value, found := object[key]
		if !found {
			current = nil
			break
		}
		current = value
	}
	if requireString {
		if _, ok := current.(string); !ok {
			fmt.Fprintf(stderr, "ERROR: %s is not a JSON string\n", positional[0])
			return 1
		}
	}
	fmt.Fprintln(stdout, fieldText(current))
	return 0
}

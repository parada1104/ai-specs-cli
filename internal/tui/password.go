package tui

// The hidden-input prompt (GO-20 U4d-2, S30): questionary.password, which the
// env offer uses for secret-looking variables (API_KEY / TOKEN / SECRET /
// PASSWORD / APIKEY). questionary implements it as text(..., is_password=True)
// and prompt_toolkit masks the buffer with `*` (PasswordProcessor, one `*` per
// code point), so the typed runes are NEVER echoed — while the prompt, the
// editing keys and the cancel semantics stay exactly Text's. That is why this
// is one wrapper over the shared line engine over TextSource, not a second
// rune interface.

import (
	"io"
)

// Password asks for a line of text that is never echoed. The question is drawn
// before the first key is read — `? <message> ` plus questionary's instruction
// token (` (hidden input) `, kept as a separate argument because the offer
// passes it) — and every keystroke redraws the line with one `*` per typed code
// point, cursor keys and backspace included. Enter returns the RAW buffer
// (questionary does not trim; write_env's blank rule owns the never-wipe
// invariant) with the masked line left on screen. Ctrl-C, ESC, Ctrl-D on an
// empty line, EOF and a read error all return ("", false).
func Password(out io.Writer, in KeySource, message, instruction string) (string, bool) {
	return textLine(out, in, textPrompt{
		message:     message,
		instruction: instruction,
		mask:        true,
		verbatim:    true,
	})
}

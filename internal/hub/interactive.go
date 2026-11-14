package hub

// GO-20 U3b-2 (T4c): the native interactive hub. This is the Go half that
// replaces the `lib/_internal/hub.py` bridge: the status panel, the 12-entry
// menu, the Recipes / Skills / Agents submenus, the init offer and every
// delegated child. Every string, label, order and branch is ported from
// hub.py — this file is a translation, not a redesign.
//
// The prompt loop never touches the terminal itself: it draws through
// internal/tui and reads from a tui.KeySource. Production passes a raw-mode
// tui.Terminal; a test passes scripted keys, which is what makes the menu
// testable without a pty (see invariants_test.go).

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	assets "ai-specs.dev/ai-specs"
	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/recipe"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/tui"
)

// interactive is one hub session: the panel, the key source and the delegation
// target for as long as the menu runs.
type interactive struct {
	target  string
	home    string
	keys    tui.KeySource
	term    *tui.Terminal // non-nil in production; nil when keys were injected
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	noColor bool
	// resumeFailed records that a delegated child ran and Terminal.Resume
	// could not re-enter raw mode afterwards. The terminal is canonical then,
	// so every prompt below must stop instead of reading keys it cannot decode.
	resumeFailed bool
}

// runInteractiveSession draws the hub for both interactive modes: ModeOfferInit
// offers the init wizard first and only opens the menu if it succeeded,
// ModeInteractiveHub goes straight to the menu.
func runInteractiveSession(mode Mode, target string, opts Options, stdin io.Reader, stdout, stderr io.Writer) int {
	s := &interactive{
		target:  target,
		home:    opts.Home,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		noColor: !opts.TTY,
	}
	if opts.Keys != nil {
		s.keys = opts.Keys
	} else {
		// Production: internal/tui owns the terminal. A terminal it cannot take
		// over is reported and the hub exits 1 instead of prompting blind.
		in, _ := stdin.(*os.File)
		term, err := tui.NewTerminal(in)
		if err != nil {
			fmt.Fprintf(stderr, "ai-specs hub: %v\n", err)
			return 1
		}
		s.term = term
		s.keys = term
		defer term.Close()
	}

	if mode == ModeOfferInit && !s.offerInit() {
		return 0
	}
	return s.runLoop()
}

// runLoop is hub.py's `_run_interactive_hub` while loop.
func (s *interactive) runLoop() int {
	for {
		if s.resumeFailed {
			// Raw mode was lost around a delegation: return the loop's exit code
			// instead of prompting a terminal that cannot deliver keystrokes.
			return 0
		}
		s.renderPanel()
		value, ok := tui.Select(s.stdout, s.keys, "What do you want to do?", s.menuChoices())
		if !ok {
			// hub.py: `answer if answer is not None else Action.QUIT`.
			return 0
		}
		action := Action(value)
		switch action {
		case ActionQuit:
			return 0
		case ActionRecipe:
			if code, quit := s.recipesSubmenu(); quit {
				return code
			}
			continue
		case ActionSkills:
			if code, quit := s.skillsSubmenu(); quit {
				return code
			}
			continue
		case ActionAgents:
			if code, quit := s.agentsSubmenu(); quit {
				return code
			}
			continue
		case ActionVersion:
			// The one entry that prints and re-prompts without pausing.
			fmt.Fprintln(s.stdout, readVersion(s.home))
			continue
		}
		s.printResult(s.delegate(action, nil))
		if !s.pause() {
			return 0
		}
	}
}

// menuChoices is hub.py's CommandMenu choices: `f"{title} — {desc}"`.
func (s *interactive) menuChoices() []tui.Choice {
	choices := make([]tui.Choice, 0, len(Menu()))
	for _, entry := range Menu() {
		choices = append(choices, tui.Choice{
			Title: entry.Title + " — " + entry.Description,
			Value: string(entry.Action),
		})
	}
	return choices
}

// renderPanel is hub.py's StatusPanel: the doctor runs once per loop iteration
// and the panel is drawn from the SAME run the non-interactive summary uses.
func (s *interactive) renderPanel() {
	run := doctorRunFor(s.target, s.home, s.stderr)
	data := tui.PanelData{
		Version: readVersion(s.home),
		Target:  s.target,
		Counts: tui.Counts{
			OK:    run.counts.OK,
			Info:  run.counts.Info,
			Warn:  run.counts.Warn,
			Error: run.counts.Error,
		},
		Checks: run.checks,
	}
	if resolved, via := hubTopology(s.target); resolved != "" {
		data.Topology = formatPanelTopology(resolved, via)
	}
	fmt.Fprint(s.stdout, tui.RenderPanel(data, s.noColor))
}

// formatPanelTopology is hub.py's StatusPanel topology label: an empty via is
// "auto", "auto" renders the auto→ form, anything else renders `(via x)`.
func formatPanelTopology(resolved, via string) string {
	if via == "" || via == "auto" {
		return fmt.Sprintf("%s (auto→%s)", resolved, resolved)
	}
	return fmt.Sprintf("%s (via %s)", resolved, via)
}

// printResult is hub.py's `✓ done` / `✗ exited N` line.
func (s *interactive) printResult(rc int) {
	if rc == 0 {
		fmt.Fprintln(s.stdout, "✓ done")
		return
	}
	fmt.Fprintf(s.stdout, "✗ exited %d\n", rc)
}

// pause is hub.py's module-level pause(). It is the ONE tui.Pause call site in
// this package, which the source invariant test pins.
func (s *interactive) pause() bool {
	if s.resumeFailed {
		// The terminal is canonical: drawing and reading would hang or misread.
		return false
	}
	return tui.Pause(s.stdout, s.keys, "Press Enter to return…")
}

// pauseResult is the submenus' shared tail: `if not pause(): return 0`. The exit code
// is always 0 — leaving the hub and returning to the menu differ only in the bool,
// which is the only part of this pair any caller reads.
func (s *interactive) pauseResult() (int, bool) {
	return 0, !s.pause()
}

// delegate is hub.py's DelegateRunner.run: argv from delegateArgv, child
// inherits the hub's streams and $AI_SPECS_HOME, raw mode is suspended around
// it. A child that cannot start at all is an error, not a crash.
func (s *interactive) delegate(action Action, extra []string) int {
	argv := delegateArgv(filepath.Join(s.home, "bin", "ai-specs"), action, extra, s.target)
	if s.term != nil {
		s.term.Suspend()
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = s.stdin
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr
	cmd.Env = childEnv(s.home)
	err := cmd.Run()
	if s.term != nil {
		if rerr := s.term.Resume(); rerr != nil {
			fmt.Fprintf(s.stderr, "ai-specs hub: %v\n", rerr)
			s.resumeFailed = true
		}
	}
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	fmt.Fprintf(s.stderr, "ai-specs hub: %v\n", err)
	return 1
}

// offerInit is hub.py's _offer_init. A decline or a failed init returns false,
// which makes the hub exit 0 without ever opening the menu.
func (s *interactive) offerInit() bool {
	fmt.Fprintf(s.stdout, "No ai-specs project found at %s.\n", s.target)
	answer, ok := tui.Confirm(s.stdout, s.keys, "Run the init wizard now?", true)
	if !ok || !answer {
		return false
	}
	if rc := s.delegate(ActionInit, nil); s.resumeFailed || rc != 0 || !isInitialized(s.target) {
		return false
	}
	// A successful init is followed by sync so the hub opens on a clean status.
	fmt.Fprintln(s.stdout, "✓ init complete — running sync to generate agent configs…")
	return s.delegate(ActionSync, nil) == 0 && !s.resumeFailed
}

// ── recipes submenu ──────────────────────────────────────────────────────────

var recipesSubmenuChoices = []tui.Choice{
	{Title: "List recipes", Value: "list"},
	{Title: "Add recipe", Value: "add"},
	{Title: "Remove recipe", Value: "remove"},
	{Title: "Configure recipes (whole project, configure-recipes)", Value: "configure"},
	{Title: "Back", Value: "back"},
}

// recipesSubmenu is hub.py's _run_recipes_submenu.
func (s *interactive) recipesSubmenu() (int, bool) {
	sub, ok := tui.Select(s.stdout, s.keys, "Recipes:", recipesSubmenuChoices)
	if !ok || sub == "back" {
		return 0, false
	}

	var rc int
	switch sub {
	case "list":
		rc = s.delegate(ActionRecipe, []string{"list"})
	case "add":
		recipes, err := recipe.List(s.target, s.home)
		if err != nil {
			fmt.Fprintf(s.stdout, "Recipes unavailable: %v\n", err)
			return s.pauseResult()
		}
		choices := recipeAddChoices(recipes)
		if len(choices) == 0 {
			fmt.Fprintln(s.stdout, "No catalog recipes available to add.")
			return s.pauseResult()
		}
		rid, ok := tui.Select(s.stdout, s.keys, "Recipe to add:", choices)
		if !ok {
			return 0, false
		}
		rc = s.delegate(ActionRecipe, []string{"add", rid})
	case "remove":
		recipes, err := recipe.List(s.target, s.home)
		if err != nil {
			fmt.Fprintf(s.stdout, "Recipes unavailable: %v\n", err)
			return s.pauseResult()
		}
		choices := recipeRemoveChoices(recipes)
		if len(choices) == 0 {
			fmt.Fprintln(s.stdout, "No recipes installed to remove.")
			return s.pauseResult()
		}
		rid, ok := tui.Select(s.stdout, s.keys, "Recipe to remove:", choices)
		if !ok {
			return 0, false
		}
		rc = s.delegate(ActionRecipe, []string{"remove", rid})
	case "configure":
		rc = s.delegate(ActionConfigureRecipes, nil)
	default:
		return 0, false
	}

	s.printResult(rc)
	return s.pauseResult()
}

// recipeAddChoices is hub.py's recipe_add_choices: `available` only.
func recipeAddChoices(recipes []recipe.Entry) []tui.Choice {
	var out []tui.Choice
	for _, r := range recipes {
		if r.Status != recipe.StatusAvailable {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.ID
		}
		out = append(out, tui.Choice{
			Title: fmt.Sprintf("%s (%s)  v%s", name, r.ID, r.Version),
			Value: r.ID,
		})
	}
	return out
}

// recipeRemoveChoices is hub.py's recipe_remove_choices: installed and disabled.
func recipeRemoveChoices(recipes []recipe.Entry) []tui.Choice {
	var out []tui.Choice
	for _, r := range recipes {
		if r.Status != recipe.StatusInstalled && r.Status != recipe.StatusDisabled {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.ID
		}
		out = append(out, tui.Choice{
			Title: fmt.Sprintf("%s (%s)  [%s]", name, r.ID, r.Status),
			Value: r.ID,
		})
	}
	return out
}

// ── skills submenu ───────────────────────────────────────────────────────────

// skillBuckets is hub.py's _SKILL_BUCKET_KEYS in order: the four origin buckets
// and their frozen titles.
var skillBuckets = []struct{ key, title string }{
	{"bundled", "Bundled (CLI-shipped)"},
	{"local", "Local / vendored (project)"},
	{"recipe", "Provided by recipes / catalog"},
	{"dep", "Registered deps"},
}

var skillsSubmenuChoices = []tui.Choice{
	{Title: "List skills (categorized)", Value: "list"},
	{Title: "Inspect a skill", Value: "inspect"},
	{Title: "Back", Value: "back"},
}

// skillEntry is one categorized skill: what the list shows and what inspect
// prints.
type skillEntry struct {
	id   string
	path string
	desc string
}

// skillsSubmenu is hub.py's _run_skills_submenu.
func (s *interactive) skillsSubmenu() (int, bool) {
	sub, ok := tui.Select(s.stdout, s.keys, "Skills:", skillsSubmenuChoices)
	if !ok || sub == "back" {
		return 0, false
	}
	buckets := categorizeSkills(s.target, s.home, s.stderr)

	switch sub {
	case "list":
		s.renderSkillBuckets(buckets)
		return s.pauseResult()
	case "inspect":
		var options []tui.Choice
		for _, bucket := range skillBuckets {
			for _, e := range buckets[bucket.key] {
				options = append(options, tui.Choice{
					Title: fmt.Sprintf("%s  [%s]", e.id, bucket.key),
					Value: e.id,
				})
			}
		}
		if len(options) == 0 {
			fmt.Fprintln(s.stdout, "No skills found to inspect.")
			return s.pauseResult()
		}
		sid, ok := tui.Select(s.stdout, s.keys, "Skill to inspect:", options)
		if !ok {
			return 0, false
		}
		if found, ok := findSkill(buckets, sid); ok {
			fmt.Fprintln(s.stdout, found.id)
			fmt.Fprintf(s.stdout, "  path: %s\n", filepath.Join(found.path, "SKILL.md"))
			desc := found.desc
			if desc == "" {
				desc = "(none)"
			}
			fmt.Fprintf(s.stdout, "  desc: %s\n", desc)
		} else {
			fmt.Fprintf(s.stdout, "Skill not found: %s\n", sid)
		}
		return s.pauseResult()
	}
	return 0, false
}

// renderSkillBuckets is hub.py's _render_skills_buckets.
func (s *interactive) renderSkillBuckets(buckets map[string][]skillEntry) {
	for _, bucket := range skillBuckets {
		fmt.Fprintf(s.stdout, "── %s ──\n", bucket.title)
		entries := buckets[bucket.key]
		if len(entries) == 0 {
			fmt.Fprintln(s.stdout, "  (none)")
		} else {
			for _, e := range entries {
				line := "  " + e.id
				if e.desc != "" {
					line += "  — " + e.desc
				}
				fmt.Fprintln(s.stdout, line)
			}
		}
		fmt.Fprintln(s.stdout)
	}
}

func findSkill(buckets map[string][]skillEntry, id string) (skillEntry, bool) {
	for _, bucket := range skillBuckets {
		for _, e := range buckets[bucket.key] {
			if e.id == id {
				return e, true
			}
		}
	}
	return skillEntry{}, false
}

// categorizeSkills is hub.py's categorize_skills: partition the resolved skills
// into the four buckets. A LOCAL skill whose id is also bundled is reported as
// bundled, which is the legacy quirk the Python code keeps.
func categorizeSkills(projectRoot, cliHome string, stderr io.Writer) map[string][]skillEntry {
	bundled := map[string]bool{}
	for _, name := range bundledSkillNames() {
		bundled[name] = true
	}
	collected := skills.CollectSkillsTo(projectRoot, cliHome, stderr)
	buckets := map[string][]skillEntry{}
	for _, bucket := range skillBuckets {
		buckets[bucket.key] = []skillEntry{}
	}
	for id, resolved := range collected {
		entry := skillEntry{id: id, path: resolved.Path, desc: skills.Description(resolved.Path)}
		if resolved.Source == "local" {
			if bundled[id] {
				buckets["bundled"] = append(buckets["bundled"], entry)
			} else {
				buckets["local"] = append(buckets["local"], entry)
			}
		} else if _, known := buckets[resolved.Source]; known {
			buckets[resolved.Source] = append(buckets[resolved.Source], entry)
		}
	}
	for key := range buckets {
		entries := buckets[key]
		sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	}
	return buckets
}

// bundledSkillNames mirrors doctor.bundled_skill_names() against the binary's
// embedded assets (card [Go 06]), which is the native source of truth the Go
// doctor uses.
func bundledSkillNames() []string {
	afs, err := assets.FS("bundled-skills")
	if err != nil {
		return nil
	}
	entries, err := fs.ReadDir(afs, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// ── agents submenu ───────────────────────────────────────────────────────────

// supportedAgents is hub.py's supported list, in order.
var supportedAgents = []string{"claude", "cursor", "opencode", "codex", "copilot", "gemini", "pi", "omp"}

// agentsSubmenu is hub.py's _run_agents_submenu.
func (s *interactive) agentsSubmenu() (int, bool) {
	manifestPath := filepath.Join(s.target, "ai-specs", "ai-specs.toml")
	if !isInitialized(s.target) {
		fmt.Fprintln(s.stdout, "Manifest not found — run ai-specs init first")
		return 0, false
	}
	// A manifest nobody can parse is NOT "no agents enabled": opening the picker on an
	// empty selection and saving it would drop the list the user already had
	// (odd/tasks/go-20-u3b-advisories.md S6).
	enabled, err := config.EnabledAgents(manifestPath)
	if err != nil {
		fmt.Fprintf(s.stderr, "ai-specs hub: cannot read %s: %v\n", manifestPath, err)
		return 0, false
	}
	current := map[string]bool{}
	for _, id := range enabled {
		current[id] = true
	}
	choices := make([]tui.Choice, 0, len(supportedAgents))
	checked := make([]bool, 0, len(supportedAgents))
	for _, id := range supportedAgents {
		choices = append(choices, tui.Choice{Title: id, Value: id})
		checked = append(checked, current[id])
	}
	selected, ok := tui.MultiSelect(s.stdout, s.keys, "Select agents to enable:", choices, checked)
	if !ok {
		return 0, false
	}
	if err := config.SetEnabledAgents(manifestPath, selected); err != nil {
		fmt.Fprintf(s.stderr, "ai-specs hub: %v\n", err)
		return 0, false
	}
	label := "(none)"
	if len(selected) > 0 {
		label = strings.Join(selected, ", ")
	}
	fmt.Fprintf(s.stdout, "  ✓ agents updated: %s\n", label)
	fmt.Fprintln(s.stdout, "  Run sync to regenerate agent configs.")
	return s.pauseResult()
}

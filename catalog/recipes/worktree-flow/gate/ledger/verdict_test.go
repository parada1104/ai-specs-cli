package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// vtNow is a fixed clock so conflict snapshots are deterministic.
var vtNow = time.Date(2026, 9, 13, 12, 30, 0, 0, time.UTC)

// availIdentity is the Identity form of the canonical store identity (ident("")).
func availIdentity() Identity {
	i := ident("")
	return Identity{CommonDir: i.CommonDir, Branch: i.Branch, Key: i.Key()}
}

// boundBinding is the activating witness (A4/D6): only state bound activates.
func boundBinding() Binding {
	return Binding{State: WitnessBound, RecipeID: "trello-mcp-workflow"}
}

// storeWithOpenItem is a store holding one open primary item carrying itemID.
func storeWithOpenItem(itemID string) Store {
	s := Store{V: StoreVersion}
	s.OpenItem(ident(""), "trello-mcp-workflow", t0)
	s.Items[len(s.Items)-1].ItemID = itemID
	return s
}

// consistentEvidence is the four-sides-agree case.
func consistentEvidence(value string) Evidence {
	return Evidence{Local: value, Remote: value, Code: value, Git: value}
}

// vWant is the expected verdict for one (scenario, mode) cell.
type vWant struct {
	decision string
	reason   string
	exit     int
	doctor   string
	active   bool
}

// assertVerdict pins decision, reason, exit code, doctor severity and activation.
func assertVerdict(t *testing.T, label string, got Verdict, want vWant) {
	t.Helper()
	if got.Decision != want.decision {
		t.Fatalf("%s: decision = %q, want %q (%+v)", label, got.Decision, want.decision, got)
	}
	if got.Reason != want.reason {
		t.Fatalf("%s: reason = %q, want %q", label, got.Reason, want.reason)
	}
	if got.ExitCode() != want.exit {
		t.Fatalf("%s: exit = %d, want %d", label, got.ExitCode(), want.exit)
	}
	if got.Doctor.Severity != want.doctor {
		t.Fatalf("%s: doctor severity = %q, want %q (%+v)", label, got.Doctor.Severity, want.doctor, got.Doctor)
	}
	if got.Doctor.Name != CheckName {
		t.Fatalf("%s: doctor name = %q, want %q", label, got.Doctor.Name, CheckName)
	}
	if got.Active != want.active {
		t.Fatalf("%s: active = %v, want %v", label, got.Active, want.active)
	}
}

// TestGradePostureMatrix pins the design posture matrix for the five checkpoints
// across the three modes, including always blocking missing/conflicted state
// (needs-item), pre-merge identity_unavailable, warn never blocking, and ask
// returning decision=ask with exit 0.
func TestGradePostureMatrixFiveCheckpointsThreeModes(t *testing.T) {
	if len(Checkpoints) != 5 {
		t.Fatalf("checkpoints = %v, want exactly five", Checkpoints)
	}
	if len(LedgerModes) != 3 {
		t.Fatalf("modes = %v, want exactly three", LedgerModes)
	}

	twoOpen := Store{V: StoreVersion}
	twoOpen.OpenItem(ident(""), "trello-mcp-workflow", t0)
	twoOpen.OpenItem(ident(""), "trello-mcp-workflow", t0)

	exempt := storeWithOpenItem("")
	exempt.Items[0].Exemption = "tracker.none"

	type scenario struct {
		name  string
		input func(checkpoint, mode string) Input
		want  map[string]vWant
	}
	scenarios := []scenario{
		{
			name: "open item with consistent evidence allows in every mode",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					Store: storeWithOpenItem("card-1"), Evidence: consistentEvidence("card-1"), Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionAllow, "", 0, SeverityOK, true},
				ModeAsk:    {DecisionAllow, "", 0, SeverityOK, true},
				ModeWarn:   {DecisionAllow, "", 0, SeverityOK, true},
			},
		},
		{
			name: "missing item blocks only in always",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					Store: Store{V: StoreVersion}, Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionBlock, ReasonNeedsItem, 2, SeverityOK, true},
				ModeAsk:    {DecisionAsk, ReasonNeedsItem, 0, SeverityOK, true},
				ModeWarn:   {DecisionAllow, ReasonNeedsItem, 0, SeverityOK, true},
			},
		},
		{
			name: "four-side disagreement has no default winner",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					Store:    storeWithOpenItem("local"),
					Evidence: Evidence{Local: "local", Remote: "remote", Code: "code", Git: "git"},
					Now:      vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionBlock, ReasonConflict, 2, SeverityWarn, true},
				ModeAsk:    {DecisionAsk, ReasonConflict, 0, SeverityWarn, true},
				ModeWarn:   {DecisionAllow, ReasonConflict, 0, SeverityWarn, true},
			},
		},
		{
			name: "identity unavailable blocks in always and is reported otherwise",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(),
					Identity: Identity{Reason: ReasonIdentityUnavailable}, Store: storeWithOpenItem("card-1"), Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionBlock, ReasonIdentityUnavailable, 2, SeverityOK, true},
				ModeAsk:    {DecisionAsk, ReasonIdentityUnavailable, 0, SeverityOK, true},
				ModeWarn:   {DecisionAllow, ReasonIdentityUnavailable, 0, SeverityOK, true},
			},
		},
		{
			name: "dormant witness skips the ledger in every mode",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode,
					Binding:  Binding{State: WitnessUnbound, Reason: ReasonWitnessMissing},
					Identity: availIdentity(), Store: storeWithOpenItem("card-1"), Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionDormant, ReasonWitnessMissing, 0, SeverityWarn, false},
				ModeAsk:    {DecisionDormant, ReasonWitnessMissing, 0, SeverityWarn, false},
				ModeWarn:   {DecisionDormant, ReasonWitnessMissing, 0, SeverityWarn, false},
			},
		},
		{
			name: "corrupt store is unevaluable and fails open",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					StoreErr: &StoreCorruptError{Path: "/repo/.git/ai-specs/ledger/state.json", Err: errors.New("bad json")},
					Now:      vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionUnevaluable, ReasonStoreCorrupt, 0, SeverityError, true},
				ModeAsk:    {DecisionUnevaluable, ReasonStoreCorrupt, 0, SeverityError, true},
				ModeWarn:   {DecisionUnevaluable, ReasonStoreCorrupt, 0, SeverityError, true},
			},
		},
		{
			name: "two open items are a conflict, not a pick",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					Store: twoOpen, Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionBlock, ReasonConflict, 2, SeverityWarn, true},
				ModeAsk:    {DecisionAsk, ReasonConflict, 0, SeverityWarn, true},
				ModeWarn:   {DecisionAllow, ReasonConflict, 0, SeverityWarn, true},
			},
		},
		{
			name: "recorded exemption allows in every mode",
			input: func(cp, mode string) Input {
				return Input{Checkpoint: cp, Mode: mode, Binding: boundBinding(), Identity: availIdentity(),
					Store: exempt, Now: vtNow}
			},
			want: map[string]vWant{
				ModeAlways: {DecisionAllow, ReasonExempt, 0, SeverityOK, true},
				ModeAsk:    {DecisionAllow, ReasonExempt, 0, SeverityOK, true},
				ModeWarn:   {DecisionAllow, ReasonExempt, 0, SeverityOK, true},
			},
		},
	}

	for _, sc := range scenarios {
		for _, cp := range Checkpoints {
			for _, mode := range LedgerModes {
				want, ok := sc.want[mode]
				if !ok {
					t.Fatalf("scenario %q has no expectation for mode %q", sc.name, mode)
				}
				label := sc.name + "/" + cp + "/" + mode
				assertVerdict(t, label, Grade(sc.input(cp, mode)), want)
			}
		}
	}
}

// TestGradeAskPromptAndExitZero pins that ask is a prompt outcome, never a stop:
// decision=ask, a prompt is carried, and the host exits 0.
func TestGradeAskPromptAndExitZero(t *testing.T) {
	for _, cp := range Checkpoints {
		got := Grade(Input{Checkpoint: cp, Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(),
			Store: Store{V: StoreVersion}, Now: vtNow})
		if got.Decision != DecisionAsk {
			t.Fatalf("%s: decision = %q, want ask", cp, got.Decision)
		}
		if got.ExitCode() != 0 {
			t.Fatalf("%s: ask exit = %d, want 0", cp, got.ExitCode())
		}
		if got.Prompt == nil || got.Prompt.Reason != ReasonNeedsItem {
			t.Fatalf("%s: prompt = %+v, want a needs-item prompt", cp, got.Prompt)
		}
	}
}

// TestGradeWarnNeverBlocksAnyCheckpoint pins "warn never blocks": missing,
// conflicted, collided and identity-unavailable state all allow with exit 0.
func TestGradeWarnNeverBlocksAnyCheckpoint(t *testing.T) {
	twoOpen := Store{V: StoreVersion}
	twoOpen.OpenItem(ident(""), "trello-mcp-workflow", t0)
	twoOpen.OpenItem(ident(""), "trello-mcp-workflow", t0)

	inputs := []struct {
		name string
		in   Input
	}{
		{"missing item", Input{Binding: boundBinding(), Identity: availIdentity(), Store: Store{V: StoreVersion}}},
		{"conflict", Input{Binding: boundBinding(), Identity: availIdentity(),
			Store: storeWithOpenItem("local"), Evidence: Evidence{Local: "local", Remote: "remote"}}},
		{"identity unavailable", Input{Binding: boundBinding(), Identity: Identity{Reason: ReasonIdentityUnavailable}}},
		{"two open items", Input{Binding: boundBinding(), Identity: availIdentity(), Store: twoOpen}},
	}
	for _, tc := range inputs {
		for _, cp := range Checkpoints {
			in := tc.in
			in.Checkpoint, in.Mode, in.Now = cp, ModeWarn, vtNow
			got := Grade(in)
			if got.Decision != DecisionAllow || got.ExitCode() != 0 {
				t.Fatalf("%s at %s: decision = %q (exit %d), want allow/0", tc.name, cp, got.Decision, got.ExitCode())
			}
		}
	}
}

// TestGradeAlwaysBlocksPreMergeIdentityUnavailable pins the spec scenario:
// always at pre-merge blocks an unavailable identity (exit 2).
func TestGradeAlwaysBlocksPreMergeIdentityUnavailable(t *testing.T) {
	got := Grade(Input{Checkpoint: CheckpointPreMerge, Mode: ModeAlways, Binding: boundBinding(),
		Identity: Identity{Reason: ReasonIdentityUnavailable}, Now: vtNow})
	assertVerdict(t, "pre-merge/always/identity_unavailable", got, vWant{DecisionBlock, ReasonIdentityUnavailable, 2, SeverityOK, true})
}

// TestGradeClosedOnlyStoreNeedsItemUnlessExplicitlyReported pins D17 at the
// predicate: a closed row is never primary, so a plain grade of a closed-only
// store reports needs-item. Only an explicit report item — the close write's own
// row — lets that one grade evaluate the closed row.
func TestGradeClosedOnlyStoreNeedsItemUnlessExplicitlyReported(t *testing.T) {
	var store Store
	closed := store.OpenItem(ident(""), "trello-mcp-workflow", t0)
	if err := store.CloseItem(closed.ID, Decision{At: "2026-09-13T13:00:00Z", Checkpoint: CheckpointArchiveClose}); err != nil {
		t.Fatal(err)
	}
	input := Input{
		Checkpoint: CheckpointArchiveClose,
		Mode:       ModeAlways,
		Binding:    boundBinding(),
		Identity:   availIdentity(),
		Store:      store,
		Now:        vtNow,
	}

	plain := Grade(input)
	if plain.Decision != DecisionBlock || plain.Reason != ReasonNeedsItem {
		t.Fatalf("closed-only plain grade = %q/%q, want block/needs-item", plain.Decision, plain.Reason)
	}
	if plain.Item != nil {
		t.Fatalf("plain grade selected a closed row: %+v", plain.Item)
	}

	reported, ok := store.LatestClosed(availIdentity().Key)
	if !ok {
		t.Fatal("LatestClosed found no closed row")
	}
	input.ReportItem = &reported
	got := Grade(input)
	if got.Decision != DecisionAllow || got.Reason != "" {
		t.Fatalf("reported close grade = %q/%q, want allow", got.Decision, got.Reason)
	}
	if got.Item == nil || got.Item.Status != StatusClosed {
		t.Fatalf("reported close grade item = %+v, want the closed row", got.Item)
	}
}

// TestGradeCheckpointScopedOptOutD19 pins D19: an ask opt-out allows only the
// checkpoint that was answered; the next checkpoint asks again.
func TestGradeCheckpointScopedOptOutD19(t *testing.T) {
	s := storeWithOpenItem("")
	s.Items[0].Decisions = append(s.Items[0].Decisions,
		Decision{At: t0.Format(time.RFC3339), Checkpoint: CheckpointApplyStart, Kind: DecisionOptOut, Choice: "continue"})
	base := Input{Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(), Store: s,
		Evidence: Evidence{Remote: "remote"}, Now: vtNow}

	apply := base
	apply.Checkpoint = CheckpointApplyStart
	assertVerdict(t, "apply-start opt-out", Grade(apply), vWant{DecisionAllow, ReasonOptOut, 0, SeverityOK, true})

	pre := base
	pre.Checkpoint = CheckpointPreMerge
	assertVerdict(t, "pre-merge after apply-start opt-out", Grade(pre), vWant{DecisionAsk, ReasonConflict, 0, SeverityWarn, true})
}

// TestGradeAdjudicatedConflictAllows pins that a persisted human choice resolves
// the conflict for that checkpoint and allows the grade.
func TestGradeAdjudicatedConflictAllows(t *testing.T) {
	s := storeWithOpenItem("local")
	s.Items[0].Decisions = append(s.Items[0].Decisions,
		Decision{At: t0.Format(time.RFC3339), Checkpoint: CheckpointPreMerge, Kind: DecisionAdjudicate, Choice: "local"})
	got := Grade(Input{Checkpoint: CheckpointPreMerge, Mode: ModeAlways, Binding: boundBinding(), Identity: availIdentity(),
		Store: s, Evidence: Evidence{Local: "local", Remote: "remote"}, Now: vtNow})
	assertVerdict(t, "pre-merge adjudicated", got, vWant{DecisionAllow, ReasonAdjudicated, 0, SeverityOK, true})
	if got.Conflict != nil {
		t.Fatalf("adjudicated conflict still reported: %+v", got.Conflict)
	}
}

// TestGradeAskConflictPromptCarriesEveryChoice pins that a conflict prompt lists
// the design human choices and prints the four evidence sides.
func TestGradeAskConflictPromptCarriesEveryChoice(t *testing.T) {
	ev := Evidence{Local: "local", Remote: "remote", Code: "code", Git: "git"}
	got := Grade(Input{Checkpoint: CheckpointPreMerge, Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(),
		Store: storeWithOpenItem("local"), Evidence: ev, Now: vtNow})
	if got.Decision != DecisionAsk || got.ExitCode() != 0 {
		t.Fatalf("decision = %q exit %d, want ask/0", got.Decision, got.ExitCode())
	}
	if got.Conflict == nil || got.Conflict.Sides != ev {
		t.Fatalf("conflict = %+v, want the four sides %+v", got.Conflict, ev)
	}
	if got.Prompt == nil {
		t.Fatalf("conflict prompt missing")
	}
	for _, choice := range DecisionChoices {
		if !containsChoice(got.Prompt.Choices, choice) {
			t.Fatalf("prompt choices %v missing %q", got.Prompt.Choices, choice)
		}
	}
}

func containsChoice(choices []string, want string) bool {
	for _, c := range choices {
		if c == want {
			return true
		}
	}
	return false
}

// TestGradeConflictSnapshotUsesItemSnapshotAsLocalSide pins that an omitted local
// side defaults to the ledger item snapshot.
func TestGradeConflictSnapshotUsesItemSnapshotAsLocalSide(t *testing.T) {
	got := Grade(Input{Checkpoint: CheckpointPreMerge, Mode: ModeAsk, Binding: boundBinding(), Identity: availIdentity(),
		Store: storeWithOpenItem("card-abc"), Evidence: Evidence{Remote: "card-xyz"}, Now: vtNow})
	if got.Conflict == nil {
		t.Fatalf("expected a conflict for a remote/local disagreement")
	}
	if got.Conflict.Sides.Local != "card-abc" {
		t.Fatalf("local side = %q, want the item snapshot card-abc", got.Conflict.Sides.Local)
	}
	if got.Conflict.RecordedAt != vtNow.UTC().Format(time.RFC3339) {
		t.Fatalf("recorded_at = %q, want the injected clock", got.Conflict.RecordedAt)
	}
}

// TestGradeDormantBeatsCorruptStore pins the short-circuit order: an inactive
// ledger never inspects the store, so a corrupt file cannot turn dormancy into
// an infrastructure error.
func TestGradeDormantBeatsCorruptStore(t *testing.T) {
	got := Grade(Input{Checkpoint: CheckpointWorkStart, Mode: ModeAlways,
		Binding:  Binding{State: WitnessAmbiguous, Candidates: []string{"a", "b"}},
		Identity: availIdentity(),
		StoreErr: &StoreCorruptError{Path: "state.json", Err: errors.New("bad json")}, Now: vtNow})
	assertVerdict(t, "dormant beats corrupt", got, vWant{DecisionDormant, WitnessAmbiguous, 0, SeverityWarn, false})
}

// TestGradeUnknownCheckpointIsUnevaluable pins the fail-open handling of an
// unknown checkpoint value: unevaluable, exit 0, doctor ERROR.
func TestGradeUnknownCheckpointIsUnevaluable(t *testing.T) {
	got := Grade(Input{Checkpoint: "bogus", Mode: ModeAlways, Binding: boundBinding(), Identity: availIdentity(), Now: vtNow})
	assertVerdict(t, "unknown checkpoint", got, vWant{DecisionUnevaluable, ReasonUnknownCheckpoint, 0, SeverityError, false})
}

// TestCheckpointAndModeEnumSurface pins the exact design vocabularies.
func TestCheckpointAndModeEnumSurface(t *testing.T) {
	wantCheckpoints := []string{"work-start", "apply-start", "pr-review", "pre-merge", "archive-close"}
	if strings.Join(Checkpoints, ",") != strings.Join(wantCheckpoints, ",") {
		t.Fatalf("checkpoints = %v, want %v", Checkpoints, wantCheckpoints)
	}
	wantModes := []string{"always", "ask", "warn"}
	if strings.Join(LedgerModes, ",") != strings.Join(wantModes, ",") {
		t.Fatalf("modes = %v, want %v", LedgerModes, wantModes)
	}
	for _, cp := range wantCheckpoints {
		if !ValidCheckpoint(cp) {
			t.Fatalf("ValidCheckpoint(%q) = false", cp)
		}
	}
	if ValidCheckpoint("") || ValidCheckpoint("bogus") {
		t.Fatalf("ValidCheckpoint accepted an unknown checkpoint")
	}
	for _, m := range wantModes {
		if !ValidMode(m) {
			t.Fatalf("ValidMode(%q) = false", m)
		}
	}
	if ValidMode("") || ValidMode("off") {
		t.Fatalf("ValidMode accepted a non-ledger mode")
	}
	if got := NormalizeMode(""); got != ModeWarn {
		t.Fatalf("NormalizeMode(\"\") = %q, want warn (warn-first adoption)", got)
	}
	if got := NormalizeMode("bogus"); got != ModeWarn {
		t.Fatalf("NormalizeMode(bogus) = %q, want warn", got)
	}
	if got := NormalizeMode(ModeAlways); got != ModeAlways {
		t.Fatalf("NormalizeMode(always) = %q, want always", got)
	}
}

// TestGradeAmbiguousAndDeclaredCarryTheirOwnDoctorSeverity pins A10 for the two
// remaining dormant states.
func TestGradeAmbiguousAndDeclaredCarryTheirOwnDoctorSeverity(t *testing.T) {
	ambiguous := Grade(Input{Checkpoint: CheckpointWorkStart, Mode: ModeWarn,
		Binding: Binding{State: WitnessAmbiguous, Candidates: []string{"a", "b"}}})
	assertVerdict(t, "ambiguous", ambiguous, vWant{DecisionDormant, WitnessAmbiguous, 0, SeverityWarn, false})
	if len(ambiguous.Doctor.Message) == 0 {
		t.Fatalf("ambiguous doctor message is empty")
	}

	declared := Grade(Input{Checkpoint: CheckpointWorkStart, Mode: ModeWarn,
		Binding: Binding{State: WitnessDeclaredNotBound}})
	assertVerdict(t, "declared-not-bound", declared, vWant{DecisionDormant, WitnessDeclaredNotBound, 0, SeverityWarn, false})
}

// TestExitCodeBlockOnly pins "exit 2 only when the host must stop".
func TestExitCodeBlockOnly(t *testing.T) {
	cases := map[string]int{
		DecisionAllow:       0,
		DecisionAsk:         0,
		DecisionDormant:     0,
		DecisionUnevaluable: 0,
		DecisionBlock:       2,
	}
	for decision, want := range cases {
		got := Verdict{Decision: decision}.ExitCode()
		if got != want {
			t.Fatalf("ExitCode(%q) = %d, want %d", decision, got, want)
		}
	}
}

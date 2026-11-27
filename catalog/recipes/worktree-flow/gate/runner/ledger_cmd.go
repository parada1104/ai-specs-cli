package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// ledgerOptions is the parsed --ledger flag surface (design CLI contract). The
// worktree gate flags are never read here: the ledger does not couple to
// worktree semantics (D4/A1).
type ledgerOptions struct {
	checkpoint string
	mode       string
	// gateMode is the raw stamped legacy tracker gate_mode hint. It is consulted
	// only when --ledger-mode is omitted, and is never resolved against the
	// worktree gate policy here.
	gateMode    string
	projectRoot string
	witness     string
	store       string
	evidence    string
	decide      string
	write       string
	// reconcile is the opt-in, off-hot-path comparison: an observation payload
	// path plus the event the caller asks to compare. Absent, the stdout contract
	// is the legacy verdict byte for byte.
	reconcile      string
	reconcileEvent string
}

// ledgerIdentJSON is the design identity object: exactly four keys, with change
// null when the identity carries no slug.
type ledgerIdentJSON struct {
	CommonDir string  `json:"common_dir"`
	Branch    string  `json:"branch"`
	Change    *string `json:"change"`
	Key       string  `json:"key"`
}

// ledgerItemJSON is the item as a host sees it: the tracker record plus the
// worktree lifecycle status the tracker ledger does not store. The files stay
// split (L8); the stdout contract keeps answering exactly what it always answered,
// with status in the position it always had, because hosts and parity pin these
// bytes.
type ledgerItemJSON struct {
	ID         string              `json:"id"`
	Identity   ledger.ItemIdentity `json:"identity"`
	Status     string              `json:"status"`
	ItemID     string              `json:"item_id"`
	ProviderID string              `json:"provider_id"`
	NativeType string              `json:"native_type"`
	URL        string              `json:"url"`
	State      string              `json:"state"`
	Provider   json.RawMessage     `json:"provider"`
	Exemption  string              `json:"exemption"`
	Conflict   json.RawMessage     `json:"conflict"`
	Decisions  []ledger.Decision   `json:"decisions"`
}

// itemWithStatus is the joined item as the host sees it.
func itemWithStatus(item *ledger.Item, status string) *ledgerItemJSON {
	if item == nil {
		return nil
	}
	return &ledgerItemJSON{
		ID:         item.ID,
		Identity:   item.Identity,
		Status:     status,
		ItemID:     item.ItemID,
		ProviderID: item.ProviderID,
		NativeType: item.NativeType,
		URL:        item.URL,
		State:      item.State,
		Provider:   item.Provider,
		Exemption:  item.Exemption,
		Conflict:   item.Conflict,
		Decisions:  item.Decisions,
	}
}

// ledgerOut is the design stdout contract consumed by hosts, doctor and parity.
// Write is the DW4 sidecar: present only on a --write invocation that succeeded.
type ledgerOut struct {
	Capability string               `json:"capability"`
	Active     bool                 `json:"active"`
	Checkpoint string               `json:"checkpoint"`
	Mode       string               `json:"mode"`
	Decision   string               `json:"decision"`
	Reason     string               `json:"reason"`
	Identity   ledgerIdentJSON      `json:"identity"`
	Item       *ledgerItemJSON      `json:"item"`
	Conflict   *ledger.Conflict     `json:"conflict"`
	Prompt     *ledger.Prompt       `json:"prompt"`
	Doctor     ledger.DoctorFinding `json:"doctor"`
	Write      *ledger.WriteOutcome `json:"write,omitempty"`
	Reconcile  *ledgerReconcileJSON `json:"reconcile,omitempty"`
}

// runLedger is the --ledger dispatcher: acquire read-only inputs, apply at most
// one write or human decision, grade once, print the JSON verdict, and exit 0/2.
// Every failure except a --write/--decide persist fails open, matching the gate's
// blast radius (design "Unevaluable / outage").
func runLedger(opts ledgerOptions, stdout, stderr io.Writer) int {
	if opts.write != "" && opts.decide != "" {
		// DW2: the two mutation vehicles are mutually exclusive. Refuse before any
		// IO so nothing is persisted and no verdict is printed.
		fmt.Fprintln(stderr, "worktree-gate: ledger --write and --decide are mutually exclusive")
		return 2
	}
	if opts.reconcile != "" && (opts.write != "" || opts.decide != "") {
		// Reconciliation describes the graded item and is read-only, so it is refused
		// with a mutation vehicle before any IO: a comparison can never be asked about
		// a store the same invocation is changing. The only other write this
		// invocation could reach, recording a conflict snapshot, is suppressed below.
		fmt.Fprintln(stderr, "worktree-gate: ledger --reconcile cannot be combined with --write or --decide")
		return 2
	}

	dir := opts.projectRoot
	if dir == "" {
		dir = processCwd()
	}

	// The owner repository supplies the common dir; the planning root supplies
	// the change slug. Both default to --project-root (assumption 6).
	common := ""
	if c := gitCommon(dir); c != "" {
		common = RealPath(c)
	}
	witnessPath := opts.witness
	if witnessPath == "" && common != "" {
		witnessPath = ledger.WitnessPath(common)
	}
	// An explicit --store names one file: it is read and written as given, with no
	// pre-split fallback. Only the repository-derived location carries the
	// one-version double read (S11).
	loc := ledger.LedgersAtStore(opts.store)
	if opts.store == "" && common != "" {
		loc = ledger.LedgersAt(common)
	}

	binding := ledger.ReadBinding(witnessPath)
	view, reports, storeErr := loc.Load()
	if storeErr == nil {
		reports.Note(stderr, "worktree-gate")
	}
	if opts.write != "" || opts.decide != "" {
		// A backup that cannot be written is reported, never silent. The copy is
		// one-shot, so this is a no-op once it exists.
		loc.Backup().Note(stderr, "worktree-gate")
	}

	// The effective mode is the explicit --ledger-mode when given (an explicit
	// normalized mode, unchanged for existing callers); otherwise Go resolves it
	// from env, the bound recipe's configuration and the stamped legacy hint. An
	// `off` resolution disables the checkpoint before any grading, matching the
	// shell behavior this resolver takes ownership of.
	mode := opts.mode
	if mode == "" {
		mode = resolveLedgerMode(dir, binding, opts.gateMode, stderr)
		if mode == ledgerModeOff {
			fmt.Fprintf(stderr, "worktree-gate: ledger_mode off; skipping %s\n", opts.checkpoint)
			return 0
		}
	}

	facts := ledger.Facts(gitMemo)
	ident := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, PlanningRoot: dir, Facts: facts})
	// A stored slug survives its folder being archived mid-item (A11): when the
	// open item already names a change, that slug wins over re-derivation.
	ident = ledgerIdentityWithStoredSlug(ident, view, storeErr, dir, facts)

	if opts.decide != "" && binding.Active() {
		if err := persistLedgerDecision(loc, ident.Key, opts.checkpoint, opts.decide); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: ledger --decide failed: %v\n", err)
			return 2
		}
		view, _, storeErr = loc.Load()
	}

	// A write is explicit, so it is attempted whenever it was asked for: the
	// post-write grade is what reflects the result, and a failure persists nothing
	// and exits 2 with no stdout JSON (L6).
	var writeOutcome *ledger.WriteOutcome
	// reportItem is the just-closed row a close write hands to the post-write grade:
	// a close leaves no open primary, so without it the grade would report
	// needs-item for the row the same invocation just persisted (D17 keeps that row
	// out of Primary, so it is passed explicitly and read-only).
	var reportItem *ledger.Item
	if opts.write != "" {
		if ledgerWriteKind(opts.write) == ledger.WriteClose {
			// A close targets the row an earlier open created, so its stored slug
			// is recovered even from a closed row; this is close-only and never
			// lets that row become primary for new work (D17).
			ident = ledgerIdentityForClose(ident, view, storeErr, dir, facts)
		}
		outcome, err := applyLedgerWrite(loc, ident, binding, opts.checkpoint, opts.write)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: ledger --write failed: %v\n", err)
			return 2
		}
		writeOutcome = &outcome
		view, _, storeErr = loc.Load()
		// The explicit slug a write just recorded is the stored slug: the re-grade
		// must select the item that write created, not a stale ambiguous identity.
		ident = ledgerIdentityWithStoredSlug(ident, view, storeErr, dir, facts)
		if outcome.Kind == ledger.WriteClose {
			if closed, ok := view.LatestClosed(ident.Key); ok {
				reportItem = &closed
			}
		}
	}

	ev, err := loadLedgerEvidence(opts.evidence)
	if err != nil {
		// Host-supplied evidence is an input, not a grader: a bad file fails open.
		fmt.Fprintf(stderr, "worktree-gate: ledger evidence ignored: %v\n", err)
		ev = ledger.Evidence{}
	}

	now := time.Now()
	verdict := ledger.Grade(ledger.Input{
		Checkpoint: opts.checkpoint,
		Mode:       mode,
		Binding:    binding,
		Identity:   ident,
		View:       view,
		StoreErr:   storeErr,
		Evidence:   ev,
		Now:        now,
		ReportItem: reportItem,
	})

	// Grading is pure: no grade path persists the conflict snapshot, in any mode.
	// The conflict itself still reaches the verdict and the sidecar below, so nothing
	// is hidden from the caller.

	// Reconciliation is explicit and off the hot path: it rides along as a sidecar
	// and never alters the graded decision, writes the store, or resolves anything.
	var reconcileOut *ledgerReconcileJSON
	if opts.reconcile != "" {
		reconcileOut = reconcileSidecar(dir, binding, verdict, opts.reconcile, opts.reconcileEvent, now)
	}

	payload, err := json.Marshal(newLedgerOut(verdict, writeOutcome, reconcileOut, view))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: ledger: %v\n", err)
		return 0
	}
	fmt.Fprintln(stdout, string(payload))
	return verdict.ExitCode()
}

// newLedgerOut maps a verdict onto the exact stdout contract. The DW4 write and
// the reconciliation sidecars are omitted entirely when they were not requested.
func newLedgerOut(v ledger.Verdict, write *ledger.WriteOutcome, reconcile *ledgerReconcileJSON, view ledger.View) ledgerOut {
	var item *ledgerItemJSON
	if v.Item != nil {
		item = itemWithStatus(v.Item, view.Status(v.Item.ID))
	}
	out := ledgerOut{
		Capability: ledger.CapabilityTracker,
		Active:     v.Active,
		Checkpoint: v.Checkpoint,
		Mode:       v.Mode,
		Decision:   v.Decision,
		Reason:     v.Reason,
		Item:       item,
		Conflict:   v.Conflict,
		Prompt:     v.Prompt,
		Doctor:     v.Doctor,
		Write:      write,
		Reconcile:  reconcile,
		Identity: ledgerIdentJSON{
			CommonDir: v.Identity.CommonDir,
			Branch:    v.Identity.Branch,
			Key:       v.Identity.Key,
		},
	}
	if v.Identity.Change != "" {
		change := v.Identity.Change
		out.Identity.Change = &change
	}
	return out
}

// ledgerIdentityWithStoredSlug re-derives the identity with the slug already
// recorded on the single open item for its common dir and branch (A11). A stored
// slug survives its folder being archived mid-item, and an explicit --write change
// becomes the stored slug so the re-grade selects what the write recorded. A
// branch-level bind deliberately stores an empty slug; that is still a found row
// and must not fall back to the planning tree's current slug.
func ledgerIdentityWithStoredSlug(ident ledger.Identity, view ledger.View, storeErr error, dir string, facts ledger.Facts) ledger.Identity {
	if !ident.Available() || storeErr != nil {
		return ident
	}
	slug, found := storedLedgerSlug(view, ident.CommonDir, ident.Branch)
	if !found || (slug == ident.Change && ident.Collision == "") {
		return ident
	}
	if slug == "" {
		ident.Change = ""
		ident.Collision = ""
		ident.Key = ledger.IdentityKey(ident.CommonDir, ident.Branch, "")
		return ident
	}
	return ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, PlanningRoot: dir, StoredSlug: slug, Facts: facts})
}

// storedLedgerSlug returns the change slug already recorded on the single open
// item for the common dir and branch, plus whether a row was found. The boolean
// distinguishes a deliberate branch-level empty slug from no open row (A11).
func storedLedgerSlug(view ledger.View, common, branch string) (string, bool) {
	var changes []string
	for _, item := range view.Worktree.Items {
		if item.Status != ledger.StatusOpen {
			continue
		}
		if item.Identity.CommonDir != common || item.Identity.Branch != branch {
			continue
		}
		changes = append(changes, item.Identity.Change)
	}
	if len(changes) == 1 {
		return changes[0], true
	}
	return "", false
}

// ledgerIdentityForClose re-derives the identity for an explicit close write,
// recovering the change slug from the row the close targets: the single open
// primary when one exists, otherwise the latest closed row for this common dir
// and branch. It is close-only by design — a plain grade or a new open never
// adopts a closed row's slug, so a closed row can never become primary for new
// work (D17). An idempotent close retry therefore still targets the row a prior
// close persisted after its change folder was archived, or while several active
// change folders make the fresh identity ambiguous.
func ledgerIdentityForClose(ident ledger.Identity, view ledger.View, storeErr error, dir string, facts ledger.Facts) ledger.Identity {
	if !ident.Available() || storeErr != nil {
		return ident
	}
	slug, found := storedLedgerSlug(view, ident.CommonDir, ident.Branch)
	if !found {
		slug = latestClosedSlug(view, ident.CommonDir, ident.Branch)
	}
	if slug == ident.Change && ident.Collision == "" {
		return ident
	}
	if slug == "" {
		ident.Change = ""
		ident.Collision = ""
		ident.Key = ledger.IdentityKey(ident.CommonDir, ident.Branch, "")
		return ident
	}
	return ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, PlanningRoot: dir, StoredSlug: slug, Facts: facts})
}

// latestClosedSlug returns the change slug on the most recently stored closed
// row for the common dir and branch, so a close retry after an archive still
// targets the row the original close persisted. It is read-only and only ever
// consulted by the explicit close path.
func latestClosedSlug(view ledger.View, common, branch string) string {
	for i := len(view.Worktree.Items) - 1; i >= 0; i-- {
		item := view.Worktree.Items[i]
		if item.Status != ledger.StatusClosed {
			continue
		}
		if item.Identity.CommonDir != common || item.Identity.Branch != branch {
			continue
		}
		return item.Identity.Change
	}
	return ""
}

// ledgerWriteKind is the normalized machine-write verb of a --write payload, or
// "" when the payload is not decodable. It decides the close-only identity
// recovery before the write; an undecodable payload still fails closed in
// applyLedgerWrite with the same exit 2 and stderr as before.
func ledgerWriteKind(raw string) string {
	var req ledger.WriteRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return ""
	}
	return req.Normalize().Kind
}

// loadLedgerEvidence reads the host-supplied evidence file. Local defaults to
// the store snapshot inside Grade; the file supplies remote/code/git.
func loadLedgerEvidence(path string) (ledger.Evidence, error) {
	if path == "" {
		return ledger.Evidence{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ledger.Evidence{}, err
	}
	var file struct {
		Local  string `json:"local"`
		Remote string `json:"remote"`
		Code   string `json:"code"`
		Git    string `json:"git"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return ledger.Evidence{}, fmt.Errorf("invalid evidence JSON: %w", err)
	}
	return ledger.Evidence{Local: file.Local, Remote: file.Remote, Code: file.Code, Git: file.Git}, nil
}

// applyLedgerWrite parses and applies one --write payload under the bounded store
// lock. The binding's recipe id is the provider recorded when the write has to
// open the item; the checkpoint flag is the write's checkpoint.
func applyLedgerWrite(loc ledger.Ledgers, ident ledger.Identity, binding ledger.Binding, checkpoint, raw string) (ledger.WriteOutcome, error) {
	var req ledger.WriteRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return ledger.WriteOutcome{}, fmt.Errorf("invalid --write JSON: %w", err)
	}
	return ledger.ApplyWrite(loc, ledger.ApplyWriteRequest{
		Identity: ledger.ItemIdentity{
			CommonDir: ident.CommonDir,
			Branch:    ident.Branch,
			Change:    ident.Change,
		},
		Checkpoint: checkpoint,
		ProviderID: binding.RecipeID,
		Collision:  ident.Collision,
		Request:    req,
	}, time.Now())
}

// persistLedgerDecision parses and appends the human answer. The checkpoint flag
// is the default when the payload omits it.
func persistLedgerDecision(loc ledger.Ledgers, key, checkpoint, raw string) error {
	var req ledger.DecisionRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return fmt.Errorf("invalid --decide JSON: %w", err)
	}
	if req.Checkpoint == "" {
		req.Checkpoint = checkpoint
	}
	_, err := ledger.PersistDecision(loc, key, req, time.Now())
	return err
}

// ledgerWriteSelftest exercises the machine-write surface in-process and offline:
// every request rule of the closed write vocabulary, plus the open-if-absent
// invariant that makes a retried open a no-op instead of a self-inflicted
// collision. No store is touched and no network is used.
func ledgerWriteSelftest() error {
	invalid := []ledger.WriteRequest{
		{},                         // no kind
		{Kind: "invented"},         // unknown kind
		{Kind: ledger.WriteBind},   // bind without item_id
		{Kind: ledger.WriteLink},   // link without item_id
		{Kind: ledger.WriteExempt}, // exempt without a reason
	}
	for _, req := range invalid {
		if err := req.Normalize().Validate(ledger.CheckpointApplyStart); err == nil {
			return fmt.Errorf("write %+v was accepted", req)
		}
	}
	valid := ledger.WriteRequest{Kind: ledger.WriteOpen}
	if err := valid.Normalize().Validate(ledger.CheckpointApplyStart); err != nil {
		return fmt.Errorf("open write rejected: %v", err)
	}
	if err := valid.Normalize().Validate("bogus"); err == nil {
		return fmt.Errorf("write accepted an unknown checkpoint")
	}
	if len(ledger.WriteKinds) != 5 {
		return fmt.Errorf("write kinds = %v, want the closed five-verb set", ledger.WriteKinds)
	}

	// Open-if-absent is idempotent in memory: the retried open adds no row and the
	// first one is the single primary.
	ident := ledger.ItemIdentity{CommonDir: "/selftest/.git", Branch: "selftest"}
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var view ledger.View
	if _, created, err := view.OpenIfAbsent(ident, "selftest", stamp); err != nil || !created {
		return fmt.Errorf("first open: created=%v err=%v", created, err)
	}
	if _, created, err := view.OpenIfAbsent(ident, "selftest", stamp); err != nil || created {
		return fmt.Errorf("retried open must be a no-op: created=%v err=%v", created, err)
	}
	if _, err := view.Primary(ident.Key()); err != nil {
		return fmt.Errorf("open must leave exactly one primary: %v", err)
	}
	if len(view.Tracker.Items) != 1 {
		return fmt.Errorf("tracker rows = %d, want the one open recorded", len(view.Tracker.Items))
	}
	return nil
}

// ledgerSelftest exercises the ledger enum and posture invariants in-process,
// with no network and no store IO. --selftest prints its usual "ok" only when
// this returns nil, so the release toolchain verifies both modes.
func ledgerSelftest() error {
	if err := ledgerWriteSelftest(); err != nil {
		return err
	}
	for _, cp := range ledger.Checkpoints {
		if !ledger.ValidCheckpoint(cp) {
			return fmt.Errorf("checkpoint %q is not valid", cp)
		}
	}
	for _, mode := range ledger.LedgerModes {
		if !ledger.ValidMode(mode) {
			return fmt.Errorf("mode %q is not valid", mode)
		}
	}
	if got := ledger.NormalizeMode(""); got != ledger.ModeWarn {
		return fmt.Errorf("default mode = %q, want %q", got, ledger.ModeWarn)
	}

	bound := ledger.Binding{State: ledger.WitnessBound, RecipeID: "selftest"}
	ident := ledger.Identity{
		CommonDir: "/selftest/.git",
		Branch:    "selftest",
		Key:       ledger.IdentityKey("/selftest/.git", "selftest", ""),
	}
	empty := ledger.View{}

	dormant := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointWorkStart, Mode: ledger.ModeWarn,
		Binding: ledger.Binding{State: ledger.WitnessUnbound, Reason: ledger.ReasonWitnessMissing},
	})
	if dormant.Decision != ledger.DecisionDormant {
		return fmt.Errorf("missing witness graded %q, want %q", dormant.Decision, ledger.DecisionDormant)
	}

	warned := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointApplyStart, Mode: ledger.ModeWarn,
		Binding: bound, Identity: ident, View: empty,
	})
	if warned.Decision != ledger.DecisionAllow || warned.ExitCode() != 0 {
		return fmt.Errorf("warn graded %q (exit %d), want allow/0", warned.Decision, warned.ExitCode())
	}

	blocked := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointApplyStart, Mode: ledger.ModeAlways,
		Binding: bound, Identity: ident, View: empty,
	})
	if blocked.Decision != ledger.DecisionBlock || blocked.Reason != ledger.ReasonNeedsItem || blocked.ExitCode() != 2 {
		return fmt.Errorf("always graded %q/%q (exit %d), want block/needs-item/2", blocked.Decision, blocked.Reason, blocked.ExitCode())
	}

	asked := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointPreMerge, Mode: ledger.ModeAsk,
		Binding: bound, Identity: ident, View: empty,
	})
	if asked.Decision != ledger.DecisionAsk || asked.ExitCode() != 0 {
		return fmt.Errorf("ask graded %q (exit %d), want ask/0", asked.Decision, asked.ExitCode())
	}

	if !(ledger.Evidence{Local: "a", Remote: "b"}).Conflict() {
		return fmt.Errorf("disagreeing sides were not a conflict")
	}
	if (ledger.Evidence{Local: "a", Remote: "a"}).Conflict() {
		return fmt.Errorf("agreeing sides were a conflict")
	}
	return nil
}

package main

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
	checkpoint  string
	mode        string
	projectRoot string
	witness     string
	store       string
	evidence    string
	decide      string
}

// ledgerIdentJSON is the design identity object: exactly four keys, with change
// null when the identity carries no slug.
type ledgerIdentJSON struct {
	CommonDir string  `json:"common_dir"`
	Branch    string  `json:"branch"`
	Change    *string `json:"change"`
	Key       string  `json:"key"`
}

// ledgerOut is the design stdout contract consumed by hosts, doctor and parity.
type ledgerOut struct {
	Capability string               `json:"capability"`
	Active     bool                 `json:"active"`
	Checkpoint string               `json:"checkpoint"`
	Mode       string               `json:"mode"`
	Decision   string               `json:"decision"`
	Reason     string               `json:"reason"`
	Identity   ledgerIdentJSON      `json:"identity"`
	Item       *ledger.Item         `json:"item"`
	Conflict   *ledger.Conflict     `json:"conflict"`
	Prompt     *ledger.Prompt       `json:"prompt"`
	Doctor     ledger.DoctorFinding `json:"doctor"`
}

// runLedger is the --ledger dispatcher: acquire read-only inputs, grade once,
// print the JSON verdict, and exit 0/2. Every failure except a --decide persist
// fails open, matching the gate's blast radius (design "Unevaluable / outage").
func runLedger(opts ledgerOptions, stdout, stderr io.Writer) int {
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
	storePath := opts.store
	if storePath == "" && common != "" {
		storePath = ledger.StorePath(common)
	}

	binding := ledger.ReadBinding(witnessPath)
	store, storeErr := ledger.LoadStore(storePath)

	facts := ledger.Facts(gitMemo)
	ident := ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, PlanningRoot: dir, Facts: facts})
	// A stored slug survives its folder being archived mid-item (A11): when the
	// open item already names a change, that slug wins over re-derivation.
	if ident.Available() && storeErr == nil {
		if slug := storedLedgerSlug(store, ident.CommonDir, ident.Branch); slug != "" && slug != ident.Change {
			ident = ledger.DeriveIdentity(ledger.IdentityOptions{Dir: dir, PlanningRoot: dir, StoredSlug: slug, Facts: facts})
		}
	}

	if opts.decide != "" && binding.Active() {
		if err := persistLedgerDecision(storePath, ident.Key, opts.checkpoint, opts.decide); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: ledger --decide failed: %v\n", err)
			return 2
		}
		store, storeErr = ledger.LoadStore(storePath)
	}

	ev, err := loadLedgerEvidence(opts.evidence)
	if err != nil {
		// Host-supplied evidence is an input, not a grader: a bad file fails open.
		fmt.Fprintf(stderr, "worktree-gate: ledger evidence ignored: %v\n", err)
		ev = ledger.Evidence{}
	}

	verdict := ledger.Grade(ledger.Input{
		Checkpoint: opts.checkpoint,
		Mode:       opts.mode,
		Binding:    binding,
		Identity:   ident,
		Store:      store,
		StoreErr:   storeErr,
		Evidence:   ev,
		Now:        time.Now(),
	})

	if verdict.Conflict != nil && verdict.Item != nil {
		if err := ledger.PersistConflict(storePath, ident.Key, *verdict.Conflict); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: ledger conflict not recorded: %v\n", err)
		}
	}

	payload, err := json.Marshal(newLedgerOut(verdict))
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: ledger: %v\n", err)
		return 0
	}
	fmt.Fprintln(stdout, string(payload))
	return verdict.ExitCode()
}

// newLedgerOut maps a verdict onto the exact stdout contract.
func newLedgerOut(v ledger.Verdict) ledgerOut {
	out := ledgerOut{
		Capability: ledger.CapabilityTracker,
		Active:     v.Active,
		Checkpoint: v.Checkpoint,
		Mode:       v.Mode,
		Decision:   v.Decision,
		Reason:     v.Reason,
		Item:       v.Item,
		Conflict:   v.Conflict,
		Prompt:     v.Prompt,
		Doctor:     v.Doctor,
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

// storedLedgerSlug returns the change slug already recorded on the single open
// item for the common dir and branch, so a mid-item archive does not rewrite the
// identity (A11).
func storedLedgerSlug(store ledger.Store, common, branch string) string {
	var changes []string
	for _, item := range store.Items {
		if item.Status != ledger.StatusOpen {
			continue
		}
		if item.Identity.CommonDir != common || item.Identity.Branch != branch {
			continue
		}
		changes = append(changes, item.Identity.Change)
	}
	if len(changes) == 1 {
		return changes[0]
	}
	return ""
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

// persistLedgerDecision parses and appends the human answer. The checkpoint flag
// is the default when the payload omits it.
func persistLedgerDecision(storePath, key, checkpoint, raw string) error {
	var req ledger.DecisionRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return fmt.Errorf("invalid --decide JSON: %w", err)
	}
	if req.Checkpoint == "" {
		req.Checkpoint = checkpoint
	}
	_, err := ledger.PersistDecision(storePath, key, req, time.Now())
	return err
}

// ledgerSelftest exercises the ledger enum and posture invariants in-process,
// with no network and no store IO. --selftest prints its usual "ok" only when
// this returns nil, so the release toolchain verifies both modes.
func ledgerSelftest() error {
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
	empty := ledger.Store{V: ledger.StoreVersion}

	dormant := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointWorkStart, Mode: ledger.ModeWarn,
		Binding: ledger.Binding{State: ledger.WitnessUnbound, Reason: ledger.ReasonWitnessMissing},
	})
	if dormant.Decision != ledger.DecisionDormant {
		return fmt.Errorf("missing witness graded %q, want %q", dormant.Decision, ledger.DecisionDormant)
	}

	warned := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointApplyStart, Mode: ledger.ModeWarn,
		Binding: bound, Identity: ident, Store: empty,
	})
	if warned.Decision != ledger.DecisionAllow || warned.ExitCode() != 0 {
		return fmt.Errorf("warn graded %q (exit %d), want allow/0", warned.Decision, warned.ExitCode())
	}

	blocked := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointApplyStart, Mode: ledger.ModeAlways,
		Binding: bound, Identity: ident, Store: empty,
	})
	if blocked.Decision != ledger.DecisionBlock || blocked.Reason != ledger.ReasonNeedsItem || blocked.ExitCode() != 2 {
		return fmt.Errorf("always graded %q/%q (exit %d), want block/needs-item/2", blocked.Decision, blocked.Reason, blocked.ExitCode())
	}

	asked := ledger.Grade(ledger.Input{
		Checkpoint: ledger.CheckpointPreMerge, Mode: ledger.ModeAsk,
		Binding: bound, Identity: ident, Store: empty,
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

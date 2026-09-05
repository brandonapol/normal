package audit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/engine"
)

var start = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

func entry(intent string, from, to int) audit.Entry {
	return audit.Entry{
		TransactionID: "txn-" + intent,
		Intent:        intent,
		ApprovedBy:    "user",
		FromRevision:  from,
		ToRevision:    to,
		ConfigBefore:  strings.Repeat("a", 64),
		ConfigAfter:   strings.Repeat("b", 64),
		Files:         []string{"/etc/normal/launcher.json"},
		Services:      []string{"normal-launcher"},
		Outcome:       audit.OutcomeApplied,
		StartedAt:     start,
		FinishedAt:    start.Add(time.Second),
	}
}

func digestFor(step int) string {
	return fmt.Sprintf("%064x", step)
}

func chain(t *testing.T, count int) []audit.Entry {
	t.Helper()
	entries := make([]audit.Entry, 0, count)
	for i := 0; i < count; i++ {
		next := entry("change", i, i+1)
		next.ConfigBefore = digestFor(i)
		next.ConfigAfter = digestFor(i + 1)
		entries = append(entries, audit.Link(audit.Head(entries), next))
	}
	return entries
}

func newStore(t *testing.T) (audit.Store, engine.MemoryPorts) {
	t.Helper()
	ports := engine.NewMemoryPorts(engine.MemoryOptions{})
	return audit.NewStore(ports.FS, "/etc/normal"), ports
}

func TestChainLinksAndVerifies(t *testing.T) {
	entries := chain(t, 4)

	if entries[0].PreviousHash != audit.GenesisHash {
		t.Fatal("the first entry must link to genesis")
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].PreviousHash != entries[i-1].Hash {
			t.Fatalf("entry %d does not link to entry %d", i, i-1)
		}
		if entries[i].Sequence != i {
			t.Fatalf("entry %d has sequence %d", i, entries[i].Sequence)
		}
	}

	report := audit.Verify(entries, audit.DecodeReport{}, nil)
	if !report.Valid() {
		t.Fatalf("a well-formed chain should verify, got %v", report.Problems)
	}
	if report.Incomplete {
		t.Fatal("a complete chain should not be reported as incomplete")
	}
}

func TestEditingAnEntryBreaksVerification(t *testing.T) {
	entries := chain(t, 5)
	entries[2].Intent = "something else entirely"

	report := audit.Verify(entries, audit.DecodeReport{}, nil)
	if report.Valid() {
		t.Fatal("an edited entry must not verify")
	}
	first := report.Problems[0]
	if first.Sequence != 2 {
		t.Fatalf("verification should name the first broken entry, got %d", first.Sequence)
	}
	if first.Kind != audit.ProblemHashMismatch {
		t.Fatalf("expected a hash mismatch, got %s", first.Kind)
	}
	if !strings.Contains(first.Message, "edited after being written") {
		t.Fatalf("the message should say what happened, got %q", first.Message)
	}
}

func TestRemovingAnEntryBreaksVerification(t *testing.T) {
	entries := chain(t, 5)
	without := append(append([]audit.Entry{}, entries[:2]...), entries[3:]...)

	report := audit.Verify(without, audit.DecodeReport{}, nil)
	if report.Valid() {
		t.Fatal("removing an entry must not verify")
	}
	if report.Problems[0].Sequence != 3 {
		t.Fatalf("expected the gap to be reported at 3, got %d", report.Problems[0].Sequence)
	}
	if report.Problems[0].Kind != audit.ProblemSequenceGap {
		t.Fatalf("expected a sequence gap, got %s", report.Problems[0].Kind)
	}
}

func TestReorderingBreaksVerification(t *testing.T) {
	entries := chain(t, 4)
	entries[1], entries[2] = entries[2], entries[1]

	if audit.Verify(entries, audit.DecodeReport{}, nil).Valid() {
		t.Fatal("reordered entries must not verify")
	}
}

func TestTruncatedLogIsIncompleteNotCorrupt(t *testing.T) {
	entries := chain(t, 3)
	encoded, err := audit.Encode(entries)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	partial := string(encoded) + `{"sequence":3,"previousHa`
	decoded, report, decodeErr := audit.Decode([]byte(partial))
	if decodeErr != nil {
		t.Fatalf("a truncated trailing line should not fail decoding: %v", decodeErr)
	}
	if !report.Truncated {
		t.Fatal("decode should notice the truncation")
	}
	if len(decoded) != 3 {
		t.Fatalf("the intact entries should still decode, got %d", len(decoded))
	}

	verified := audit.Verify(decoded, report, nil)
	if !verified.Incomplete {
		t.Fatal("a truncated log is incomplete")
	}
	if verified.Problems[0].Kind != audit.ProblemTruncated {
		t.Fatalf("expected truncation to be named, got %s", verified.Problems[0].Kind)
	}
}

func TestPendingMarkerIsReportedAsUnfinished(t *testing.T) {
	entries := chain(t, 2)
	pending := &audit.Pending{
		TransactionID: "txn-0003",
		Intent:        "interrupted change",
		StartedAt:     start,
	}

	report := audit.Verify(entries, audit.DecodeReport{}, pending)
	if !report.Incomplete {
		t.Fatal("a pending marker means the log is incomplete")
	}
	if report.Problems[0].Kind != audit.ProblemUnfinished {
		t.Fatalf("expected unfinished-transaction, got %s", report.Problems[0].Kind)
	}
	if !strings.Contains(report.Problems[0].Message, "txn-0003") {
		t.Fatalf("the problem should name the transaction, got %q", report.Problems[0].Message)
	}
}

func TestUnknownOutcomeIsRejected(t *testing.T) {
	broken := entry("odd", 0, 1)
	broken.Outcome = "probably-fine"
	linked := audit.Link(nil, broken)

	report := audit.Verify([]audit.Entry{linked}, audit.DecodeReport{}, nil)
	if report.Valid() {
		t.Fatal("an unrecognised outcome must not verify")
	}
	if report.Problems[0].Kind != audit.ProblemMissingOutcome {
		t.Fatalf("expected missing-outcome, got %s", report.Problems[0].Kind)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		next := entry("change", i, i+1)
		next.ConfigBefore = digestFor(i)
		next.ConfigAfter = digestFor(i + 1)
		if _, err := store.Commit(ctx, next); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	report := store.VerifyLog(ctx)
	if !report.Valid() || report.Entries != 3 {
		t.Fatalf("expected three valid entries, got %+v", report)
	}
}

func TestStoreDetectsTamperingOnDisk(t *testing.T) {
	store, ports := newStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		next := entry("change", i, i+1)
		next.ConfigBefore = digestFor(i)
		next.ConfigAfter = digestFor(i + 1)
		if _, err := store.Commit(ctx, next); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	raw := ports.FS.Snapshot()[store.LogPath]
	tampered := strings.Replace(raw, `"intent":"change"`, `"intent":"harmless"`, 1)
	if tampered == raw {
		t.Fatal("test setup failed to tamper with the log")
	}
	if err := ports.FS.Write(ctx, store.LogPath, tampered); err != nil {
		t.Fatalf("Write: %v", err)
	}

	report := store.VerifyLog(ctx)
	if report.Valid() {
		t.Fatal("an edit on disk must be detected")
	}
	if report.Problems[0].Kind != audit.ProblemHashMismatch {
		t.Fatalf("expected a hash mismatch, got %s", report.Problems[0].Kind)
	}
}

func TestBeginLeavesAPendingMarkerUntilCommit(t *testing.T) {
	store, ports := newStore(t)
	ctx := context.Background()

	if err := store.Begin(ctx, audit.Pending{TransactionID: "txn-0001", StartedAt: start}); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, ok := ports.FS.Snapshot()[store.PendingPath]; !ok {
		t.Fatal("Begin should write a pending marker")
	}

	report := store.VerifyLog(ctx)
	if !report.Incomplete {
		t.Fatal("an outstanding pending marker means incomplete")
	}

	if _, err := store.Commit(ctx, entry("done", 0, 1)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, ok := ports.FS.Snapshot()[store.PendingPath]; ok {
		t.Fatal("Commit should clear the pending marker")
	}

	sealed := store.VerifyLog(ctx)
	if !sealed.Valid() || sealed.Incomplete {
		t.Fatalf("a committed transaction should verify cleanly, got %+v", sealed)
	}
}

func TestRenderingIsReadable(t *testing.T) {
	entries := chain(t, 2)
	rendered := audit.RenderLog(entries, nil)

	for _, want := range []string{"#0", "#1", "revision 0 -> 1", "applied", "approved by user"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered log is missing %q:\n%s", want, rendered)
		}
	}

	if !strings.Contains(audit.RenderLog(nil, nil), "no transactions recorded") {
		t.Error("an empty log should say so")
	}
}

func TestConfigChangedOutsideTheEngineIsDetected(t *testing.T) {
	first := audit.Link(nil, entry("first", 0, 1))

	second := entry("second", 1, 2)
	second.ConfigBefore = strings.Repeat("c", 64)
	linked := audit.Link(&first, second)

	report := audit.Verify([]audit.Entry{first, linked}, audit.DecodeReport{}, nil)
	if report.Valid() {
		t.Fatal("a config gap between transactions must be reported")
	}
	if report.Problems[0].Kind != audit.ProblemConfigGap {
		t.Fatalf("expected config-changed-outside-engine, got %s", report.Problems[0].Kind)
	}
	if !strings.Contains(report.Problems[0].Message, "outside the engine") {
		t.Fatalf("the message should explain what it means, got %q", report.Problems[0].Message)
	}
}

func TestContinuousConfigChainVerifies(t *testing.T) {
	first := audit.Link(nil, entry("first", 0, 1))

	second := entry("second", 1, 2)
	second.ConfigBefore = first.ConfigAfter
	second.ConfigAfter = strings.Repeat("d", 64)
	linked := audit.Link(&first, second)

	if report := audit.Verify([]audit.Entry{first, linked}, audit.DecodeReport{}, nil); !report.Valid() {
		t.Fatalf("a continuous chain should verify, got %v", report.Problems)
	}
}

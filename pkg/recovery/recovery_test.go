package recovery_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/baseline"
	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
	"github.com/brandonapol/normal/pkg/recovery"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func signer(t *testing.T, offset byte) *audit.SoftwareSigner {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i) + offset
	}
	made, err := audit.NewSoftwareSignerFromSeed(seed)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return made
}

func sealedBaseline(t *testing.T, key audit.Signer) baseline.Sealed {
	t.Helper()
	sealed, err := baseline.Seal(config.Baseline(), key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return sealed
}

// powerCut leaves the disk exactly as an interrupted transaction would:
// a pending marker carrying the pre-change snapshot, and a partially written file.
func powerCut(t *testing.T, faults ...engine.Fault) (engine.MemoryPorts, audit.Store, map[string]string) {
	t.Helper()

	before, err := engine.Render(config.Baseline())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: before})
	store := audit.NewStore(ports.FS, "/etc/normal")
	ctx := context.Background()

	changed := config.Baseline()
	changed.Spec.Launcher.Columns = 5
	changed.Metadata.Revision = 1
	after, err := engine.Render(changed)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	snapshot := make([]audit.FileState, 0, 2)
	for _, path := range []string{engine.FileLauncher, engine.FileMetadata} {
		snapshot = append(snapshot, audit.FileState{Path: path, Contents: before[path], Existed: true})
	}
	if err := store.Begin(ctx, audit.Pending{
		TransactionID: "txn-0001",
		Intent:        "five columns",
		FromRevision:  0,
		ToRevision:    1,
		ConfigBefore:  engine.Digest(before),
		Files:         []string{engine.FileLauncher, engine.FileMetadata},
		Services:      []string{engine.ServiceLauncher},
		StartedAt:     now,
		Snapshot:      snapshot,
	}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := ports.FS.Write(ctx, engine.FileLauncher, after[engine.FileLauncher]); err != nil {
		t.Fatalf("partial write: %v", err)
	}

	for _, fault := range faults {
		ports.AddFault(fault)
	}
	return ports, store, before
}

func TestNothingToRecoverOnACleanDevice(t *testing.T) {
	files, _ := engine.Render(config.Baseline())
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files})
	store := audit.NewStore(ports.FS, "/etc/normal")

	result, err := recovery.Recover(context.Background(), ports.Ports, store, recovery.Options{Now: now})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if result.Outcome != recovery.OutcomeNothingToDo {
		t.Fatalf("expected nothing-to-do, got %s", result.Outcome)
	}
	if result.NeedsAttention() {
		t.Fatal("a clean device needs no attention")
	}
}

func TestRecoversFromAPowerCutMidTransaction(t *testing.T) {
	ports, store, before := powerCut(t)
	ctx := context.Background()

	if ports.FS.Snapshot()[engine.FileLauncher] == before[engine.FileLauncher] {
		t.Fatal("test setup did not leave a partially applied file")
	}

	key := signer(t, 0)
	sealed := sealedBaseline(t, key)
	result, err := recovery.Recover(ctx, ports.Ports, store, recovery.Options{
		Sealed: &sealed, PublicKey: key.PublicKey(), Now: now,
	})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}

	if result.Outcome != recovery.OutcomeRestored {
		t.Fatalf("expected restored-from-snapshot, got %s (%s)", result.Outcome, result.Reason)
	}
	for _, path := range []string{engine.FileLauncher, engine.FileMetadata} {
		if ports.FS.Snapshot()[path] != before[path] {
			t.Errorf("%s was not restored to its pre-transaction contents", path)
		}
	}
	if _, stillPending := ports.FS.Snapshot()[store.PendingPath]; stillPending {
		t.Error("a completed recovery should clear the pending marker")
	}

	restarted := strings.Join(ports.Services.Restarts(), ",")
	if !strings.Contains(restarted, engine.ServiceLauncher) {
		t.Error("services reading a restored file must be restarted")
	}
}

func TestRecoveryIsAudited(t *testing.T) {
	ports, store, _ := powerCut(t)
	ctx := context.Background()

	key := signer(t, 0)
	sealed := sealedBaseline(t, key)
	if _, err := recovery.Recover(ctx, ports.Ports, store, recovery.Options{
		Sealed: &sealed, PublicKey: key.PublicKey(), Now: now,
	}); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	entries, _, _, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("recovery should leave exactly one record, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Intent, "interrupted") {
		t.Fatalf("the record should say what happened, got %q", entries[0].Intent)
	}
	if report := store.VerifyLog(ctx); !report.Valid() {
		t.Fatalf("the chain should be intact after recovery, got %v", report.Problems)
	}
}

func TestFallsBackToTheBaselineWhenRestoreFails(t *testing.T) {
	ports, store, _ := powerCut(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileLauncher,
		Error:  &engine.IOError{Code: engine.ErrDenied, Target: engine.FileLauncher, Message: "read-only"},
		Times:  1,
	})
	ctx := context.Background()

	key := signer(t, 0)
	sealed := sealedBaseline(t, key)
	result, err := recovery.Recover(ctx, ports.Ports, store, recovery.Options{
		Sealed: &sealed, PublicKey: key.PublicKey(), Now: now,
	})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}

	if result.Outcome != recovery.OutcomeBaseline {
		t.Fatalf("expected fell-back-to-baseline, got %s (%s)", result.Outcome, result.Reason)
	}
	if result.NeedsAttention() {
		t.Fatal("falling back to the baseline is a successful recovery")
	}

	expected, _ := engine.Render(config.Baseline())
	if ports.FS.Snapshot()[engine.FileAttention] != expected[engine.FileAttention] {
		t.Error("the device should be carrying the baseline after fallback")
	}
	if !strings.Contains(result.Reason, "could not be restored") {
		t.Errorf("the reason should explain the fallback, got %q", result.Reason)
	}
}

func TestUnrecoverableWithoutASealedBaseline(t *testing.T) {
	ports, store, _ := powerCut(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileLauncher,
		Error:  &engine.IOError{Code: engine.ErrDenied, Target: engine.FileLauncher, Message: "read-only"},
	})

	result, err := recovery.Recover(context.Background(), ports.Ports, store, recovery.Options{Now: now})
	if err != nil {
		t.Fatalf("Recover should report, not error: %v", err)
	}
	if result.Outcome != recovery.OutcomeUnrecoverable {
		t.Fatalf("expected unrecoverable, got %s", result.Outcome)
	}
	if !result.NeedsAttention() {
		t.Fatal("an unrecoverable device needs attention")
	}
	if !strings.Contains(result.String(), "could not be repaired") {
		t.Fatalf("the message should be plain about it, got %q", result.String())
	}
	if !strings.Contains(result.Reason, "no sealed baseline") {
		t.Fatalf("the reason should name the missing fallback, got %q", result.Reason)
	}
}

func TestUnrecoverableWithAnUntrustworthyBaseline(t *testing.T) {
	ports, store, _ := powerCut(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileLauncher,
		Error:  &engine.IOError{Code: engine.ErrDenied, Target: engine.FileLauncher, Message: "read-only"},
	})

	ours := signer(t, 0)
	theirs := signer(t, 100)
	foreign := sealedBaseline(t, theirs)

	result, err := recovery.Recover(context.Background(), ports.Ports, store, recovery.Options{
		Sealed: &foreign, PublicKey: ours.PublicKey(), Now: now,
	})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if result.Outcome != recovery.OutcomeUnrecoverable {
		t.Fatalf("expected unrecoverable, got %s", result.Outcome)
	}
	if !strings.Contains(result.Reason, "not trustworthy") {
		t.Fatalf("the reason should name the untrusted baseline, got %q", result.Reason)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	ports, store, _ := powerCut(t)
	before := ports.FS.Snapshot()

	result, err := recovery.Recover(context.Background(), ports.Ports, store, recovery.Options{
		Now: now, DryRun: true,
	})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if result.Outcome != recovery.OutcomeRestored {
		t.Fatalf("a dry run should report what it would do, got %s", result.Outcome)
	}
	if len(result.Actions) == 0 {
		t.Fatal("a dry run should list its intended actions")
	}

	after := ports.FS.Snapshot()
	if len(after) != len(before) {
		t.Fatal("a dry run must not add or remove files")
	}
	for path, contents := range before {
		if after[path] != contents {
			t.Fatalf("a dry run modified %s", path)
		}
	}
}

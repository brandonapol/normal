package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
)

func baselineFiles(t *testing.T) map[string]string {
	t.Helper()
	files, err := engine.Render(config.Baseline())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return files
}

func seeded(t *testing.T, faults ...engine.Fault) engine.MemoryPorts {
	t.Helper()
	return engine.NewMemoryPorts(engine.MemoryOptions{Files: baselineFiles(t), Faults: faults})
}

func withColumns(columns int) config.Config {
	desired := config.Baseline()
	desired.Spec.Launcher.Columns = columns
	return engine.WithNextRevision(config.Baseline(), desired)
}

func withPageSize(pageSize int) config.Config {
	desired := config.Baseline()
	desired.Spec.Attention.InfiniteScroll.PageSize = pageSize
	return engine.WithNextRevision(config.Baseline(), desired)
}

func planFor(t *testing.T, desired config.Config) engine.Plan {
	t.Helper()
	plan, err := engine.PlanApply(config.Baseline(), desired)
	if err != nil {
		t.Fatalf("PlanApply: %v", err)
	}
	return plan
}

func writtenPaths(plan engine.Plan) []string {
	paths := make([]string, 0)
	for _, action := range plan.Actions {
		if action.Kind == engine.ActionWriteFile {
			paths = append(paths, action.Path)
		}
	}
	return paths
}

func TestDiffIsEmptyForIdenticalConfigs(t *testing.T) {
	diff, err := engine.DiffConfigs(config.Baseline(), config.Baseline())
	if err != nil {
		t.Fatalf("DiffConfigs: %v", err)
	}
	if !diff.IsEmpty() {
		t.Fatalf("expected no changes, got %v", diff.Changes)
	}
}

func TestDiffReportsScalarReplacement(t *testing.T) {
	desired := config.Baseline()
	desired.Spec.Launcher.Columns = 2
	diff, _ := engine.DiffConfigs(config.Baseline(), desired)
	if len(diff.Changes) != 1 {
		t.Fatalf("expected one change, got %v", diff.Changes)
	}
	change := diff.Changes[0]
	if change.Path != "/spec/launcher/columns" || change.Op != engine.OpReplace {
		t.Fatalf("unexpected change %+v", change)
	}
}

func TestDiffAddressesKeyedCollectionsByKey(t *testing.T) {
	desired := config.Baseline()
	entries := append([]config.AppEntry(nil), desired.Spec.Apps.Entries...)
	for i := range entries {
		if entries[i].Package == "com.spotify.music" {
			entries[i].Network = "wifi-only"
		}
	}
	desired.Spec.Apps.Entries = entries

	diff, _ := engine.DiffConfigs(config.Baseline(), desired)
	if len(diff.Changes) != 1 {
		t.Fatalf("expected one change, got %v", diff.Changes)
	}
	if diff.Changes[0].Path != "/spec/apps/entries/com.spotify.music/network" {
		t.Fatalf("expected a key-addressed path, got %q", diff.Changes[0].Path)
	}
}

func TestDiffReportsSingleAddAndRemove(t *testing.T) {
	added := config.Baseline()
	added.Spec.Apps.Entries = append(append([]config.AppEntry(nil), added.Spec.Apps.Entries...), config.AppEntry{
		Package: "org.fdroid.fdroid", Source: "fdroid", State: "installed",
		Network: "wifi-only", Permissions: map[string]string{},
	})
	diff, _ := engine.DiffConfigs(config.Baseline(), added)
	if len(diff.Changes) != 1 || diff.Changes[0].Op != engine.OpAdd {
		t.Fatalf("expected a single add, got %v", diff.Changes)
	}
	if diff.Changes[0].Path != "/spec/apps/entries/org.fdroid.fdroid" {
		t.Fatalf("unexpected path %q", diff.Changes[0].Path)
	}

	removed := config.Baseline()
	kept := make([]config.AppEntry, 0)
	for _, entry := range removed.Spec.Apps.Entries {
		if entry.Package != "com.spotify.music" {
			kept = append(kept, entry)
		}
	}
	removed.Spec.Apps.Entries = kept
	diff, _ = engine.DiffConfigs(config.Baseline(), removed)
	if len(diff.Changes) != 1 || diff.Changes[0].Op != engine.OpRemove {
		t.Fatalf("expected a single remove, got %v", diff.Changes)
	}
}

func TestDiffNoticesReordering(t *testing.T) {
	desired := config.Baseline()
	entries := append([]config.AppEntry(nil), desired.Spec.Apps.Entries...)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	desired.Spec.Apps.Entries = entries

	diff, _ := engine.DiffConfigs(config.Baseline(), desired)
	if len(diff.Changes) != 1 || !strings.HasSuffix(diff.Changes[0].Path, "$order") {
		t.Fatalf("expected a single order change, got %v", diff.Changes)
	}
}

func TestDiffTreatsUnkeyedArraysAsAtomic(t *testing.T) {
	desired := config.Baseline()
	desired.Spec.Launcher.Dock = []string{"os.normal.phone"}
	diff, _ := engine.DiffConfigs(config.Baseline(), desired)
	if len(diff.Changes) != 1 || diff.Changes[0].Path != "/spec/launcher/dock" {
		t.Fatalf("expected one atomic dock change, got %v", diff.Changes)
	}
}

func TestPlanIsNoOpForUnchangedConfig(t *testing.T) {
	plan := planFor(t, config.Baseline())
	if !plan.IsNoOp() || len(plan.Services) != 0 {
		t.Fatalf("expected a no-op plan, got %+v", plan)
	}
}

func TestPlanWritesOnlyTouchedFiles(t *testing.T) {
	plan := planFor(t, withColumns(3))
	want := []string{engine.FileLauncher, engine.FileMetadata}
	if !reflect.DeepEqual(writtenPaths(plan), want) {
		t.Fatalf("expected %v, got %v", want, writtenPaths(plan))
	}
	if !reflect.DeepEqual(plan.Services, []string{engine.ServiceLauncher}) {
		t.Fatalf("expected only the launcher to restart, got %v", plan.Services)
	}
}

func TestPlanFansAttentionChangeOutToShim(t *testing.T) {
	plan := planFor(t, withPageSize(10))
	paths := strings.Join(writtenPaths(plan), ",")
	if !strings.Contains(paths, engine.FileAttention) || !strings.Contains(paths, engine.FileWebviewShim) {
		t.Fatalf("expected both attention files, got %v", writtenPaths(plan))
	}
	want := []string{engine.ServiceAttentiond, engine.ServiceWebviewShim}
	if !reflect.DeepEqual(plan.Services, want) {
		t.Fatalf("expected %v, got %v", want, plan.Services)
	}
}

func TestPlanSkipsServiceWhoseFilesDidNotChange(t *testing.T) {
	desired := config.Baseline()
	entries := append([]config.AppEntry(nil), desired.Spec.Apps.Entries...)
	for i := range entries {
		if entries[i].Package == "com.spotify.music" {
			entries[i].Label = "Music"
		}
	}
	desired.Spec.Apps.Entries = entries

	plan := planFor(t, engine.WithNextRevision(config.Baseline(), desired))
	want := []string{engine.ServiceAppd, engine.ServiceLauncher}
	if !reflect.DeepEqual(plan.Services, want) {
		t.Fatalf("expected %v, got %v", want, plan.Services)
	}
}

func TestPlanOrdersWritesBeforeRestarts(t *testing.T) {
	plan := planFor(t, withColumns(4))
	lastWrite, firstRestart := -1, len(plan.Actions)
	for i, action := range plan.Actions {
		if action.Kind == engine.ActionRestartService {
			if i < firstRestart {
				firstRestart = i
			}
			continue
		}
		lastWrite = i
	}
	if lastWrite >= firstRestart {
		t.Fatalf("writes must precede restarts, got %+v", plan.Actions)
	}
}

func TestPlanRejectsStaleRevision(t *testing.T) {
	desired := config.Baseline()
	desired.Spec.Launcher.Columns = 3
	_, err := engine.PlanApply(config.Baseline(), desired)
	var planErr *engine.PlanError
	if !asPlanError(err, &planErr) || !hasCode(planErr, "stale-revision") {
		t.Fatalf("expected stale-revision, got %v", err)
	}
}

func TestPlanRejectsImmutableField(t *testing.T) {
	desired := config.Baseline()
	desired.APIVersion = "normal.os/v1"
	_, err := engine.PlanApply(config.Baseline(), engine.WithNextRevision(config.Baseline(), desired))
	var planErr *engine.PlanError
	if !asPlanError(err, &planErr) || !hasCode(planErr, "immutable-field") {
		t.Fatalf("expected immutable-field, got %v", err)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, _ := engine.Render(config.Baseline())
	second, _ := engine.Render(config.Baseline())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("render is not deterministic")
	}
}

func TestRenderDerivesWebviewShim(t *testing.T) {
	files, _ := engine.Render(config.Baseline())
	var shim struct {
		Enforcement  string   `json:"enforcement"`
		MaxAutoLoads int      `json:"maxAutoLoads"`
		DomSignals   []string `json:"domSignals"`
	}
	if err := json.Unmarshal([]byte(files[engine.FileWebviewShim]), &shim); err != nil {
		t.Fatalf("unmarshal shim: %v", err)
	}
	if shim.Enforcement != "paginate" || shim.MaxAutoLoads != 0 {
		t.Fatalf("unexpected shim policy %+v", shim)
	}
	if !strings.Contains(strings.Join(shim.DomSignals, ","), "sentinel-intersection-observer") {
		t.Fatalf("expected dom signals to reach the shim, got %v", shim.DomSignals)
	}
}

func TestApplyWritesFilesAndRestartsServices(t *testing.T) {
	ports := seeded(t)
	report, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(3)), ports.Ports)
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if !strings.Contains(ports.FS.Snapshot()[engine.FileLauncher], `"columns": 3`) {
		t.Fatal("launcher file was not updated")
	}
	if !reflect.DeepEqual(ports.Services.Restarts(), []string{engine.ServiceLauncher}) {
		t.Fatalf("unexpected restarts %v", ports.Services.Restarts())
	}
	for _, step := range report.Steps {
		if step.Status != engine.StepApplied {
			t.Fatalf("expected every step applied, got %+v", step)
		}
	}
}

func TestApplyConverges(t *testing.T) {
	ports := seeded(t)
	desired := withColumns(3)
	if _, err := engine.ApplyPlan(context.Background(), planFor(t, desired), ports.Ports); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	afterFirst := ports.FS.Snapshot()

	second, err := engine.PlanApply(desired, desired)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if !second.IsNoOp() {
		t.Fatalf("expected re-planning the same state to be a no-op, got %+v", second.Actions)
	}
	if _, err := engine.ApplyPlan(context.Background(), second, ports.Ports); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !reflect.DeepEqual(ports.FS.Snapshot(), afterFirst) {
		t.Fatal("a no-op apply changed the filesystem")
	}
}

func TestRollbackRestoresFilesOnWriteFailure(t *testing.T) {
	ports := seeded(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileWebviewShim,
		Error:  &engine.IOError{Code: engine.ErrIOFailure, Target: engine.FileWebviewShim, Message: "disk full"},
		Times:  1,
	})
	before := ports.FS.Snapshot()

	_, err := engine.ApplyPlan(context.Background(), planFor(t, withPageSize(10)), ports.Ports)
	failure := asFailure(t, err)
	if !failure.RolledBack || failure.DeviceDirty {
		t.Fatalf("expected a clean rollback, got %+v", failure)
	}
	if failure.Cause.Message != "disk full" {
		t.Fatalf("unexpected cause %v", failure.Cause)
	}
	if !reflect.DeepEqual(ports.FS.Snapshot(), before) {
		t.Fatal("rollback did not restore the filesystem")
	}
}

func TestRollbackOnRestartFailure(t *testing.T) {
	ports := seeded(t, engine.Fault{
		Kind:   engine.FaultRestart,
		Target: engine.ServiceLauncher,
		Error:  &engine.IOError{Code: engine.ErrUnavailable, Target: engine.ServiceLauncher, Message: "unit refused to start"},
		Times:  1,
	})
	before := ports.FS.Snapshot()

	_, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(5)), ports.Ports)
	failure := asFailure(t, err)
	if !failure.RolledBack {
		t.Fatalf("expected rollback, got %+v", failure)
	}
	if !reflect.DeepEqual(ports.FS.Snapshot(), before) {
		t.Fatal("rollback did not restore the filesystem")
	}
	if got := ports.Services.Restarts(); len(got) != 2 {
		t.Fatalf("expected a restart during apply and one during rollback, got %v", got)
	}
}

func TestRollbackWhenServiceComesBackUnhealthy(t *testing.T) {
	ports := seeded(t)
	ports.Services.SetState(engine.ServiceLauncher, engine.ServiceRunning)
	plan := planFor(t, withColumns(6))
	before := ports.FS.Snapshot()

	unhealthy := ports.Ports
	unhealthy.Services = stuckServiceHost{MemoryServiceHost: ports.Services}

	_, err := engine.ApplyPlan(context.Background(), plan, unhealthy)
	failure := asFailure(t, err)
	if failure.FailedAction != nil {
		t.Fatalf("expected the health check to fail, not an action: %+v", failure.FailedAction)
	}
	if failure.Cause.Code != engine.ErrUnavailable || !failure.RolledBack {
		t.Fatalf("unexpected failure %+v", failure)
	}
	if !reflect.DeepEqual(ports.FS.Snapshot(), before) {
		t.Fatal("rollback did not restore the filesystem")
	}
}

func TestFailedRollbackIsReportedAsDirty(t *testing.T) {
	ports := seeded(t,
		engine.Fault{
			Kind:   engine.FaultWrite,
			Target: engine.FileMetadata,
			Error:  &engine.IOError{Code: engine.ErrIOFailure, Target: engine.FileMetadata, Message: "metadata write failed"},
		},
		engine.Fault{
			Kind:   engine.FaultWrite,
			Target: engine.FileLauncher,
			Error:  &engine.IOError{Code: engine.ErrDenied, Target: engine.FileLauncher, Message: "read-only filesystem"},
		},
	)

	_, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(7)), ports.Ports)
	failure := asFailure(t, err)
	if failure.RolledBack || !failure.DeviceDirty {
		t.Fatalf("expected a dirty device, got %+v", failure)
	}
	found := false
	for _, rollbackErr := range failure.RollbackErrors {
		if rollbackErr.Message == "read-only filesystem" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the rollback error to be reported, got %v", failure.RollbackErrors)
	}
}

func TestRollbackRemovesFilesItCreated(t *testing.T) {
	ports := engine.NewMemoryPorts(engine.MemoryOptions{
		Files: map[string]string{},
		Faults: []engine.Fault{{
			Kind:   engine.FaultRestart,
			Target: engine.ServiceLauncher,
			Error:  &engine.IOError{Code: engine.ErrIOFailure, Target: engine.ServiceLauncher, Message: "boom"},
		}},
	})

	_, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(3)), ports.Ports)
	failure := asFailure(t, err)
	if failure.RolledBack {
		t.Fatal("expected the rollback restart to fail")
	}
	if len(ports.FS.Snapshot()) != 0 {
		t.Fatalf("expected created files to be removed, got %v", engine.SortedPaths(ports.FS.Snapshot()))
	}
}

func TestApplyJournalsTheTransaction(t *testing.T) {
	ports := seeded(t)
	if _, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(3)), ports.Ports); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	kinds := ports.Logger.Kinds()
	if len(kinds) == 0 || kinds[0] != "transaction-start" {
		t.Fatalf("expected a journal, got %v", kinds)
	}
	if kinds[len(kinds)-1] != "transaction-commit" {
		t.Fatalf("expected a commit record, got %v", kinds)
	}
}

type stuckServiceHost struct {
	*engine.MemoryServiceHost
}

func (s stuckServiceHost) Status(context.Context, string) (engine.ServiceState, error) {
	return engine.ServiceFailed, nil
}

func asFailure(t *testing.T, err error) *engine.Failure {
	t.Helper()
	if err == nil {
		t.Fatal("expected a failure")
	}
	var failure *engine.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("expected *engine.Failure, got %T: %v", err, err)
	}
	return failure
}

func asPlanError(err error, target **engine.PlanError) bool {
	return errors.As(err, target)
}

func hasCode(err *engine.PlanError, code string) bool {
	for _, got := range err.Codes() {
		if got == code {
			return true
		}
	}
	return false
}

func auditPorts(t *testing.T, faults ...engine.Fault) (engine.MemoryPorts, audit.Store) {
	t.Helper()
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: baselineFiles(t), Faults: faults})
	store := audit.NewStore(ports.FS, "/etc/normal")
	ports.Audit = store
	return ports, store
}

func TestApplyRecordsExactlyOneAuditEntry(t *testing.T) {
	ports, store := auditPorts(t)
	plan := planFor(t, withColumns(3))
	plan.Intent = "two columns please"
	plan.ApprovedBy = "user"

	if _, err := engine.ApplyPlan(context.Background(), plan, ports.Ports); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	report := store.VerifyLog(context.Background())
	if report.Entries != 1 {
		t.Fatalf("expected exactly one entry, got %d", report.Entries)
	}
	if !report.Valid() || report.Incomplete {
		t.Fatalf("the log should be intact and complete, got %+v", report)
	}

	entries, _, _, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	recorded := entries[0]
	if recorded.Intent != "two columns please" || recorded.ApprovedBy != "user" {
		t.Fatalf("the entry lost its intent or approver: %+v", recorded)
	}
	if recorded.Outcome != audit.OutcomeApplied {
		t.Fatalf("expected applied, got %s", recorded.Outcome)
	}
	if recorded.ConfigBefore == recorded.ConfigAfter {
		t.Fatal("a change should move the config digest")
	}
}

func TestRolledBackApplyIsRecordedAsSuch(t *testing.T) {
	ports, store := auditPorts(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileLauncher,
		Error:  &engine.IOError{Code: engine.ErrIOFailure, Target: engine.FileLauncher, Message: "disk full"},
		Times:  1,
	})

	if _, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(5)), ports.Ports); err == nil {
		t.Fatal("expected the apply to fail")
	}

	entries, _, pending, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("a rolled-back transaction is still worth one entry, got %d", len(entries))
	}
	if entries[0].Outcome != audit.OutcomeRolledBack {
		t.Fatalf("expected rolled-back, got %s", entries[0].Outcome)
	}
	if entries[0].ConfigBefore != entries[0].ConfigAfter {
		t.Fatal("a rolled-back transaction left the config where it started, and should say so")
	}
	if pending != nil {
		t.Fatal("a completed rollback should clear the pending marker")
	}
}

func TestSuccessiveAppliesExtendTheChain(t *testing.T) {
	ports, store := auditPorts(t)

	afterFirst := withColumns(3)
	first := planFor(t, afterFirst)
	if _, err := engine.ApplyPlan(context.Background(), first, ports.Ports); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	afterSecond := afterFirst
	afterSecond.Spec.Attention.InfiniteScroll.PageSize = 40
	afterSecond = engine.WithNextRevision(afterFirst, afterSecond)

	second, planErr := engine.PlanApply(afterFirst, afterSecond)
	if planErr != nil {
		t.Fatalf("planning the second change: %v", planErr)
	}
	if _, err := engine.ApplyPlan(context.Background(), second, ports.Ports); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	entries, _, _, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two entries, got %d", len(entries))
	}
	if entries[1].PreviousHash != entries[0].Hash {
		t.Fatal("the second entry must link to the first")
	}
	if entries[1].ConfigBefore != entries[0].ConfigAfter {
		t.Fatal("the second transaction must start where the first left off")
	}

	report := store.VerifyLog(context.Background())
	if !report.Valid() {
		t.Fatalf("the chain should verify, got %v", report.Problems)
	}
}

func TestApplyWithoutAnAuditSinkStillWorks(t *testing.T) {
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: baselineFiles(t)})
	if _, err := engine.ApplyPlan(context.Background(), planFor(t, withColumns(3)), ports.Ports); err != nil {
		t.Fatalf("the audit sink is optional: %v", err)
	}
}

func TestNoOpApplyRecordsNothing(t *testing.T) {
	ports, store := auditPorts(t)
	plan := planFor(t, config.Baseline())

	if _, err := engine.ApplyPlan(context.Background(), plan, ports.Ports); err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	report := store.VerifyLog(context.Background())
	if report.Entries != 0 {
		t.Fatalf("a no-op changed nothing and should record nothing, got %d entries", report.Entries)
	}
}

package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
)

type StepStatus string

const (
	StepApplied StepStatus = "applied"
	StepFailed  StepStatus = "failed"
)

type StepRecord struct {
	Index  int        `json:"index"`
	Action Action     `json:"action"`
	Status StepStatus `json:"status"`
}

type fileSnapshot struct {
	path     string
	contents string
	existed  bool
}

type Report struct {
	TransactionID string       `json:"transactionId"`
	StartedAt     time.Time    `json:"startedAt"`
	FinishedAt    time.Time    `json:"finishedAt"`
	Plan          Plan         `json:"plan"`
	Steps         []StepRecord `json:"steps"`
}

type Failure struct {
	TransactionID  string       `json:"transactionId"`
	StartedAt      time.Time    `json:"startedAt"`
	FinishedAt     time.Time    `json:"finishedAt"`
	Plan           Plan         `json:"plan"`
	Steps          []StepRecord `json:"steps"`
	FailedAction   *Action      `json:"failedAction"`
	Cause          *IOError     `json:"cause"`
	RolledBack     bool         `json:"rolledBack"`
	RollbackErrors []*IOError   `json:"rollbackErrors"`
	DeviceDirty    bool         `json:"deviceDirty"`
}

func (f *Failure) Error() string {
	if f.DeviceDirty {
		return fmt.Sprintf("apply failed and rollback did not fully succeed: %s", f.Cause)
	}
	return fmt.Sprintf("apply failed and was rolled back: %s", f.Cause)
}

func asIOError(err error) *IOError {
	var ioErr *IOError
	if errors.As(err, &ioErr) {
		return ioErr
	}
	return &IOError{Code: ErrIOFailure, Target: "", Message: err.Error()}
}

func filePaths(plan Plan) []string {
	seen := make(map[string]bool)
	paths := make([]string, 0)
	for _, action := range plan.Actions {
		if action.Kind == ActionRestartService {
			continue
		}
		if !seen[action.Path] {
			seen[action.Path] = true
			paths = append(paths, action.Path)
		}
	}
	return paths
}

func capture(ctx context.Context, plan Plan, ports Ports) ([]fileSnapshot, error) {
	snapshots := make([]fileSnapshot, 0, len(plan.Actions))
	for _, path := range filePaths(plan) {
		exists, err := ports.FS.Exists(ctx, path)
		if err != nil {
			return nil, err
		}
		if !exists {
			snapshots = append(snapshots, fileSnapshot{path: path, existed: false})
			continue
		}
		contents, err := ports.FS.Read(ctx, path)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, fileSnapshot{path: path, contents: contents, existed: true})
	}
	return snapshots, nil
}

func runAction(ctx context.Context, action Action, ports Ports) error {
	switch action.Kind {
	case ActionWriteFile:
		return ports.FS.Write(ctx, action.Path, action.Contents)
	case ActionDeleteFile:
		return ports.FS.Remove(ctx, action.Path)
	case ActionRestartService:
		return ports.Services.Restart(ctx, action.Service)
	}
	return &IOError{Code: ErrIOFailure, Target: string(action.Kind), Message: "unknown action"}
}

func verify(ctx context.Context, plan Plan, ports Ports) *IOError {
	for _, check := range plan.Checks {
		state, err := ports.Services.Status(ctx, check.Service)
		if err != nil {
			return asIOError(err)
		}
		if state != ServiceRunning {
			return &IOError{
				Code:    ErrUnavailable,
				Target:  check.Service,
				Message: fmt.Sprintf("service is %s after restart", state),
			}
		}
	}
	return nil
}

func restore(ctx context.Context, snapshots []fileSnapshot, plan Plan, ports Ports) []*IOError {
	failures := make([]*IOError, 0)
	for _, snapshot := range snapshots {
		if snapshot.existed {
			if err := ports.FS.Write(ctx, snapshot.path, snapshot.contents); err != nil {
				failures = append(failures, asIOError(err))
			}
			continue
		}
		err := ports.FS.Remove(ctx, snapshot.path)
		if err != nil {
			if ioErr := asIOError(err); ioErr.Code != ErrNotFound {
				failures = append(failures, ioErr)
			}
		}
	}
	for _, service := range plan.Services {
		if err := ports.Services.Restart(ctx, service); err != nil {
			failures = append(failures, asIOError(err))
		}
	}
	return failures
}

func ApplyPlan(ctx context.Context, plan Plan, ports Ports) (Report, error) {
	transactionID := ports.Clock.NextID()
	startedAt := ports.Clock.Now()
	ports.log(transactionID, "transaction-start", fmt.Sprintf(
		"%d actions, revision %d -> %d", len(plan.Actions), plan.FromRevision, plan.ToRevision))

	if len(plan.Actions) == 0 {
		ports.log(transactionID, "transaction-noop", "nothing to apply")
		return Report{
			TransactionID: transactionID,
			StartedAt:     startedAt,
			FinishedAt:    ports.Clock.Now(),
			Plan:          plan,
			Steps:         []StepRecord{},
		}, nil
	}

	snapshots, err := capture(ctx, plan, ports)
	if err != nil {
		cause := asIOError(err)
		ports.log(transactionID, "capture-failed", cause.Message)
		return Report{}, &Failure{
			TransactionID:  transactionID,
			StartedAt:      startedAt,
			FinishedAt:     ports.Clock.Now(),
			Plan:           plan,
			Steps:          []StepRecord{},
			Cause:          cause,
			RolledBack:     true,
			RollbackErrors: []*IOError{},
		}
	}

	touched := filePaths(plan)
	beginAudit(ctx, plan, ports, transactionID, startedAt, touched)

	steps := make([]StepRecord, 0, len(plan.Actions))
	rollback := func(failed *Action, cause *IOError) error {
		ports.log(transactionID, "rollback-start", cause.Message)
		rollbackErrors := restore(ctx, snapshots, plan, ports)
		rolledBack := len(rollbackErrors) == 0
		kind := "rollback-complete"
		if !rolledBack {
			kind = "rollback-failed"
		}
		ports.log(transactionID, kind, fmt.Sprintf("%d rollback errors", len(rollbackErrors)))
		outcome := audit.OutcomeRolledBack
		if !rolledBack {
			outcome = audit.OutcomeDirty
		}
		commitAudit(ctx, plan, ports, transactionID, startedAt, touched, outcome)
		return &Failure{
			TransactionID:  transactionID,
			StartedAt:      startedAt,
			FinishedAt:     ports.Clock.Now(),
			Plan:           plan,
			Steps:          steps,
			FailedAction:   failed,
			Cause:          cause,
			RolledBack:     rolledBack,
			RollbackErrors: rollbackErrors,
			DeviceDirty:    !rolledBack,
		}
	}

	for index, action := range plan.Actions {
		if err := runAction(ctx, action, ports); err != nil {
			steps = append(steps, StepRecord{Index: index, Action: action, Status: StepFailed})
			cause := asIOError(err)
			ports.log(transactionID, "step-failed", fmt.Sprintf("%s #%d: %s", action.Kind, index, cause.Message))
			return Report{}, rollback(&action, cause)
		}
		steps = append(steps, StepRecord{Index: index, Action: action, Status: StepApplied})
		ports.log(transactionID, "step-applied", fmt.Sprintf("%s #%d", action.Kind, index))
	}

	if cause := verify(ctx, plan, ports); cause != nil {
		ports.log(transactionID, "verify-failed", cause.Message)
		return Report{}, rollback(nil, cause)
	}

	commitAudit(ctx, plan, ports, transactionID, startedAt, touched, audit.OutcomeApplied)
	ports.log(transactionID, "transaction-commit", fmt.Sprintf("revision %d", plan.ToRevision))
	return Report{
		TransactionID: transactionID,
		StartedAt:     startedAt,
		FinishedAt:    ports.Clock.Now(),
		Plan:          plan,
		Steps:         steps,
	}, nil
}

func beginAudit(ctx context.Context, plan Plan, ports Ports, transactionID string, startedAt time.Time, touched []string) {
	if ports.Audit == nil {
		return
	}
	pending := audit.Pending{
		TransactionID: transactionID,
		Intent:        plan.Intent,
		FromRevision:  plan.FromRevision,
		ToRevision:    plan.ToRevision,
		ConfigBefore:  plan.DigestBefore,
		Files:         touched,
		Services:      plan.Services,
		StartedAt:     startedAt,
	}
	if err := ports.Audit.Begin(ctx, pending); err != nil {
		ports.log(transactionID, "audit-begin-failed", err.Error())
	}
}

func commitAudit(ctx context.Context, plan Plan, ports Ports, transactionID string, startedAt time.Time, touched []string, outcome audit.Outcome) {
	if ports.Audit == nil {
		return
	}
	entry := audit.Entry{
		TransactionID: transactionID,
		Intent:        plan.Intent,
		ApprovedBy:    plan.ApprovedBy,
		FromRevision:  plan.FromRevision,
		ToRevision:    plan.ToRevision,
		ConfigBefore:  plan.DigestBefore,
		ConfigAfter:   plan.DigestAfter,
		Files:         touched,
		Services:      plan.Services,
		Outcome:       outcome,
		StartedAt:     startedAt,
		FinishedAt:    ports.Clock.Now(),
	}
	if outcome != audit.OutcomeApplied {
		entry.ConfigAfter = plan.DigestBefore
	}
	if _, err := ports.Audit.Commit(ctx, entry); err != nil {
		ports.log(transactionID, "audit-commit-failed", err.Error())
	}
}

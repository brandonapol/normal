package recovery

import (
	"context"
	"fmt"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/baseline"
	"github.com/brandonapol/normal/pkg/engine"
)

type Outcome string

const (
	OutcomeNothingToDo   Outcome = "nothing-to-do"
	OutcomeRestored      Outcome = "restored-from-snapshot"
	OutcomeBaseline      Outcome = "fell-back-to-baseline"
	OutcomeUnrecoverable Outcome = "unrecoverable"
)

type Result struct {
	Outcome       Outcome  `json:"outcome"`
	TransactionID string   `json:"transactionId,omitempty"`
	Restored      []string `json:"restored,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Actions       []string `json:"actions,omitempty"`
}

func (r Result) NeedsAttention() bool { return r.Outcome == OutcomeUnrecoverable }

func (r Result) String() string {
	switch r.Outcome {
	case OutcomeNothingToDo:
		return "no interrupted transaction; nothing to recover"
	case OutcomeRestored:
		return fmt.Sprintf("transaction %s was interrupted; restored %d file(s) to the state before it started",
			r.TransactionID, len(r.Restored))
	case OutcomeBaseline:
		return fmt.Sprintf("transaction %s could not be undone (%s); fell back to the sealed baseline",
			r.TransactionID, r.Reason)
	default:
		return fmt.Sprintf("transaction %s left this device inconsistent and it could not be repaired: %s",
			r.TransactionID, r.Reason)
	}
}

type Options struct {
	Sealed    *baseline.Sealed
	PublicKey []byte
	Now       time.Time
	DryRun    bool
}

func Detect(ctx context.Context, store audit.Store) (*audit.Pending, error) {
	_, _, pending, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	return pending, nil
}

func Recover(ctx context.Context, ports engine.Ports, store audit.Store, options Options) (Result, error) {
	pending, err := Detect(ctx, store)
	if err != nil {
		return Result{Outcome: OutcomeUnrecoverable, Reason: err.Error()}, err
	}
	if pending == nil {
		return Result{Outcome: OutcomeNothingToDo}, nil
	}

	result := Result{TransactionID: pending.TransactionID}

	if len(pending.Snapshot) == 0 {
		return fallBack(ctx, ports, store, options, result,
			"the interrupted transaction recorded no snapshot to restore")
	}

	if options.DryRun {
		result.Outcome = OutcomeRestored
		for _, file := range pending.Snapshot {
			result.Restored = append(result.Restored, file.Path)
			result.Actions = append(result.Actions, describe(file))
		}
		return result, nil
	}

	failures := make([]string, 0)
	for _, file := range pending.Snapshot {
		if restoreErr := restore(ctx, ports, file); restoreErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", file.Path, restoreErr))
			continue
		}
		result.Restored = append(result.Restored, file.Path)
	}

	if len(failures) > 0 {
		return fallBack(ctx, ports, store, options, result,
			fmt.Sprintf("%d file(s) could not be restored", len(failures)))
	}

	for _, service := range pending.Services {
		if restartErr := ports.Services.Restart(ctx, service); restartErr != nil {
			return fallBack(ctx, ports, store, options, result,
				fmt.Sprintf("%s did not restart after restoring: %v", service, restartErr))
		}
	}

	result.Outcome = OutcomeRestored
	record(ctx, ports, store, options, pending, audit.OutcomeRolledBack,
		"recovered an interrupted transaction by restoring the state before it")
	return result, nil
}

func describe(file audit.FileState) string {
	if file.Existed {
		return "restore " + file.Path
	}
	return "remove " + file.Path
}

func restore(ctx context.Context, ports engine.Ports, file audit.FileState) error {
	if file.Existed {
		return ports.FS.Write(ctx, file.Path, file.Contents)
	}
	err := ports.FS.Remove(ctx, file.Path)
	if err == nil {
		return nil
	}
	exists, existsErr := ports.FS.Exists(ctx, file.Path)
	if existsErr == nil && !exists {
		return nil
	}
	return err
}

func fallBack(ctx context.Context, ports engine.Ports, store audit.Store, options Options,
	result Result, reason string) (Result, error) {
	result.Reason = reason

	if options.Sealed == nil {
		result.Outcome = OutcomeUnrecoverable
		result.Reason = reason + ", and this device has no sealed baseline to fall back to"
		record(ctx, ports, store, options, nil, audit.OutcomeDirty, result.Reason)
		return result, nil
	}

	if problems := options.Sealed.Verify(options.PublicKey, options.Now); len(problems) > 0 {
		result.Outcome = OutcomeUnrecoverable
		result.Reason = fmt.Sprintf("%s, and the sealed baseline is not trustworthy: %s",
			reason, problems[0].String())
		record(ctx, ports, store, options, nil, audit.OutcomeDirty, result.Reason)
		return result, nil
	}

	target, configErr := options.Sealed.Config()
	if configErr != nil {
		result.Outcome = OutcomeUnrecoverable
		result.Reason = fmt.Sprintf("%s, and the sealed baseline is unreadable: %v", reason, configErr)
		return result, nil
	}

	files, renderErr := engine.Render(target)
	if renderErr != nil {
		result.Outcome = OutcomeUnrecoverable
		result.Reason = fmt.Sprintf("%s, and the baseline could not be rendered: %v", reason, renderErr)
		return result, nil
	}

	if options.DryRun {
		result.Outcome = OutcomeBaseline
		for path := range files {
			result.Actions = append(result.Actions, "write "+path)
		}
		return result, nil
	}

	for path, contents := range files {
		if writeErr := ports.FS.Write(ctx, path, contents); writeErr != nil {
			result.Outcome = OutcomeUnrecoverable
			result.Reason = fmt.Sprintf("%s, and writing the baseline failed at %s: %v",
				reason, path, writeErr)
			record(ctx, ports, store, options, nil, audit.OutcomeDirty, result.Reason)
			return result, nil
		}
		result.Restored = append(result.Restored, path)
	}

	result.Outcome = OutcomeBaseline
	record(ctx, ports, store, options, nil, audit.OutcomeApplied,
		"recovery fell back to the sealed baseline: "+reason)
	return result, nil
}

func record(ctx context.Context, ports engine.Ports, store audit.Store, options Options,
	pending *audit.Pending, outcome audit.Outcome, intent string) {
	entry := audit.Entry{
		TransactionID: "recovery",
		Intent:        intent,
		ApprovedBy:    "recovery",
		Outcome:       outcome,
		StartedAt:     options.Now,
		FinishedAt:    options.Now,
	}
	if pending != nil {
		entry.TransactionID = pending.TransactionID + "-recovery"
		entry.FromRevision = pending.FromRevision
		entry.ToRevision = pending.FromRevision
		entry.ConfigBefore = pending.ConfigBefore
		entry.ConfigAfter = pending.ConfigBefore
		entry.Files = pending.Files
		entry.Services = pending.Services
	}
	if outcome == audit.OutcomeApplied && options.Sealed != nil {
		if target, err := options.Sealed.Config(); err == nil {
			if files, renderErr := engine.Render(target); renderErr == nil {
				entry.ConfigAfter = engine.Digest(files)
			}
		}
	}
	_, _ = store.Commit(ctx, entry)
	if ports.Logger != nil {
		ports.Logger.Log(engine.LogEvent{
			TransactionID: entry.TransactionID,
			At:            options.Now,
			Kind:          "recovery",
			Detail:        intent,
		})
	}
}

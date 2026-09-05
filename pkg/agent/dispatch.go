package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/brandonapol/normal/pkg/engine"
)

type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type ToolResult struct {
	OK    bool       `json:"ok"`
	Data  any        `json:"data,omitempty"`
	Error *ToolError `json:"error,omitempty"`
}

func succeed(data any) ToolResult {
	return ToolResult{OK: true, Data: data}
}

func failWith(code, message string, details ...any) ToolResult {
	result := ToolResult{OK: false, Error: &ToolError{Code: code, Message: message}}
	if len(details) > 0 {
		result.Error.Details = details[0]
	}
	return result
}

func stringArg(args map[string]any, key string) (string, bool) {
	value, ok := args[key].(string)
	return value, ok
}

func parseOperations(value any) ([]Operation, bool) {
	raw, isSlice := value.([]any)
	if !isSlice {
		return nil, false
	}
	operations := make([]Operation, 0, len(raw))
	for _, item := range raw {
		record, isRecord := item.(map[string]any)
		if !isRecord {
			return nil, false
		}
		path, hasPath := record["path"].(string)
		op, hasOp := record["op"].(string)
		if !hasPath || !hasOp {
			return nil, false
		}
		switch op {
		case "remove":
			operations = append(operations, Operation{Op: "remove", Path: path})
		case "set":
			operations = append(operations, Operation{Op: "set", Path: path, Value: record["value"]})
		default:
			return nil, false
		}
	}
	return operations, true
}

var bookkeepingPaths = map[string]bool{"/metadata/revision": true}

func userVisible(diff engine.Diff) engine.Diff {
	changes := make([]engine.Change, 0, len(diff.Changes))
	for _, change := range diff.Changes {
		if bookkeepingPaths[change.Path] {
			continue
		}
		changes = append(changes, change)
	}
	return engine.Diff{Changes: changes}
}

type ProposalSummary struct {
	ProposalID       string        `json:"proposalId"`
	Intent           string        `json:"intent"`
	Status           string        `json:"status"`
	RequiresApproval bool          `json:"requiresApproval"`
	ToRevision       int           `json:"toRevision"`
	ChangeCount      int           `json:"changeCount"`
	Diff             string        `json:"diff"`
	Review           []PolicyIssue `json:"review"`
	ServicesAffected []string      `json:"servicesAffected"`
	Plan             string        `json:"plan,omitempty"`
}

func summarize(proposal Proposal) ProposalSummary {
	visible := userVisible(proposal.Evaluation.Diff)
	review := proposal.Evaluation.Review
	if review == nil {
		review = []PolicyIssue{}
	}
	services := proposal.Evaluation.Plan.Services
	if services == nil {
		services = []string{}
	}
	return ProposalSummary{
		ProposalID:       proposal.ID,
		Intent:           proposal.Intent,
		Status:           string(proposal.Status),
		RequiresApproval: proposal.Evaluation.RequiresApproval,
		ToRevision:       proposal.Evaluation.Plan.ToRevision,
		ChangeCount:      len(visible.Changes),
		Diff:             engine.FormatDiff(visible),
		Review:           review,
		ServicesAffected: services,
	}
}

type AppSummary struct {
	Package string `json:"package"`
	Label   string `json:"label"`
	State   string `json:"state"`
	Source  string `json:"source"`
	Network string `json:"network"`
}

type RevisionSummary struct {
	Revision      int    `json:"revision"`
	AppliedAt     string `json:"appliedAt"`
	TransactionID string `json:"transactionId"`
	Intent        string `json:"intent"`
}

type ApplySummary struct {
	ProposalID        string   `json:"proposalId"`
	TransactionID     string   `json:"transactionId"`
	Revision          int      `json:"revision"`
	ServicesRestarted []string `json:"servicesRestarted"`
}

func Dispatch(ctx context.Context, session *Session, call ToolCall) ToolResult {
	if !IsToolName(call.Name) {
		return failWith("unknown-tool", fmt.Sprintf("%q is not a tool this agent may call", call.Name))
	}
	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}

	switch call.Name {
	case "get_config":
		return dispatchGetConfig(session, args)
	case "describe_schema":
		path, _ := stringArg(args, "path")
		docs := DocsFor(path)
		if len(docs) == 0 {
			return failWith("not-found", fmt.Sprintf("no documented schema at %q", path))
		}
		return succeed(docs)
	case "list_apps":
		apps := make([]AppSummary, 0)
		for _, entry := range session.Current().Spec.Apps.Entries {
			label := entry.Label
			if label == "" {
				label = entry.Package
			}
			apps = append(apps, AppSummary{
				Package: entry.Package, Label: label, State: entry.State,
				Source: entry.Source, Network: entry.Network,
			})
		}
		return succeed(apps)
	case "propose_change":
		return dispatchProposeChange(session, args)
	case "preview_proposal":
		return dispatchPreview(session, args)
	case "apply_proposal":
		return dispatchApply(ctx, session, args)
	case "discard_proposal":
		id, ok := stringArg(args, "proposalId")
		if !ok {
			return failWith("invalid-arguments", "'proposalId' must be a string")
		}
		proposal, rejection := session.Discard(id)
		if rejection != nil {
			return failWith(string(rejection.Kind), rejection.Message)
		}
		return succeed(map[string]any{"proposalId": proposal.ID, "status": proposal.Status})
	case "list_revisions":
		revisions := make([]RevisionSummary, 0)
		for _, revision := range session.Revisions() {
			revisions = append(revisions, RevisionSummary{
				Revision:      revision.Revision,
				AppliedAt:     revision.AppliedAt.Format("2006-01-02T15:04:05Z07:00"),
				TransactionID: revision.TransactionID,
				Intent:        revision.Intent,
			})
		}
		return succeed(revisions)
	case "propose_rollback":
		return dispatchProposeRollback(session, args)
	}

	return failWith("unknown-tool", fmt.Sprintf("%q is not implemented", call.Name))
}

func dispatchGetConfig(session *Session, args map[string]any) ToolResult {
	current := session.Current()
	section, provided := stringArg(args, "section")
	if !provided || section == "" {
		return succeed(current)
	}
	switch section {
	case "metadata":
		return succeed(current.Metadata)
	case "launcher":
		return succeed(current.Spec.Launcher)
	case "apps":
		return succeed(current.Spec.Apps)
	case "notifications":
		return succeed(current.Spec.Notifications)
	case "attention":
		return succeed(current.Spec.Attention)
	}
	return failWith("invalid-arguments", fmt.Sprintf("unknown section %q", section))
}

func dispatchProposeChange(session *Session, args map[string]any) ToolResult {
	intent, ok := stringArg(args, "intent")
	if !ok {
		return failWith("invalid-arguments", "'intent' must be a string")
	}
	operations, parsed := parseOperations(args["operations"])
	if !parsed {
		return failWith("invalid-arguments", "'operations' must be an array of {op, path, value?}")
	}
	proposal, rejection := session.Propose(intent, operations)
	if rejection != nil {
		return failWith("proposal-rejected", rejection.Error(), rejection)
	}
	return succeed(summarize(proposal))
}

func dispatchPreview(session *Session, args map[string]any) ToolResult {
	id, ok := stringArg(args, "proposalId")
	if !ok {
		return failWith("invalid-arguments", "'proposalId' must be a string")
	}
	proposal, found := session.Proposal(id)
	if !found {
		return failWith("not-found", fmt.Sprintf("no proposal %q", id))
	}
	summary := summarize(proposal)
	summary.Plan = engine.FormatPlan(proposal.Evaluation.Plan)
	return succeed(summary)
}

func dispatchApply(ctx context.Context, session *Session, args map[string]any) ToolResult {
	id, ok := stringArg(args, "proposalId")
	if !ok {
		return failWith("invalid-arguments", "'proposalId' must be a string")
	}
	outcome, rejection := session.Apply(ctx, id)
	if rejection != nil {
		if rejection.Kind == RejectApplyFailed && rejection.Failure != nil {
			return failWith("apply-failed", rejection.Message, map[string]any{
				"rolledBack":    rejection.Failure.RolledBack,
				"deviceDirty":   rejection.Failure.DeviceDirty,
				"transactionId": rejection.Failure.TransactionID,
			})
		}
		return failWith(string(rejection.Kind), rejection.Message)
	}
	services := outcome.Report.Plan.Services
	if services == nil {
		services = []string{}
	}
	return succeed(ApplySummary{
		ProposalID:        outcome.Proposal.ID,
		TransactionID:     outcome.Report.TransactionID,
		Revision:          outcome.Report.Plan.ToRevision,
		ServicesRestarted: services,
	})
}

func dispatchProposeRollback(session *Session, args map[string]any) ToolResult {
	raw, present := args["revision"]
	if !present {
		return failWith("invalid-arguments", "'revision' must be an integer")
	}
	revision, ok := asInteger(raw)
	if !ok {
		return failWith("invalid-arguments", "'revision' must be an integer")
	}
	proposal, rejection := session.ProposeRollback(revision)
	if rejection != nil {
		return failWith("proposal-rejected", rejection.Error(), rejection)
	}
	return succeed(summarize(proposal))
}

func asInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	}
	return 0, false
}

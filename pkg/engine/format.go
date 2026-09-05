package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

func preview(value any) string {
	if value == nil {
		return "none"
	}
	if text, ok := value.(string); ok {
		return truncate(text)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return truncate(string(raw))
}

func truncate(text string) string {
	if len(text) <= 120 {
		return text
	}
	return text[:117] + "..."
}

func FormatChange(change Change) string {
	switch change.Op {
	case OpAdd:
		return fmt.Sprintf("+ %s = %s", change.Path, preview(change.After))
	case OpRemove:
		return fmt.Sprintf("- %s (was %s)", change.Path, preview(change.Before))
	default:
		return fmt.Sprintf("~ %s: %s -> %s", change.Path, preview(change.Before), preview(change.After))
	}
}

func FormatDiff(diff Diff) string {
	if diff.IsEmpty() {
		return "(no changes)"
	}
	lines := make([]string, 0, len(diff.Changes))
	for _, change := range diff.Changes {
		lines = append(lines, FormatChange(change))
	}
	return strings.Join(lines, "\n")
}

func FormatPlan(plan Plan) string {
	lines := []string{
		fmt.Sprintf("revision %d -> %d", plan.FromRevision, plan.ToRevision),
		FormatDiff(plan.Diff),
		"actions:",
	}
	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionWriteFile:
			lines = append(lines, "  write "+action.Path)
		case ActionDeleteFile:
			lines = append(lines, "  delete "+action.Path)
		case ActionRestartService:
			lines = append(lines, "  restart "+action.Service)
		}
	}
	return strings.Join(lines, "\n")
}

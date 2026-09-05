package audit

import (
	"fmt"
	"strings"
	"time"
)

func stamp(at time.Time) string {
	return at.UTC().Format("2006-01-02 15:04:05Z")
}

func RenderEntry(entry Entry) string {
	intent := entry.Intent
	if intent == "" {
		intent = "(no intent recorded)"
	}

	lines := []string{
		fmt.Sprintf("#%d  %s  revision %d -> %d  [%s]",
			entry.Sequence, stamp(entry.FinishedAt), entry.FromRevision, entry.ToRevision, entry.Outcome),
		fmt.Sprintf("    %s", intent),
	}
	if entry.ApprovedBy != "" {
		lines = append(lines, fmt.Sprintf("    approved by %s", entry.ApprovedBy))
	}
	lines = append(lines, fmt.Sprintf("    transaction %s", entry.TransactionID))
	if len(entry.Files) > 0 {
		lines = append(lines, fmt.Sprintf("    files    %s", strings.Join(entry.Files, ", ")))
	}
	if len(entry.Services) > 0 {
		lines = append(lines, fmt.Sprintf("    services %s", strings.Join(entry.Services, ", ")))
	}
	lines = append(lines,
		fmt.Sprintf("    config   %s -> %s", short(entry.ConfigBefore), short(entry.ConfigAfter)),
		fmt.Sprintf("    chain    %s <- %s", short(entry.PreviousHash), short(entry.Hash)),
	)
	return strings.Join(lines, "\n")
}

func RenderLog(entries []Entry, pending *Pending) string {
	if len(entries) == 0 && pending == nil {
		return "no transactions recorded"
	}

	blocks := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		blocks = append(blocks, RenderEntry(entry))
	}
	if pending != nil {
		blocks = append(blocks, fmt.Sprintf(
			"#%d  %s  revision %d -> %d  [unfinished]\n    %s\n    transaction %s started and never completed",
			len(entries), stamp(pending.StartedAt), pending.FromRevision, pending.ToRevision,
			pending.Intent, pending.TransactionID))
	}
	return strings.Join(blocks, "\n\n")
}

func RenderReport(report Report) string {
	lines := []string{fmt.Sprintf("%d entries", report.Entries)}

	if report.Valid() {
		lines = append(lines, "chain intact: every entry hashes to what it claims and links to the one before it")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "")
	for _, problem := range report.Problems {
		lines = append(lines, "  "+problem.String())
	}
	if report.Incomplete {
		lines = append(lines, "",
			"The log is incomplete rather than corrupt: entries before this point are intact.")
	}
	return strings.Join(lines, "\n")
}

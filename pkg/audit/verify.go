package audit

import "fmt"

type ProblemKind string

const (
	ProblemHashMismatch   ProblemKind = "hash-mismatch"
	ProblemBrokenLink     ProblemKind = "broken-link"
	ProblemSequenceGap    ProblemKind = "sequence-gap"
	ProblemTruncated      ProblemKind = "truncated"
	ProblemUnfinished     ProblemKind = "unfinished-transaction"
	ProblemMissingOutcome ProblemKind = "missing-outcome"
	ProblemConfigGap      ProblemKind = "config-changed-outside-engine"
)

type Problem struct {
	Sequence int         `json:"sequence"`
	Kind     ProblemKind `json:"kind"`
	Message  string      `json:"message"`
}

func (p Problem) String() string {
	return fmt.Sprintf("entry %d: %s (%s)", p.Sequence, p.Message, p.Kind)
}

type Report struct {
	Entries    int       `json:"entries"`
	Problems   []Problem `json:"problems"`
	Incomplete bool      `json:"incomplete"`
	Pending    *Pending  `json:"pending,omitempty"`
}

func (r Report) Valid() bool { return len(r.Problems) == 0 }

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeApplied, OutcomeRolledBack, OutcomeDirty:
		return true
	}
	return false
}

func Verify(entries []Entry, decode DecodeReport, pending *Pending) Report {
	report := Report{Entries: len(entries), Problems: []Problem{}, Pending: pending}

	if decode.Truncated {
		report.Incomplete = true
		report.Problems = append(report.Problems, Problem{
			Sequence: len(entries),
			Kind:     ProblemTruncated,
			Message:  "the log ends mid-entry, so the last transaction did not finish writing",
		})
	}

	previousHash := GenesisHash
	for index, entry := range entries {
		if entry.Sequence != index {
			report.Problems = append(report.Problems, Problem{
				Sequence: entry.Sequence,
				Kind:     ProblemSequenceGap,
				Message:  fmt.Sprintf("expected sequence %d, found %d", index, entry.Sequence),
			})
			return report
		}
		if entry.PreviousHash != previousHash {
			report.Problems = append(report.Problems, Problem{
				Sequence: entry.Sequence,
				Kind:     ProblemBrokenLink,
				Message: fmt.Sprintf("links to %s but the previous entry hashes to %s",
					short(entry.PreviousHash), short(previousHash)),
			})
			return report
		}
		if recomputed := ComputeHash(entry); recomputed != entry.Hash {
			report.Problems = append(report.Problems, Problem{
				Sequence: entry.Sequence,
				Kind:     ProblemHashMismatch,
				Message: fmt.Sprintf("content hashes to %s but the entry claims %s; it was edited after being written",
					short(recomputed), short(entry.Hash)),
			})
			return report
		}
		if !validOutcome(entry.Outcome) {
			report.Problems = append(report.Problems, Problem{
				Sequence: entry.Sequence,
				Kind:     ProblemMissingOutcome,
				Message:  fmt.Sprintf("outcome %q is not a recognised result", entry.Outcome),
			})
			return report
		}
		if index > 0 {
			previous := entries[index-1]
			if previous.ConfigAfter != "" && entry.ConfigBefore != "" && previous.ConfigBefore != "" &&
				entry.ConfigBefore != previous.ConfigAfter {
				report.Problems = append(report.Problems, Problem{
					Sequence: entry.Sequence,
					Kind:     ProblemConfigGap,
					Message: fmt.Sprintf(
						"starts from config %s but the previous transaction left %s; the config changed outside the engine",
						short(entry.ConfigBefore), short(previous.ConfigAfter)),
				})
				return report
			}
		}

		previousHash = entry.Hash
	}

	if pending != nil {
		report.Incomplete = true
		report.Problems = append(report.Problems, Problem{
			Sequence: len(entries),
			Kind:     ProblemUnfinished,
			Message: fmt.Sprintf("transaction %s started at %s and never finished",
				pending.TransactionID, pending.StartedAt.Format("2006-01-02T15:04:05Z07:00")),
		})
	}

	return report
}

func short(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func Head(entries []Entry) *Entry {
	if len(entries) == 0 {
		return nil
	}
	return &entries[len(entries)-1]
}

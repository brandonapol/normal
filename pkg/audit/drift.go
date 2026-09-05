package audit

import "fmt"

func CheckConfigDrift(report Report, entries []Entry, observedDigest string) Report {
	head := Head(entries)
	if head == nil || observedDigest == "" {
		return report
	}
	if head.ConfigAfter == observedDigest {
		return report
	}
	report.Problems = append(report.Problems, Problem{
		Sequence: head.Sequence,
		Kind:     ProblemConfigDrift,
		Message: fmt.Sprintf(
			"the config on disk hashes to %s but the last transaction left %s; it was changed outside the engine",
			short(observedDigest), short(head.ConfigAfter)),
	})
	return report
}

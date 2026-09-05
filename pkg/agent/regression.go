package agent

import (
	"fmt"
	"sort"

	"github.com/brandonapol/normal/pkg/config"
)

var permissionStrength = map[string]int{"deny": 0, "ask": 1, "allow": 2}

var networkStrength = map[string]int{"deny": 0, "wifi-only": 1, "allow": 2}

var stateStrength = map[string]int{"absent": 0, "blocked": 0, "installed": 2}

var appsPolicyStrength = map[string]int{"allowlist": 0, "denylist": 1}

func weaker(strength map[string]int, before, after string) bool {
	if before == after {
		return false
	}
	beforeRank, beforeKnown := strength[before]
	afterRank, afterKnown := strength[after]
	if !beforeKnown || !afterKnown {
		return false
	}
	return afterRank > beforeRank
}

func regression(path, message string) PolicyIssue {
	return PolicyIssue{
		Path:     path,
		Code:     "security-regression",
		Severity: SeverityReview,
		Message:  message,
	}
}

func appsByPackage(spec config.Apps) map[string]config.AppEntry {
	out := make(map[string]config.AppEntry, len(spec.Entries))
	for _, entry := range spec.Entries {
		out[entry.Package] = entry
	}
	return out
}

func appRegressions(before, after config.Config) []PolicyIssue {
	issues := make([]PolicyIssue, 0)

	if weaker(appsPolicyStrength, before.Spec.Apps.Policy, after.Spec.Apps.Policy) {
		issues = append(issues, regression("/spec/apps/policy", fmt.Sprintf(
			"app policy loosens from %q to %q", before.Spec.Apps.Policy, after.Spec.Apps.Policy)))
	}

	previous := appsByPackage(before.Spec.Apps)
	for _, entry := range after.Spec.Apps.Entries {
		was, existed := previous[entry.Package]
		if !existed {
			continue
		}
		base := "/spec/apps/entries/" + entry.Package

		if weaker(stateStrength, was.State, entry.State) {
			issues = append(issues, regression(base+"/state", fmt.Sprintf(
				"%s goes from %s to %s", entry.Package, was.State, entry.State)))
		}
		if weaker(networkStrength, was.Network, entry.Network) {
			issues = append(issues, regression(base+"/network", fmt.Sprintf(
				"%s network access widens from %s to %s", entry.Package, was.Network, entry.Network)))
		}

		names := make([]string, 0, len(entry.Permissions))
		for name := range entry.Permissions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			granted := entry.Permissions[name]
			held := was.Permissions[name]
			if held == "" {
				held = "deny"
			}
			if weaker(permissionStrength, held, granted) {
				issues = append(issues, regression(base+"/permissions/"+name, fmt.Sprintf(
					"%s gains %s: %s becomes %s", entry.Package, name, held, granted)))
			}
		}
	}

	return issues
}

func SecurityRegressions(before, after config.Config) []PolicyIssue {
	return append(appRegressions(before, after), weakenings(before, after)...)
}

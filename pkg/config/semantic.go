package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type issueList struct {
	issues []Issue
}

func (l *issueList) add(path, code, message string) {
	l.issues = append(l.issues, Issue{Path: path, Code: code, Message: message})
}

func (l *issueList) duplicates(base string, ids []string, code string) {
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if seen[id] {
			l.add(fmt.Sprintf("%s/%d", base, i), code, fmt.Sprintf("duplicate id %q", id))
		}
		seen[id] = true
	}
}

func installedPackages(c Config) map[string]bool {
	installed := make(map[string]bool, len(c.Spec.Apps.Entries))
	for _, entry := range c.Spec.Apps.Entries {
		if entry.State == "installed" {
			installed[entry.Package] = true
		}
	}
	return installed
}

func semanticIssues(c Config, now time.Time, limits Limits) []Issue {
	l := &issueList{}
	checkLauncher(l, c, limits)
	checkApps(l, c)
	checkNotifications(l, c)
	checkAttention(l, c, now, limits)
	checkReferences(l, c)
	if len(l.issues) == 0 {
		return nil
	}
	return l.issues
}

func checkLauncher(l *issueList, c Config, limits Limits) {
	launcher := c.Spec.Launcher
	pageIDs := make([]string, 0, len(launcher.Pages))
	for i, page := range launcher.Pages {
		pageIDs = append(pageIDs, page.ID)
		base := fmt.Sprintf("/spec/launcher/pages/%d", i)
		if len(page.Items) > launcher.MaxItemsPerPage {
			l.add(base+"/items", "too-many", fmt.Sprintf(
				"page holds %d items, maxItemsPerPage is %d", len(page.Items), launcher.MaxItemsPerPage))
		}
		itemIDs := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			itemIDs = append(itemIDs, item.ID)
		}
		l.duplicates(base+"/items", itemIDs, "duplicate-item-id")
	}
	l.duplicates("/spec/launcher/pages", pageIDs, "duplicate-page-id")

	if len(c.Metadata.Labels) > limits.MaxLabelCount {
		l.add("/metadata/labels", "too-many", fmt.Sprintf("at most %d labels", limits.MaxLabelCount))
	}
}

func checkApps(l *issueList, c Config) {
	packages := make([]string, 0, len(c.Spec.Apps.Entries))
	for _, entry := range c.Spec.Apps.Entries {
		packages = append(packages, entry.Package)
	}
	l.duplicates("/spec/apps/entries", packages, "duplicate-package")
}

func checkNotifications(l *issueList, c Config) {
	notifications := c.Spec.Notifications
	if notifications.Bundling.Enabled && len(notifications.Bundling.DeliveryWindows) == 0 {
		l.add("/spec/notifications/bundling/deliveryWindows", "empty",
			"bundling is enabled but no delivery windows are declared")
	}

	windowIDs := make([]string, 0, len(notifications.QuietHours))
	for _, window := range notifications.QuietHours {
		windowIDs = append(windowIDs, window.ID)
	}
	l.duplicates("/spec/notifications/quietHours", windowIDs, "duplicate-window-id")

	ruleIDs := make([]string, 0, len(notifications.Rules))
	for i, rule := range notifications.Rules {
		ruleIDs = append(ruleIDs, rule.ID)
		if rule.Match.IsEmpty() {
			l.add(fmt.Sprintf("/spec/notifications/rules/%d/match", i), "empty",
				"a rule must constrain at least one field")
		}
	}
	l.duplicates("/spec/notifications/rules", ruleIDs, "duplicate-rule-id")
}

func checkAttention(l *issueList, c Config, now time.Time, limits Limits) {
	policy := c.Spec.Attention.InfiniteScroll

	detectorIDs := make([]string, 0, len(policy.Detectors))
	for i, detector := range policy.Detectors {
		detectorIDs = append(detectorIDs, detector.ID)
		if detector.Kind == "url-pattern" {
			if _, err := regexp.Compile(detector.Pattern); err != nil {
				l.add(fmt.Sprintf("/spec/attention/infiniteScroll/detectors/%d/pattern", i),
					"invalid-format", "expected a valid regular expression")
			}
		}
	}
	l.duplicates("/spec/attention/infiniteScroll/detectors", detectorIDs, "duplicate-detector-id")

	exemptionIDs := make([]string, 0, len(policy.Exemptions))
	for i, exemption := range policy.Exemptions {
		exemptionIDs = append(exemptionIDs, exemption.ID)
		base := fmt.Sprintf("/spec/attention/infiniteScroll/exemptions/%d", i)

		if len(strings.TrimSpace(exemption.Reason)) < limits.MinExemptionReasonLength {
			l.add(base+"/reason", "reason-too-short", fmt.Sprintf(
				"an exemption needs a stated reason of at least %d characters", limits.MinExemptionReasonLength))
		}
		expires, err := time.Parse(time.RFC3339, exemption.ExpiresAt)
		if err != nil {
			l.add(base+"/expiresAt", "invalid-format", "expected an RFC-3339 timestamp")
			continue
		}
		if !expires.After(now) {
			l.add(base+"/expiresAt", "expired", "an exemption must expire in the future")
			continue
		}
		if expires.Sub(now) > time.Duration(limits.MaxExemptionDays)*24*time.Hour {
			l.add(base+"/expiresAt", "too-distant", fmt.Sprintf(
				"an exemption may not run longer than %d days", limits.MaxExemptionDays))
		}
	}
	l.duplicates("/spec/attention/infiniteScroll/exemptions", exemptionIDs, "duplicate-exemption-id")

	budgetIDs := make([]string, 0, len(c.Spec.Attention.SessionBudgets))
	for i, budget := range c.Spec.Attention.SessionBudgets {
		budgetIDs = append(budgetIDs, budget.ID)
		if budget.SessionMinutes > budget.DailyMinutes {
			l.add(fmt.Sprintf("/spec/attention/sessionBudgets/%d/sessionMinutes", i), "inconsistent",
				"sessionMinutes cannot exceed dailyMinutes")
		}
	}
	l.duplicates("/spec/attention/sessionBudgets", budgetIDs, "duplicate-budget-id")
}

func checkReferences(l *issueList, c Config) {
	installed := installedPackages(c)
	require := func(path, pkg string) {
		if pkg == "" || installed[pkg] {
			return
		}
		l.add(path, "dangling-reference", fmt.Sprintf(
			"%q is not an installed app in /spec/apps/entries", pkg))
	}

	for i, pkg := range c.Spec.Launcher.Dock {
		require(fmt.Sprintf("/spec/launcher/dock/%d", i), pkg)
	}
	for pageIndex, page := range c.Spec.Launcher.Pages {
		for itemIndex, item := range page.Items {
			base := fmt.Sprintf("/spec/launcher/pages/%d/items/%d", pageIndex, itemIndex)
			if item.Kind == "app" {
				require(base+"/package", item.Package)
			}
			if item.Kind == "shortcut" && item.Action != nil && item.Action.Kind == "open-app" {
				require(base+"/action/package", item.Action.Package)
			}
		}
	}
	for gesture, action := range c.Spec.Launcher.Gestures {
		if action.Kind == "open-app" {
			require("/spec/launcher/gestures/"+gesture+"/package", action.Package)
		}
	}
	for i, window := range c.Spec.Notifications.QuietHours {
		for j, pkg := range window.Breakthrough {
			require(fmt.Sprintf("/spec/notifications/quietHours/%d/breakthrough/%d", i, j), pkg)
		}
	}
	for i, rule := range c.Spec.Notifications.Rules {
		require(fmt.Sprintf("/spec/notifications/rules/%d/match/package", i), rule.Match.Package)
	}
	for i, exemption := range c.Spec.Attention.InfiniteScroll.Exemptions {
		require(fmt.Sprintf("/spec/attention/infiniteScroll/exemptions/%d/package", i), exemption.Package)
	}
	for i, detector := range c.Spec.Attention.InfiniteScroll.Detectors {
		if detector.Kind == "app-surface" {
			require(fmt.Sprintf("/spec/attention/infiniteScroll/detectors/%d/package", i), detector.Package)
		}
	}
	for i, budget := range c.Spec.Attention.SessionBudgets {
		if budget.Scope.Kind == "app" {
			require(fmt.Sprintf("/spec/attention/sessionBudgets/%d/scope/package", i), budget.Scope.Package)
		}
	}
}

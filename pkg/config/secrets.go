package config

import (
	"regexp"
	"strconv"
	"strings"
)

type secretPattern struct {
	name  string
	match func(string) bool
}

func matches(expression string) func(string) bool {
	compiled := regexp.MustCompile(expression)
	return compiled.MatchString
}

func contains(needle string) func(string) bool {
	return func(value string) bool { return strings.Contains(value, needle) }
}

var secretPatterns = []secretPattern{
	{"a PEM-encoded key or certificate", contains("-----BEGIN ")},
	{"a PuTTY private key", contains("PuTTY-User-Key-File")},
	{"a JSON web token", matches(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{"a GitHub token", matches(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)},
	{"a Slack token", matches(`\bxox[baps]-[A-Za-z0-9-]{10,}`)},
	{"an AWS access key id", matches(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{"a Google API key", matches(`\bAIza[A-Za-z0-9_-]{30,}`)},
	{"an OpenAI-style API key", matches(`\bsk-[A-Za-z0-9_-]{20,}`)},
	{"a GitLab token", matches(`\bglpat-[A-Za-z0-9_-]{15,}`)},
	{"an npm token", matches(`\bnpm_[A-Za-z0-9]{30,}`)},
	{"a Hugging Face token", matches(`\bhf_[A-Za-z0-9]{25,}`)},
}

func LooksLikeSecret(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 12 {
		return "", false
	}
	for _, pattern := range secretPatterns {
		if pattern.match(trimmed) {
			return pattern.name, true
		}
	}
	return "", false
}

func checkSecrets(l *issueList, c Config) {
	scan := func(path, value string) {
		kind, found := LooksLikeSecret(value)
		if !found {
			return
		}
		l.add(path, "plaintext-secret",
			"this looks like "+kind+"; config is rendered into files that services read, so it must not carry credentials")
	}

	scan("/metadata/description", c.Metadata.Description)
	for key, value := range c.Metadata.Labels {
		scan("/metadata/labels/"+key, key)
		scan("/metadata/labels/"+key, value)
	}

	for pageIndex, page := range c.Spec.Launcher.Pages {
		scan(pagePath(pageIndex)+"/title", page.Title)
		for itemIndex, item := range page.Items {
			base := pagePath(pageIndex) + "/items/" + strconv.Itoa(itemIndex)
			scan(base+"/label", item.Label)
			scan(base+"/provider", item.Provider)
			if item.Action != nil {
				scan(base+"/action/url", item.Action.URL)
			}
		}
	}

	for index, entry := range c.Spec.Apps.Entries {
		base := "/spec/apps/entries/" + strconv.Itoa(index)
		scan(base+"/label", entry.Label)
		scan(base+"/sandboxProfile", entry.SandboxProfile)
	}

	for index, rule := range c.Spec.Notifications.Rules {
		base := "/spec/notifications/rules/" + strconv.Itoa(index) + "/match"
		scan(base+"/channel", rule.Match.Channel)
		scan(base+"/titleContains", rule.Match.TitleContains)
	}

	policy := c.Spec.Attention.InfiniteScroll
	for index, detector := range policy.Detectors {
		base := "/spec/attention/infiniteScroll/detectors/" + strconv.Itoa(index)
		scan(base+"/pattern", detector.Pattern)
		scan(base+"/surface", detector.Surface)
	}
	for index, exemption := range policy.Exemptions {
		scan("/spec/attention/infiniteScroll/exemptions/"+strconv.Itoa(index)+"/reason", exemption.Reason)
	}
	for index, budget := range c.Spec.Attention.SessionBudgets {
		scan("/spec/attention/sessionBudgets/"+strconv.Itoa(index)+"/scope/domain", budget.Scope.Domain)
	}
}

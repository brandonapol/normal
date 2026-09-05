package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/brandonapol/normal/pkg/config"
)

const MaxOperations = 64

type Severity string

const (
	SeverityDeny   Severity = "deny"
	SeverityReview Severity = "review"
)

type PolicyIssue struct {
	Path     string   `json:"path"`
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type Verdict struct {
	Denied           []PolicyIssue
	Review           []PolicyIssue
	RequiresApproval bool
}

var writableRoots = []string{"/spec", "/metadata/description", "/metadata/labels"}

var engineManaged = []string{"/apiVersion", "/kind", "/metadata/name", "/metadata/revision"}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/spec/attention(/|$)`),
	regexp.MustCompile(`^/spec/apps/entries/[^/]+/(permissions|network|state)(/|$)`),
	regexp.MustCompile(`^/spec/apps/policy$`),
}

func covers(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

func isWritable(path string) bool {
	for _, root := range writableRoots {
		if covers(root, path) {
			return true
		}
	}
	return false
}

func isEngineManaged(path string) bool {
	for _, root := range engineManaged {
		if covers(root, path) {
			return true
		}
	}
	return false
}

func isSensitive(path string) bool {
	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

var enforcementStrength = map[string]int{"warn": 1, "paginate": 2, "block": 3}

func weakenings(before, after config.Config) []PolicyIssue {
	issues := make([]PolicyIssue, 0)
	from := before.Spec.Attention.InfiniteScroll
	to := after.Spec.Attention.InfiniteScroll

	if enforcementStrength[to.Enforcement] < enforcementStrength[from.Enforcement] {
		issues = append(issues, PolicyIssue{
			Path:     "/spec/attention/infiniteScroll/enforcement",
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message:  fmt.Sprintf("enforcement drops from %q to %q", from.Enforcement, to.Enforcement),
		})
	}
	if to.MaxAutoLoads > from.MaxAutoLoads {
		issues = append(issues, PolicyIssue{
			Path:     "/spec/attention/infiniteScroll/maxAutoLoads",
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message: fmt.Sprintf("automatic loads per session rise from %d to %d",
				from.MaxAutoLoads, to.MaxAutoLoads),
		})
	}
	if to.PageSize > from.PageSize {
		issues = append(issues, PolicyIssue{
			Path:     "/spec/attention/infiniteScroll/pageSize",
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message:  fmt.Sprintf("page size grows from %d to %d", from.PageSize, to.PageSize),
		})
	}

	existing := make(map[string]bool, len(from.Exemptions))
	for _, exemption := range from.Exemptions {
		existing[exemption.Package] = true
	}
	for _, exemption := range to.Exemptions {
		if existing[exemption.Package] {
			continue
		}
		issues = append(issues, PolicyIssue{
			Path:     "/spec/attention/infiniteScroll/exemptions",
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message: fmt.Sprintf("adds a scroll exemption for %q until %s",
				exemption.Package, exemption.ExpiresAt),
		})
	}

	if from.Webview.InterceptIntersectionObserver && !to.Webview.InterceptIntersectionObserver {
		issues = append(issues, PolicyIssue{
			Path:     "/spec/attention/infiniteScroll/webview/interceptIntersectionObserver",
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message:  "stops intercepting the sentinel pattern web feeds use to auto-load",
		})
	}

	budgets := make(map[string]config.SessionBudget, len(before.Spec.Attention.SessionBudgets))
	for _, budget := range before.Spec.Attention.SessionBudgets {
		budgets[budget.ID] = budget
	}
	for _, budget := range after.Spec.Attention.SessionBudgets {
		previous, existed := budgets[budget.ID]
		if !existed || budget.DailyMinutes <= previous.DailyMinutes {
			continue
		}
		issues = append(issues, PolicyIssue{
			Path:     fmt.Sprintf("/spec/attention/sessionBudgets/%s/dailyMinutes", budget.ID),
			Code:     "weakens-attention-policy",
			Severity: SeverityReview,
			Message: fmt.Sprintf("daily budget rises from %d to %d minutes",
				previous.DailyMinutes, budget.DailyMinutes),
		})
	}

	return issues
}

func CheckOperationCount(operations []Operation) []PolicyIssue {
	if len(operations) == 0 {
		return []PolicyIssue{{
			Path: "/", Code: "invalid-operation-count", Severity: SeverityDeny,
			Message: "a proposal must contain at least one operation",
		}}
	}
	if len(operations) > MaxOperations {
		return []PolicyIssue{{
			Path: "/", Code: "invalid-operation-count", Severity: SeverityDeny,
			Message: fmt.Sprintf("a proposal may contain at most %d operations", MaxOperations),
		}}
	}
	return nil
}

func CheckOperationPaths(operations []Operation) []PolicyIssue {
	issues := make([]PolicyIssue, 0)
	for _, operation := range operations {
		switch {
		case isEngineManaged(operation.Path):
			issues = append(issues, PolicyIssue{
				Path: operation.Path, Code: "engine-managed-field", Severity: SeverityDeny,
				Message: "this field is managed by the mutation engine and cannot be set by an agent",
			})
		case !isWritable(operation.Path):
			issues = append(issues, PolicyIssue{
				Path: operation.Path, Code: "path-not-writable", Severity: SeverityDeny,
				Message: "agents may only write under " + joinMessages(writableRoots),
			})
		case isSensitive(operation.Path):
			issues = append(issues, PolicyIssue{
				Path: operation.Path, Code: "sensitive-path", Severity: SeverityReview,
				Message: "this path changes attention or app-permission policy and needs explicit approval",
			})
		}
	}
	return issues
}

func CheckOperations(operations []Operation) []PolicyIssue {
	return append(CheckOperationCount(operations), CheckOperationPaths(operations)...)
}

func CheckPolicy(operations []Operation, before, after config.Config) Verdict {
	issues := append(CheckOperationPaths(operations), SecurityRegressions(before, after)...)
	verdict := Verdict{Denied: make([]PolicyIssue, 0), Review: make([]PolicyIssue, 0)}
	for _, issue := range issues {
		if issue.Severity == SeverityDeny {
			verdict.Denied = append(verdict.Denied, issue)
			continue
		}
		verdict.Review = append(verdict.Review, issue)
	}
	verdict.RequiresApproval = len(verdict.Review) > 0
	return verdict
}

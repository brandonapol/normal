package config_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
)

func nested(depth int) any {
	var value any = "leaf"
	for i := 0; i < depth; i++ {
		value = map[string]any{"a": value}
	}
	return value
}

func codeOf(t *testing.T, issues []config.Issue) string {
	t.Helper()
	if len(issues) == 0 {
		t.Fatal("expected the document to be rejected")
	}
	return issues[0].Code
}

func TestRejectsOversizedDocument(t *testing.T) {
	limits := config.MustLimits()
	document := map[string]any{
		"apiVersion": config.APIVersion,
		"kind":       config.Kind,
		"filler":     strings.Repeat("x", limits.MaxDocumentBytes+1),
	}
	if got := codeOf(t, config.Validate(document, now)); got != "document-too-large" {
		t.Fatalf("expected document-too-large, got %q", got)
	}
}

func TestRejectsDeeplyNestedDocument(t *testing.T) {
	limits := config.MustLimits()
	document := map[string]any{"spec": nested(limits.MaxDocumentDepth + 10)}
	if got := codeOf(t, config.Validate(document, now)); got != "document-too-deep" {
		t.Fatalf("expected document-too-deep, got %q", got)
	}
}

func TestRejectsOverlyComplexDocument(t *testing.T) {
	limits := config.MustLimits()
	values := make([]any, limits.MaxDocumentNodes+1)
	for i := range values {
		values[i] = i
	}
	if got := codeOf(t, config.Validate(map[string]any{"spec": values}, now)); got != "document-too-complex" {
		t.Fatalf("expected document-too-complex, got %q", got)
	}
}

func TestDeepNestingDoesNotStackOverflow(t *testing.T) {
	document := map[string]any{"spec": nested(50_000)}
	issues := config.Validate(document, now)
	if len(issues) == 0 {
		t.Fatal("expected rejection")
	}
	if issues[0].Code != "document-too-deep" {
		t.Fatalf("expected document-too-deep, got %q", issues[0].Code)
	}
}

func TestGuardsRunBeforeSchemaEvaluation(t *testing.T) {
	limits := config.MustLimits()
	document := map[string]any{"totally": "invalid", "and": nested(limits.MaxDocumentDepth + 5)}
	issues := config.Validate(document, now)
	if len(issues) != 1 {
		t.Fatalf("a guarded document should fail fast with one issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Code != "document-too-deep" {
		t.Fatalf("expected the guard to fire before schema evaluation, got %q", issues[0].Code)
	}
}

func TestBaselineIsComfortablyInsideTheGuards(t *testing.T) {
	limits := config.MustLimits()
	raw, err := json.Marshal(config.Baseline())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(raw) > limits.MaxDocumentBytes/4 {
		t.Fatalf("baseline is %d bytes, uncomfortably close to the %d limit",
			len(raw), limits.MaxDocumentBytes)
	}
}

func TestRejectsOverlongDetectorPattern(t *testing.T) {
	limits := config.MustLimits()
	document := mutate(t, func(m map[string]any) {
		policy := scrollPolicy(m)
		policy["detectors"] = append(policy["detectors"].([]any), map[string]any{
			"kind":    "url-pattern",
			"id":      "huge",
			"pattern": strings.Repeat("(a|b)", limits.MaxPatternLength),
		})
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/attention/infiniteScroll/detectors/2/pattern"), "pattern-too-long") {
		t.Fatalf("expected pattern-too-long, got %v", issues)
	}
}

func TestAcceptsReasonableDetectorPattern(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		policy := scrollPolicy(m)
		policy["detectors"] = append(policy["detectors"].([]any), map[string]any{
			"kind":    "url-pattern",
			"id":      "feeds",
			"pattern": `^https://[^/]+/(feed|timeline|for-you)`,
		})
	})
	if issues := config.Validate(document, now); len(issues) > 0 {
		t.Fatalf("a normal pattern should validate, got %v", issues)
	}
}

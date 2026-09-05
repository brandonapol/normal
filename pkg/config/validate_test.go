package config_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/brandonapol/normal/pkg/config"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func mutate(t *testing.T, apply func(m map[string]any)) map[string]any {
	t.Helper()
	document, err := config.ToDocument(config.Baseline())
	if err != nil {
		t.Fatalf("ToDocument: %v", err)
	}
	apply(document)
	return document
}

func spec(m map[string]any) map[string]any {
	return m["spec"].(map[string]any)
}

func scrollPolicy(m map[string]any) map[string]any {
	return spec(m)["attention"].(map[string]any)["infiniteScroll"].(map[string]any)
}

func codesAt(issues []config.Issue, path string) []string {
	var codes []string
	for _, issue := range issues {
		if issue.Path == path {
			codes = append(codes, issue.Code)
		}
	}
	return codes
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestBaselineIsValid(t *testing.T) {
	if issues := config.ValidateConfig(config.Baseline(), now); len(issues) > 0 {
		t.Fatalf("baseline should validate, got %v", issues)
	}
}

func TestRejectsUnknownAPIVersion(t *testing.T) {
	document := mutate(t, func(m map[string]any) { m["apiVersion"] = "normal.os/v99" })
	issues := config.Validate(document, now)
	if len(issues) == 0 {
		t.Fatal("expected an issue")
	}
	if !contains(codesAt(issues, "/apiVersion"), "unsupported-version") {
		t.Fatalf("expected unsupported-version at /apiVersion, got %v", issues)
	}
}

func TestRejectsUnknownField(t *testing.T) {
	document := mutate(t, func(m map[string]any) { m["extras"] = map[string]any{"rogue": true} })
	if issues := config.Validate(document, now); len(issues) == 0 {
		t.Fatal("closed schema should reject an unknown top-level field")
	}
}

func TestRejectsDanglingDockReference(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		spec(m)["launcher"].(map[string]any)["dock"] = []any{"com.example.ghost"}
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/launcher/dock/0"), "dangling-reference") {
		t.Fatalf("expected dangling-reference, got %v", issues)
	}
}

func TestEnforcementHasNoOffSwitch(t *testing.T) {
	document := mutate(t, func(m map[string]any) { scrollPolicy(m)["enforcement"] = "off" })
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/attention/infiniteScroll/enforcement"), "invalid-enforcement") {
		t.Fatalf("expected invalid-enforcement, got %v", issues)
	}
}

func TestDetectorsCannotBeEmptied(t *testing.T) {
	document := mutate(t, func(m map[string]any) { scrollPolicy(m)["detectors"] = []any{} })
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/attention/infiniteScroll/detectors"), "empty-detectors") {
		t.Fatalf("expected empty-detectors, got %v", issues)
	}
}

func TestWebviewShimCannotBeDisabled(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["webview"].(map[string]any)["injectShim"] = false
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/attention/infiniteScroll/webview/injectShim"), "policy-violation") {
		t.Fatalf("expected policy-violation, got %v", issues)
	}
}

func TestBoundedExemptionIsAccepted(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["exemptions"] = []any{map[string]any{
			"id":        "maps-transit",
			"package":   "com.google.android.apps.maps",
			"reason":    "transit departure board is a continuous list by nature",
			"expiresAt": "2026-01-10T00:00:00Z",
		}}
	})
	if issues := config.Validate(document, now); len(issues) > 0 {
		t.Fatalf("expected a bounded exemption to validate, got %v", issues)
	}
}

func TestExemptionMustBeJustifiedAndBounded(t *testing.T) {
	base := "/spec/attention/infiniteScroll/exemptions/0"

	expired := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["exemptions"] = []any{map[string]any{
			"id": "sloppy", "package": "com.google.android.apps.maps",
			"reason": "a properly stated reason", "expiresAt": "2025-01-01T00:00:00Z",
		}}
	})
	if !contains(codesAt(config.Validate(expired, now), base+"/expiresAt"), "expired") {
		t.Errorf("expected an expired exemption to be rejected")
	}

	distant := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["exemptions"] = []any{map[string]any{
			"id": "sloppy", "package": "com.google.android.apps.maps",
			"reason": "a properly stated reason", "expiresAt": "2027-01-01T00:00:00Z",
		}}
	})
	if !contains(codesAt(config.Validate(distant, now), base+"/expiresAt"), "too-distant") {
		t.Errorf("expected an over-long exemption to be rejected")
	}

	unjustified := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["exemptions"] = []any{map[string]any{
			"id": "sloppy", "package": "com.google.android.apps.maps",
			"reason": "why not", "expiresAt": "2026-01-10T00:00:00Z",
		}}
	})
	if len(config.Validate(unjustified, now)) == 0 {
		t.Errorf("expected an unjustified exemption to be rejected")
	}
}

func TestExemptionsAreCapped(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		exemptions := make([]any, 4)
		for i := range exemptions {
			exemptions[i] = map[string]any{
				"id":        string(rune('a'+i)) + "-exemption",
				"package":   "com.google.android.apps.maps",
				"reason":    "a properly stated reason for this one",
				"expiresAt": "2026-01-10T00:00:00Z",
			}
		}
		scrollPolicy(m)["exemptions"] = exemptions
	})
	if len(config.Validate(document, now)) == 0 {
		t.Fatal("expected a fourth exemption to be rejected")
	}
}

func TestRejectsRuleThatMatchesNothing(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		rules := spec(m)["notifications"].(map[string]any)["rules"].([]any)
		rules[0].(map[string]any)["match"] = map[string]any{}
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/notifications/rules/0/match"), "empty") {
		t.Fatalf("expected empty match to be rejected, got %v", issues)
	}
}

func TestRejectsOverfullPage(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		spec(m)["launcher"].(map[string]any)["maxItemsPerPage"] = 2
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/launcher/pages/0/items"), "too-many") {
		t.Fatalf("expected too-many, got %v", issues)
	}
}

func TestLimitsComeFromTheSchema(t *testing.T) {
	limits, err := config.SchemaLimits()
	if err != nil {
		t.Fatalf("SchemaLimits: %v", err)
	}
	if limits.MaxExemptions != 3 || limits.MinExemptionReasonLength != 12 {
		t.Fatalf("limits did not decode from CUE: %+v", limits)
	}
}

func TestBaselineRoundTripsThroughJSON(t *testing.T) {
	raw, err := json.Marshal(config.Baseline())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := config.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if issues := config.ValidateConfig(parsed, now); len(issues) > 0 {
		t.Fatalf("round-tripped baseline should validate, got %v", issues)
	}
}

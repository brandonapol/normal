package config_test

import (
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
)

// Credential shapes are assembled at run time rather than written as literals:
// a file full of realistic-looking tokens trips every other secret scanner,
// including the one guarding pushes to this repository.
func credentialShapes() []string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	digits := "0123456789012345678901234567890123456789"
	return []string{
		"gh" + "p_" + letters + digits[:10],
		"github" + "_pat_" + strings.ToUpper(letters[:12]) + digits[:12],
		"xox" + "b-" + digits[:12] + "-" + digits[:13] + "-" + letters,
		"AK" + "IA" + strings.ToUpper(letters[:16]),
		"AI" + "za" + letters + digits[:8],
		"sk" + "-" + letters + digits[:6],
		"gl" + "pat-" + letters[:20],
		"np" + "m_" + letters + digits[:6],
		"hf" + "_" + letters + digits[:4],
		"ey" + "Jhbabcdefgh." + letters[:12] + "." + letters[:14],
		"-----BEGIN " + "OPENSSH PRIVATE KEY-----",
	}
}

func TestRecognisesCommonCredentialShapes(t *testing.T) {
	for _, value := range credentialShapes() {
		if _, found := config.LooksLikeSecret(value); !found {
			t.Errorf("should have been recognised as a credential: %.30q", value)
		}
	}
}

func TestDoesNotFireOnOrdinaryConfigText(t *testing.T) {
	for _, value := range []string{
		"",
		"Spotify",
		"Default Normal phone configuration",
		"transit departure board is a continuous list by nature",
		`^https://[^/]+/(feed|timeline|for-you)`,
		"com.google.android.apps.maps",
		"os.normal.browser",
		"sk",
		"a normal sentence about tokens and secrets and passwords",
		"AKIA",
		"skiing-holiday-notes",
	} {
		if kind, found := config.LooksLikeSecret(value); found {
			t.Errorf("false positive on %q, matched %s", value, kind)
		}
	}
}

func TestRejectsASecretPastedIntoAnOrdinaryField(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		m["metadata"].(map[string]any)["description"] = "sync token " + credentialShapes()[0] + " for backups"
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/metadata/description"), "plaintext-secret") {
		t.Fatalf("expected plaintext-secret, got %v", issues)
	}
}

func TestRejectsASecretInAnExemptionReason(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		scrollPolicy(m)["exemptions"] = []any{map[string]any{
			"id":        "maps-transit",
			"package":   "com.google.android.apps.maps",
			"reason":    credentialShapes()[10] + " pasted by mistake",
			"expiresAt": "2026-01-10T00:00:00Z",
		}}
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/attention/infiniteScroll/exemptions/0/reason"), "plaintext-secret") {
		t.Fatalf("expected plaintext-secret, got %v", issues)
	}
}

func TestRejectsASecretInAnAppLabel(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		entries := spec(m)["apps"].(map[string]any)["entries"].([]any)
		entries[0].(map[string]any)["label"] = credentialShapes()[3]
	})
	issues := config.Validate(document, now)
	if !contains(codesAt(issues, "/spec/apps/entries/0/label"), "plaintext-secret") {
		t.Fatalf("expected plaintext-secret, got %v", issues)
	}
}

func TestTheExplanationSaysWhyItMatters(t *testing.T) {
	document := mutate(t, func(m map[string]any) {
		m["metadata"].(map[string]any)["description"] = credentialShapes()[3]
	})
	for _, issue := range config.Validate(document, now) {
		if issue.Code != "plaintext-secret" {
			continue
		}
		if !strings.Contains(issue.Message, "services read") {
			t.Fatalf("the message should explain the consequence, got %q", issue.Message)
		}
		return
	}
	t.Fatal("expected a plaintext-secret issue")
}

func TestBaselineCarriesNoSecrets(t *testing.T) {
	for _, issue := range config.ValidateConfig(config.Baseline(), now) {
		if issue.Code == "plaintext-secret" {
			t.Fatalf("the shipped baseline must not carry credentials: %v", issue)
		}
	}
}

package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const fixedNow = "2026-01-01T00:00:00Z"

func capture(t *testing.T, command string, args ...string) (string, error) {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer

	runErr := run(command, args)

	writer.Close()
	os.Stdout = original
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return string(output), runErr
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func TestValidateAcceptsBaseline(t *testing.T) {
	output, err := capture(t, "validate", "--now", fixedNow, repoPath("examples", "baseline.config.json"))
	if err != nil {
		t.Fatalf("expected the baseline to validate, got %v", err)
	}
	if !strings.Contains(output, "is valid") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestValidateRejectsCorpusFixtures(t *testing.T) {
	for _, name := range []string{
		"enforcement-off",
		"detectors-emptied",
		"webview-shim-disabled",
		"unknown-launcher-field",
	} {
		t.Run(name, func(t *testing.T) {
			path := repoPath("testdata", "invariants", "reject", name+".json")
			output, err := capture(t, "validate", "--now", fixedNow, path)
			if !errors.Is(err, errInvalidConfig) {
				t.Fatalf("expected %s to be rejected, got %v", name, err)
			}
			if !strings.Contains(output, "issue(s)") {
				t.Fatalf("expected the issues to be printed, got %q", output)
			}
		})
	}
}

func TestValidateReportsUnreadableFile(t *testing.T) {
	if _, err := capture(t, "validate", filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestValidateRejectsBadNow(t *testing.T) {
	_, err := capture(t, "validate", "--now", "yesterday", repoPath("examples", "baseline.config.json"))
	if err == nil || !strings.Contains(err.Error(), "RFC-3339") {
		t.Fatalf("expected an RFC-3339 complaint, got %v", err)
	}
}

func TestValidateRequiresExactlyOneFile(t *testing.T) {
	if _, err := capture(t, "validate"); err == nil {
		t.Fatal("expected an error when no file is given")
	}
}

func TestBaselineEmitsValidJSON(t *testing.T) {
	output, err := capture(t, "baseline")
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("baseline output is not JSON: %v", err)
	}
	if document["apiVersion"] != "normal.os/v0" {
		t.Fatalf("unexpected apiVersion %v", document["apiVersion"])
	}
}

func TestSchemaEmitsTheCUESource(t *testing.T) {
	output, err := capture(t, "schema")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	if !strings.Contains(output, "#PhoneConfig") {
		t.Fatal("schema output is missing the root definition")
	}
	if !regexp.MustCompile(`injectShim:\s+true`).MatchString(output) {
		t.Fatal("schema output does not pin the webview shim to true")
	}
}

func TestRenderPrintsEveryFile(t *testing.T) {
	output, err := capture(t, "render", repoPath("examples", "baseline.config.json"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, file := range []string{"launcher.json", "apps.json", "notifications.json", "attention.json", "webview-shim.json"} {
		if !strings.Contains(output, file) {
			t.Errorf("render output is missing %s", file)
		}
	}
}

func desiredConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(repoPath("examples", "baseline.config.json"))
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	var document map[string]any
	if decodeErr := json.Unmarshal(raw, &document); decodeErr != nil {
		t.Fatalf("parse baseline: %v", decodeErr)
	}
	document["metadata"].(map[string]any)["revision"] = 1
	document["spec"].(map[string]any)["launcher"].(map[string]any)["columns"] = 2

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode desired: %v", err)
	}
	path := filepath.Join(t.TempDir(), "desired.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write desired: %v", err)
	}
	return path
}

func TestDiffShowsChangedPaths(t *testing.T) {
	output, err := capture(t, "diff", repoPath("examples", "baseline.config.json"), desiredConfig(t))
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(output, "/spec/launcher/columns") {
		t.Fatalf("unexpected diff %q", output)
	}
}

func TestPlanDryRunsAgainstMemory(t *testing.T) {
	output, err := capture(t, "plan", repoPath("examples", "baseline.config.json"), desiredConfig(t))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, want := range []string{"restart normal-launcher", "dry run", "ok"} {
		if !strings.Contains(output, want) {
			t.Errorf("plan output is missing %q: %s", want, output)
		}
	}
}

func TestPlanRejectsStaleRevision(t *testing.T) {
	baseline := repoPath("examples", "baseline.config.json")
	if _, err := capture(t, "plan", baseline, baseline); err != nil {
		t.Fatalf("planning a no-op should succeed, got %v", err)
	}
}

func TestPairCommandsNeedTwoFiles(t *testing.T) {
	if _, err := capture(t, "diff", repoPath("examples", "baseline.config.json")); err == nil {
		t.Fatal("expected diff to require two files")
	}
	if _, err := capture(t, "plan"); err == nil {
		t.Fatal("expected plan to require two files")
	}
}

func TestUnknownCommand(t *testing.T) {
	if _, err := capture(t, "frobnicate"); err == nil {
		t.Fatal("expected an unknown command to error")
	}
}

func TestHelp(t *testing.T) {
	output, err := capture(t, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(output, "normalctl") {
		t.Fatalf("unexpected help output %q", output)
	}
}

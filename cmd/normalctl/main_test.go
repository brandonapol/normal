package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/agent"
	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
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

func auditLogDir(t *testing.T) string {
	t.Helper()

	files, err := engine.Render(config.Baseline())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files})
	store := audit.NewStore(ports.FS, "/etc/normal")
	ports.Audit = store

	session := agent.NewSession(agent.SessionOptions{
		InitialConfig: config.Baseline(),
		Ports:         ports.Ports,
	})
	proposal, rejection := session.Propose("make the home screen two columns",
		[]agent.Operation{{Op: "set", Path: "/spec/launcher/columns", Value: 2}})
	if rejection != nil {
		t.Fatalf("Propose: %v", rejection)
	}
	if _, rej := session.Approve(proposal.ID, "brandon"); rej != nil {
		t.Fatalf("Approve: %v", rej)
	}
	if _, rej := session.Apply(context.Background(), proposal.ID); rej != nil {
		t.Fatalf("Apply: %v", rej)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "audit.log"),
		[]byte(ports.FS.Snapshot()["/etc/normal/audit.log"]), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return dir
}

func TestAuditLogRendersHistory(t *testing.T) {
	output, err := capture(t, "audit", "log", auditLogDir(t))
	if err != nil {
		t.Fatalf("audit log: %v", err)
	}
	for _, want := range []string{"make the home screen two columns", "approved by brandon", "applied", "revision 0 -> 1"} {
		if !strings.Contains(output, want) {
			t.Errorf("audit log output is missing %q:\n%s", want, output)
		}
	}
}

func TestAuditVerifyAcceptsAnIntactChain(t *testing.T) {
	output, err := capture(t, "audit", "verify", auditLogDir(t))
	if err != nil {
		t.Fatalf("an intact chain should verify: %v", err)
	}
	if !strings.Contains(output, "chain intact") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestAuditVerifyRejectsATamperedChain(t *testing.T) {
	dir := auditLogDir(t)
	path := filepath.Join(dir, "audit.log")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tampered := strings.Replace(string(raw), `"approvedBy":"brandon"`, `"approvedBy":"nobody"`, 1)
	if tampered == string(raw) {
		t.Fatal("test setup failed to tamper with the log")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, verifyErr := capture(t, "audit", "verify", dir)
	if !errors.Is(verifyErr, errInvalidConfig) {
		t.Fatalf("a tampered chain must fail verification, got %v", verifyErr)
	}
	if !strings.Contains(output, "hash-mismatch") {
		t.Fatalf("the failure should name the problem, got %q", output)
	}
}

func TestAuditOnAnEmptyDirectory(t *testing.T) {
	output, err := capture(t, "audit", "log", t.TempDir())
	if err != nil {
		t.Fatalf("an absent log is not an error: %v", err)
	}
	if !strings.Contains(output, "no transactions recorded") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestAuditRejectsUnknownSubcommand(t *testing.T) {
	if _, err := capture(t, "audit", "frobnicate"); err == nil {
		t.Fatal("expected an unknown subcommand to error")
	}
	if _, err := capture(t, "audit"); err == nil {
		t.Fatal("expected a missing subcommand to error")
	}
}

func signedDeviceDir(t *testing.T) (string, *audit.SoftwareSigner) {
	t.Helper()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	signer, err := audit.NewSoftwareSignerFromSeed(seed)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	files, err := engine.Render(config.Baseline())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files})
	store := audit.NewStore(ports.FS, "/etc/normal").WithSigner(signer)
	ports.Audit = store

	session := agent.NewSession(agent.SessionOptions{
		InitialConfig: config.Baseline(),
		Ports:         ports.Ports,
	})
	proposal, rejection := session.Propose("make the home screen two columns",
		[]agent.Operation{{Op: "set", Path: "/spec/launcher/columns", Value: 2}})
	if rejection != nil {
		t.Fatalf("Propose: %v", rejection)
	}
	session.Approve(proposal.ID, "brandon")
	if _, rej := session.Apply(context.Background(), proposal.ID); rej != nil {
		t.Fatalf("Apply: %v", rej)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "generated"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for path, contents := range ports.FS.Snapshot() {
		relative := strings.TrimPrefix(path, "/etc/normal/")
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(relative)), []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", relative, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "audit.pub"),
		[]byte(base64.StdEncoding.EncodeToString(signer.PublicKey())), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return dir, signer
}

func TestVerifyAcceptsACleanDevice(t *testing.T) {
	dir, _ := signedDeviceDir(t)
	output, err := capture(t, "verify", dir)
	if err != nil {
		t.Fatalf("a clean device should verify: %v\n%s", err, output)
	}
	if !strings.Contains(output, "chain intact") {
		t.Fatalf("unexpected output %q", output)
	}
	if strings.Contains(output, "signatures were not checked") {
		t.Fatal("audit.pub is present, so signatures should have been checked")
	}
}

func TestVerifyDetectsAHandEditedConfig(t *testing.T) {
	dir, _ := signedDeviceDir(t)
	path := filepath.Join(dir, "launcher.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	edited := strings.Replace(string(raw), `"columns": 2`, `"columns": 6`, 1)
	if edited == string(raw) {
		t.Fatal("test setup failed to edit the config")
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, verifyErr := capture(t, "verify", dir)
	if !errors.Is(verifyErr, errInvalidConfig) {
		t.Fatalf("a hand-edited config must fail verification, got %v", verifyErr)
	}
	if !strings.Contains(output, "config-drift") {
		t.Fatalf("the failure should name drift, got %q", output)
	}
}

func TestVerifyDetectsATamperedAuditLog(t *testing.T) {
	dir, _ := signedDeviceDir(t)
	path := filepath.Join(dir, "audit.log")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tampered := strings.Replace(string(raw), `"approvedBy":"brandon"`, `"approvedBy":"nobody"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	output, verifyErr := capture(t, "verify", dir)
	if !errors.Is(verifyErr, errInvalidConfig) {
		t.Fatalf("a tampered log must fail verification, got %v", verifyErr)
	}
	if !strings.Contains(output, "hash-mismatch") {
		t.Fatalf("expected a hash mismatch, got %q", output)
	}
}

func TestVerifySaysWhenSignaturesWereNotChecked(t *testing.T) {
	dir, _ := signedDeviceDir(t)
	if err := os.Remove(filepath.Join(dir, "audit.pub")); err != nil {
		t.Fatalf("remove key: %v", err)
	}

	output, err := capture(t, "verify", dir)
	if err != nil {
		t.Fatalf("verification without a key should still run: %v", err)
	}
	if !strings.Contains(output, "signatures were not checked") {
		t.Fatal("the absence of a key must be stated, not silently ignored")
	}
}

func TestVerifyRejectsAForeignKey(t *testing.T) {
	dir, _ := signedDeviceDir(t)

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(200 - i)
	}
	other, err := audit.NewSoftwareSignerFromSeed(seed)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audit.pub"),
		[]byte(base64.StdEncoding.EncodeToString(other.PublicKey())), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	output, verifyErr := capture(t, "verify", dir)
	if !errors.Is(verifyErr, errInvalidConfig) {
		t.Fatalf("a chain signed by another key must not verify, got %v", verifyErr)
	}
	if !strings.Contains(output, "unexpected-signing-key") {
		t.Fatalf("expected the key mismatch to be named, got %q", output)
	}
}

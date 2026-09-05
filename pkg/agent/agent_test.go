package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/agent"
	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
)

func newSession(t *testing.T, faults ...engine.Fault) (*agent.Session, engine.MemoryPorts) {
	t.Helper()
	files, err := engine.Render(config.Baseline())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files, Faults: faults})
	session := agent.NewSession(agent.SessionOptions{
		InitialConfig: config.Baseline(),
		Ports:         ports.Ports,
	})
	return session, ports
}

func newPermissiveSession(t *testing.T) *agent.Session {
	t.Helper()
	files, _ := engine.Render(config.Baseline())
	ports := engine.NewMemoryPorts(engine.MemoryOptions{Files: files})
	return agent.NewSession(agent.SessionOptions{
		InitialConfig:                 config.Baseline(),
		Ports:                         ports.Ports,
		ApprovalRequiredForEverything: agent.ApprovalNotRequiredForEverything(),
	})
}

func call(session *agent.Session, name string, args map[string]any) agent.ToolResult {
	return agent.Dispatch(context.Background(), session, agent.ToolCall{Name: name, Arguments: args})
}

func mustSummary(t *testing.T, result agent.ToolResult) agent.ProposalSummary {
	t.Helper()
	if !result.OK {
		t.Fatalf("expected success, got %s: %s", result.Error.Code, result.Error.Message)
	}
	summary, ok := result.Data.(agent.ProposalSummary)
	if !ok {
		t.Fatalf("expected a proposal summary, got %T", result.Data)
	}
	return summary
}

func mustError(t *testing.T, result agent.ToolResult) *agent.ToolError {
	t.Helper()
	if result.OK {
		t.Fatalf("expected failure, got %+v", result.Data)
	}
	return result.Error
}

func setColumns(session *agent.Session, columns int) agent.ToolResult {
	return call(session, "propose_change", map[string]any{
		"intent": "change the column count",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/launcher/columns", "value": columns},
		},
	})
}

func TestToolDefinitionsMatchNames(t *testing.T) {
	definitions := agent.ToolDefinitions()
	if len(definitions) != len(agent.ToolNames) {
		t.Fatalf("expected %d definitions, got %d", len(agent.ToolNames), len(definitions))
	}
	seen := make(map[string]bool)
	for _, definition := range definitions {
		if seen[definition.Name] {
			t.Fatalf("duplicate tool %q", definition.Name)
		}
		seen[definition.Name] = true
		if !agent.IsToolName(definition.Name) {
			t.Fatalf("undeclared tool %q", definition.Name)
		}
		if definition.InputSchema["additionalProperties"] != false {
			t.Fatalf("tool %q accepts unknown arguments", definition.Name)
		}
		if len(definition.Description) < 20 {
			t.Fatalf("tool %q needs a real description", definition.Name)
		}
	}
}

func TestNoToolTouchesTheFilesystem(t *testing.T) {
	forbidden := []string{"file", "write", "read", "shell", "exec", "fetch", "http"}
	for _, name := range agent.ToolNames {
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("tool %q looks like raw device access", name)
			}
		}
	}
}

func TestNoToolCanApproveAProposal(t *testing.T) {
	for _, name := range agent.ToolNames {
		if strings.Contains(name, "approve") {
			t.Fatalf("approval must not be reachable from a tool, found %q", name)
		}
	}
}

func TestGuidanceStatesTheInvariant(t *testing.T) {
	if !strings.Contains(agent.SystemGuidance(), "no 'off' switch") {
		t.Fatal("system guidance must state the infinite-scroll invariant")
	}
}

func TestUnknownToolIsRejected(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "write_file", map[string]any{"path": "/etc/passwd"})
	if mustError(t, result).Code != "unknown-tool" {
		t.Fatalf("expected unknown-tool, got %+v", result.Error)
	}
}

func TestGetConfigSection(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "get_config", map[string]any{"section": "launcher"})
	if !result.OK {
		t.Fatalf("expected success, got %+v", result.Error)
	}
	launcher, ok := result.Data.(config.Launcher)
	if !ok || launcher.Columns != 1 {
		t.Fatalf("unexpected launcher %+v", result.Data)
	}

	unknown := call(session, "get_config", map[string]any{"section": "wallpaper"})
	if mustError(t, unknown).Code != "invalid-arguments" {
		t.Fatalf("expected invalid-arguments, got %+v", unknown.Error)
	}
}

func TestProposeDoesNotTouchTheDevice(t *testing.T) {
	session, ports := newSession(t)
	before := ports.FS.Snapshot()

	summary := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "make the home screen a two-column grid",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/launcher/layout", "value": "grid"},
			map[string]any{"op": "set", "path": "/spec/launcher/columns", "value": 2},
		},
	}))

	if summary.Status != "pending" || summary.ChangeCount != 2 {
		t.Fatalf("unexpected summary %+v", summary)
	}
	if !strings.Contains(summary.Diff, "/spec/launcher/columns") {
		t.Fatalf("diff should mention the changed path, got %q", summary.Diff)
	}
	if len(ports.FS.Snapshot()) != len(before) {
		t.Fatal("proposing changed the filesystem")
	}
	for path, contents := range before {
		if ports.FS.Snapshot()[path] != contents {
			t.Fatalf("proposing modified %s", path)
		}
	}
}

func TestAgentCannotWriteOutsideWritableRoots(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "bump the api version",
		"operations": []any{
			map[string]any{"op": "set", "path": "/apiVersion", "value": "normal.os/v1"},
		},
	})
	toolErr := mustError(t, result)
	if toolErr.Code != "proposal-rejected" || !strings.Contains(toolErr.Message, "managed by the mutation engine") {
		t.Fatalf("unexpected error %+v", toolErr)
	}
}

func TestAgentCannotSetItsOwnRevision(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "skip ahead",
		"operations": []any{
			map[string]any{"op": "set", "path": "/metadata/revision", "value": 99},
		},
	})
	if mustError(t, result).Code != "proposal-rejected" {
		t.Fatalf("expected rejection, got %+v", result.Error)
	}
}

func TestInvalidResultIsRejectedBeforePlanning(t *testing.T) {
	session, _ := newSession(t)
	result := setColumns(session, 20)
	if !strings.Contains(mustError(t, result).Message, "would not be a valid config") {
		t.Fatalf("unexpected error %+v", result.Error)
	}
}

func TestMalformedOperationsAreRejected(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "nonsense",
		"operations": []any{
			map[string]any{"op": "delete", "path": "/spec/launcher"},
		},
	})
	if mustError(t, result).Code != "invalid-arguments" {
		t.Fatalf("expected invalid-arguments, got %+v", result.Error)
	}
}

func TestEmptyOperationListIsRejected(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "do nothing", "operations": []any{},
	})
	if mustError(t, result).Code != "proposal-rejected" {
		t.Fatalf("expected rejection, got %+v", result.Error)
	}
}

func TestScrollEnforcementCannotBeSwitchedOff(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "turn off the scroll blocker, it is annoying",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/attention/infiniteScroll/enforcement", "value": "off"},
		},
	})
	if !strings.Contains(mustError(t, result).Message, "would not be a valid config") {
		t.Fatalf("unexpected error %+v", result.Error)
	}
}

func TestScrollEnforcementSurvivesEmptiedDetectors(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "remove all detectors",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/attention/infiniteScroll/detectors", "value": []any{}},
		},
	})
	if !strings.Contains(mustError(t, result).Message, "at least one detector") {
		t.Fatalf("unexpected error %+v", result.Error)
	}
}

func TestWebviewShimCannotBeDisabledByAgent(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_change", map[string]any{
		"intent": "stop injecting the shim",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/attention/infiniteScroll/webview/injectShim", "value": false},
		},
	})
	if !strings.Contains(mustError(t, result).Message, "cannot be disabled") {
		t.Fatalf("unexpected error %+v", result.Error)
	}
}

func TestBoundedExemptionAlwaysNeedsApproval(t *testing.T) {
	session := newPermissiveSession(t)
	summary := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "let the transit board in Maps scroll freely for a week",
		"operations": []any{
			map[string]any{
				"op":   "set",
				"path": "/spec/attention/infiniteScroll/exemptions/maps-transit",
				"value": map[string]any{
					"id":        "maps-transit",
					"package":   "com.google.android.apps.maps",
					"reason":    "transit departure board is a continuous list by nature",
					"expiresAt": "2026-01-08T00:00:00Z",
				},
			},
		},
	}))

	if !summary.RequiresApproval {
		t.Fatal("adding an exemption must require approval")
	}
	found := false
	for _, issue := range summary.Review {
		if issue.Code == "weakens-attention-policy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a weakening to be flagged, got %+v", summary.Review)
	}
}

func TestStrengtheningIsNotCalledAWeakening(t *testing.T) {
	session := newPermissiveSession(t)
	summary := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "be stricter about feeds",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/attention/infiniteScroll/enforcement", "value": "block"},
		},
	}))

	codes := make([]string, 0)
	for _, issue := range summary.Review {
		codes = append(codes, issue.Code)
	}
	joined := strings.Join(codes, ",")
	if strings.Contains(joined, "weakens-attention-policy") {
		t.Fatalf("strengthening should not be flagged as weakening, got %v", codes)
	}
	if !strings.Contains(joined, "sensitive-path") || !summary.RequiresApproval {
		t.Fatalf("attention changes must still need approval, got %v", codes)
	}
}

func TestApplyRefusesWithoutApproval(t *testing.T) {
	session, ports := newSession(t)
	before := ports.FS.Snapshot()
	summary := mustSummary(t, setColumns(session, 2))

	result := call(session, "apply_proposal", map[string]any{"proposalId": summary.ProposalID})
	if mustError(t, result).Code != "approval-required" {
		t.Fatalf("expected approval-required, got %+v", result.Error)
	}
	if session.Current().Spec.Launcher.Columns != 1 {
		t.Fatal("config changed without approval")
	}
	for path, contents := range before {
		if ports.FS.Snapshot()[path] != contents {
			t.Fatalf("device changed without approval at %s", path)
		}
	}
}

func TestApplyAfterOutOfBandApproval(t *testing.T) {
	session, ports := newSession(t)
	summary := mustSummary(t, setColumns(session, 2))

	if _, rejection := session.Approve(summary.ProposalID, "user"); rejection != nil {
		t.Fatalf("approve: %v", rejection)
	}
	result := call(session, "apply_proposal", map[string]any{"proposalId": summary.ProposalID})
	if !result.OK {
		t.Fatalf("expected success, got %+v", result.Error)
	}
	applied, ok := result.Data.(agent.ApplySummary)
	if !ok || applied.Revision != 1 {
		t.Fatalf("unexpected apply summary %+v", result.Data)
	}
	if session.Current().Spec.Launcher.Columns != 2 {
		t.Fatal("session config was not advanced")
	}
	if !strings.Contains(ports.FS.Snapshot()[engine.FileLauncher], `"columns": 2`) {
		t.Fatal("device file was not written")
	}
}

func TestApplyIsNotRepeatable(t *testing.T) {
	session, _ := newSession(t)
	summary := mustSummary(t, setColumns(session, 2))
	session.Approve(summary.ProposalID, "user")
	call(session, "apply_proposal", map[string]any{"proposalId": summary.ProposalID})

	second := call(session, "apply_proposal", map[string]any{"proposalId": summary.ProposalID})
	if mustError(t, second).Code != "not-applicable" {
		t.Fatalf("expected not-applicable, got %+v", second.Error)
	}
}

func TestStaleProposalIsRejectedAtApplyTime(t *testing.T) {
	session, _ := newSession(t)

	stale := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "put spotify on wifi only",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/apps/entries/com.spotify.music/network", "value": "wifi-only"},
		},
	}))

	removal := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "uninstall spotify",
		"operations": []any{
			map[string]any{"op": "remove", "path": "/spec/apps/entries/com.spotify.music"},
			map[string]any{"op": "remove", "path": "/spec/launcher/pages/home/items/home-spotify"},
		},
	}))
	session.Approve(removal.ProposalID, "user")
	if applied := call(session, "apply_proposal", map[string]any{"proposalId": removal.ProposalID}); !applied.OK {
		t.Fatalf("removal should apply, got %+v", applied.Error)
	}

	session.Approve(stale.ProposalID, "user")
	result := call(session, "apply_proposal", map[string]any{"proposalId": stale.ProposalID})
	if mustError(t, result).Code != "stale" {
		t.Fatalf("expected stale, got %+v", result.Error)
	}
}

func TestFailedApplyRollsBackAndKeepsRevision(t *testing.T) {
	session, ports := newSession(t, engine.Fault{
		Kind:   engine.FaultWrite,
		Target: engine.FileLauncher,
		Error:  &engine.IOError{Code: engine.ErrIOFailure, Target: engine.FileLauncher, Message: "disk full"},
		Times:  1,
	})
	before := ports.FS.Snapshot()

	summary := mustSummary(t, setColumns(session, 2))
	session.Approve(summary.ProposalID, "user")
	result := call(session, "apply_proposal", map[string]any{"proposalId": summary.ProposalID})

	toolErr := mustError(t, result)
	if toolErr.Code != "apply-failed" || !strings.Contains(toolErr.Message, "rolled back") {
		t.Fatalf("unexpected error %+v", toolErr)
	}
	if session.Current().Metadata.Revision != 0 || len(session.Revisions()) != 1 {
		t.Fatal("a failed apply must not advance the revision")
	}
	for path, contents := range before {
		if ports.FS.Snapshot()[path] != contents {
			t.Fatalf("rollback left %s modified", path)
		}
	}
}

func TestRollbackGoesThroughTheSameEnginePath(t *testing.T) {
	session, ports := newSession(t)

	change := mustSummary(t, call(session, "propose_change", map[string]any{
		"intent": "silence everything by default",
		"operations": []any{
			map[string]any{"op": "set", "path": "/spec/notifications/defaultDisposition", "value": "block"},
		},
	}))
	session.Approve(change.ProposalID, "user")
	if applied := call(session, "apply_proposal", map[string]any{"proposalId": change.ProposalID}); !applied.OK {
		t.Fatalf("apply: %+v", applied.Error)
	}
	if session.Current().Spec.Notifications.DefaultDisposition != "block" {
		t.Fatal("first change did not take effect")
	}

	rollback := mustSummary(t, call(session, "propose_rollback", map[string]any{"revision": 0}))
	if !rollback.RequiresApproval {
		t.Fatal("rollback must require approval")
	}
	session.Approve(rollback.ProposalID, "user")
	if applied := call(session, "apply_proposal", map[string]any{"proposalId": rollback.ProposalID}); !applied.OK {
		t.Fatalf("rollback apply: %+v", applied.Error)
	}

	if session.Current().Spec.Notifications.DefaultDisposition != "bundle" {
		t.Fatal("rollback did not restore the earlier value")
	}
	if session.Current().Metadata.Revision != 2 {
		t.Fatalf("rollback should move forward to revision 2, got %d",
			session.Current().Metadata.Revision)
	}
	if !strings.Contains(ports.FS.Snapshot()[engine.FileNotifications], `"defaultDisposition": "bundle"`) {
		t.Fatal("rollback did not reach the device")
	}
	if len(session.Revisions()) != 3 {
		t.Fatalf("expected three revisions, got %d", len(session.Revisions()))
	}
}

func TestRollbackToUnknownRevisionIsRejected(t *testing.T) {
	session, _ := newSession(t)
	result := call(session, "propose_rollback", map[string]any{"revision": 7})
	if !strings.Contains(mustError(t, result).Message, "no applied revision 7") {
		t.Fatalf("unexpected error %+v", result.Error)
	}
}

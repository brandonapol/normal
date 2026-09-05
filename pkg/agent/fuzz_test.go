package agent_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/brandonapol/normal/pkg/agent"
	"github.com/brandonapol/normal/pkg/config"
)

func FuzzApplyPatch(f *testing.F) {
	for _, seed := range []string{
		`[{"op":"set","path":"/spec/launcher/columns","value":3}]`,
		`[{"op":"remove","path":"/spec/apps/entries/com.spotify.music"}]`,
		`[{"op":"set","path":"/apiVersion","value":"x"}]`,
		`[{"op":"set","path":"/spec/attention/infiniteScroll/enforcement","value":"off"}]`,
		`[{"op":"set","path":"/spec/apps/entries/","value":{}}]`,
		`[{"op":"bogus","path":"/spec"}]`,
		`[]`,
		`null`,
		`[{"op":"set","path":"","value":null}]`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, operationsJSON string) {
		var operations []agent.Operation
		if err := json.Unmarshal([]byte(operationsJSON), &operations); err != nil {
			return
		}

		document, err := config.ToDocument(config.Baseline())
		if err != nil {
			t.Fatalf("ToDocument: %v", err)
		}
		before, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		patched, issues := agent.ApplyPatch(document, operations)

		after, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("ApplyPatch mutated its input document")
		}

		if len(issues) > 0 {
			for _, issue := range issues {
				if issue.Code == "" {
					t.Fatalf("patch issue with empty code: %+v", issue)
				}
			}
			return
		}
		if patched == nil && len(operations) > 0 {
			t.Fatal("ApplyPatch reported success but returned no document")
		}
	})
}

func FuzzProposeOperations(f *testing.F) {
	for _, seed := range []string{
		`[{"op":"set","path":"/spec/launcher/columns","value":2}]`,
		`[{"op":"set","path":"/metadata/revision","value":99}]`,
		`[{"op":"set","path":"/spec/attention/infiniteScroll/detectors","value":[]}]`,
		`[{"op":"remove","path":"/spec/attention"}]`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, operationsJSON string) {
		var operations []agent.Operation
		if err := json.Unmarshal([]byte(operationsJSON), &operations); err != nil {
			return
		}

		session := newPermissiveSession(t)
		proposal, rejection := session.Propose("fuzz", operations)
		if rejection != nil {
			if rejection.Error() == "" {
				t.Fatal("rejection with no explanation")
			}
			return
		}

		if proposal.Evaluation.Desired.Spec.Attention.InfiniteScroll.Enforcement == "" {
			t.Fatal("accepted a proposal with no scroll enforcement")
		}
		if !proposal.Evaluation.Desired.Spec.Attention.InfiniteScroll.Webview.InjectShim {
			t.Fatal("accepted a proposal that disabled the webview shim")
		}
		if len(proposal.Evaluation.Desired.Spec.Attention.InfiniteScroll.Detectors) == 0 {
			t.Fatal("accepted a proposal with no scroll detectors")
		}
		if proposal.Evaluation.Desired.Metadata.Revision <= config.Baseline().Metadata.Revision {
			t.Fatal("accepted a proposal that did not advance the revision")
		}
	})
}

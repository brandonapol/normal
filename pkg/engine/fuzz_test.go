package engine_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
)

func FuzzDiffDocuments(f *testing.F) {
	baseline, err := json.Marshal(config.Baseline())
	if err != nil {
		f.Fatalf("marshal baseline: %v", err)
	}
	f.Add(string(baseline), string(baseline))
	f.Add("{}", "{}")
	f.Add(`{"a":1}`, `{"a":2}`)
	f.Add(`{"a":[{"id":"x"}]}`, `{"a":[{"id":"y"}]}`)
	f.Add(`{"spec":{"apps":{"entries":[]}}}`, `{"spec":{"apps":{"entries":[{"package":"a.b"}]}}}`)
	f.Add("null", "[]")

	f.Fuzz(func(t *testing.T, beforeJSON, afterJSON string) {
		var before, after any
		if err := json.Unmarshal([]byte(beforeJSON), &before); err != nil {
			return
		}
		if err := json.Unmarshal([]byte(afterJSON), &after); err != nil {
			return
		}

		diff := engine.DiffDocuments(before, after)
		equal := reflect.DeepEqual(before, after)

		if equal && !diff.IsEmpty() {
			t.Fatalf("identical documents produced %d changes: %v", len(diff.Changes), diff.Changes)
		}
		if !equal && diff.IsEmpty() {
			t.Fatal("differing documents produced an empty diff")
		}

		if !reflect.DeepEqual(diff, engine.DiffDocuments(before, after)) {
			t.Fatal("diff is not deterministic")
		}

		for _, change := range diff.Changes {
			if change.Path == "" && len(diff.Changes) > 1 {
				t.Fatalf("root-level change reported alongside %d others", len(diff.Changes)-1)
			}
			if _, err := config.ParsePointer(change.Path); err != nil {
				t.Fatalf("diff produced an unparseable path %q: %v", change.Path, err)
			}
		}
	})
}

package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brandonapol/normal/pkg/config"
)

const invariantDir = "../../testdata/invariants"

type rejectCase struct {
	File string `json:"file"`
	Code string `json:"code"`
	Why  string `json:"why"`
}

type acceptCase struct {
	File string `json:"file"`
	Why  string `json:"why"`
}

type invariantManifest struct {
	Now    string       `json:"now"`
	Reject []rejectCase `json:"reject"`
	Accept []acceptCase `json:"accept"`
}

func loadManifest(t *testing.T) (invariantManifest, time.Time) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(invariantDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest invariantManifest
	if decodeErr := json.Unmarshal(raw, &manifest); decodeErr != nil {
		t.Fatalf("parse manifest: %v", decodeErr)
	}
	at, err := time.Parse(time.RFC3339, manifest.Now)
	if err != nil {
		t.Fatalf("parse manifest now: %v", err)
	}
	return manifest, at
}

func loadFixture(t *testing.T, file string) any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(invariantDir, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	document, err := config.ParseDocument(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return document
}

func TestInvariantCorpusRejects(t *testing.T) {
	manifest, at := loadManifest(t)
	if len(manifest.Reject) == 0 {
		t.Fatal("the reject corpus is empty; the invariants are no longer guarded")
	}

	for _, testCase := range manifest.Reject {
		t.Run(testCase.File, func(t *testing.T) {
			issues := config.Validate(loadFixture(t, testCase.File), at)
			if len(issues) == 0 {
				t.Fatalf("%s was accepted, but %s", testCase.File, testCase.Why)
			}
			for _, issue := range issues {
				if issue.Code == testCase.Code {
					return
				}
			}
			t.Fatalf("expected code %q (%s), got %v", testCase.Code, testCase.Why, issues)
		})
	}
}

func TestInvariantCorpusAccepts(t *testing.T) {
	manifest, at := loadManifest(t)
	if len(manifest.Accept) == 0 {
		t.Fatal("the accept corpus is empty; the schema is no longer proven usable")
	}

	for _, testCase := range manifest.Accept {
		t.Run(testCase.File, func(t *testing.T) {
			if issues := config.Validate(loadFixture(t, testCase.File), at); len(issues) > 0 {
				t.Fatalf("%s was rejected, but %s: %v", testCase.File, testCase.Why, issues)
			}
		})
	}
}

func TestEveryFixtureIsListedInTheManifest(t *testing.T) {
	manifest, _ := loadManifest(t)

	listed := make(map[string]bool)
	for _, testCase := range manifest.Reject {
		listed[testCase.File] = true
	}
	for _, testCase := range manifest.Accept {
		listed[testCase.File] = true
	}

	for _, dir := range []string{"reject", "accept"} {
		entries, err := os.ReadDir(filepath.Join(invariantDir, dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := dir + "/" + entry.Name()
			if !listed[name] {
				t.Errorf("%s exists but is not listed in manifest.json, so CI would skip it", name)
			}
		}
	}
}

func TestAttentionInvariantsAreCovered(t *testing.T) {
	manifest, _ := loadManifest(t)

	required := map[string]bool{
		"invalid-enforcement": false,
		"empty-detectors":     false,
		"policy-violation":    false,
		"reason-too-short":    false,
		"expired":             false,
		"too-distant":         false,
		"too-many":            false,
	}
	for _, testCase := range manifest.Reject {
		if _, wanted := required[testCase.Code]; wanted {
			required[testCase.Code] = true
		}
	}
	for code, covered := range required {
		if !covered {
			t.Errorf("no fixture guards %q; that invariant could be removed unnoticed", code)
		}
	}
}

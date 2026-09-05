package config_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
)

const examplePath = "../../examples/baseline.config.json"

func TestExampleMatchesBaseline(t *testing.T) {
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}

	var onDisk, inCode any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal example: %v", err)
	}
	encoded, err := json.Marshal(config.Baseline())
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	if err := json.Unmarshal(encoded, &inCode); err != nil {
		t.Fatalf("unmarshal baseline: %v", err)
	}

	if !reflect.DeepEqual(onDisk, inCode) {
		t.Fatal("examples/baseline.config.json is stale; regenerate with: go run ./cmd/normalctl baseline > examples/baseline.config.json")
	}
}

func TestExampleValidates(t *testing.T) {
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	parsed, err := config.ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	if issues := config.ValidateConfig(parsed, now); len(issues) > 0 {
		t.Fatalf("example should validate, got %v", issues)
	}
}

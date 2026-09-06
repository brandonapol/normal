package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
	"github.com/brandonapol/normal/schema"
)

const usage = `normalctl - work with Normal phone configs

usage:
  normalctl validate [--now RFC3339] <config.json>
                                            validate a config against the schema
  normalctl render <config.json>            print the files a config renders to
  normalctl diff <current.json> <desired.json>
  normalctl plan <current.json> <desired.json>
  normalctl verify [dir]                    check the audit chain and config for drift
  normalctl audit verify [dir]              check the audit chain (default /etc/normal)
  normalctl audit log [dir]                 render transaction history
  normalctl baseline                        print the baseline config
  normalctl seal <key.seed>                 print a signed, sealed baseline
  normalctl schema                          print the CUE schema
`

var errInvalidConfig = errors.New("configuration is not valid")

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2:]); err != nil {
		if !errors.Is(err, errInvalidConfig) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}

func run(command string, args []string) error {
	switch command {
	case "validate":
		return validate(args)
	case "render":
		return render(args)
	case "diff":
		return diff(args)
	case "plan":
		return plan(args)
	case "verify":
		return verifyCommand(args)
	case "audit":
		return auditCommand(args)
	case "baseline":
		return emit(config.Baseline())
	case "seal":
		return sealCommand(args)
	case "schema":
		_, err := os.Stdout.Write(schema.Source)
		return err
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q", command)
}

func load(path string) (document any, parsed config.Config, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, config.Config{}, err
	}
	document, err = config.ParseDocument(raw)
	if err != nil {
		return nil, config.Config{}, err
	}
	parsed, err = config.ParseConfig(raw)
	if err != nil {
		return nil, config.Config{}, err
	}
	return document, parsed, nil
}

func loadValid(path string) (config.Config, error) {
	document, parsed, err := load(path)
	if err != nil {
		return config.Config{}, err
	}
	if issues := config.Validate(document, time.Now().UTC()); len(issues) > 0 {
		return config.Config{}, fmt.Errorf("%s is not valid:\n%s", path, formatIssues(issues))
	}
	return parsed, nil
}

func parseNow(value string) (time.Time, error) {
	if value == "" {
		return time.Now().UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--now must be an RFC-3339 timestamp: %w", err)
	}
	return parsed.UTC(), nil
}

func formatIssues(issues []config.Issue) string {
	out := ""
	for _, issue := range issues {
		out += fmt.Sprintf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Message)
	}
	return out
}

func emit(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func validate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	nowFlag := flags.String("now", "", "evaluate time-dependent rules at this RFC-3339 instant")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("validate takes one config file")
	}

	now, err := parseNow(*nowFlag)
	if err != nil {
		return err
	}
	path := flags.Arg(0)
	document, parsed, err := load(path)
	if err != nil {
		return err
	}

	issues := config.Validate(document, now)
	if len(issues) == 0 {
		fmt.Printf("%s is valid (revision %d)\n", path, parsed.Metadata.Revision)
		return nil
	}
	fmt.Printf("%s has %d issue(s):\n%s", path, len(issues), formatIssues(issues))
	return errInvalidConfig
}

func render(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("render takes one config file")
	}
	loaded, err := loadValid(args[0])
	if err != nil {
		return err
	}
	files, err := engine.Render(loaded)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Printf("--- %s\n%s", path, files[path])
	}
	return nil
}

func loadPair(args []string, command string) (current, desired config.Config, err error) {
	if len(args) != 2 {
		return config.Config{}, config.Config{}, fmt.Errorf("%s takes a current and a desired config", command)
	}
	current, err = loadValid(args[0])
	if err != nil {
		return config.Config{}, config.Config{}, err
	}
	desired, err = loadValid(args[1])
	if err != nil {
		return config.Config{}, config.Config{}, err
	}
	return current, desired, nil
}

func diff(args []string) error {
	current, desired, err := loadPair(args, "diff")
	if err != nil {
		return err
	}
	computed, err := engine.DiffConfigs(current, desired)
	if err != nil {
		return err
	}
	fmt.Println(engine.FormatDiff(computed))
	return nil
}

func plan(args []string) error {
	current, desired, err := loadPair(args, "plan")
	if err != nil {
		return err
	}
	computed, err := engine.PlanApply(current, desired)
	if err != nil {
		return err
	}
	fmt.Println(engine.FormatPlan(computed))

	ports := engine.NewMemoryPorts(engine.MemoryOptions{})
	if _, err := engine.ApplyPlan(context.Background(), computed, ports.Ports); err != nil {
		return fmt.Errorf("dry run failed: %w", err)
	}
	fmt.Printf("\ndry run against an in-memory device: ok (%d files, %d services)\n",
		len(ports.FS.Snapshot()), len(computed.Services))
	return nil
}

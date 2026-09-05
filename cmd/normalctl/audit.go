package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brandonapol/normal/pkg/audit"
)

type readOnlyFS struct{}

func (readOnlyFS) Read(_ context.Context, path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (readOnlyFS) Exists(_ context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (readOnlyFS) Write(context.Context, string, string) error {
	return fmt.Errorf("normalctl does not write the audit log; the mutation engine does")
}

func (readOnlyFS) Remove(context.Context, string) error {
	return fmt.Errorf("normalctl does not remove audit entries")
}

func auditCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("audit takes a subcommand: verify or log")
	}

	root := "/etc/normal"
	if len(args) > 1 {
		root = args[1]
	}
	store := audit.NewStore(readOnlyFS{}, filepath.Clean(root))
	ctx := context.Background()

	switch args[0] {
	case "verify":
		report := store.VerifyLog(ctx)
		fmt.Println(audit.RenderReport(report))
		if !report.Valid() {
			return errInvalidConfig
		}
		return nil

	case "log":
		entries, _, pending, err := store.Load(ctx)
		if err != nil {
			return err
		}
		fmt.Println(audit.RenderLog(entries, pending))
		return nil
	}

	return fmt.Errorf("unknown audit subcommand %q; expected verify or log", args[0])
}

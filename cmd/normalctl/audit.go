package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/engine"
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

func loadPublicKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, "audit.pub")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s is not base64: %w", path, err)
	}
	return decoded, nil
}

func renderedOnDisk(dir string) (engine.FileSet, int, error) {
	files := engine.FileSet{}
	found := 0
	for _, canonical := range []string{
		engine.FileMetadata, engine.FileLauncher, engine.FileApps,
		engine.FileNotifications, engine.FileAttention, engine.FileWebviewShim,
	} {
		relative := strings.TrimPrefix(canonical, engine.ConfigRoot+"/")
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relative)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, found, err
		}
		files[canonical] = string(raw)
		found++
	}
	return files, found, nil
}

func verifyCommand(args []string) error {
	dir := "/etc/normal"
	if len(args) > 0 {
		dir = filepath.Clean(args[0])
	}
	ctx := context.Background()
	store := audit.NewStore(readOnlyFS{}, dir)

	publicKey, err := loadPublicKey(dir)
	if err != nil {
		return err
	}

	report := store.VerifyLogWith(ctx, audit.Options{PublicKey: publicKey})
	entries, _, _, loadErr := store.Load(ctx)
	if loadErr == nil {
		files, found, readErr := renderedOnDisk(dir)
		if readErr != nil {
			return readErr
		}
		if found > 0 {
			report = audit.CheckConfigDrift(report, entries, engine.Digest(files))
		}
	}

	fmt.Println(audit.RenderReport(report))
	if len(publicKey) == 0 {
		fmt.Println("\nNo audit.pub in this directory, so signatures were not checked.")
	}
	if !report.Valid() {
		return errInvalidConfig
	}
	return nil
}

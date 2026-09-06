package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/baseline"
	"github.com/brandonapol/normal/pkg/engine"
	"github.com/brandonapol/normal/pkg/recovery"
)

type rootedFS struct{ base string }

func (r rootedFS) resolve(path string) string {
	if r.base == engine.ConfigRoot {
		return path
	}
	relative, rooted := strings.CutPrefix(path, engine.ConfigRoot+"/")
	if !rooted {
		return path
	}
	return filepath.Join(r.base, filepath.FromSlash(relative))
}

func (r rootedFS) Read(ctx context.Context, path string) (string, error) {
	return readOnlyFS{}.Read(ctx, r.resolve(path))
}

func (r rootedFS) Exists(ctx context.Context, path string) (bool, error) {
	return readOnlyFS{}.Exists(ctx, r.resolve(path))
}

func (r rootedFS) Write(_ context.Context, path, contents string) error {
	target := r.resolve(path)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(contents), 0o600)
}

func (r rootedFS) Remove(_ context.Context, path string) error {
	err := os.Remove(r.resolve(path))
	if os.IsNotExist(err) {
		return &engine.IOError{Code: engine.ErrNotFound, Target: path, Message: "no such file"}
	}
	return err
}

type inertServices struct{}

func (inertServices) Restart(context.Context, string) error { return nil }

func (inertServices) Status(context.Context, string) (engine.ServiceState, error) {
	return engine.ServiceRunning, nil
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now().UTC() }
func (wallClock) NextID() string { return fmt.Sprintf("recover-%d", time.Now().UTC().Unix()) }

func recoverCommand(args []string) error {
	flags := flag.NewFlagSet("recover", flag.ContinueOnError)
	apply := flags.Bool("apply", false, "perform the recovery instead of reporting what it would do")
	if err := flags.Parse(args); err != nil {
		return err
	}

	dir := "/etc/normal"
	if flags.NArg() > 0 {
		dir = filepath.Clean(flags.Arg(0))
	}
	ctx := context.Background()

	fs := rootedFS{base: dir}
	store := audit.NewStore(fs, dir)
	ports := engine.Ports{FS: fs, Services: inertServices{}, Clock: wallClock{}}

	publicKey, err := loadPublicKey(dir)
	if err != nil {
		return err
	}
	options := recovery.Options{PublicKey: publicKey, Now: time.Now().UTC(), DryRun: !*apply}

	sealed, found, sealErr := baseline.Read(ctx, fs, dir)
	if sealErr == nil && found {
		options.Sealed = &sealed
	}

	result, err := recovery.Recover(ctx, ports, store, options)
	if err != nil {
		return err
	}

	fmt.Println(result.String())
	for _, action := range result.Actions {
		fmt.Println("  would " + action)
	}
	for _, path := range result.Restored {
		if *apply {
			fmt.Println("  " + path)
		}
	}
	if !*apply && result.Outcome != recovery.OutcomeNothingToDo {
		fmt.Println("\nThis was a dry run. Re-run with --apply to carry it out.")
	}
	if result.NeedsAttention() {
		return errInvalidConfig
	}
	return nil
}

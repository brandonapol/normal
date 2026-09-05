package audit

import (
	"context"
	"encoding/json"
	"fmt"
)

type FileSystem interface {
	Read(ctx context.Context, path string) (string, error)
	Write(ctx context.Context, path, contents string) error
	Remove(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
}

type Store struct {
	FS          FileSystem
	LogPath     string
	PendingPath string
}

func NewStore(fs FileSystem, root string) Store {
	return Store{
		FS:          fs,
		LogPath:     root + "/audit.log",
		PendingPath: root + "/audit.pending",
	}
}

func (s Store) Load(ctx context.Context) ([]Entry, DecodeReport, *Pending, error) {
	var (
		entries []Entry
		report  DecodeReport
	)

	exists, err := s.FS.Exists(ctx, s.LogPath)
	if err != nil {
		return nil, report, nil, fmt.Errorf("checking audit log: %w", err)
	}
	if exists {
		raw, readErr := s.FS.Read(ctx, s.LogPath)
		if readErr != nil {
			return nil, report, nil, fmt.Errorf("reading audit log: %w", readErr)
		}
		entries, report, err = Decode([]byte(raw))
		if err != nil {
			return entries, report, nil, err
		}
	}

	pending, err := s.loadPending(ctx)
	if err != nil {
		return entries, report, nil, err
	}
	return entries, report, pending, nil
}

func (s Store) loadPending(ctx context.Context) (*Pending, error) {
	exists, err := s.FS.Exists(ctx, s.PendingPath)
	if err != nil || !exists {
		return nil, err
	}
	raw, err := s.FS.Read(ctx, s.PendingPath)
	if err != nil {
		return nil, fmt.Errorf("reading pending marker: %w", err)
	}
	var pending Pending
	if err := json.Unmarshal([]byte(raw), &pending); err != nil {
		return nil, fmt.Errorf("pending marker is unreadable: %w", err)
	}
	return &pending, nil
}

func (s Store) Begin(ctx context.Context, pending Pending) error {
	raw, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encoding pending marker: %w", err)
	}
	return s.FS.Write(ctx, s.PendingPath, string(raw))
}

func (s Store) Commit(ctx context.Context, entry Entry) (Entry, error) {
	existing, _, _, err := s.Load(ctx)
	if err != nil {
		return Entry{}, err
	}

	linked := Link(Head(existing), entry)
	line, err := EncodeEntry(linked)
	if err != nil {
		return Entry{}, err
	}

	previous := ""
	if exists, existsErr := s.FS.Exists(ctx, s.LogPath); existsErr != nil {
		return Entry{}, existsErr
	} else if exists {
		if previous, err = s.FS.Read(ctx, s.LogPath); err != nil {
			return Entry{}, err
		}
	}

	if err := s.FS.Write(ctx, s.LogPath, previous+string(line)); err != nil {
		return Entry{}, fmt.Errorf("appending audit entry: %w", err)
	}
	s.clearPending(ctx)
	return linked, nil
}

func (s Store) clearPending(ctx context.Context) {
	_ = s.FS.Remove(ctx, s.PendingPath)
}

func (s Store) VerifyLog(ctx context.Context) Report {
	entries, decode, pending, loadErr := s.Load(ctx)
	if loadErr != nil {
		return Report{
			Entries: len(entries),
			Problems: []Problem{{
				Sequence: len(entries),
				Kind:     ProblemTruncated,
				Message:  loadErr.Error(),
			}},
			Incomplete: true,
		}
	}
	return Verify(entries, decode, pending)
}

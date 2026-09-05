package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

type Outcome string

const (
	OutcomeApplied    Outcome = "applied"
	OutcomeRolledBack Outcome = "rolled-back"
	OutcomeDirty      Outcome = "dirty"
)

type Entry struct {
	Sequence      int       `json:"sequence"`
	PreviousHash  string    `json:"previousHash"`
	TransactionID string    `json:"transactionId"`
	Intent        string    `json:"intent"`
	ApprovedBy    string    `json:"approvedBy,omitempty"`
	FromRevision  int       `json:"fromRevision"`
	ToRevision    int       `json:"toRevision"`
	ConfigBefore  string    `json:"configBefore"`
	ConfigAfter   string    `json:"configAfter"`
	Files         []string  `json:"files"`
	Services      []string  `json:"services"`
	Outcome       Outcome   `json:"outcome"`
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	Hash          string    `json:"hash"`
}

type Pending struct {
	TransactionID string    `json:"transactionId"`
	Intent        string    `json:"intent"`
	FromRevision  int       `json:"fromRevision"`
	ToRevision    int       `json:"toRevision"`
	ConfigBefore  string    `json:"configBefore"`
	Files         []string  `json:"files"`
	Services      []string  `json:"services"`
	StartedAt     time.Time `json:"startedAt"`
}

func ComputeHash(entry Entry) string {
	entry.Hash = ""
	raw, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func Link(previous *Entry, entry Entry) Entry {
	if previous == nil {
		entry.Sequence = 0
		entry.PreviousHash = GenesisHash
	} else {
		entry.Sequence = previous.Sequence + 1
		entry.PreviousHash = previous.Hash
	}
	entry.Hash = ComputeHash(entry)
	return entry
}

func Encode(entries []Entry) ([]byte, error) {
	var out strings.Builder
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("encode entry %d: %w", entry.Sequence, err)
		}
		out.Write(raw)
		out.WriteByte('\n')
	}
	return []byte(out.String()), nil
}

func EncodeEntry(entry Entry) ([]byte, error) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

type DecodeReport struct {
	Truncated bool
	Malformed int
}

func Decode(data []byte) ([]Entry, DecodeReport, error) {
	text := string(data)
	if text == "" {
		return nil, DecodeReport{}, nil
	}

	lines := strings.Split(text, "\n")
	report := DecodeReport{}

	trailing := lines[len(lines)-1]
	lines = lines[:len(lines)-1]
	if strings.TrimSpace(trailing) != "" {
		report.Truncated = true
	}

	entries := make([]Entry, 0, len(lines))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			report.Malformed++
			return entries, report, fmt.Errorf("line %d is not a valid entry: %w", index+1, err)
		}
		entries = append(entries, entry)
	}
	return entries, report, nil
}

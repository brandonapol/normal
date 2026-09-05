package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/brandonapol/normal/pkg/config"
)

type ActionKind string

const (
	ActionWriteFile      ActionKind = "write-file"
	ActionDeleteFile     ActionKind = "delete-file"
	ActionRestartService ActionKind = "restart-service"
)

type Action struct {
	Kind     ActionKind `json:"kind"`
	Path     string     `json:"path,omitempty"`
	Contents string     `json:"contents,omitempty"`
	Service  string     `json:"service,omitempty"`
}

type HealthCheck struct {
	Service string `json:"service"`
}

type Plan struct {
	FromRevision int           `json:"fromRevision"`
	ToRevision   int           `json:"toRevision"`
	Intent       string        `json:"intent,omitempty"`
	ApprovedBy   string        `json:"approvedBy,omitempty"`
	DigestBefore string        `json:"digestBefore,omitempty"`
	DigestAfter  string        `json:"digestAfter,omitempty"`
	Diff         Diff          `json:"diff"`
	Actions      []Action      `json:"actions"`
	Services     []string      `json:"services"`
	Checks       []HealthCheck `json:"checks"`
}

func (p Plan) IsNoOp() bool { return len(p.Actions) == 0 }

type PlanIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PlanError struct {
	Issues []PlanIssue
}

func (e *PlanError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	return "plan rejected: " + strings.Join(parts, "; ")
}

func (e *PlanError) Codes() []string {
	codes := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

var immutablePaths = []string{"/apiVersion", "/kind"}

func diffFileSets(before, after FileSet) []Action {
	paths := make(map[string]bool, len(before)+len(after))
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	writes := make([]Action, 0)
	deletes := make([]Action, 0)
	for _, path := range ordered {
		next, present := after[path]
		if !present {
			deletes = append(deletes, Action{Kind: ActionDeleteFile, Path: path})
			continue
		}
		if before[path] != next {
			writes = append(writes, Action{Kind: ActionWriteFile, Path: path, Contents: next})
		}
	}
	return append(writes, deletes...)
}

func planIssues(changes []Change) []PlanIssue {
	issues := make([]PlanIssue, 0)
	for _, change := range changes {
		immutable := false
		for _, path := range immutablePaths {
			if covers(path, change.Path) {
				immutable = true
				break
			}
		}
		if immutable {
			issues = append(issues, PlanIssue{
				Path:    change.Path,
				Code:    "immutable-field",
				Message: "apiVersion and kind cannot be changed by a mutation; migrate the config instead",
			})
			continue
		}
		if !IsOwnedPath(change.Path) {
			issues = append(issues, PlanIssue{
				Path:    change.Path,
				Code:    "unowned-path",
				Message: "no service owns this path, so the engine cannot apply it",
			})
		}
	}
	return issues
}

func PlanApply(current, desired config.Config) (Plan, error) {
	diff, err := DiffConfigs(current, desired)
	if err != nil {
		return Plan{}, err
	}

	if diff.IsEmpty() {
		return Plan{
			FromRevision: current.Metadata.Revision,
			ToRevision:   desired.Metadata.Revision,
			Diff:         diff,
			Actions:      []Action{},
			Services:     []string{},
			Checks:       []HealthCheck{},
		}, nil
	}

	issues := planIssues(diff.Changes)
	if desired.Metadata.Revision <= current.Metadata.Revision {
		issues = append(issues, PlanIssue{
			Path:    "/metadata/revision",
			Code:    "stale-revision",
			Message: fmt.Sprintf("revision must advance past %d", current.Metadata.Revision),
		})
	}
	if len(issues) > 0 {
		return Plan{}, &PlanError{Issues: issues}
	}

	currentFiles, err := Render(current)
	if err != nil {
		return Plan{}, err
	}
	desiredFiles, err := Render(desired)
	if err != nil {
		return Plan{}, err
	}

	fileActions := diffFileSets(currentFiles, desiredFiles)
	digestBefore := Digest(currentFiles)
	digestAfter := Digest(desiredFiles)
	touched := make([]string, 0, len(fileActions))
	for _, action := range fileActions {
		touched = append(touched, action.Path)
	}
	services := ServicesForFiles(touched)

	actions := make([]Action, 0, len(fileActions)+len(services))
	actions = append(actions, fileActions...)
	checks := make([]HealthCheck, 0, len(services))
	for _, service := range services {
		actions = append(actions, Action{Kind: ActionRestartService, Service: service})
		checks = append(checks, HealthCheck{Service: service})
	}

	return Plan{
		FromRevision: current.Metadata.Revision,
		ToRevision:   desired.Metadata.Revision,
		DigestBefore: digestBefore,
		DigestAfter:  digestAfter,
		Diff:         diff,
		Actions:      actions,
		Services:     services,
		Checks:       checks,
	}, nil
}

func WithNextRevision(current, desired config.Config) config.Config {
	next := desired
	next.Metadata.Revision = current.Metadata.Revision + 1
	return next
}

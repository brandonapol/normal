package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/brandonapol/normal/pkg/config"
)

type Operation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type PatchIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ApplyPatch(document any, operations []Operation) (any, []PatchIssue) {
	draft := document
	for index, operation := range operations {
		segments, err := config.ParsePointer(operation.Path)
		if err != nil {
			return nil, []PatchIssue{{
				Path: operation.Path, Code: "invalid-pointer", Message: err.Error(),
			}}
		}

		var next any
		var mutationErr error
		switch operation.Op {
		case "set":
			next, mutationErr = config.SetAtPath(draft, segments, operation.Value)
		case "remove":
			next, mutationErr = config.RemoveAtPath(draft, segments)
		default:
			return nil, []PatchIssue{{
				Path:    operation.Path,
				Code:    "unknown-operation",
				Message: fmt.Sprintf("operation %d has unknown op %q", index, operation.Op),
			}}
		}
		if mutationErr != nil {
			var pointerErr *config.PointerError
			code := "patch-failed"
			message := mutationErr.Error()
			if ok := asPointerError(mutationErr, &pointerErr); ok {
				code = string(pointerErr.Code)
				message = pointerErr.Message
			}
			return nil, []PatchIssue{{
				Path:    operation.Path,
				Code:    code,
				Message: fmt.Sprintf("operation %d (%s) failed: %s", index, operation.Op, message),
			}}
		}
		draft = next
	}
	return draft, nil
}

func asPointerError(err error, target **config.PointerError) bool {
	pointerErr, ok := err.(*config.PointerError)
	if ok {
		*target = pointerErr
	}
	return ok
}

func PatchConfig(current config.Config, operations []Operation, now time.Time) (config.Config, *Rejection) {
	document, err := config.ToDocument(current)
	if err != nil {
		return config.Config{}, &Rejection{Kind: RejectPatch, Message: err.Error()}
	}

	patched, issues := ApplyPatch(document, operations)
	if len(issues) > 0 {
		return config.Config{}, &Rejection{Kind: RejectPatch, PatchIssues: issues}
	}

	if validationIssues := config.Validate(patched, now); len(validationIssues) > 0 {
		return config.Config{}, &Rejection{Kind: RejectValidation, ValidationIssues: validationIssues}
	}

	candidate, err := config.FromDocument(patched)
	if err != nil {
		return config.Config{}, &Rejection{Kind: RejectPatch, Message: err.Error()}
	}
	return candidate, nil
}

func joinMessages(messages []string) string {
	return strings.Join(messages, "; ")
}

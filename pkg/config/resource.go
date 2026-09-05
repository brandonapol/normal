package config

import (
	"fmt"
	"time"

	"cuelang.org/go/cue"
)

type documentShape struct {
	depth int
	nodes int
}

func measure(value any, depth int, limits Limits, shape *documentShape) bool {
	if depth > shape.depth {
		shape.depth = depth
	}
	shape.nodes++

	if shape.depth > limits.MaxDocumentDepth || shape.nodes > limits.MaxDocumentNodes {
		return false
	}

	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if !measure(child, depth+1, limits, shape) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !measure(child, depth+1, limits, shape) {
				return false
			}
		}
	}
	return true
}

func resourceIssues(raw []byte, document any, limits Limits) []Issue {
	if limits.MaxDocumentBytes > 0 && len(raw) > limits.MaxDocumentBytes {
		return []Issue{{
			Path: "/",
			Code: "document-too-large",
			Message: fmt.Sprintf("document is %d bytes; the limit is %d",
				len(raw), limits.MaxDocumentBytes),
		}}
	}

	shape := &documentShape{}
	if !measure(document, 1, limits, shape) {
		if shape.depth > limits.MaxDocumentDepth {
			return []Issue{{
				Path: "/",
				Code: "document-too-deep",
				Message: fmt.Sprintf("document nests deeper than %d levels",
					limits.MaxDocumentDepth),
			}}
		}
		return []Issue{{
			Path: "/",
			Code: "document-too-complex",
			Message: fmt.Sprintf("document holds more than %d values",
				limits.MaxDocumentNodes),
		}}
	}
	return nil
}

func validateWithTimeout(unified cue.Value, limits Limits) (err error, timedOut bool) {
	if limits.ValidationTimeoutMs <= 0 {
		return unified.Validate(cue.Concrete(true), cue.All()), false
	}

	done := make(chan error, 1)
	go func() {
		done <- unified.Validate(cue.Concrete(true), cue.All())
	}()

	timer := time.NewTimer(time.Duration(limits.ValidationTimeoutMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case result := <-done:
		return result, false
	case <-timer.C:
		return nil, true
	}
}

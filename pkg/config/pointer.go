package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type PointerErrorCode string

const (
	ErrNotFound       PointerErrorCode = "not-found"
	ErrNotTraversable PointerErrorCode = "not-traversable"
	ErrInvalidPointer PointerErrorCode = "invalid-pointer"
)

type PointerError struct {
	Code    PointerErrorCode
	Pointer string
	Message string
}

func (e *PointerError) Error() string {
	return fmt.Sprintf("%s at %q: %s", e.Code, e.Pointer, e.Message)
}

var numeric = regexp.MustCompile(`^\d+$`)

func unescapeSegment(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
}

func escapeSegment(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func ParsePointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, &PointerError{Code: ErrInvalidPointer, Pointer: pointer, Message: "pointer must start with '/'"}
	}
	raw := strings.Split(pointer[1:], "/")
	segments := make([]string, len(raw))
	for i, segment := range raw {
		segments[i] = unescapeSegment(segment)
	}
	return segments, nil
}

func FormatPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = escapeSegment(segment)
	}
	return "/" + strings.Join(escaped, "/")
}

type walked struct {
	segment string
	inArray bool
}

func patternOf(trail []walked) string {
	if len(trail) == 0 {
		return ""
	}
	parts := make([]string, len(trail))
	for i, step := range trail {
		if step.inArray {
			parts[i] = "*"
		} else {
			parts[i] = step.segment
		}
	}
	return "/" + strings.Join(parts, "/")
}

func extendTrail(trail []walked, step walked) []walked {
	next := make([]walked, len(trail), len(trail)+1)
	copy(next, trail)
	return append(next, step)
}

func segmentsOf(trail []walked) []string {
	out := make([]string, len(trail))
	for i, step := range trail {
		out[i] = step.segment
	}
	return out
}

func resolveIndex(array []any, pattern, segment string) int {
	if keyField, ok := KeyFieldFor(pattern); ok {
		for i, item := range array {
			record, isRecord := item.(map[string]any)
			if !isRecord {
				continue
			}
			if value, found := record[keyField]; found {
				if text, isText := value.(string); isText && text == segment {
					return i
				}
			}
		}
	}
	if numeric.MatchString(segment) {
		index, err := strconv.Atoi(segment)
		if err == nil && index < len(array) {
			return index
		}
	}
	return -1
}

func GetAtPath(root any, segments []string) (any, error) {
	trail := make([]walked, 0, len(segments))
	current := root
	for _, segment := range segments {
		if array, isArray := current.([]any); isArray {
			index := resolveIndex(array, patternOf(trail), segment)
			if index < 0 {
				return nil, &PointerError{
					Code:    ErrNotFound,
					Pointer: FormatPointer(append(segmentsOf(trail), segment)),
					Message: fmt.Sprintf("no element matching %q", segment),
				}
			}
			current = array[index]
			trail = append(trail, walked{segment: segment, inArray: true})
			continue
		}
		record, isRecord := current.(map[string]any)
		if !isRecord {
			return nil, &PointerError{
				Code:    ErrNotTraversable,
				Pointer: FormatPointer(append(segmentsOf(trail), segment)),
				Message: fmt.Sprintf("cannot traverse into %q", segment),
			}
		}
		value, found := record[segment]
		if !found {
			return nil, &PointerError{
				Code:    ErrNotFound,
				Pointer: FormatPointer(append(segmentsOf(trail), segment)),
				Message: fmt.Sprintf("no property %q", segment),
			}
		}
		current = value
		trail = append(trail, walked{segment: segment, inArray: false})
	}
	return current, nil
}

type mutation struct {
	remove bool
	value  any
}

func copyRecord(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func copyArray(source []any) []any {
	out := make([]any, len(source))
	copy(out, source)
	return out
}

func applyMutation(container any, segments []string, trail []walked, op mutation) (any, error) {
	if len(segments) == 0 {
		if op.remove {
			return nil, &PointerError{
				Code:    ErrInvalidPointer,
				Pointer: FormatPointer(segmentsOf(trail)),
				Message: "cannot remove the document root",
			}
		}
		return op.value, nil
	}

	segment, rest := segments[0], segments[1:]
	here := FormatPointer(append(segmentsOf(trail), segment))

	if array, isArray := container.([]any); isArray {
		index := resolveIndex(array, patternOf(trail), segment)
		if index < 0 {
			if len(rest) > 0 || op.remove {
				return nil, &PointerError{Code: ErrNotFound, Pointer: here, Message: fmt.Sprintf("no element matching %q", segment)}
			}
			return append(copyArray(array), op.value), nil
		}
		if len(rest) == 0 && op.remove {
			out := make([]any, 0, len(array)-1)
			out = append(out, array[:index]...)
			out = append(out, array[index+1:]...)
			return out, nil
		}
		child, err := applyMutation(array[index], rest, extendTrail(trail, walked{segment: segment, inArray: true}), op)
		if err != nil {
			return nil, err
		}
		next := copyArray(array)
		next[index] = child
		return next, nil
	}

	record, isRecord := container.(map[string]any)
	if !isRecord {
		return nil, &PointerError{Code: ErrNotTraversable, Pointer: here, Message: fmt.Sprintf("cannot traverse into %q", segment)}
	}

	_, present := record[segment]
	if len(rest) == 0 && op.remove {
		if !present {
			return nil, &PointerError{Code: ErrNotFound, Pointer: here, Message: fmt.Sprintf("no property %q", segment)}
		}
		next := copyRecord(record)
		delete(next, segment)
		return next, nil
	}
	if len(rest) > 0 && !present {
		return nil, &PointerError{Code: ErrNotFound, Pointer: here, Message: fmt.Sprintf("no property %q", segment)}
	}

	child, err := applyMutation(record[segment], rest, extendTrail(trail, walked{segment: segment, inArray: false}), op)
	if err != nil {
		return nil, err
	}
	next := copyRecord(record)
	next[segment] = child
	return next, nil
}

func SetAtPath(root any, segments []string, value any) (any, error) {
	return applyMutation(root, segments, nil, mutation{value: value})
}

func RemoveAtPath(root any, segments []string) (any, error) {
	return applyMutation(root, segments, nil, mutation{remove: true})
}

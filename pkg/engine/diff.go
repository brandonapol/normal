package engine

import (
	"reflect"
	"sort"

	"github.com/brandonapol/normal/pkg/config"
)

type ChangeOp string

const (
	OpAdd     ChangeOp = "add"
	OpReplace ChangeOp = "replace"
	OpRemove  ChangeOp = "remove"
)

type Change struct {
	Op     ChangeOp `json:"op"`
	Path   string   `json:"path"`
	Before any      `json:"before,omitempty"`
	After  any      `json:"after,omitempty"`
}

type Diff struct {
	Changes []Change `json:"changes"`
}

func (d Diff) IsEmpty() bool { return len(d.Changes) == 0 }

func (d Diff) Paths() []string {
	paths := make([]string, 0, len(d.Changes))
	for _, change := range d.Changes {
		paths = append(paths, change.Path)
	}
	return paths
}

func DeepEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

func keyOf(value any, keyField string) (string, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	raw, present := record[keyField]
	if !present {
		return "", false
	}
	text, isText := raw.(string)
	return text, isText
}

func diffKeyedArray(segments []string, pattern, keyField string, before, after []any) []Change {
	beforeByKey := make(map[string]any, len(before))
	afterByKey := make(map[string]any, len(after))
	beforeOrder := make([]string, 0, len(before))
	afterOrder := make([]string, 0, len(after))

	for _, item := range before {
		key, ok := keyOf(item, keyField)
		if !ok {
			return []Change{{Op: OpReplace, Path: config.FormatPointer(segments), Before: before, After: after}}
		}
		beforeByKey[key] = item
		beforeOrder = append(beforeOrder, key)
	}
	for _, item := range after {
		key, ok := keyOf(item, keyField)
		if !ok {
			return []Change{{Op: OpReplace, Path: config.FormatPointer(segments), Before: before, After: after}}
		}
		afterByKey[key] = item
		afterOrder = append(afterOrder, key)
	}
	if len(beforeByKey) != len(before) || len(afterByKey) != len(after) {
		return []Change{{Op: OpReplace, Path: config.FormatPointer(segments), Before: before, After: after}}
	}

	keys := make([]string, 0, len(beforeByKey)+len(afterByKey))
	seen := make(map[string]bool)
	for _, key := range append(append([]string{}, beforeOrder...), afterOrder...) {
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	changes := make([]Change, 0)
	for _, key := range keys {
		next := append(append([]string{}, segments...), key)
		beforeItem, inBefore := beforeByKey[key]
		afterItem, inAfter := afterByKey[key]
		switch {
		case !inBefore:
			changes = append(changes, Change{Op: OpAdd, Path: config.FormatPointer(next), After: afterItem})
		case !inAfter:
			changes = append(changes, Change{Op: OpRemove, Path: config.FormatPointer(next), Before: beforeItem})
		default:
			changes = append(changes, diffValue(next, pattern+"/*", beforeItem, afterItem)...)
		}
	}

	survivingBefore := make([]string, 0, len(beforeOrder))
	for _, key := range beforeOrder {
		if _, ok := afterByKey[key]; ok {
			survivingBefore = append(survivingBefore, key)
		}
	}
	survivingAfter := make([]string, 0, len(afterOrder))
	for _, key := range afterOrder {
		if _, ok := beforeByKey[key]; ok {
			survivingAfter = append(survivingAfter, key)
		}
	}
	if !reflect.DeepEqual(survivingBefore, survivingAfter) {
		orderPath := config.FormatPointer(append(append([]string{}, segments...), "$order"))
		changes = append(changes, Change{Op: OpReplace, Path: orderPath, Before: beforeOrder, After: afterOrder})
	}

	return changes
}

func diffValue(segments []string, pattern string, before, after any) []Change {
	if reflect.DeepEqual(before, after) {
		return nil
	}

	beforeArray, beforeIsArray := before.([]any)
	afterArray, afterIsArray := after.([]any)
	if beforeIsArray && afterIsArray {
		if keyField, ok := config.KeyFieldFor(pattern); ok {
			return diffKeyedArray(segments, pattern, keyField, beforeArray, afterArray)
		}
		return []Change{{Op: OpReplace, Path: config.FormatPointer(segments), Before: before, After: after}}
	}

	beforeRecord, beforeIsRecord := before.(map[string]any)
	afterRecord, afterIsRecord := after.(map[string]any)
	if beforeIsRecord && afterIsRecord {
		keys := make([]string, 0, len(beforeRecord)+len(afterRecord))
		seen := make(map[string]bool)
		for key := range beforeRecord {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
		for key := range afterRecord {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)

		changes := make([]Change, 0)
		for _, key := range keys {
			next := append(append([]string{}, segments...), key)
			beforeValue, inBefore := beforeRecord[key]
			afterValue, inAfter := afterRecord[key]
			switch {
			case !inBefore:
				changes = append(changes, Change{Op: OpAdd, Path: config.FormatPointer(next), After: afterValue})
			case !inAfter:
				changes = append(changes, Change{Op: OpRemove, Path: config.FormatPointer(next), Before: beforeValue})
			default:
				changes = append(changes, diffValue(next, pattern+"/"+key, beforeValue, afterValue)...)
			}
		}
		return changes
	}

	return []Change{{Op: OpReplace, Path: config.FormatPointer(segments), Before: before, After: after}}
}

func DiffDocuments(before, after any) Diff {
	changes := diffValue(nil, "", before, after)
	if changes == nil {
		changes = []Change{}
	}
	return Diff{Changes: changes}
}

func DiffConfigs(before, after config.Config) (Diff, error) {
	beforeDocument, err := config.ToDocument(before)
	if err != nil {
		return Diff{}, err
	}
	afterDocument, err := config.ToDocument(after)
	if err != nil {
		return Diff{}, err
	}
	return DiffDocuments(beforeDocument, afterDocument), nil
}

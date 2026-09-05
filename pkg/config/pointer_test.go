package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
)

func at(t *testing.T, pointer string) []string {
	t.Helper()
	segments, err := config.ParsePointer(pointer)
	if err != nil {
		t.Fatalf("ParsePointer(%q): %v", pointer, err)
	}
	return segments
}

func document(t *testing.T) map[string]any {
	t.Helper()
	doc, err := config.ToDocument(config.Baseline())
	if err != nil {
		t.Fatalf("ToDocument: %v", err)
	}
	return doc
}

func TestPointerRoundTrip(t *testing.T) {
	segments := at(t, "/a~1b/c~0d")
	if !reflect.DeepEqual(segments, []string{"a/b", "c~d"}) {
		t.Fatalf("unexpected segments %v", segments)
	}
	if got := config.FormatPointer(segments); got != "/a~1b/c~0d" {
		t.Fatalf("round trip produced %q", got)
	}
}

func TestPointerMustStartWithSlash(t *testing.T) {
	if _, err := config.ParsePointer("spec/launcher"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGetScalar(t *testing.T) {
	value, err := config.GetAtPath(document(t), at(t, "/spec/launcher/columns"))
	if err != nil {
		t.Fatalf("GetAtPath: %v", err)
	}
	if value != float64(1) {
		t.Fatalf("expected 1, got %v (%T)", value, value)
	}
}

func TestGetByCollectionKey(t *testing.T) {
	value, err := config.GetAtPath(document(t), at(t, "/spec/apps/entries/com.spotify.music/network"))
	if err != nil {
		t.Fatalf("GetAtPath: %v", err)
	}
	if value != "allow" {
		t.Fatalf("expected allow, got %v", value)
	}
}

func TestGetByNumericIndexStillWorks(t *testing.T) {
	value, err := config.GetAtPath(document(t), at(t, "/spec/apps/entries/0/package"))
	if err != nil {
		t.Fatalf("GetAtPath: %v", err)
	}
	if value != "os.normal.phone" {
		t.Fatalf("expected the first entry, got %v", value)
	}
}

func TestGetMissingKeyIsNotFound(t *testing.T) {
	_, err := config.GetAtPath(document(t), at(t, "/spec/apps/entries/com.example.ghost"))
	var pointerErr *config.PointerError
	if !errors.As(err, &pointerErr) || pointerErr.Code != config.ErrNotFound {
		t.Fatalf("expected not-found, got %v", err)
	}
}

func TestSetDoesNotMutateTheSource(t *testing.T) {
	source := document(t)
	updated, err := config.SetAtPath(source, at(t, "/spec/launcher/columns"), 3)
	if err != nil {
		t.Fatalf("SetAtPath: %v", err)
	}
	original, _ := config.GetAtPath(source, at(t, "/spec/launcher/columns"))
	if original != float64(1) {
		t.Fatalf("source was mutated, columns is now %v", original)
	}
	changed, _ := config.GetAtPath(updated, at(t, "/spec/launcher/columns"))
	if changed != 3 {
		t.Fatalf("expected 3, got %v", changed)
	}
}

func TestSetAppendsToKeyedCollection(t *testing.T) {
	entry := map[string]any{
		"package":     "org.fdroid.fdroid",
		"source":      "fdroid",
		"state":       "installed",
		"network":     "wifi-only",
		"permissions": map[string]any{},
	}
	updated, err := config.SetAtPath(document(t), at(t, "/spec/apps/entries/org.fdroid.fdroid"), entry)
	if err != nil {
		t.Fatalf("SetAtPath: %v", err)
	}
	value, err := config.GetAtPath(updated, at(t, "/spec/apps/entries/org.fdroid.fdroid/network"))
	if err != nil || value != "wifi-only" {
		t.Fatalf("expected the appended entry to be addressable, got %v (%v)", value, err)
	}
}

func TestRemoveKeyedElement(t *testing.T) {
	source := document(t)
	updated, err := config.RemoveAtPath(source, at(t, "/spec/apps/entries/com.spotify.music"))
	if err != nil {
		t.Fatalf("RemoveAtPath: %v", err)
	}
	if _, err := config.GetAtPath(updated, at(t, "/spec/apps/entries/com.spotify.music")); err == nil {
		t.Fatal("entry should be gone")
	}
	if _, err := config.GetAtPath(source, at(t, "/spec/apps/entries/com.spotify.music")); err != nil {
		t.Fatal("source document was mutated")
	}
}

func TestCannotTraverseIntoScalar(t *testing.T) {
	_, err := config.SetAtPath(document(t), at(t, "/spec/launcher/columns/nope"), 1)
	var pointerErr *config.PointerError
	if !errors.As(err, &pointerErr) || pointerErr.Code != config.ErrNotTraversable {
		t.Fatalf("expected not-traversable, got %v", err)
	}
}

func TestNumericSegmentsRejectLeadingZeros(t *testing.T) {
	document := document(t)

	first, err := config.GetAtPath(document, at(t, "/spec/apps/entries/0/package"))
	if err != nil {
		t.Fatalf("index 0 should resolve: %v", err)
	}
	if first != "os.normal.phone" {
		t.Fatalf("unexpected first entry %v", first)
	}

	for _, alias := range []string{"00", "000", "01"} {
		if _, err := config.GetAtPath(document, at(t, "/spec/apps/entries/"+alias)); err == nil {
			t.Errorf("%q must not alias to an array index; RFC 6901 forbids leading zeros", alias)
		}
	}
}

func TestSetRefusesEmptySegmentInCollection(t *testing.T) {
	_, err := config.SetAtPath(document(t), at(t, "/spec/apps/entries/"), map[string]any{})
	var pointerErr *config.PointerError
	if !errors.As(err, &pointerErr) || pointerErr.Code != config.ErrInvalidPointer {
		t.Fatalf("a trailing slash must not silently append; got %v", err)
	}
}

func TestSetRefusesKeyMismatch(t *testing.T) {
	entry := map[string]any{
		"package":     "org.actual.name",
		"source":      "fdroid",
		"state":       "installed",
		"network":     "allow",
		"permissions": map[string]any{},
	}
	_, err := config.SetAtPath(document(t), at(t, "/spec/apps/entries/org.claimed.name"), entry)
	var pointerErr *config.PointerError
	if !errors.As(err, &pointerErr) || pointerErr.Code != config.ErrInvalidPointer {
		t.Fatalf("the path and the value's key must agree; got %v", err)
	}
	if !strings.Contains(pointerErr.Message, "org.actual.name") {
		t.Fatalf("the error should name the mismatch, got %q", pointerErr.Message)
	}
}

func TestSetRefusesNonNumericIndexOnUnkeyedArray(t *testing.T) {
	_, err := config.SetAtPath(document(t), at(t, "/spec/launcher/dock/somewhere"), "os.normal.phone")
	if err == nil {
		t.Fatal("an unkeyed array is addressed by position; a name must not append")
	}
}

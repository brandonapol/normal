package config_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/brandonapol/normal/pkg/config"
)

func baselineDocument(t *testing.T) map[string]any {
	t.Helper()
	document, err := config.ToDocument(config.Baseline())
	if err != nil {
		t.Fatalf("ToDocument: %v", err)
	}
	return document
}

func snapshot(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func FuzzParsePointer(f *testing.F) {
	for _, seed := range []string{
		"",
		"/",
		"/spec",
		"/spec/launcher/columns",
		"/spec/apps/entries/com.spotify.music/network",
		"/a~1b/c~0d",
		"/~",
		"/~2",
		"//",
		"/spec//launcher",
		"spec/launcher",
		"/spec/launcher/",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, pointer string) {
		segments, err := config.ParsePointer(pointer)
		if err != nil {
			return
		}

		formatted := config.FormatPointer(segments)
		reparsed, err := config.ParsePointer(formatted)
		if err != nil {
			t.Fatalf("formatting %q produced unparseable %q: %v", pointer, formatted, err)
		}
		if !reflect.DeepEqual(segments, reparsed) {
			t.Fatalf("round trip changed segments: %q -> %v -> %q -> %v",
				pointer, segments, formatted, reparsed)
		}
	})
}

func FuzzSetAtPath(f *testing.F) {
	for _, seed := range []struct {
		pointer string
		value   string
	}{
		{"/spec/launcher/columns", "3"},
		{"/spec/launcher/layout", `"grid"`},
		{"/spec/apps/entries/com.spotify.music/network", `"deny"`},
		{"/spec/apps/entries/org.new.app", `{"package":"org.new.app"}`},
		{"/spec/attention/infiniteScroll/exemptions/x", `{"id":"x"}`},
		{"/spec/launcher/columns/deeper", "1"},
		{"/nonexistent/path", "null"},
		{"", "{}"},
		{"/spec/launcher/pages/home/items/home-phone/label", `"Call"`},
	} {
		f.Add(seed.pointer, seed.value)
	}

	f.Fuzz(func(t *testing.T, pointer, valueJSON string) {
		segments, err := config.ParsePointer(pointer)
		if err != nil {
			return
		}
		var value any
		if decodeErr := json.Unmarshal([]byte(valueJSON), &value); decodeErr != nil {
			return
		}

		document := baselineDocument(t)
		before := snapshot(t, document)

		updated, err := config.SetAtPath(document, segments, value)

		if after := snapshot(t, document); after != before {
			t.Fatalf("SetAtPath mutated its input at %q", pointer)
		}
		if err != nil {
			return
		}

		readBack, err := config.GetAtPath(updated, segments)
		if err != nil {
			t.Fatalf("set %q succeeded but reading it back failed: %v", pointer, err)
		}
		if snapshot(t, readBack) != snapshot(t, value) {
			t.Fatalf("set %q stored %s but read back %s",
				pointer, snapshot(t, value), snapshot(t, readBack))
		}
	})
}

func FuzzRemoveAtPath(f *testing.F) {
	for _, seed := range []string{
		"/spec/launcher/columns",
		"/spec/apps/entries/com.spotify.music",
		"/spec/attention/infiniteScroll/detectors/dom-default",
		"/spec/launcher/pages/home/items/home-phone",
		"/metadata/description",
		"/nonexistent",
		"",
		"/spec/apps/entries/9999",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, pointer string) {
		segments, err := config.ParsePointer(pointer)
		if err != nil {
			return
		}

		document := baselineDocument(t)
		before := snapshot(t, document)

		updated, err := config.RemoveAtPath(document, segments)

		if after := snapshot(t, document); after != before {
			t.Fatalf("RemoveAtPath mutated its input at %q", pointer)
		}
		if err != nil {
			return
		}

		if snapshot(t, updated) == before {
			t.Fatalf("removed %q but the document is unchanged", pointer)
		}

		last := segments[len(segments)-1]
		if numericSegment.MatchString(last) {
			return
		}
		if _, err := config.GetAtPath(updated, segments); err == nil {
			t.Fatalf("removed %q but it is still readable", pointer)
		}
	})
}

var numericSegment = regexp.MustCompile(`^(0|[1-9]\d*)$`)

func FuzzValidate(f *testing.F) {
	baseline, err := json.Marshal(config.Baseline())
	if err != nil {
		f.Fatalf("marshal baseline: %v", err)
	}
	f.Add(string(baseline))
	for _, seed := range []string{
		"{}",
		"null",
		"[]",
		`{"apiVersion":"normal.os/v0"}`,
		`{"apiVersion":"normal.os/v0","kind":"PhoneConfig","metadata":{},"spec":{}}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, documentJSON string) {
		document, err := config.ParseDocument([]byte(documentJSON))
		if err != nil {
			return
		}
		issues := config.Validate(document, now)
		for _, issue := range issues {
			if issue.Code == "" {
				t.Fatalf("issue with empty code at %q: %s", issue.Path, issue.Message)
			}
		}
	})
}

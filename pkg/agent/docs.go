package agent

import (
	"fmt"
	"strings"

	"github.com/brandonapol/normal/pkg/config"
)

type SchemaDoc struct {
	Path     string `json:"path"`
	Summary  string `json:"summary"`
	Writable bool   `json:"writable"`
	Notes    string `json:"notes,omitempty"`
}

func SchemaDocs() []SchemaDoc {
	limits := config.MustLimits()
	return []SchemaDoc{
		{
			Path:     "/metadata",
			Summary:  "Identity and revision of this configuration.",
			Writable: false,
			Notes: "Only /metadata/description and /metadata/labels accept agent writes; " +
				"revision is engine-managed.",
		},
		{
			Path:     "/spec/launcher",
			Summary:  "Home screen layout: pages, items, dock, app drawer, gestures.",
			Writable: true,
			Notes: fmt.Sprintf("columns is %d-%d; a page holds at most maxItemsPerPage items.",
				limits.MinColumns, limits.MaxColumns),
		},
		{
			Path:     "/spec/apps",
			Summary:  "Installed apps with their source, state, network policy, and permissions.",
			Writable: true,
			Notes: "Entries are keyed by package id. Permission and network changes always need " +
				"user approval.",
		},
		{
			Path:     "/spec/notifications",
			Summary:  "Default disposition, bundling windows, quiet hours, and match rules.",
			Writable: true,
			Notes: "Rules are evaluated highest priority first; a rule must constrain at least " +
				"one field.",
		},
		{
			Path:     "/spec/attention/infiniteScroll",
			Summary:  "The no-infinite-scroll policy: enforcement mode, page size, detectors, exemptions.",
			Writable: true,
			Notes: fmt.Sprintf("There is no 'off' enforcement mode. Detectors cannot be emptied and the "+
				"webview shim cannot be disabled. At most %d exemptions, each needing a stated reason and "+
				"an expiry within %d days. Every change here needs user approval.",
				limits.MaxExemptions, limits.MaxExemptionDays),
		},
		{
			Path:     "/spec/attention/sessionBudgets",
			Summary:  "Per-app, per-domain, or system-wide time budgets and what happens when they run out.",
			Writable: true,
			Notes:    "Raising a budget is treated as weakening policy and needs user approval.",
		},
	}
}

func DocsFor(path string) []SchemaDoc {
	all := SchemaDocs()
	if path == "" {
		return all
	}
	matched := make([]SchemaDoc, 0)
	for _, doc := range all {
		if doc.Path == path ||
			strings.HasPrefix(doc.Path, path+"/") ||
			strings.HasPrefix(path, doc.Path+"/") {
			matched = append(matched, doc)
		}
	}
	return matched
}

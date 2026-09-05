package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/brandonapol/normal/pkg/config"
)

const ConfigRoot = "/etc/normal"

var (
	FileMetadata      = ConfigRoot + "/metadata.json"
	FileLauncher      = ConfigRoot + "/launcher.json"
	FileApps          = ConfigRoot + "/apps.json"
	FileNotifications = ConfigRoot + "/notifications.json"
	FileAttention     = ConfigRoot + "/attention.json"
	FileWebviewShim   = ConfigRoot + "/generated/webview-shim.json"
)

type FileSet map[string]string

func stableEncode(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(normalized); err != nil {
		return "", err
	}
	return out.String(), nil
}

type webviewShim struct {
	Enforcement                       string       `json:"enforcement"`
	PageSize                          int          `json:"pageSize"`
	MaxAutoLoads                      int          `json:"maxAutoLoads"`
	Continuation                      string       `json:"continuation"`
	ContinuationDelaySeconds          int          `json:"continuationDelaySeconds"`
	InterceptIntersectionObserver     bool         `json:"interceptIntersectionObserver"`
	InterceptHistoryScrollRestoration bool         `json:"interceptHistoryScrollRestoration"`
	MaxDocumentHeightMultiplier       float64      `json:"maxDocumentHeightMultiplier"`
	URLPatterns                       []string     `json:"urlPatterns"`
	DomSignals                        []string     `json:"domSignals"`
	ExemptPackages                    []string     `json:"exemptPackages"`
	PerApp                            []perAppShim `json:"perApp"`
}

type perAppShim struct {
	Package     string `json:"package"`
	Enforcement string `json:"enforcement"`
	PageSize    int    `json:"pageSize"`
}

func renderWebviewShim(c config.Config) webviewShim {
	policy := c.Spec.Attention.InfiniteScroll

	urlPatterns := make([]string, 0)
	domSignals := make([]string, 0)
	for _, detector := range policy.Detectors {
		switch detector.Kind {
		case "url-pattern":
			urlPatterns = append(urlPatterns, detector.Pattern)
		case "dom-heuristic":
			domSignals = append(domSignals, detector.Signals...)
		}
	}

	exempt := make([]string, 0, len(policy.Exemptions))
	for _, exemption := range policy.Exemptions {
		exempt = append(exempt, exemption.Package)
	}

	perApp := make([]perAppShim, 0)
	for _, entry := range c.Spec.Apps.Entries {
		if entry.Attention == nil {
			continue
		}
		enforcement := entry.Attention.Enforcement
		if enforcement == "" {
			enforcement = policy.Enforcement
		}
		pageSize := entry.Attention.PageSize
		if pageSize == 0 {
			pageSize = policy.PageSize
		}
		perApp = append(perApp, perAppShim{Package: entry.Package, Enforcement: enforcement, PageSize: pageSize})
	}

	return webviewShim{
		Enforcement:                       policy.Enforcement,
		PageSize:                          policy.PageSize,
		MaxAutoLoads:                      policy.MaxAutoLoads,
		Continuation:                      policy.Continuation,
		ContinuationDelaySeconds:          policy.ContinuationDelaySeconds,
		InterceptIntersectionObserver:     policy.Webview.InterceptIntersectionObserver,
		InterceptHistoryScrollRestoration: policy.Webview.InterceptHistoryScrollRestoration,
		MaxDocumentHeightMultiplier:       policy.Webview.MaxDocumentHeightMultiplier,
		URLPatterns:                       urlPatterns,
		DomSignals:                        domSignals,
		ExemptPackages:                    exempt,
		PerApp:                            perApp,
	}
}

func Digest(files FileSet) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	hasher := sha256.New()
	for _, path := range paths {
		_, _ = fmt.Fprintf(hasher, "%s\x00%s\x00", path, files[path])
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func Render(c config.Config) (FileSet, error) {
	metadata := map[string]any{
		"apiVersion":  c.APIVersion,
		"kind":        c.Kind,
		"name":        c.Metadata.Name,
		"revision":    c.Metadata.Revision,
		"description": c.Metadata.Description,
		"labels":      c.Metadata.Labels,
	}
	if c.Metadata.Description == "" {
		delete(metadata, "description")
	}
	if c.Metadata.Labels == nil {
		delete(metadata, "labels")
	}

	sections := []struct {
		file  string
		value any
	}{
		{FileMetadata, metadata},
		{FileLauncher, c.Spec.Launcher},
		{FileApps, c.Spec.Apps},
		{FileNotifications, c.Spec.Notifications},
		{FileAttention, c.Spec.Attention},
		{FileWebviewShim, renderWebviewShim(c)},
	}

	files := make(FileSet, len(sections))
	for _, section := range sections {
		encoded, err := stableEncode(section.value)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", section.file, err)
		}
		files[section.file] = encoded
	}
	return files, nil
}

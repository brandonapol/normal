package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"

	"github.com/brandonapol/normal/schema"
)

type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (i Issue) String() string {
	return fmt.Sprintf("%s: %s (%s)", i.Path, i.Message, i.Code)
}

type Limits struct {
	MinColumns                  int `json:"minColumns"`
	MaxColumns                  int `json:"maxColumns"`
	MaxPages                    int `json:"maxPages"`
	MaxItemsPerPage             int `json:"maxItemsPerPage"`
	MaxDockItems                int `json:"maxDockItems"`
	MinPageSize                 int `json:"minPageSize"`
	MaxPageSize                 int `json:"maxPageSize"`
	MaxAutoLoads                int `json:"maxAutoLoads"`
	MaxContinuationDelaySeconds int `json:"maxContinuationDelaySeconds"`
	MaxDocumentHeightMultiplier int `json:"maxDocumentHeightMultiplier"`
	MaxExemptions               int `json:"maxExemptions"`
	MaxExemptionDays            int `json:"maxExemptionDays"`
	MinExemptionReasonLength    int `json:"minExemptionReasonLength"`
	MaxNotificationRules        int `json:"maxNotificationRules"`
	MaxQuietHoursWindows        int `json:"maxQuietHoursWindows"`
	MaxSessionBudgets           int `json:"maxSessionBudgets"`
	MaxDetectors                int `json:"maxDetectors"`
	MaxLabelCount               int `json:"maxLabelCount"`
}

type compiled struct {
	ctx    *cue.Context
	def    cue.Value
	limits Limits
	err    error
}

var (
	loadOnce sync.Once
	loaded   compiled
)

func load() compiled {
	loadOnce.Do(func() {
		ctx := cuecontext.New()
		root := ctx.CompileBytes(schema.Source, cue.Filename("normal.cue"))
		if root.Err() != nil {
			loaded = compiled{err: root.Err()}
			return
		}
		def := root.LookupPath(cue.ParsePath("#PhoneConfig"))
		if def.Err() != nil {
			loaded = compiled{err: def.Err()}
			return
		}
		var limits Limits
		if err := root.LookupPath(cue.ParsePath("limits")).Decode(&limits); err != nil {
			loaded = compiled{err: err}
			return
		}
		loaded = compiled{ctx: ctx, def: def, limits: limits}
	})
	return loaded
}

func SchemaLimits() (Limits, error) {
	c := load()
	return c.limits, c.err
}

func MustLimits() Limits {
	limits, err := SchemaLimits()
	if err != nil {
		panic(err)
	}
	return limits
}

func pointerFromCUE(path []string) string {
	segments := make([]string, 0, len(path))
	for _, element := range path {
		if strings.HasPrefix(element, "#") {
			continue
		}
		segments = append(segments, strings.Trim(element, `"`))
	}
	return FormatPointer(segments)
}

var exemptionReason = regexp.MustCompile(`^/spec/attention/infiniteScroll/exemptions/\d+/reason$`)

func classify(pointer, message string) (string, string) {
	switch {
	case pointer == "/spec/attention/infiniteScroll/enforcement":
		return "invalid-enforcement", "enforcement must be warn, paginate, or block; the schema has no way to switch it off"
	case pointer == "/spec/attention/infiniteScroll/webview/injectShim":
		return "policy-violation", "the webview shim is the only enforcement point for web feeds and cannot be disabled"
	case pointer == "/spec/attention/infiniteScroll/detectors":
		if strings.Contains(message, "incomplete") || strings.Contains(message, "not enough") || strings.Contains(message, "0") {
			return "empty-detectors", "at least one detector is required; enforcement cannot be disabled by emptying this list"
		}
		return "too-many", message
	case pointer == "/spec/attention/infiniteScroll/exemptions":
		return "too-many", message
	case exemptionReason.MatchString(pointer):
		return "reason-too-short", "an exemption needs a stated reason a person can read later"
	case strings.HasPrefix(pointer, "/spec/attention/"):
		return "attention-policy-violation", message
	case pointer == "/apiVersion" || pointer == "/kind":
		return "unsupported-version", message
	}
	return "schema-violation", message
}

func structuralIssues(raw []byte) []Issue {
	c := load()
	if c.err != nil {
		return []Issue{{Path: "/", Code: "schema-unavailable", Message: c.err.Error()}}
	}

	expr, err := cuejson.Extract("config.json", raw)
	if err != nil {
		return []Issue{{Path: "/", Code: "invalid-document", Message: err.Error()}}
	}
	encoded := c.ctx.BuildExpr(expr)
	if encoded.Err() != nil {
		return []Issue{{Path: "/", Code: "invalid-document", Message: encoded.Err().Error()}}
	}

	unified := c.def.Unify(encoded)
	validationErr := unified.Validate(cue.Concrete(true), cue.All())
	if validationErr == nil {
		return nil
	}

	seen := make(map[string]bool)
	issues := make([]Issue, 0)
	for _, e := range cueerrors.Errors(validationErr) {
		pointer := pointerFromCUE(e.Path())
		message := strings.TrimSpace(cueerrors.Sanitize(e).Error())
		code, friendly := classify(pointer, message)
		key := pointer + "|" + code
		if seen[key] {
			continue
		}
		seen[key] = true
		issues = append(issues, Issue{Path: pointer, Code: code, Message: friendly})
	}
	return issues
}

func Validate(document any, now time.Time) []Issue {
	raw, err := json.Marshal(document)
	if err != nil {
		return []Issue{{Path: "/", Code: "invalid-document", Message: err.Error()}}
	}
	if issues := structuralIssues(raw); len(issues) > 0 {
		return issues
	}

	var typed Config
	if err := json.Unmarshal(raw, &typed); err != nil {
		return []Issue{{Path: "/", Code: "invalid-document", Message: err.Error()}}
	}
	return semanticIssues(typed, now, load().limits)
}

func ValidateConfig(c Config, now time.Time) []Issue {
	return Validate(c, now)
}

func normalize(document any) (any, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func ToDocument(c Config) (map[string]any, error) {
	normalized, err := normalize(c)
	if err != nil {
		return nil, err
	}
	document, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config did not encode to an object")
	}
	return document, nil
}

func FromDocument(document any) (Config, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(raw)
}

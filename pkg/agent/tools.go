package agent

import (
	"fmt"
	"strings"

	"github.com/brandonapol/normal/pkg/config"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var ToolNames = []string{
	"get_config",
	"describe_schema",
	"list_apps",
	"propose_change",
	"preview_proposal",
	"apply_proposal",
	"discard_proposal",
	"list_revisions",
	"propose_rollback",
}

func IsToolName(name string) bool {
	for _, known := range ToolNames {
		if known == name {
			return true
		}
	}
	return false
}

func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func proposalIDSchema() map[string]any {
	return object(map[string]any{"proposalId": map[string]any{"type": "string"}}, "proposalId")
}

func ToolDefinitions() []ToolDefinition {
	sections := []string{"metadata", "launcher", "apps", "notifications", "attention"}

	return []ToolDefinition{
		{
			Name: "get_config",
			Description: "Read the current device configuration, or one section of it. This is the only way " +
				"to see device state; there is no filesystem access.",
			InputSchema: object(map[string]any{
				"section": map[string]any{
					"type":        "string",
					"enum":        sections,
					"description": "Omit to read the whole document.",
				},
			}),
		},
		{
			Name: "describe_schema",
			Description: "Explain what lives at a config path, whether an agent may write it, and the " +
				"constraints the schema enforces. Call this before proposing a change to an unfamiliar path.",
			InputSchema: object(map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "A JSON pointer such as /spec/launcher.",
				},
			}),
		},
		{
			Name:        "list_apps",
			Description: "List installed apps with their package id, state, source, and network policy.",
			InputSchema: object(map[string]any{}),
		},
		{
			Name: "propose_change",
			Description: "Propose a configuration change as a patch against the schema. This validates the " +
				"result, computes a plan, and returns a diff for review. It does NOT change the device.",
			InputSchema: object(map[string]any{
				"intent": map[string]any{
					"type":        "string",
					"description": "One sentence, in the user's own terms, describing what this change is for.",
				},
				"operations": map[string]any{
					"type":     "array",
					"maxItems": MaxOperations,
					"items": object(map[string]any{
						"op": map[string]any{"type": "string", "enum": []string{"set", "remove"}},
						"path": map[string]any{
							"type": "string",
							"description": "JSON pointer. Keyed collections are addressed by key, e.g. " +
								"/spec/apps/entries/com.spotify.music/network.",
						},
						"value": map[string]any{"description": "Required for 'set', omitted for 'remove'."},
					}, "op", "path"),
				},
			}, "intent", "operations"),
		},
		{
			Name:        "preview_proposal",
			Description: "Show the diff, the files that would be written, and the services that would restart.",
			InputSchema: proposalIDSchema(),
		},
		{
			Name: "apply_proposal",
			Description: "Ask the mutation engine to apply an approved proposal transactionally. Fails if the " +
				"user has not approved a proposal that needs approval. The engine rolls back on failure.",
			InputSchema: proposalIDSchema(),
		},
		{
			Name:        "discard_proposal",
			Description: "Drop a pending proposal the user did not want.",
			InputSchema: proposalIDSchema(),
		},
		{
			Name:        "list_revisions",
			Description: "List applied configuration revisions, newest last, with the intent behind each.",
			InputSchema: object(map[string]any{}),
		},
		{
			Name: "propose_rollback",
			Description: "Propose returning the device to an earlier applied revision. Always requires user " +
				"approval.",
			InputSchema: object(map[string]any{
				"revision": map[string]any{"type": "integer", "minimum": 0},
			}, "revision"),
		},
	}
}

func SystemGuidance() string {
	limits := config.MustLimits()
	return strings.Join([]string{
		"You configure a Normal phone by proposing declarative diffs. You have no filesystem, shell, or",
		"network access; the tools below are your entire surface.",
		"",
		"Workflow: read with get_config / describe_schema, then propose_change, then show the user the",
		"preview and wait for them to approve. apply_proposal will refuse anything they have not approved.",
		"",
		"The no-infinite-scroll policy is a product invariant, not a preference. There is no 'off' switch,",
		fmt.Sprintf("at most %d exemptions may exist at once, and each needs a written reason and", limits.MaxExemptions),
		fmt.Sprintf("an expiry within %d days. If a user asks you to disable it outright, explain", limits.MaxExemptionDays),
		"what you can do instead: a bounded, expiring exemption for one app.",
	}, "\n")
}

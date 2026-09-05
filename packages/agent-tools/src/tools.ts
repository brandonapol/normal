import { LIMITS } from "@normal/schema";
import { MAX_OPERATIONS } from "./policy.js";

export type JsonSchema = {
  readonly type: "object";
  readonly properties: Readonly<Record<string, unknown>>;
  readonly required?: readonly string[];
  readonly additionalProperties: false;
};

export type ToolDefinition = {
  readonly name: string;
  readonly description: string;
  readonly inputSchema: JsonSchema;
};

export const TOOL_NAMES = [
  "get_config",
  "describe_schema",
  "list_apps",
  "propose_change",
  "preview_proposal",
  "apply_proposal",
  "discard_proposal",
  "list_revisions",
  "propose_rollback",
] as const;

export type ToolName = (typeof TOOL_NAMES)[number];

const SECTIONS = ["metadata", "launcher", "apps", "notifications", "attention"] as const;

export const TOOL_DEFINITIONS: readonly ToolDefinition[] = [
  {
    name: "get_config",
    description:
      "Read the current device configuration, or one section of it. This is the only way to see " +
      "device state; there is no filesystem access.",
    inputSchema: {
      type: "object",
      properties: {
        section: {
          type: "string",
          enum: SECTIONS,
          description: "Omit to read the whole document.",
        },
      },
      additionalProperties: false,
    },
  },
  {
    name: "describe_schema",
    description:
      "Explain what lives at a config path, whether an agent may write it, and the constraints the " +
      "validator enforces. Call this before proposing a change to an unfamiliar path.",
    inputSchema: {
      type: "object",
      properties: {
        path: { type: "string", description: "A JSON pointer such as /spec/launcher." },
      },
      additionalProperties: false,
    },
  },
  {
    name: "list_apps",
    description: "List installed apps with their package id, state, source, and network policy.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
  {
    name: "propose_change",
    description:
      "Propose a configuration change as a patch against the schema. This validates the result, " +
      "computes a plan, and returns a diff for review. It does NOT change the device.",
    inputSchema: {
      type: "object",
      properties: {
        intent: {
          type: "string",
          description: "One sentence, in the user's own terms, describing what this change is for.",
        },
        operations: {
          type: "array",
          maxItems: MAX_OPERATIONS,
          items: {
            type: "object",
            properties: {
              op: { type: "string", enum: ["set", "remove"] },
              path: {
                type: "string",
                description:
                  "JSON pointer. Keyed collections are addressed by key, e.g. " +
                  "/spec/apps/entries/com.spotify.music/network.",
              },
              value: { description: "Required for 'set', omitted for 'remove'." },
            },
            required: ["op", "path"],
            additionalProperties: false,
          },
        },
      },
      required: ["intent", "operations"],
      additionalProperties: false,
    },
  },
  {
    name: "preview_proposal",
    description: "Show the diff, the files that would be written, and the services that would restart.",
    inputSchema: {
      type: "object",
      properties: { proposalId: { type: "string" } },
      required: ["proposalId"],
      additionalProperties: false,
    },
  },
  {
    name: "apply_proposal",
    description:
      "Ask the mutation engine to apply an approved proposal transactionally. Fails if the user has " +
      "not approved a proposal that needs approval. The engine rolls back on failure.",
    inputSchema: {
      type: "object",
      properties: { proposalId: { type: "string" } },
      required: ["proposalId"],
      additionalProperties: false,
    },
  },
  {
    name: "discard_proposal",
    description: "Drop a pending proposal the user did not want.",
    inputSchema: {
      type: "object",
      properties: { proposalId: { type: "string" } },
      required: ["proposalId"],
      additionalProperties: false,
    },
  },
  {
    name: "list_revisions",
    description: "List applied configuration revisions, newest last, with the intent behind each.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
  {
    name: "propose_rollback",
    description:
      "Propose returning the device to an earlier applied revision. Always requires user approval.",
    inputSchema: {
      type: "object",
      properties: { revision: { type: "integer", minimum: 0 } },
      required: ["revision"],
      additionalProperties: false,
    },
  },
];

export const SYSTEM_GUIDANCE = [
  "You configure a Normal phone by proposing declarative diffs. You have no filesystem, shell, or",
  "network access; the tools below are your entire surface.",
  "",
  "Workflow: read with get_config / describe_schema, then propose_change, then show the user the",
  "preview and wait for them to approve. apply_proposal will refuse anything they have not approved.",
  "",
  "The no-infinite-scroll policy is a product invariant, not a preference. There is no 'off' switch,",
  `at most ${LIMITS.maxExemptions} exemptions may exist at once, and each needs a written reason and`,
  `an expiry within ${LIMITS.maxExemptionDays} days. If a user asks you to disable it outright, explain`,
  "what you can do instead: a bounded, expiring exemption for one app.",
].join("\n");

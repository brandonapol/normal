import { formatDiff, formatPlan, type ConfigDiff } from "@normal/engine";
import type { AgentSession, Proposal } from "./session.js";
import { docsFor } from "./docs.js";
import type { PatchOperation } from "./patch.js";
import { TOOL_NAMES, type ToolName } from "./tools.js";

export type ToolCall = {
  readonly name: string;
  readonly arguments: Readonly<Record<string, unknown>>;
};

export type ToolResult =
  | { readonly ok: true; readonly data: unknown }
  | { readonly ok: false; readonly error: { readonly code: string; readonly message: string; readonly details?: unknown } };

const ok = (data: unknown): ToolResult => ({ ok: true, data });
const fail = (code: string, message: string, details?: unknown): ToolResult =>
  details === undefined ? { ok: false, error: { code, message } } : { ok: false, error: { code, message, details } };

const isToolName = (name: string): name is ToolName =>
  (TOOL_NAMES as readonly string[]).includes(name);

const stringArg = (args: Readonly<Record<string, unknown>>, key: string): string | undefined =>
  typeof args[key] === "string" ? (args[key] as string) : undefined;

const parseOperations = (value: unknown): PatchOperation[] | undefined => {
  if (!Array.isArray(value)) return undefined;
  const operations: PatchOperation[] = [];
  for (const raw of value) {
    if (typeof raw !== "object" || raw === null) return undefined;
    const candidate = raw as Record<string, unknown>;
    const path = candidate["path"];
    if (typeof path !== "string") return undefined;
    if (candidate["op"] === "remove") {
      operations.push({ op: "remove", path });
      continue;
    }
    if (candidate["op"] === "set") {
      operations.push({ op: "set", path, value: candidate["value"] });
      continue;
    }
    return undefined;
  }
  return operations;
};

const BOOKKEEPING_PATHS = ["/metadata/revision"];

const userVisible = (diff: ConfigDiff): ConfigDiff => ({
  changes: diff.changes.filter((change) => !BOOKKEEPING_PATHS.includes(change.path)),
});

const summarize = (proposal: Proposal): unknown => {
  const visible = userVisible(proposal.evaluation.diff);
  return {
    proposalId: proposal.id,
    intent: proposal.intent,
    status: proposal.status,
    requiresApproval: proposal.evaluation.requiresApproval,
    toRevision: proposal.evaluation.plan.toRevision,
    changeCount: visible.changes.length,
    diff: formatDiff(visible),
    review: proposal.evaluation.review,
    servicesAffected: proposal.evaluation.plan.services,
  };
};

export const dispatchTool = async (
  session: AgentSession,
  call: ToolCall,
): Promise<ToolResult> => {
  if (!isToolName(call.name)) {
    return fail("unknown-tool", `'${call.name}' is not a tool this agent may call`);
  }
  const args = call.arguments ?? {};

  switch (call.name) {
    case "get_config": {
      const section = stringArg(args, "section");
      const config = session.currentConfig();
      if (section === undefined) return ok(config);
      if (section === "metadata") return ok(config.metadata);
      const spec = config.spec as unknown as Record<string, unknown>;
      if (!(section in spec)) {
        return fail("invalid-arguments", `unknown section '${section}'`);
      }
      return ok(spec[section]);
    }

    case "describe_schema": {
      const path = stringArg(args, "path");
      const docs = docsFor(path);
      if (docs.length === 0) return fail("not-found", `no documented schema at '${path}'`);
      return ok(docs);
    }

    case "list_apps":
      return ok(
        session.currentConfig().spec.apps.entries.map((entry) => ({
          package: entry.package,
          label: entry.label ?? entry.package,
          state: entry.state,
          source: entry.source,
          network: entry.network,
        })),
      );

    case "propose_change": {
      const intent = stringArg(args, "intent");
      if (intent === undefined) return fail("invalid-arguments", "'intent' must be a string");
      const operations = parseOperations(args["operations"]);
      if (operations === undefined) {
        return fail("invalid-arguments", "'operations' must be an array of {op, path, value?}");
      }
      const proposal = session.propose(intent, operations);
      if (!proposal.ok) {
        return fail("proposal-rejected", describeRejection(proposal.error), proposal.error);
      }
      return ok(summarize(proposal.value));
    }

    case "preview_proposal": {
      const id = stringArg(args, "proposalId");
      if (id === undefined) return fail("invalid-arguments", "'proposalId' must be a string");
      const proposal = session.getProposal(id);
      if (proposal === undefined) return fail("not-found", `no proposal '${id}'`);
      return ok({ ...(summarize(proposal) as object), plan: formatPlan(proposal.evaluation.plan) });
    }

    case "apply_proposal": {
      const id = stringArg(args, "proposalId");
      if (id === undefined) return fail("invalid-arguments", "'proposalId' must be a string");
      const outcome = await session.apply(id);
      if (!outcome.ok) {
        const rejection = outcome.error;
        if (rejection.kind === "apply-failed") {
          return fail(
            "apply-failed",
            rejection.failure.deviceDirty
              ? `apply failed and rollback did not fully succeed: ${rejection.failure.error.message}`
              : `apply failed and was rolled back: ${rejection.failure.error.message}`,
            {
              rolledBack: rejection.failure.rolledBack,
              deviceDirty: rejection.failure.deviceDirty,
              transactionId: rejection.failure.transactionId,
            },
          );
        }
        return fail(rejection.kind, rejectionMessage(rejection));
      }
      return ok({
        proposalId: outcome.value.proposal.id,
        transactionId: outcome.value.report.transactionId,
        revision: outcome.value.report.plan.toRevision,
        servicesRestarted: outcome.value.report.plan.services,
      });
    }

    case "discard_proposal": {
      const id = stringArg(args, "proposalId");
      if (id === undefined) return fail("invalid-arguments", "'proposalId' must be a string");
      const discarded = session.discard(id);
      if (!discarded.ok) return fail(discarded.error.kind, rejectionMessage(discarded.error));
      return ok({ proposalId: discarded.value.id, status: discarded.value.status });
    }

    case "list_revisions":
      return ok(
        session.revisions().map((entry) => ({
          revision: entry.revision,
          appliedAt: entry.appliedAt,
          transactionId: entry.transactionId,
          intent: entry.intent,
        })),
      );

    case "propose_rollback": {
      const revision = args["revision"];
      if (typeof revision !== "number" || !Number.isInteger(revision)) {
        return fail("invalid-arguments", "'revision' must be an integer");
      }
      const proposal = session.proposeRollback(revision);
      if (!proposal.ok) {
        return fail("proposal-rejected", describeRejection(proposal.error), proposal.error);
      }
      return ok(summarize(proposal.value));
    }
  }
};

const rejectionMessage = (rejection: { readonly kind: string; readonly message?: string }): string =>
  rejection.message ?? rejection.kind;

const describeRejection = (rejection: {
  readonly kind: string;
  readonly issues?: readonly { readonly message: string }[];
  readonly message?: string;
}): string => {
  if (rejection.message !== undefined) return rejection.message;
  const issues = rejection.issues ?? [];
  const detail = issues.map((issue) => issue.message).join("; ");
  switch (rejection.kind) {
    case "policy":
      return `the agent tool boundary refused this change: ${detail}`;
    case "patch":
      return `the patch could not be applied: ${detail}`;
    case "validation":
      return `the result would not be a valid config: ${detail}`;
    case "plan":
      return `the change cannot be planned: ${detail}`;
    default:
      return detail;
  }
};

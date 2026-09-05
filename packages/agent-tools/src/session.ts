import {
  applyPlan,
  planApply,
  type ApplyFailure,
  type ApplyReport,
  type ConfigDiff,
  type EnginePorts,
  type Plan,
  type PlanIssue,
} from "@normal/engine";
import {
  err,
  ok,
  validateConfig,
  type NormalConfig,
  type Result,
  type ValidationIssue,
} from "@normal/schema";
import { applyPatch, type PatchError, type PatchOperation } from "./patch.js";
import { checkOperations, checkPolicy, type PolicyIssue } from "./policy.js";

export type ProposalRejection =
  | { readonly kind: "policy"; readonly issues: readonly PolicyIssue[] }
  | { readonly kind: "patch"; readonly issues: readonly PatchError[] }
  | { readonly kind: "validation"; readonly issues: readonly ValidationIssue[] }
  | { readonly kind: "plan"; readonly issues: readonly PlanIssue[] }
  | { readonly kind: "unknown-revision"; readonly message: string };

export type ProposalEvaluation = {
  readonly desired: NormalConfig;
  readonly diff: ConfigDiff;
  readonly plan: Plan;
  readonly review: readonly PolicyIssue[];
  readonly requiresApproval: boolean;
};

const withRevision = (config: NormalConfig, revision: number): NormalConfig => ({
  ...config,
  metadata: { ...config.metadata, revision },
});

export const evaluateOperations = (
  current: NormalConfig,
  operations: readonly PatchOperation[],
  options: { readonly now: string },
): Result<ProposalEvaluation, ProposalRejection> => {
  const denied = checkOperations(operations).filter((issue) => issue.severity === "deny");
  if (denied.length > 0) return err({ kind: "policy", issues: denied });

  const patched = applyPatch(current, operations);
  if (!patched.ok) return err({ kind: "patch", issues: patched.error });

  const validated = validateConfig(patched.value, { now: options.now });
  if (!validated.ok) return err({ kind: "validation", issues: validated.error });

  return evaluateConfig(current, validated.value, operations);
};

export const evaluateConfig = (
  current: NormalConfig,
  candidate: NormalConfig,
  operations: readonly PatchOperation[] = [],
): Result<ProposalEvaluation, ProposalRejection> => {
  const desired = withRevision(candidate, current.metadata.revision + 1);
  const plan = planApply(current, desired);
  if (!plan.ok) return err({ kind: "plan", issues: plan.error });

  const verdict = checkPolicy(operations, current, desired);
  if (verdict.denied.length > 0) return err({ kind: "policy", issues: verdict.denied });

  return ok({
    desired,
    diff: plan.value.diff,
    plan: plan.value,
    review: verdict.review,
    requiresApproval: verdict.requiresApproval,
  });
};

export type ProposalStatus = "pending" | "approved" | "applied" | "discarded" | "failed";

export type Proposal = {
  readonly id: string;
  readonly intent: string;
  readonly createdAt: string;
  readonly operations: readonly PatchOperation[];
  readonly evaluation: ProposalEvaluation;
  readonly status: ProposalStatus;
  readonly approvedBy?: string;
};

export type RevisionRecord = {
  readonly revision: number;
  readonly appliedAt: string;
  readonly transactionId: string;
  readonly intent: string;
  readonly config: NormalConfig;
};

export type ApplyOutcome = {
  readonly proposal: Proposal;
  readonly report: ApplyReport;
};

export type ApplyRejection =
  | { readonly kind: "unknown-proposal"; readonly message: string }
  | { readonly kind: "not-applicable"; readonly message: string }
  | { readonly kind: "approval-required"; readonly message: string }
  | { readonly kind: "stale"; readonly rejection: ProposalRejection }
  | { readonly kind: "apply-failed"; readonly failure: ApplyFailure };

export type AgentSessionOptions = {
  readonly initialConfig: NormalConfig;
  readonly ports: EnginePorts;
  readonly approvalRequiredForEverything?: boolean;
};

export type AgentSession = {
  readonly currentConfig: () => NormalConfig;
  readonly revisions: () => readonly RevisionRecord[];
  readonly proposals: () => readonly Proposal[];
  readonly getProposal: (id: string) => Proposal | undefined;
  readonly propose: (
    intent: string,
    operations: readonly PatchOperation[],
  ) => Result<Proposal, ProposalRejection>;
  readonly proposeRollback: (revision: number) => Result<Proposal, ProposalRejection>;
  readonly approve: (id: string, actor: string) => Result<Proposal, ApplyRejection>;
  readonly discard: (id: string) => Result<Proposal, ApplyRejection>;
  readonly apply: (id: string) => Promise<Result<ApplyOutcome, ApplyRejection>>;
};

export const createAgentSession = (options: AgentSessionOptions): AgentSession => {
  const { ports } = options;
  const alwaysApprove = options.approvalRequiredForEverything ?? true;
  let current = options.initialConfig;
  let counter = 0;
  const proposals = new Map<string, Proposal>();
  const history: RevisionRecord[] = [
    {
      revision: current.metadata.revision,
      appliedAt: ports.clock.now(),
      transactionId: "genesis",
      intent: "initial configuration",
      config: current,
    },
  ];

  const record = (
    intent: string,
    operations: readonly PatchOperation[],
    evaluation: ProposalEvaluation,
  ): Proposal => {
    counter += 1;
    const proposal: Proposal = {
      id: `proposal-${String(counter).padStart(4, "0")}`,
      intent,
      createdAt: ports.clock.now(),
      operations,
      evaluation: alwaysApprove ? { ...evaluation, requiresApproval: true } : evaluation,
      status: "pending",
    };
    proposals.set(proposal.id, proposal);
    return proposal;
  };

  const update = (proposal: Proposal, patch: Partial<Proposal>): Proposal => {
    const next = { ...proposal, ...patch };
    proposals.set(next.id, next);
    return next;
  };

  return {
    currentConfig: () => current,
    revisions: () => [...history],
    proposals: () => [...proposals.values()],
    getProposal: (id) => proposals.get(id),

    propose: (intent, operations) => {
      const evaluation = evaluateOperations(current, operations, { now: ports.clock.now() });
      return evaluation.ok ? ok(record(intent, operations, evaluation.value)) : evaluation;
    },

    proposeRollback: (revision) => {
      const target = history.find((entry) => entry.revision === revision);
      if (target === undefined) {
        return err({ kind: "unknown-revision", message: `no applied revision ${revision}` });
      }
      const evaluation = evaluateConfig(current, target.config);
      if (!evaluation.ok) return evaluation;
      return ok(
        record(`roll back to revision ${revision}`, [], {
          ...evaluation.value,
          requiresApproval: true,
        }),
      );
    },

    approve: (id, actor) => {
      const proposal = proposals.get(id);
      if (proposal === undefined) {
        return err({ kind: "unknown-proposal", message: `no proposal '${id}'` });
      }
      if (proposal.status !== "pending") {
        return err({
          kind: "not-applicable",
          message: `proposal '${id}' is ${proposal.status}`,
        });
      }
      return ok(update(proposal, { status: "approved", approvedBy: actor }));
    },

    discard: (id) => {
      const proposal = proposals.get(id);
      if (proposal === undefined) {
        return err({ kind: "unknown-proposal", message: `no proposal '${id}'` });
      }
      if (proposal.status === "applied") {
        return err({ kind: "not-applicable", message: "an applied proposal cannot be discarded" });
      }
      return ok(update(proposal, { status: "discarded" }));
    },

    apply: async (id) => {
      const proposal = proposals.get(id);
      if (proposal === undefined) {
        return err({ kind: "unknown-proposal", message: `no proposal '${id}'` });
      }
      if (proposal.status === "discarded" || proposal.status === "applied") {
        return err({ kind: "not-applicable", message: `proposal '${id}' is ${proposal.status}` });
      }
      if (proposal.evaluation.requiresApproval && proposal.status !== "approved") {
        return err({
          kind: "approval-required",
          message: `proposal '${id}' changes policy the user must confirm before it is applied`,
        });
      }

      const fresh =
        proposal.operations.length > 0
          ? evaluateOperations(current, proposal.operations, { now: ports.clock.now() })
          : evaluateConfig(current, proposal.evaluation.desired);
      if (!fresh.ok) return err({ kind: "stale", rejection: fresh.error });

      const applied = await applyPlan(fresh.value.plan, ports);
      if (!applied.ok) {
        update(proposal, { status: "failed" });
        return err({ kind: "apply-failed", failure: applied.error });
      }

      current = fresh.value.desired;
      history.push({
        revision: current.metadata.revision,
        appliedAt: applied.value.finishedAt,
        transactionId: applied.value.transactionId,
        intent: proposal.intent,
        config: current,
      });
      return ok({
        proposal: update(proposal, { status: "applied", evaluation: fresh.value }),
        report: applied.value,
      });
    },
  };
};

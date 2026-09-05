import type { NormalConfig } from "@normal/schema";
import type { PatchOperation } from "./patch.js";

export type PolicyIssue = {
  readonly path: string;
  readonly code:
    | "path-not-writable"
    | "engine-managed-field"
    | "too-many-operations"
    | "weakens-attention-policy"
    | "sensitive-path";
  readonly severity: "deny" | "review";
  readonly message: string;
};

export type PolicyVerdict = {
  readonly denied: readonly PolicyIssue[];
  readonly review: readonly PolicyIssue[];
  readonly requiresApproval: boolean;
};

export const MAX_OPERATIONS = 64;

const WRITABLE_ROOTS = ["/spec", "/metadata/description", "/metadata/labels"];

const ENGINE_MANAGED = ["/apiVersion", "/kind", "/metadata/name", "/metadata/revision"];

const SENSITIVE_PATTERNS = [
  /^\/spec\/attention(\/|$)/,
  /^\/spec\/apps\/entries\/[^/]+\/(permissions|network|state)(\/|$)/,
  /^\/spec\/apps\/policy$/,
];

const covers = (root: string, path: string): boolean => path === root || path.startsWith(`${root}/`);

const isWritable = (path: string): boolean => WRITABLE_ROOTS.some((root) => covers(root, path));

const isEngineManaged = (path: string): boolean =>
  ENGINE_MANAGED.some((root) => covers(root, path));

const isSensitive = (path: string): boolean =>
  SENSITIVE_PATTERNS.some((pattern) => pattern.test(path));

const ENFORCEMENT_STRENGTH: Readonly<Record<string, number>> = {
  warn: 1,
  paginate: 2,
  block: 3,
};

const weakenings = (before: NormalConfig, after: NormalConfig): PolicyIssue[] => {
  const issues: PolicyIssue[] = [];
  const from = before.spec.attention.infiniteScroll;
  const to = after.spec.attention.infiniteScroll;

  const fromStrength = ENFORCEMENT_STRENGTH[from.enforcement] ?? 0;
  const toStrength = ENFORCEMENT_STRENGTH[to.enforcement] ?? 0;
  if (toStrength < fromStrength) {
    issues.push({
      path: "/spec/attention/infiniteScroll/enforcement",
      code: "weakens-attention-policy",
      severity: "review",
      message: `enforcement drops from '${from.enforcement}' to '${to.enforcement}'`,
    });
  }
  if (to.maxAutoLoads > from.maxAutoLoads) {
    issues.push({
      path: "/spec/attention/infiniteScroll/maxAutoLoads",
      code: "weakens-attention-policy",
      severity: "review",
      message: `automatic loads per session rise from ${from.maxAutoLoads} to ${to.maxAutoLoads}`,
    });
  }
  if (to.pageSize > from.pageSize) {
    issues.push({
      path: "/spec/attention/infiniteScroll/pageSize",
      code: "weakens-attention-policy",
      severity: "review",
      message: `page size grows from ${from.pageSize} to ${to.pageSize}`,
    });
  }
  const beforeExempt = new Set(from.exemptions.map((exemption) => exemption.package));
  const added = to.exemptions.filter((exemption) => !beforeExempt.has(exemption.package));
  for (const exemption of added) {
    issues.push({
      path: "/spec/attention/infiniteScroll/exemptions",
      code: "weakens-attention-policy",
      severity: "review",
      message: `adds a scroll exemption for '${exemption.package}' until ${exemption.expiresAt}`,
    });
  }
  if (from.webview.interceptIntersectionObserver && !to.webview.interceptIntersectionObserver) {
    issues.push({
      path: "/spec/attention/infiniteScroll/webview/interceptIntersectionObserver",
      code: "weakens-attention-policy",
      severity: "review",
      message: "stops intercepting the sentinel pattern web feeds use to auto-load",
    });
  }
  const beforeBudgets = new Map(
    before.spec.attention.sessionBudgets.map((budget) => [budget.id, budget]),
  );
  for (const budget of after.spec.attention.sessionBudgets) {
    const previous = beforeBudgets.get(budget.id);
    if (previous !== undefined && budget.dailyMinutes > previous.dailyMinutes) {
      issues.push({
        path: `/spec/attention/sessionBudgets/${budget.id}/dailyMinutes`,
        code: "weakens-attention-policy",
        severity: "review",
        message: `daily budget rises from ${previous.dailyMinutes} to ${budget.dailyMinutes} minutes`,
      });
    }
  }
  return issues;
};

export const checkOperationCount = (operations: readonly PatchOperation[]): PolicyIssue[] => {
  if (operations.length === 0) {
    return [
      {
        path: "/",
        code: "too-many-operations",
        severity: "deny",
        message: "a proposal must contain at least one operation",
      },
    ];
  }
  if (operations.length > MAX_OPERATIONS) {
    return [
      {
        path: "/",
        code: "too-many-operations",
        severity: "deny",
        message: `a proposal may contain at most ${MAX_OPERATIONS} operations`,
      },
    ];
  }
  return [];
};

export const checkOperationPaths = (operations: readonly PatchOperation[]): PolicyIssue[] => {
  const issues: PolicyIssue[] = [];
  for (const operation of operations) {
    if (isEngineManaged(operation.path)) {
      issues.push({
        path: operation.path,
        code: "engine-managed-field",
        severity: "deny",
        message: "this field is managed by the mutation engine and cannot be set by an agent",
      });
      continue;
    }
    if (!isWritable(operation.path)) {
      issues.push({
        path: operation.path,
        code: "path-not-writable",
        severity: "deny",
        message: `agents may only write under ${WRITABLE_ROOTS.join(", ")}`,
      });
      continue;
    }
    if (isSensitive(operation.path)) {
      issues.push({
        path: operation.path,
        code: "sensitive-path",
        severity: "review",
        message: "this path changes attention or app-permission policy and needs explicit approval",
      });
    }
  }
  return issues;
};

export const checkOperations = (operations: readonly PatchOperation[]): PolicyIssue[] => [
  ...checkOperationCount(operations),
  ...checkOperationPaths(operations),
];

export const checkPolicy = (
  operations: readonly PatchOperation[],
  before: NormalConfig,
  after: NormalConfig,
): PolicyVerdict => {
  const issues = [...checkOperationPaths(operations), ...weakenings(before, after)];
  const denied = issues.filter((issue) => issue.severity === "deny");
  const review = issues.filter((issue) => issue.severity === "review");
  return { denied, review, requiresApproval: review.length > 0 };
};

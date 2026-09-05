import { err, ok, type NormalConfig, type Result } from "@normal/schema";
import { diffConfig, type Change, type ConfigDiff } from "./diff.js";
import { isOwnedPath, servicesForFiles, type ServiceName } from "./ownership.js";
import { renderConfig, type FileSet } from "./render.js";

export type FileAction =
  | { readonly kind: "write-file"; readonly path: string; readonly contents: string }
  | { readonly kind: "delete-file"; readonly path: string };

export type Action = FileAction | { readonly kind: "restart-service"; readonly service: ServiceName };

export type HealthCheck = {
  readonly service: ServiceName;
};

export type Plan = {
  readonly fromRevision: number;
  readonly toRevision: number;
  readonly diff: ConfigDiff;
  readonly actions: readonly Action[];
  readonly services: readonly ServiceName[];
  readonly checks: readonly HealthCheck[];
};

export type PlanIssue = {
  readonly path: string;
  readonly code: "immutable-field" | "unowned-path" | "stale-revision";
  readonly message: string;
};

const IMMUTABLE_PATHS = ["/apiVersion", "/kind"];

const diffFileSets = (before: FileSet, after: FileSet): FileAction[] => {
  const paths = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
  const writes: FileAction[] = [];
  const deletes: FileAction[] = [];
  for (const path of paths) {
    const previous = before[path];
    const next = after[path];
    if (next === undefined) {
      deletes.push({ kind: "delete-file", path });
      continue;
    }
    if (previous !== next) writes.push({ kind: "write-file", path, contents: next });
  }
  return [...writes, ...deletes];
};

const issuesFor = (changes: readonly Change[]): PlanIssue[] =>
  changes.flatMap((change): PlanIssue[] => {
    if (IMMUTABLE_PATHS.some((path) => change.path === path || change.path.startsWith(`${path}/`))) {
      return [
        {
          path: change.path,
          code: "immutable-field",
          message: "apiVersion and kind cannot be changed by a mutation; migrate the config instead",
        },
      ];
    }
    if (!isOwnedPath(change.path)) {
      return [
        {
          path: change.path,
          code: "unowned-path",
          message: "no service owns this path, so the engine cannot apply it",
        },
      ];
    }
    return [];
  });

export const planApply = (
  current: NormalConfig,
  desired: NormalConfig,
): Result<Plan, PlanIssue[]> => {
  const diff = diffConfig(current, desired);

  if (diff.changes.length === 0) {
    return ok({
      fromRevision: current.metadata.revision,
      toRevision: desired.metadata.revision,
      diff,
      actions: [],
      services: [],
      checks: [],
    });
  }

  const issues = issuesFor(diff.changes);
  if (desired.metadata.revision <= current.metadata.revision) {
    issues.push({
      path: "/metadata/revision",
      code: "stale-revision",
      message: `revision must advance past ${current.metadata.revision}`,
    });
  }
  if (issues.length > 0) return err(issues);

  const fileActions = diffFileSets(renderConfig(current), renderConfig(desired));
  const services = servicesForFiles(fileActions.map((action) => action.path));
  const restarts: Action[] = services.map((service) => ({ kind: "restart-service", service }));

  return ok({
    fromRevision: current.metadata.revision,
    toRevision: desired.metadata.revision,
    diff,
    actions: [...fileActions, ...restarts],
    services,
    checks: services.map((service) => ({ service })),
  });
};

export const isNoOp = (plan: Plan): boolean => plan.actions.length === 0;

export const nextRevision = (current: NormalConfig, desired: NormalConfig): NormalConfig => ({
  ...desired,
  metadata: { ...desired.metadata, revision: current.metadata.revision + 1 },
});

import { err, ok, type NormalConfig, type Result } from "@normal/schema";
import { planApply, type Action, type Plan, type PlanIssue } from "./plan.js";
import type { EnginePorts, IoError } from "./ports.js";

export type StepStatus = "applied" | "failed";

export type StepRecord = {
  readonly index: number;
  readonly action: Action;
  readonly status: StepStatus;
};

export type FileSnapshot = {
  readonly path: string;
  readonly contents: string | null;
};

export type ApplyReport = {
  readonly transactionId: string;
  readonly startedAt: string;
  readonly finishedAt: string;
  readonly plan: Plan;
  readonly steps: readonly StepRecord[];
};

export type ApplyFailure = {
  readonly transactionId: string;
  readonly startedAt: string;
  readonly finishedAt: string;
  readonly plan: Plan;
  readonly steps: readonly StepRecord[];
  readonly failedAction: Action | null;
  readonly error: IoError;
  readonly rolledBack: boolean;
  readonly rollbackErrors: readonly IoError[];
  readonly deviceDirty: boolean;
};

const filePaths = (plan: Plan): readonly string[] => [
  ...new Set(
    plan.actions.flatMap((action) => (action.kind === "restart-service" ? [] : [action.path])),
  ),
];

const capture = async (
  plan: Plan,
  ports: EnginePorts,
): Promise<Result<readonly FileSnapshot[], IoError>> => {
  const snapshots: FileSnapshot[] = [];
  for (const path of filePaths(plan)) {
    const exists = await ports.fs.exists(path);
    if (!exists.ok) return err(exists.error);
    if (!exists.value) {
      snapshots.push({ path, contents: null });
      continue;
    }
    const read = await ports.fs.read(path);
    if (!read.ok) return err(read.error);
    snapshots.push({ path, contents: read.value });
  }
  return ok(snapshots);
};

const runAction = async (action: Action, ports: EnginePorts): Promise<Result<void, IoError>> => {
  switch (action.kind) {
    case "write-file":
      return ports.fs.write(action.path, action.contents);
    case "delete-file":
      return ports.fs.remove(action.path);
    case "restart-service":
      return ports.services.restart(action.service);
  }
};

const verify = async (plan: Plan, ports: EnginePorts): Promise<Result<void, IoError>> => {
  for (const check of plan.checks) {
    const status = await ports.services.status(check.service);
    if (!status.ok) return err(status.error);
    if (status.value !== "running") {
      return err({
        code: "unavailable",
        target: check.service,
        message: `service is ${status.value} after restart`,
      });
    }
  }
  return ok(undefined);
};

const restore = async (
  snapshots: readonly FileSnapshot[],
  plan: Plan,
  ports: EnginePorts,
): Promise<readonly IoError[]> => {
  const errors: IoError[] = [];
  for (const snapshot of snapshots) {
    const result =
      snapshot.contents === null
        ? await ports.fs.remove(snapshot.path)
        : await ports.fs.write(snapshot.path, snapshot.contents);
    if (!result.ok && !(snapshot.contents === null && result.error.code === "not-found")) {
      errors.push(result.error);
    }
  }
  for (const service of plan.services) {
    const result = await ports.services.restart(service);
    if (!result.ok) errors.push(result.error);
  }
  return errors;
};

export const applyPlan = async (
  plan: Plan,
  ports: EnginePorts,
): Promise<Result<ApplyReport, ApplyFailure>> => {
  const transactionId = ports.clock.nextId();
  const startedAt = ports.clock.now();
  const log = (kind: string, detail: string): void =>
    ports.logger?.log({ transactionId, at: ports.clock.now(), kind, detail });

  log("transaction-start", `${plan.actions.length} actions, revision ${plan.fromRevision} -> ${plan.toRevision}`);

  if (plan.actions.length === 0) {
    log("transaction-noop", "nothing to apply");
    return ok({ transactionId, startedAt, finishedAt: ports.clock.now(), plan, steps: [] });
  }

  const snapshot = await capture(plan, ports);
  if (!snapshot.ok) {
    log("capture-failed", snapshot.error.message);
    return err({
      transactionId,
      startedAt,
      finishedAt: ports.clock.now(),
      plan,
      steps: [],
      failedAction: null,
      error: snapshot.error,
      rolledBack: true,
      rollbackErrors: [],
      deviceDirty: false,
    });
  }

  const steps: StepRecord[] = [];
  const fail = async (failedAction: Action | null, error: IoError): Promise<Result<never, ApplyFailure>> => {
    log("rollback-start", error.message);
    const rollbackErrors = await restore(snapshot.value, plan, ports);
    const rolledBack = rollbackErrors.length === 0;
    log(rolledBack ? "rollback-complete" : "rollback-failed", `${rollbackErrors.length} rollback errors`);
    return err({
      transactionId,
      startedAt,
      finishedAt: ports.clock.now(),
      plan,
      steps,
      failedAction,
      error,
      rolledBack,
      rollbackErrors,
      deviceDirty: !rolledBack,
    });
  };

  for (const [index, action] of plan.actions.entries()) {
    const result = await runAction(action, ports);
    if (!result.ok) {
      steps.push({ index, action, status: "failed" });
      log("step-failed", `${action.kind} #${index}: ${result.error.message}`);
      return fail(action, result.error);
    }
    steps.push({ index, action, status: "applied" });
    log("step-applied", `${action.kind} #${index}`);
  }

  const verified = await verify(plan, ports);
  if (!verified.ok) {
    log("verify-failed", verified.error.message);
    return fail(null, verified.error);
  }

  log("transaction-commit", `revision ${plan.toRevision}`);
  return ok({ transactionId, startedAt, finishedAt: ports.clock.now(), plan, steps });
};

export type ApplyConfigError =
  | { readonly kind: "plan"; readonly issues: readonly PlanIssue[] }
  | { readonly kind: "apply"; readonly failure: ApplyFailure };

export const applyConfig = async (
  current: NormalConfig,
  desired: NormalConfig,
  ports: EnginePorts,
): Promise<Result<ApplyReport, ApplyConfigError>> => {
  const plan = planApply(current, desired);
  if (!plan.ok) return err({ kind: "plan", issues: plan.error });
  const applied = await applyPlan(plan.value, ports);
  return applied.ok ? applied : err({ kind: "apply", failure: applied.error });
};

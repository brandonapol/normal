import type { Change, ConfigDiff } from "./diff.js";
import type { Plan } from "./plan.js";

const preview = (value: unknown): string => {
  if (value === undefined) return "∅";
  const text = typeof value === "string" ? value : JSON.stringify(value);
  return text.length > 120 ? `${text.slice(0, 117)}...` : text;
};

export const formatChange = (change: Change): string => {
  switch (change.op) {
    case "add":
      return `+ ${change.path} = ${preview(change.after)}`;
    case "remove":
      return `- ${change.path} (was ${preview(change.before)})`;
    case "replace":
      return `~ ${change.path}: ${preview(change.before)} -> ${preview(change.after)}`;
  }
};

export const formatDiff = (diff: ConfigDiff): string =>
  diff.changes.length === 0 ? "(no changes)" : diff.changes.map(formatChange).join("\n");

export const formatPlan = (plan: Plan): string => {
  const header = `revision ${plan.fromRevision} -> ${plan.toRevision}`;
  const files = plan.actions
    .filter((action) => action.kind !== "restart-service")
    .map((action) => `  ${action.kind === "write-file" ? "write" : "delete"} ${action.path}`);
  const services = plan.services.map((service) => `  restart ${service}`);
  return [header, formatDiff(plan.diff), "actions:", ...files, ...services].join("\n");
};

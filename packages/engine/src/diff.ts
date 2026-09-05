import { formatPointer, keyFieldFor } from "@normal/schema";

export type ChangeOp = "add" | "replace" | "remove";

export type Change = {
  readonly op: ChangeOp;
  readonly path: string;
  readonly before: unknown;
  readonly after: unknown;
};

export type ConfigDiff = {
  readonly changes: readonly Change[];
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

export const deepEqual = (a: unknown, b: unknown): boolean => {
  if (a === b) return true;
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((item, index) => deepEqual(item, b[index]));
  }
  if (isRecord(a) && isRecord(b)) {
    const aKeys = Object.keys(a).filter((key) => a[key] !== undefined);
    const bKeys = Object.keys(b).filter((key) => b[key] !== undefined);
    if (aKeys.length !== bKeys.length) return false;
    return aKeys.every((key) => key in b && deepEqual(a[key], b[key]));
  }
  return false;
};

const keyOf = (value: unknown, keyField: string): string | undefined =>
  isRecord(value) && typeof value[keyField] === "string" ? (value[keyField] as string) : undefined;

const diffKeyedArray = (
  segments: readonly string[],
  pattern: string,
  keyField: string,
  before: readonly unknown[],
  after: readonly unknown[],
): Change[] => {
  const beforeByKey = new Map<string, unknown>();
  const afterByKey = new Map<string, unknown>();
  const unkeyed: Change[] = [];

  before.forEach((item) => {
    const key = keyOf(item, keyField);
    if (key !== undefined) beforeByKey.set(key, item);
  });
  after.forEach((item) => {
    const key = keyOf(item, keyField);
    if (key !== undefined) afterByKey.set(key, item);
  });

  if (beforeByKey.size !== before.length || afterByKey.size !== after.length) {
    return [
      {
        op: "replace",
        path: formatPointer(segments),
        before,
        after,
      },
    ];
  }

  const keys = [...new Set([...beforeByKey.keys(), ...afterByKey.keys()])];
  const changes = keys.flatMap((key) => {
    const nextSegments = [...segments, key];
    const nextPattern = `${pattern}/*`;
    if (!beforeByKey.has(key)) {
      return [{ op: "add" as const, path: formatPointer(nextSegments), before: undefined, after: afterByKey.get(key) }];
    }
    if (!afterByKey.has(key)) {
      return [
        { op: "remove" as const, path: formatPointer(nextSegments), before: beforeByKey.get(key), after: undefined },
      ];
    }
    return diffValue(nextSegments, nextPattern, beforeByKey.get(key), afterByKey.get(key));
  });

  const beforeOrder = before.map((item) => keyOf(item, keyField));
  const afterOrder = after.map((item) => keyOf(item, keyField));
  const survivingBefore = beforeOrder.filter((key) => key !== undefined && afterByKey.has(key));
  const survivingAfter = afterOrder.filter((key) => key !== undefined && beforeByKey.has(key));
  if (!deepEqual(survivingBefore, survivingAfter)) {
    unkeyed.push({
      op: "replace",
      path: formatPointer([...segments, "$order"]),
      before: beforeOrder,
      after: afterOrder,
    });
  }

  return [...changes, ...unkeyed];
};

export const diffValue = (
  segments: readonly string[],
  pattern: string,
  before: unknown,
  after: unknown,
): Change[] => {
  if (deepEqual(before, after)) return [];

  if (Array.isArray(before) && Array.isArray(after)) {
    const keyField = keyFieldFor(pattern);
    if (keyField !== undefined) {
      return diffKeyedArray(segments, pattern, keyField, before, after);
    }
    return [{ op: "replace", path: formatPointer(segments), before, after }];
  }

  if (isRecord(before) && isRecord(after)) {
    const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort();
    return keys.flatMap((key) => {
      const nextSegments = [...segments, key];
      const nextPattern = `${pattern}/${key}`;
      const beforeValue = before[key];
      const afterValue = after[key];
      if (beforeValue === undefined && afterValue !== undefined) {
        return [{ op: "add" as const, path: formatPointer(nextSegments), before: undefined, after: afterValue }];
      }
      if (beforeValue !== undefined && afterValue === undefined) {
        return [
          { op: "remove" as const, path: formatPointer(nextSegments), before: beforeValue, after: undefined },
        ];
      }
      return diffValue(nextSegments, nextPattern, beforeValue, afterValue);
    });
  }

  return [{ op: "replace", path: formatPointer(segments), before, after }];
};

export const diffConfig = (before: unknown, after: unknown): ConfigDiff => ({
  changes: diffValue([], "", before, after),
});

export const isEmptyDiff = (diff: ConfigDiff): boolean => diff.changes.length === 0;

export const changedPaths = (diff: ConfigDiff): readonly string[] =>
  diff.changes.map((change) => change.path);

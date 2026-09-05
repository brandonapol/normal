import { keyFieldFor } from "./keys.js";
import { err, ok, type Result } from "./result.js";

export type PathSegment = string;

export type PointerError = {
  readonly code: "not-found" | "not-traversable" | "invalid-pointer" | "ambiguous";
  readonly pointer: string;
  readonly message: string;
};

const unescapeSegment = (segment: string): string => segment.replace(/~1/g, "/").replace(/~0/g, "~");
const escapeSegment = (segment: string): string => segment.replace(/~/g, "~0").replace(/\//g, "~1");

export const parsePointer = (pointer: string): Result<readonly PathSegment[], PointerError> => {
  if (pointer === "") return ok([]);
  if (!pointer.startsWith("/")) {
    return err({ code: "invalid-pointer", pointer, message: "pointer must start with '/'" });
  }
  return ok(pointer.slice(1).split("/").map(unescapeSegment));
};

export const formatPointer = (segments: readonly PathSegment[]): string =>
  segments.length === 0 ? "" : `/${segments.map(escapeSegment).join("/")}`;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const indexInKeyedArray = (
  array: readonly unknown[],
  keyField: string,
  segment: string,
): number => array.findIndex((item) => isRecord(item) && item[keyField] === segment);

const resolveIndex = (
  array: readonly unknown[],
  pattern: string,
  segment: string,
): number => {
  const keyField = keyFieldFor(pattern);
  if (keyField !== undefined) {
    const byKey = indexInKeyedArray(array, keyField, segment);
    if (byKey >= 0) return byKey;
  }
  if (/^\d+$/.test(segment)) {
    const index = Number(segment);
    return index < array.length ? index : -1;
  }
  return -1;
};

const patternOf = (walked: readonly { segment: string; inArray: boolean }[]): string =>
  walked.length === 0 ? "" : `/${walked.map((s) => (s.inArray ? "*" : s.segment)).join("/")}`;

type Walked = { segment: string; inArray: boolean };

export const getAtPath = (
  root: unknown,
  segments: readonly PathSegment[],
): Result<unknown, PointerError> => {
  const walked: Walked[] = [];
  let current: unknown = root;
  for (const segment of segments) {
    if (Array.isArray(current)) {
      const index = resolveIndex(current, patternOf(walked), segment);
      if (index < 0) {
        return err({
          code: "not-found",
          pointer: formatPointer([...walked.map((w) => w.segment), segment]),
          message: `no element matching '${segment}'`,
        });
      }
      current = current[index];
      walked.push({ segment, inArray: true });
      continue;
    }
    if (!isRecord(current) || !(segment in current)) {
      return err({
        code: isRecord(current) ? "not-found" : "not-traversable",
        pointer: formatPointer([...walked.map((w) => w.segment), segment]),
        message: `cannot read '${segment}'`,
      });
    }
    current = current[segment];
    walked.push({ segment, inArray: false });
  }
  return ok(current);
};

type Mutation = { readonly kind: "set"; readonly value: unknown } | { readonly kind: "remove" };

const applyMutation = (
  container: unknown,
  segments: readonly PathSegment[],
  walked: readonly Walked[],
  mutation: Mutation,
): Result<unknown, PointerError> => {
  const [segment, ...rest] = segments;
  if (segment === undefined) {
    return mutation.kind === "set" ? ok(mutation.value) : err({
      code: "invalid-pointer",
      pointer: formatPointer(walked.map((w) => w.segment)),
      message: "cannot remove the document root",
    });
  }

  if (Array.isArray(container)) {
    const pattern = patternOf(walked);
    const index = resolveIndex(container, pattern, segment);
    if (index < 0) {
      if (rest.length > 0 || mutation.kind === "remove") {
        return err({
          code: "not-found",
          pointer: formatPointer([...walked.map((w) => w.segment), segment]),
          message: `no element matching '${segment}'`,
        });
      }
      return ok([...container, mutation.value]);
    }
    if (rest.length === 0 && mutation.kind === "remove") {
      return ok(container.filter((_, i) => i !== index));
    }
    const child = applyMutation(container[index], rest, [...walked, { segment, inArray: true }], mutation);
    if (!child.ok) return child;
    return ok(container.map((item, i) => (i === index ? child.value : item)));
  }

  if (!isRecord(container)) {
    return err({
      code: "not-traversable",
      pointer: formatPointer([...walked.map((w) => w.segment), segment]),
      message: `cannot traverse into '${segment}'`,
    });
  }

  if (rest.length === 0 && mutation.kind === "remove") {
    if (!(segment in container)) {
      return err({
        code: "not-found",
        pointer: formatPointer([...walked.map((w) => w.segment), segment]),
        message: `no property '${segment}'`,
      });
    }
    const next: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(container)) if (k !== segment) next[k] = v;
    return ok(next);
  }

  if (rest.length > 0 && !(segment in container)) {
    return err({
      code: "not-found",
      pointer: formatPointer([...walked.map((w) => w.segment), segment]),
      message: `no property '${segment}'`,
    });
  }

  const child = applyMutation(container[segment], rest, [...walked, { segment, inArray: false }], mutation);
  if (!child.ok) return child;
  return ok({ ...container, [segment]: child.value });
};

export const setAtPath = (
  root: unknown,
  segments: readonly PathSegment[],
  value: unknown,
): Result<unknown, PointerError> => applyMutation(root, segments, [], { kind: "set", value });

export const removeAtPath = (
  root: unknown,
  segments: readonly PathSegment[],
): Result<unknown, PointerError> => applyMutation(root, segments, [], { kind: "remove" });

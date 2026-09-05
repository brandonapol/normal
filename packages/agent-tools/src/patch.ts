import {
  err,
  ok,
  parsePointer,
  removeAtPath,
  setAtPath,
  validateConfig,
  type NormalConfig,
  type Result,
  type ValidationIssue,
} from "@normal/schema";

export type PatchOperation =
  | { readonly op: "set"; readonly path: string; readonly value: unknown }
  | { readonly op: "remove"; readonly path: string };

export type PatchError = {
  readonly path: string;
  readonly code: string;
  readonly message: string;
};

export const applyPatch = (
  config: NormalConfig,
  operations: readonly PatchOperation[],
): Result<unknown, PatchError[]> => {
  let draft: unknown = config;
  for (const [index, operation] of operations.entries()) {
    const segments = parsePointer(operation.path);
    if (!segments.ok) {
      return err([{ path: operation.path, code: "invalid-pointer", message: segments.error.message }]);
    }
    const next =
      operation.op === "set"
        ? setAtPath(draft, segments.value, operation.value)
        : removeAtPath(draft, segments.value);
    if (!next.ok) {
      return err([
        {
          path: operation.path,
          code: next.error.code,
          message: `operation ${index} (${operation.op}) failed: ${next.error.message}`,
        },
      ]);
    }
    draft = next.value;
  }
  return ok(draft);
};

export const patchAndValidate = (
  config: NormalConfig,
  operations: readonly PatchOperation[],
  now: string,
): Result<NormalConfig, { readonly patch: PatchError[]; readonly validation: ValidationIssue[] }> => {
  const patched = applyPatch(config, operations);
  if (!patched.ok) return err({ patch: patched.error, validation: [] });
  const validated = validateConfig(patched.value, { now });
  if (!validated.ok) return err({ patch: [], validation: validated.error });
  return ok(validated.value);
};

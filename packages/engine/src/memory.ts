import { err, ok, type Result } from "@normal/schema";
import type {
  ClockPort,
  EnginePorts,
  FileSystemPort,
  IoError,
  LogEvent,
  LoggerPort,
  ServiceHostPort,
  ServiceState,
} from "./ports.js";

export type FaultKind = "read" | "write" | "remove" | "restart" | "status";

export type Fault = {
  readonly kind: FaultKind;
  readonly target: string;
  readonly error: IoError;
  readonly times?: number;
};

export type MemoryFileSystem = FileSystemPort & {
  readonly snapshot: () => Readonly<Record<string, string>>;
};

export type MemoryServiceHost = ServiceHostPort & {
  readonly restarts: () => readonly string[];
  readonly setState: (service: string, state: ServiceState) => void;
};

const matchFault = (
  faults: Fault[],
  kind: FaultKind,
  target: string,
): IoError | undefined => {
  const index = faults.findIndex((fault) => fault.kind === kind && fault.target === target);
  if (index < 0) return undefined;
  const fault = faults[index]!;
  const remaining = fault.times === undefined ? undefined : fault.times - 1;
  if (remaining !== undefined && remaining <= 0) faults.splice(index, 1);
  else if (remaining !== undefined) faults[index] = { ...fault, times: remaining };
  return fault.error;
};

export const createMemoryFileSystem = (
  seed: Readonly<Record<string, string>> = {},
  faults: Fault[] = [],
): MemoryFileSystem => {
  const files = new Map<string, string>(Object.entries(seed));
  const fail = (kind: FaultKind, target: string): Result<never, IoError> | undefined => {
    const error = matchFault(faults, kind, target);
    return error === undefined ? undefined : err(error);
  };

  return {
    read: async (path) => {
      const failure = fail("read", path);
      if (failure) return failure;
      const contents = files.get(path);
      return contents === undefined
        ? err<IoError>({ code: "not-found", target: path, message: "no such file" })
        : ok(contents);
    },
    write: async (path, contents) => {
      const failure = fail("write", path);
      if (failure) return failure;
      files.set(path, contents);
      return ok(undefined);
    },
    remove: async (path) => {
      const failure = fail("remove", path);
      if (failure) return failure;
      if (!files.delete(path)) {
        return err<IoError>({ code: "not-found", target: path, message: "no such file" });
      }
      return ok(undefined);
    },
    exists: async (path) => {
      const failure = fail("read", path);
      if (failure) return failure;
      return ok(files.has(path));
    },
    snapshot: () => Object.fromEntries([...files.entries()].sort(([a], [b]) => a.localeCompare(b))),
  };
};

export const createMemoryServiceHost = (
  initial: Readonly<Record<string, ServiceState>> = {},
  faults: Fault[] = [],
): MemoryServiceHost => {
  const states = new Map<string, ServiceState>(Object.entries(initial));
  const restarted: string[] = [];
  const fail = (kind: FaultKind, target: string): Result<never, IoError> | undefined => {
    const error = matchFault(faults, kind, target);
    return error === undefined ? undefined : err(error);
  };

  return {
    restart: async (service) => {
      const failure = fail("restart", service);
      if (failure) {
        states.set(service, "failed");
        restarted.push(service);
        return failure;
      }
      restarted.push(service);
      states.set(service, "running");
      return ok(undefined);
    },
    status: async (service) => {
      const failure = fail("status", service);
      if (failure) return failure;
      return ok(states.get(service) ?? "running");
    },
    restarts: () => [...restarted],
    setState: (service, state) => {
      states.set(service, state);
    },
  };
};

export const createStubClock = (start = "2026-01-01T00:00:00.000Z", prefix = "txn"): ClockPort => {
  let counter = 0;
  return {
    now: () => new Date(Date.parse(start) + counter * 1000).toISOString(),
    nextId: () => {
      counter += 1;
      return `${prefix}-${String(counter).padStart(4, "0")}`;
    },
  };
};

export const createMemoryLogger = (): LoggerPort & { readonly events: () => readonly LogEvent[] } => {
  const events: LogEvent[] = [];
  return {
    log: (event) => {
      events.push(event);
    },
    events: () => [...events],
  };
};

export type MemoryPorts = EnginePorts & {
  readonly fs: MemoryFileSystem;
  readonly services: MemoryServiceHost;
};

export const createMemoryPorts = (options: {
  readonly files?: Readonly<Record<string, string>>;
  readonly services?: Readonly<Record<string, ServiceState>>;
  readonly faults?: Fault[];
  readonly logger?: LoggerPort;
} = {}): MemoryPorts => {
  const faults = options.faults ?? [];
  const base = {
    fs: createMemoryFileSystem(options.files ?? {}, faults),
    services: createMemoryServiceHost(options.services ?? {}, faults),
    clock: createStubClock(),
  };
  return options.logger === undefined ? base : { ...base, logger: options.logger };
};

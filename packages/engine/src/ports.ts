import type { Result } from "@normal/schema";

export type IoError = {
  readonly code: "not-found" | "denied" | "io-failure" | "unavailable" | "timeout";
  readonly target: string;
  readonly message: string;
};

export type FileSystemPort = {
  readonly read: (path: string) => Promise<Result<string, IoError>>;
  readonly write: (path: string, contents: string) => Promise<Result<void, IoError>>;
  readonly remove: (path: string) => Promise<Result<void, IoError>>;
  readonly exists: (path: string) => Promise<Result<boolean, IoError>>;
};

export type ServiceState = "running" | "stopped" | "failed";

export type ServiceHostPort = {
  readonly restart: (service: string) => Promise<Result<void, IoError>>;
  readonly status: (service: string) => Promise<Result<ServiceState, IoError>>;
};

export type ClockPort = {
  readonly now: () => string;
  readonly nextId: () => string;
};

export type LogEvent = {
  readonly transactionId: string;
  readonly at: string;
  readonly kind: string;
  readonly detail: string;
};

export type LoggerPort = {
  readonly log: (event: LogEvent) => void;
};

export type EnginePorts = {
  readonly fs: FileSystemPort;
  readonly services: ServiceHostPort;
  readonly clock: ClockPort;
  readonly logger?: LoggerPort;
};

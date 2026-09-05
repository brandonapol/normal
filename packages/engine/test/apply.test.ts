import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG, type NormalConfig } from "@normal/schema";
import {
  applyConfig,
  applyPlan,
  createMemoryLogger,
  createMemoryPorts,
  FILES,
  nextRevision,
  planApply,
  renderConfig,
  type Fault,
  type Plan,
} from "../src/index.js";

const clone = (config: NormalConfig): NormalConfig =>
  JSON.parse(JSON.stringify(config)) as NormalConfig;

const withColumns = (columns: number): NormalConfig => {
  const draft = clone(BASELINE_CONFIG);
  (draft as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = columns;
  return nextRevision(BASELINE_CONFIG, draft);
};

const withPageSize = (pageSize: number): NormalConfig => {
  const draft = clone(BASELINE_CONFIG);
  (
    draft as unknown as { spec: { attention: { infiniteScroll: { pageSize: number } } } }
  ).spec.attention.infiniteScroll.pageSize = pageSize;
  return nextRevision(BASELINE_CONFIG, draft);
};

const planFor = (desired: NormalConfig): Plan => {
  const plan = planApply(BASELINE_CONFIG, desired);
  if (!plan.ok) throw new Error(JSON.stringify(plan.error));
  return plan.value;
};

const seeded = (faults: Fault[] = []) =>
  createMemoryPorts({ files: { ...renderConfig(BASELINE_CONFIG) }, faults });

describe("applyPlan", () => {
  it("writes the rendered files and restarts the affected services", async () => {
    const ports = seeded();
    const result = await applyPlan(planFor(withColumns(3)), ports);
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(ports.fs.snapshot()[FILES.launcher]).toContain('"columns": 3');
    expect(ports.services.restarts()).toEqual(["normal-launcher"]);
    expect(result.value.steps.every((step) => step.status === "applied")).toBe(true);
  });

  it("does nothing for a no-op plan", async () => {
    const ports = seeded();
    const before = ports.fs.snapshot();
    const plan = planFor(clone(BASELINE_CONFIG));
    const result = await applyPlan(plan, ports);
    expect(result.ok).toBe(true);
    expect(ports.fs.snapshot()).toEqual(before);
    expect(ports.services.restarts()).toEqual([]);
  });

  it("converges: re-planning the same desired state yields no work", async () => {
    const ports = seeded();
    const desired = withColumns(3);
    await applyPlan(planFor(desired), ports);
    const afterFirst = ports.fs.snapshot();

    const secondPlan = planApply(desired, clone(desired));
    expect(secondPlan.ok).toBe(true);
    if (!secondPlan.ok) return;
    expect(secondPlan.value.actions).toEqual([]);
    await applyPlan(secondPlan.value, ports);
    expect(ports.fs.snapshot()).toEqual(afterFirst);
    expect(ports.services.restarts()).toEqual(["normal-launcher"]);
  });
});

describe("rollback", () => {
  it("restores every file when a later write fails", async () => {
    const ports = seeded([
      {
        kind: "write",
        target: FILES.webviewShim,
        error: { code: "io-failure", target: FILES.webviewShim, message: "disk full" },
        times: 1,
      },
    ]);
    const before = ports.fs.snapshot();
    const result = await applyPlan(planFor(withPageSize(10)), ports);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.rolledBack).toBe(true);
    expect(result.error.deviceDirty).toBe(false);
    expect(result.error.error.message).toBe("disk full");
    expect(ports.fs.snapshot()).toEqual(before);
  });

  it("restores files when a service restart fails", async () => {
    const ports = seeded([
      {
        kind: "restart",
        target: "normal-launcher",
        error: { code: "unavailable", target: "normal-launcher", message: "unit refused to start" },
        times: 1,
      },
    ]);
    const before = ports.fs.snapshot();
    const result = await applyPlan(planFor(withColumns(5)), ports);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.rolledBack).toBe(true);
    expect(ports.fs.snapshot()).toEqual(before);
    expect(ports.services.restarts()).toEqual(["normal-launcher", "normal-launcher"]);
  });

  it("rolls back when a service restarts but comes back unhealthy", async () => {
    const ports = seeded();
    ports.services.setState("normal-launcher", "running");
    const plan = planFor(withColumns(6));
    const failing = createMemoryPorts({ files: { ...renderConfig(BASELINE_CONFIG) } });
    const before = failing.fs.snapshot();
    const stubbedServices = {
      ...failing.services,
      status: async () => ({ ok: true as const, value: "failed" as const }),
    };
    const result = await applyPlan(plan, { ...failing, services: stubbedServices });

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.failedAction).toBeNull();
    expect(result.error.error.code).toBe("unavailable");
    expect(result.error.rolledBack).toBe(true);
    expect(failing.fs.snapshot()).toEqual(before);
  });

  it("reports the device as dirty when rollback itself fails", async () => {
    const ports = seeded([
      {
        kind: "write",
        target: FILES.metadata,
        error: { code: "io-failure", target: FILES.metadata, message: "metadata write failed" },
      },
      {
        kind: "write",
        target: FILES.launcher,
        error: { code: "denied", target: FILES.launcher, message: "read-only filesystem" },
      },
    ]);
    const result = await applyPlan(planFor(withColumns(7)), ports);

    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.rolledBack).toBe(false);
    expect(result.error.deviceDirty).toBe(true);
    expect(result.error.rollbackErrors.map((error) => error.message)).toContain(
      "read-only filesystem",
    );
  });

  it("removes files it created when rolling back onto a bare device", async () => {
    const ports = createMemoryPorts({ files: {} });
    const result = await applyPlan(
      planFor(withColumns(3)),
      {
        ...ports,
        services: {
          ...ports.services,
          restart: async (service: string) =>
            service === "normal-launcher"
              ? { ok: false as const, error: { code: "io-failure" as const, target: service, message: "boom" } }
              : ports.services.restart(service),
        },
      },
    );
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.rolledBack).toBe(false);
    expect(Object.keys(ports.fs.snapshot())).toEqual([]);
  });

  it("journals the transaction through the logger", async () => {
    const logger = createMemoryLogger();
    const ports = createMemoryPorts({ files: { ...renderConfig(BASELINE_CONFIG) }, logger });
    await applyPlan(planFor(withColumns(3)), ports);
    const kinds = logger.events().map((event) => event.kind);
    expect(kinds[0]).toBe("transaction-start");
    expect(kinds).toContain("step-applied");
    expect(kinds.at(-1)).toBe("transaction-commit");
  });
});

describe("applyConfig", () => {
  it("surfaces plan issues without touching the device", async () => {
    const ports = seeded();
    const before = ports.fs.snapshot();
    const stale = clone(BASELINE_CONFIG);
    (stale as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = 3;
    const result = await applyConfig(BASELINE_CONFIG, stale, ports);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.kind).toBe("plan");
    expect(ports.fs.snapshot()).toEqual(before);
  });

  it("applies a valid transition end to end", async () => {
    const ports = seeded();
    const result = await applyConfig(BASELINE_CONFIG, withPageSize(50), ports);
    expect(result.ok).toBe(true);
    expect(ports.fs.snapshot()[FILES.attention]).toContain('"pageSize": 50');
    expect(ports.fs.snapshot()[FILES.webviewShim]).toContain('"pageSize": 50');
  });
});

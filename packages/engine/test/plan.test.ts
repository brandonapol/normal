import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG, type NormalConfig } from "@normal/schema";
import { FILES, isNoOp, nextRevision, planApply, renderConfig } from "../src/index.js";

const clone = (config: NormalConfig): NormalConfig =>
  JSON.parse(JSON.stringify(config)) as NormalConfig;

const bump = (mutate: (draft: NormalConfig) => void): NormalConfig => {
  const draft = clone(BASELINE_CONFIG);
  mutate(draft);
  return nextRevision(BASELINE_CONFIG, draft);
};

describe("planApply", () => {
  it("produces a no-op plan for an unchanged config", () => {
    const plan = planApply(BASELINE_CONFIG, clone(BASELINE_CONFIG));
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    expect(isNoOp(plan.value)).toBe(true);
    expect(plan.value.services).toEqual([]);
  });

  it("writes only the files a change actually touches", () => {
    const desired = bump((draft) => {
      (draft as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = 3;
    });
    const plan = planApply(BASELINE_CONFIG, desired);
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    const written = plan.value.actions
      .filter((action) => action.kind === "write-file")
      .map((action) => (action.kind === "write-file" ? action.path : ""));
    expect(written).toEqual([FILES.launcher, FILES.metadata]);
    expect(plan.value.services).toEqual(["normal-launcher"]);
  });

  it("fans an attention change out to the generated shim file and both readers", () => {
    const desired = bump((draft) => {
      (
        draft as unknown as { spec: { attention: { infiniteScroll: { pageSize: number } } } }
      ).spec.attention.infiniteScroll.pageSize = 10;
    });
    const plan = planApply(BASELINE_CONFIG, desired);
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    const written = plan.value.actions
      .filter((action) => action.kind === "write-file")
      .map((action) => (action.kind === "write-file" ? action.path : ""));
    expect(written).toContain(FILES.attention);
    expect(written).toContain(FILES.webviewShim);
    expect(plan.value.services).toEqual(["normal-attentiond", "normal-webview-shim"]);
  });

  it("does not restart a service whose files did not change", () => {
    const desired = bump((draft) => {
      const entries = draft.spec.apps.entries as unknown as { package: string; label?: string }[];
      entries.find((entry) => entry.package === "com.spotify.music")!.label = "Music";
    });
    const plan = planApply(BASELINE_CONFIG, desired);
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    expect(plan.value.services).not.toContain("normal-webview-shim");
    expect(plan.value.services).toEqual(["normal-appd", "normal-launcher"]);
  });

  it("orders every file write ahead of every restart", () => {
    const desired = bump((draft) => {
      (draft as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = 4;
    });
    const plan = planApply(BASELINE_CONFIG, desired);
    expect(plan.ok).toBe(true);
    if (!plan.ok) return;
    const firstRestart = plan.value.actions.findIndex((action) => action.kind === "restart-service");
    const lastWrite = plan.value.actions.reduce(
      (acc, action, index) => (action.kind === "restart-service" ? acc : index),
      -1,
    );
    expect(lastWrite).toBeLessThan(firstRestart);
  });

  it("rejects a stale revision", () => {
    const desired = clone(BASELINE_CONFIG);
    (desired as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = 3;
    const plan = planApply(BASELINE_CONFIG, desired);
    expect(plan.ok).toBe(false);
    if (plan.ok) return;
    expect(plan.error.map((issue) => issue.code)).toContain("stale-revision");
  });

  it("rejects a change to apiVersion", () => {
    const desired = { ...clone(BASELINE_CONFIG), apiVersion: "normal.os/v1" } as NormalConfig;
    const plan = planApply(BASELINE_CONFIG, nextRevision(BASELINE_CONFIG, desired));
    expect(plan.ok).toBe(false);
    if (plan.ok) return;
    expect(plan.error.map((issue) => issue.code)).toContain("immutable-field");
  });

  it("rejects a change nothing owns", () => {
    const desired = { ...clone(BASELINE_CONFIG), extras: { rogue: true } } as unknown as NormalConfig;
    const plan = planApply(BASELINE_CONFIG, nextRevision(BASELINE_CONFIG, desired));
    expect(plan.ok).toBe(false);
    if (plan.ok) return;
    expect(plan.error.map((issue) => issue.code)).toContain("unowned-path");
  });
});

describe("renderConfig", () => {
  it("is deterministic regardless of key order", () => {
    const reordered = JSON.parse(
      JSON.stringify({ spec: BASELINE_CONFIG.spec, metadata: BASELINE_CONFIG.metadata, kind: BASELINE_CONFIG.kind, apiVersion: BASELINE_CONFIG.apiVersion }),
    ) as NormalConfig;
    expect(renderConfig(reordered)).toEqual(renderConfig(BASELINE_CONFIG));
  });

  it("derives the webview shim from the attention policy", () => {
    const shim = JSON.parse(renderConfig(BASELINE_CONFIG)[FILES.webviewShim]!) as {
      enforcement: string;
      maxAutoLoads: number;
      domSignals: string[];
    };
    expect(shim.enforcement).toBe("paginate");
    expect(shim.maxAutoLoads).toBe(0);
    expect(shim.domSignals).toContain("sentinel-intersection-observer");
  });
});

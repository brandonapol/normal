import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG, validateConfig, type NormalConfig } from "../src/index.js";

const NOW = "2026-01-01T00:00:00.000Z";
const options = { now: NOW };

const clone = (config: NormalConfig): NormalConfig =>
  JSON.parse(JSON.stringify(config)) as NormalConfig;

const codesAt = (issues: readonly { path: string; code: string }[], path: string): string[] =>
  issues.filter((issue) => issue.path === path).map((issue) => issue.code);

describe("validateConfig", () => {
  it("accepts the baseline config", () => {
    const result = validateConfig(BASELINE_CONFIG, options);
    expect(result.ok ? [] : result.error).toEqual([]);
  });

  it("rejects a non-object document", () => {
    const result = validateConfig("nope", options);
    expect(result.ok).toBe(false);
  });

  it("rejects an unknown apiVersion", () => {
    const config = { ...clone(BASELINE_CONFIG), apiVersion: "normal.os/v99" };
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/apiVersion")).toContain("unsupported-version");
  });

  it("rejects dangling launcher references", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { launcher: { dock: string[] } };
    };
    mutable.spec.launcher.dock = ["com.example.ghost"];
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/launcher/dock/0")).toContain("dangling-reference");
  });

  it("rejects duplicate app entries", () => {
    const config = clone(BASELINE_CONFIG);
    const apps = config.spec.apps.entries as unknown as unknown[];
    apps.push(JSON.parse(JSON.stringify(apps[0])));
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.some((issue) => issue.code === "duplicate-package")).toBe(true);
  });

  it("rejects a page that exceeds its own maxItemsPerPage", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as { spec: { launcher: { maxItemsPerPage: number } } };
    mutable.spec.launcher.maxItemsPerPage = 2;
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/launcher/pages/0/items")).toContain("too-many");
  });

  it("rejects a notification rule that matches nothing", () => {
    const config = clone(BASELINE_CONFIG);
    const rules = config.spec.notifications.rules as unknown as { match: unknown }[];
    rules[0]!.match = {};
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/notifications/rules/0/match")).toContain("empty");
  });
});

describe("infinite-scroll invariants", () => {
  it("has no enforcement mode that disables the policy", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { enforcement: string } } };
    };
    mutable.spec.attention.infiniteScroll.enforcement = "off";
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/attention/infiniteScroll/enforcement")).toContain("invalid-enum");
  });

  it("rejects an empty detector list", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { detectors: unknown[] } } };
    };
    mutable.spec.attention.infiniteScroll.detectors = [];
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/attention/infiniteScroll/detectors")).toContain("empty");
  });

  it("rejects disabling the webview shim", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { webview: { injectShim: boolean } } } };
    };
    mutable.spec.attention.infiniteScroll.webview.injectShim = false;
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/attention/infiniteScroll/webview/injectShim")).toContain(
      "policy-violation",
    );
  });

  it("accepts a bounded, justified exemption", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { exemptions: unknown[] } } };
    };
    mutable.spec.attention.infiniteScroll.exemptions = [
      {
        id: "maps-transit-list",
        package: "com.google.android.apps.maps",
        reason: "transit departure board needs a continuous list",
        expiresAt: "2026-01-10T00:00:00.000Z",
      },
    ];
    expect(validateConfig(config, options).ok).toBe(true);
  });

  it("rejects an exemption without a real reason, in the past, or beyond the cap", () => {
    const exemption = {
      id: "sloppy",
      package: "com.google.android.apps.maps",
      reason: "why not",
      expiresAt: "2025-01-01T00:00:00.000Z",
    };
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { exemptions: unknown[] } } };
    };
    mutable.spec.attention.infiniteScroll.exemptions = [exemption];
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    const base = "/spec/attention/infiniteScroll/exemptions/0";
    expect(codesAt(result.error, `${base}/reason`)).toContain("too-short");
    expect(codesAt(result.error, `${base}/expiresAt`)).toContain("expired");

    mutable.spec.attention.infiniteScroll.exemptions = [
      { ...exemption, reason: "a properly stated reason", expiresAt: "2027-01-01T00:00:00.000Z" },
    ];
    const distant = validateConfig(config, options);
    expect(distant.ok).toBe(false);
    if (distant.ok) return;
    expect(codesAt(distant.error, `${base}/expiresAt`)).toContain("too-distant");
  });

  it("caps the number of simultaneous exemptions", () => {
    const config = clone(BASELINE_CONFIG);
    const mutable = config as unknown as {
      spec: { attention: { infiniteScroll: { exemptions: unknown[] } } };
    };
    mutable.spec.attention.infiniteScroll.exemptions = Array.from({ length: 4 }, (_, i) => ({
      id: `exemption-${i}`,
      package: "com.google.android.apps.maps",
      reason: "a properly stated reason for this one",
      expiresAt: "2026-01-10T00:00:00.000Z",
    }));
    const result = validateConfig(config, options);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(codesAt(result.error, "/spec/attention/infiniteScroll/exemptions")).toContain("too-many");
  });
});

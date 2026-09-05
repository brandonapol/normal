import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG, type NormalConfig } from "@normal/schema";
import { deepEqual, diffConfig } from "../src/index.js";

const clone = (config: NormalConfig): NormalConfig =>
  JSON.parse(JSON.stringify(config)) as NormalConfig;

describe("diffConfig", () => {
  it("reports nothing for identical configs", () => {
    expect(diffConfig(BASELINE_CONFIG, clone(BASELINE_CONFIG)).changes).toEqual([]);
  });

  it("is symmetric in emptiness and does not mutate its inputs", () => {
    const before = JSON.stringify(BASELINE_CONFIG);
    diffConfig(BASELINE_CONFIG, clone(BASELINE_CONFIG));
    expect(JSON.stringify(BASELINE_CONFIG)).toBe(before);
  });

  it("reports a scalar replacement at its pointer", () => {
    const desired = clone(BASELINE_CONFIG);
    (desired as unknown as { spec: { launcher: { columns: number } } }).spec.launcher.columns = 2;
    expect(diffConfig(BASELINE_CONFIG, desired).changes).toEqual([
      { op: "replace", path: "/spec/launcher/columns", before: 1, after: 2 },
    ]);
  });

  it("addresses keyed collection members by key, not index", () => {
    const desired = clone(BASELINE_CONFIG);
    const entries = desired.spec.apps.entries as unknown as { package: string; network: string }[];
    const spotify = entries.find((entry) => entry.package === "com.spotify.music")!;
    spotify.network = "wifi-only";
    expect(diffConfig(BASELINE_CONFIG, desired).changes).toEqual([
      {
        op: "replace",
        path: "/spec/apps/entries/com.spotify.music/network",
        before: "allow",
        after: "wifi-only",
      },
    ]);
  });

  it("reports one add for a new collection member", () => {
    const desired = clone(BASELINE_CONFIG);
    const entries = desired.spec.apps.entries as unknown as unknown[];
    entries.push({
      package: "org.fdroid.fdroid",
      source: "fdroid",
      state: "installed",
      network: "wifi-only",
      permissions: {},
    });
    const changes = diffConfig(BASELINE_CONFIG, desired).changes;
    expect(changes).toHaveLength(1);
    expect(changes[0]!.op).toBe("add");
    expect(changes[0]!.path).toBe("/spec/apps/entries/org.fdroid.fdroid");
  });

  it("reports one remove for a dropped collection member", () => {
    const desired = clone(BASELINE_CONFIG);
    const mutable = desired as unknown as { spec: { apps: { entries: { package: string }[] } } };
    mutable.spec.apps.entries = mutable.spec.apps.entries.filter(
      (entry) => entry.package !== "com.spotify.music",
    );
    const changes = diffConfig(BASELINE_CONFIG, desired).changes;
    expect(changes).toHaveLength(1);
    expect(changes[0]!.op).toBe("remove");
    expect(changes[0]!.path).toBe("/spec/apps/entries/com.spotify.music");
  });

  it("notices reordering of a keyed collection", () => {
    const desired = clone(BASELINE_CONFIG);
    const mutable = desired as unknown as { spec: { apps: { entries: unknown[] } } };
    mutable.spec.apps.entries = [...mutable.spec.apps.entries].reverse();
    const changes = diffConfig(BASELINE_CONFIG, desired).changes;
    expect(changes).toHaveLength(1);
    expect(changes[0]!.path).toBe("/spec/apps/entries/$order");
  });

  it("treats unkeyed arrays as atomic values", () => {
    const desired = clone(BASELINE_CONFIG);
    const mutable = desired as unknown as { spec: { launcher: { dock: string[] } } };
    mutable.spec.launcher.dock = ["os.normal.phone"];
    const changes = diffConfig(BASELINE_CONFIG, desired).changes;
    expect(changes).toHaveLength(1);
    expect(changes[0]).toMatchObject({ op: "replace", path: "/spec/launcher/dock" });
  });

  it("splits an object rewrite into per-field changes", () => {
    const desired = clone(BASELINE_CONFIG);
    const mutable = desired as unknown as {
      spec: { attention: { infiniteScroll: { pageSize: number; continuation: string } } };
    };
    mutable.spec.attention.infiniteScroll.pageSize = 10;
    mutable.spec.attention.infiniteScroll.continuation = "hold";
    const paths = diffConfig(BASELINE_CONFIG, desired).changes.map((change) => change.path);
    expect(paths).toEqual([
      "/spec/attention/infiniteScroll/continuation",
      "/spec/attention/infiniteScroll/pageSize",
    ]);
  });
});

describe("deepEqual", () => {
  it("ignores explicit undefined properties", () => {
    expect(deepEqual({ a: 1, b: undefined }, { a: 1 })).toBe(true);
    expect(deepEqual({ a: 1 }, { a: 2 })).toBe(false);
    expect(deepEqual([1, 2], [1, 2])).toBe(true);
    expect(deepEqual([1, 2], [2, 1])).toBe(false);
  });
});

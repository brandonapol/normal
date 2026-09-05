import { describe, expect, it } from "vitest";
import {
  BASELINE_CONFIG,
  formatPointer,
  getAtPath,
  parsePointer,
  removeAtPath,
  setAtPath,
} from "../src/index.js";

const at = (pointer: string): readonly string[] => {
  const parsed = parsePointer(pointer);
  if (!parsed.ok) throw new Error(parsed.error.message);
  return parsed.value;
};

describe("pointer", () => {
  it("round-trips pointers with escapes", () => {
    expect(formatPointer(at("/a~1b/c~0d"))).toBe("/a~1b/c~0d");
    expect(at("/a~1b/c~0d")).toEqual(["a/b", "c~d"]);
  });

  it("rejects pointers that do not start with a slash", () => {
    expect(parsePointer("spec/launcher").ok).toBe(false);
  });

  it("reads scalars", () => {
    const result = getAtPath(BASELINE_CONFIG, at("/spec/launcher/columns"));
    expect(result.ok && result.value).toBe(1);
  });

  it("addresses keyed collections by key rather than index", () => {
    const result = getAtPath(BASELINE_CONFIG, at("/spec/apps/entries/com.spotify.music/network"));
    expect(result.ok && result.value).toBe("allow");
  });

  it("still accepts numeric indices", () => {
    const result = getAtPath(BASELINE_CONFIG, at("/spec/apps/entries/0/package"));
    expect(result.ok && result.value).toBe("os.normal.phone");
  });

  it("reports a missing key", () => {
    const result = getAtPath(BASELINE_CONFIG, at("/spec/apps/entries/com.example.ghost"));
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.code).toBe("not-found");
  });

  it("sets without mutating the source", () => {
    const before = JSON.stringify(BASELINE_CONFIG);
    const updated = setAtPath(BASELINE_CONFIG, at("/spec/launcher/columns"), 3);
    expect(updated.ok).toBe(true);
    expect(JSON.stringify(BASELINE_CONFIG)).toBe(before);
    if (!updated.ok) return;
    const read = getAtPath(updated.value, at("/spec/launcher/columns"));
    expect(read.ok && read.value).toBe(3);
  });

  it("appends to a keyed collection when the key is new", () => {
    const entry = {
      package: "org.fdroid.fdroid",
      source: "fdroid",
      state: "installed",
      network: "wifi-only",
      permissions: {},
    };
    const updated = setAtPath(BASELINE_CONFIG, at("/spec/apps/entries/org.fdroid.fdroid"), entry);
    expect(updated.ok).toBe(true);
    if (!updated.ok) return;
    const read = getAtPath(updated.value, at("/spec/apps/entries/org.fdroid.fdroid/network"));
    expect(read.ok && read.value).toBe("wifi-only");
  });

  it("removes a keyed element", () => {
    const updated = removeAtPath(BASELINE_CONFIG, at("/spec/apps/entries/com.spotify.music"));
    expect(updated.ok).toBe(true);
    if (!updated.ok) return;
    expect(getAtPath(updated.value, at("/spec/apps/entries/com.spotify.music")).ok).toBe(false);
    expect(BASELINE_CONFIG.spec.apps.entries.length).toBe(6);
  });

  it("refuses to traverse into a scalar", () => {
    const result = setAtPath(BASELINE_CONFIG, at("/spec/launcher/columns/nope"), 1);
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error.code).toBe("not-traversable");
  });
});

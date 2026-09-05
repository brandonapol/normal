import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG, validateConfig } from "../src/index.js";

describe("examples/baseline.config.json", () => {
  it("matches the baseline config in code", () => {
    const onDisk = JSON.parse(readFileSync("examples/baseline.config.json", "utf8")) as unknown;
    expect(onDisk).toEqual(BASELINE_CONFIG);
  });

  it("validates against the schema", () => {
    const onDisk = JSON.parse(readFileSync("examples/baseline.config.json", "utf8")) as unknown;
    const result = validateConfig(onDisk, { now: "2026-01-01T00:00:00.000Z" });
    expect(result.ok ? [] : result.error).toEqual([]);
  });
});

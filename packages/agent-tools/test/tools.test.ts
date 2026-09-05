import { describe, expect, it } from "vitest";
import { TOOL_DEFINITIONS, TOOL_NAMES, SYSTEM_GUIDANCE } from "../src/index.js";

describe("tool definitions", () => {
  it("defines exactly the declared tool names, once each", () => {
    const names = TOOL_DEFINITIONS.map((tool) => tool.name);
    expect(new Set(names).size).toBe(names.length);
    expect([...names].sort()).toEqual([...TOOL_NAMES].sort());
  });

  it("exposes no tool that touches the filesystem, shell, or network", () => {
    const forbidden = ["file", "write", "read", "shell", "exec", "fetch", "http"];
    for (const name of TOOL_NAMES) {
      expect(forbidden.some((word) => name.includes(word))).toBe(false);
    }
  });

  it("exposes no tool that lets the agent approve its own proposal", () => {
    expect(TOOL_NAMES).not.toContain("approve_proposal");
    expect(TOOL_NAMES.some((name) => name.includes("approve"))).toBe(false);
  });

  it("closes every input schema to unknown arguments", () => {
    for (const tool of TOOL_DEFINITIONS) {
      expect(tool.inputSchema.additionalProperties).toBe(false);
      expect(tool.inputSchema.type).toBe("object");
      expect(tool.description.length).toBeGreaterThan(20);
    }
  });

  it("states the infinite-scroll invariant in the system guidance", () => {
    expect(SYSTEM_GUIDANCE).toContain("no 'off' switch");
  });
});

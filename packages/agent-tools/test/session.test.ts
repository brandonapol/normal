import { describe, expect, it } from "vitest";
import { BASELINE_CONFIG } from "@normal/schema";
import { createMemoryPorts, FILES, renderConfig, type MemoryPorts } from "@normal/engine";
import {
  createAgentSession,
  dispatchTool,
  type AgentSession,
  type ToolResult,
} from "../src/index.js";

const setup = (
  options: { readonly approvalRequiredForEverything?: boolean } = {},
): { session: AgentSession; ports: MemoryPorts } => {
  const ports = createMemoryPorts({ files: { ...renderConfig(BASELINE_CONFIG) } });
  const session = createAgentSession({
    initialConfig: BASELINE_CONFIG,
    ports,
    ...options,
  });
  return { session, ports };
};

const data = (result: ToolResult): Record<string, unknown> => {
  if (!result.ok) throw new Error(`expected success, got ${result.error.code}: ${result.error.message}`);
  return result.data as Record<string, unknown>;
};

const proposalIdOf = (result: ToolResult): string => data(result)["proposalId"] as string;

const errorOf = (result: ToolResult): { code: string; message: string } => {
  if (result.ok) throw new Error("expected failure");
  return result.error;
};

describe("read tools", () => {
  it("returns a section of the config", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, { name: "get_config", arguments: { section: "launcher" } });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect((result.data as { columns: number }).columns).toBe(1);
  });

  it("rejects an unknown section", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, { name: "get_config", arguments: { section: "wallpaper" } });
    expect(errorOf(result).code).toBe("invalid-arguments");
  });

  it("rejects an unknown tool instead of throwing", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, { name: "write_file", arguments: { path: "/etc/passwd" } });
    expect(errorOf(result).code).toBe("unknown-tool");
  });

  it("documents the attention policy", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "describe_schema",
      arguments: { path: "/spec/attention/infiniteScroll" },
    });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(JSON.stringify(result.data)).toContain("no 'off' enforcement mode");
  });
});

describe("propose_change", () => {
  it("returns a diff without touching the device", async () => {
    const { session, ports } = setup();
    const before = ports.fs.snapshot();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "make the home screen a two-column grid",
        operations: [
          { op: "set", path: "/spec/launcher/layout", value: "grid" },
          { op: "set", path: "/spec/launcher/columns", value: 2 },
        ],
      },
    });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    const summary = result.data as { changeCount: number; diff: string; status: string };
    expect(summary.status).toBe("pending");
    expect(summary.changeCount).toBe(2);
    expect(summary.diff).toContain("/spec/launcher/columns");
    expect(ports.fs.snapshot()).toEqual(before);
  });

  it("refuses to write outside the agent's writable roots", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "bump the api version",
        operations: [{ op: "set", path: "/apiVersion", value: "normal.os/v1" }],
      },
    });
    const error = errorOf(result);
    expect(error.code).toBe("proposal-rejected");
    expect(error.message).toContain("managed by the mutation engine");
  });

  it("refuses to set the revision itself", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "skip ahead",
        operations: [{ op: "set", path: "/metadata/revision", value: 99 }],
      },
    });
    expect(errorOf(result).code).toBe("proposal-rejected");
  });

  it("rejects a patch whose result would not validate", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "twenty columns",
        operations: [{ op: "set", path: "/spec/launcher/columns", value: 20 }],
      },
    });
    expect(errorOf(result).message).toContain("would not be a valid config");
  });

  it("rejects an empty operation list", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: { intent: "do nothing", operations: [] },
    });
    expect(errorOf(result).code).toBe("proposal-rejected");
  });

  it("rejects malformed operations", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: { intent: "nonsense", operations: [{ op: "delete", path: "/spec/launcher" }] },
    });
    expect(errorOf(result).code).toBe("invalid-arguments");
  });
});

describe("the no-infinite-scroll invariant", () => {
  it("cannot be switched off at all", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "turn off the scroll blocker, it is annoying",
        operations: [{ op: "set", path: "/spec/attention/infiniteScroll/enforcement", value: "off" }],
      },
    });
    expect(errorOf(result).message).toContain("would not be a valid config");
  });

  it("cannot be defeated by emptying the detector list", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "remove all detectors",
        operations: [{ op: "set", path: "/spec/attention/infiniteScroll/detectors", value: [] }],
      },
    });
    expect(errorOf(result).message).toContain("at least one detector");
  });

  it("cannot be defeated by disabling the webview shim", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "stop injecting the shim",
        operations: [{ op: "set", path: "/spec/attention/infiniteScroll/webview/injectShim", value: false }],
      },
    });
    expect(errorOf(result).message).toContain("cannot be disabled");
  });

  it("allows a bounded exemption but always flags it for approval", async () => {
    const { session } = setup({ approvalRequiredForEverything: false });
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "let the transit board in Maps scroll freely for a week",
        operations: [
          {
            op: "set",
            path: "/spec/attention/infiniteScroll/exemptions/maps-transit",
            value: {
              id: "maps-transit",
              package: "com.google.android.apps.maps",
              reason: "transit departure board is a continuous list by nature",
              expiresAt: "2026-01-08T00:00:00.000Z",
            },
          },
        ],
      },
    });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    const summary = result.data as {
      requiresApproval: boolean;
      review: { code: string; message: string }[];
    };
    expect(summary.requiresApproval).toBe(true);
    expect(summary.review.map((issue) => issue.code)).toContain("weakens-attention-policy");
  });

  it("flags a raised page size as a weakening even when nothing else changes", async () => {
    const { session } = setup({ approvalRequiredForEverything: false });
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "show more items per page",
        operations: [{ op: "set", path: "/spec/attention/infiniteScroll/pageSize", value: 60 }],
      },
    });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect((result.data as { requiresApproval: boolean }).requiresApproval).toBe(true);
  });

  it("does not call a strengthening a weakening", async () => {
    const { session } = setup({ approvalRequiredForEverything: false });
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "be stricter about feeds",
        operations: [{ op: "set", path: "/spec/attention/infiniteScroll/enforcement", value: "block" }],
      },
    });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    const summary = result.data as { requiresApproval: boolean; review: { code: string }[] };
    expect(summary.review.map((issue) => issue.code)).not.toContain("weakens-attention-policy");
    expect(summary.review.map((issue) => issue.code)).toContain("sensitive-path");
    expect(summary.requiresApproval).toBe(true);
  });
});

describe("apply_proposal", () => {
  const proposeColumns = async (session: AgentSession): Promise<string> => {
    const result = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "two columns please",
        operations: [{ op: "set", path: "/spec/launcher/columns", value: 2 }],
      },
    });
    return proposalIdOf(result);
  };

  it("refuses to apply what the user has not approved", async () => {
    const { session, ports } = setup();
    const before = ports.fs.snapshot();
    const id = await proposeColumns(session);
    const result = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    expect(errorOf(result).code).toBe("approval-required");
    expect(ports.fs.snapshot()).toEqual(before);
    expect(session.currentConfig().spec.launcher.columns).toBe(1);
  });

  it("applies once the user approves out of band", async () => {
    const { session, ports } = setup();
    const id = await proposeColumns(session);
    expect(session.approve(id, "user").ok).toBe(true);

    const result = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect((result.data as { revision: number }).revision).toBe(1);
    expect(ports.fs.snapshot()[FILES.launcher]).toContain('"columns": 2');
    expect(ports.services.restarts()).toEqual(["normal-launcher"]);
    expect(session.currentConfig().spec.launcher.columns).toBe(2);
  });

  it("will not apply the same proposal twice", async () => {
    const { session } = setup();
    const id = await proposeColumns(session);
    session.approve(id, "user");
    await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    const second = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    expect(errorOf(second).code).toBe("not-applicable");
  });

  it("re-evaluates a stale proposal against the current config", async () => {
    const { session } = setup();
    const stale = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "put spotify on wifi only",
        operations: [{ op: "set", path: "/spec/apps/entries/com.spotify.music/network", value: "wifi-only" }],
      },
    });
    const staleId = proposalIdOf(stale);

    const removal = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "uninstall spotify",
        operations: [
          { op: "remove", path: "/spec/apps/entries/com.spotify.music" },
          { op: "remove", path: "/spec/launcher/pages/home/items/home-spotify" },
        ],
      },
    });
    const removalId = proposalIdOf(removal);
    session.approve(removalId, "user");
    const applied = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: removalId } });
    expect(applied.ok).toBe(true);

    session.approve(staleId, "user");
    const result = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: staleId } });
    expect(errorOf(result).code).toBe("stale");
  });

  it("reports a rolled-back failure without advancing the revision", async () => {
    const ports = createMemoryPorts({
      files: { ...renderConfig(BASELINE_CONFIG) },
      faults: [
        {
          kind: "write",
          target: FILES.launcher,
          error: { code: "io-failure", target: FILES.launcher, message: "disk full" },
          times: 1,
        },
      ],
    });
    const session = createAgentSession({ initialConfig: BASELINE_CONFIG, ports });
    const before = ports.fs.snapshot();
    const id = await proposeColumns(session);
    session.approve(id, "user");

    const result = await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    const error = errorOf(result);
    expect(error.code).toBe("apply-failed");
    expect(error.message).toContain("rolled back");
    expect(ports.fs.snapshot()).toEqual(before);
    expect(session.currentConfig().metadata.revision).toBe(0);
    expect(session.revisions()).toHaveLength(1);
  });
});

describe("revisions and rollback", () => {
  it("records history and rolls back through the same engine path", async () => {
    const { session, ports } = setup();
    const proposal = await dispatchTool(session, {
      name: "propose_change",
      arguments: {
        intent: "silence everything by default",
        operations: [{ op: "set", path: "/spec/notifications/defaultDisposition", value: "block" }],
      },
    });
    const id = proposalIdOf(proposal);
    session.approve(id, "user");
    await dispatchTool(session, { name: "apply_proposal", arguments: { proposalId: id } });
    expect(session.currentConfig().spec.notifications.defaultDisposition).toBe("block");

    const rollback = await dispatchTool(session, { name: "propose_rollback", arguments: { revision: 0 } });
    expect(rollback.ok).toBe(true);
    if (!rollback.ok) return;
    const rollbackId = (rollback.data as { proposalId: string; requiresApproval: boolean }).proposalId;
    expect((rollback.data as { requiresApproval: boolean }).requiresApproval).toBe(true);

    session.approve(rollbackId, "user");
    const applied = await dispatchTool(session, {
      name: "apply_proposal",
      arguments: { proposalId: rollbackId },
    });
    expect(applied.ok).toBe(true);
    expect(session.currentConfig().spec.notifications.defaultDisposition).toBe("bundle");
    expect(session.currentConfig().metadata.revision).toBe(2);
    expect(ports.fs.snapshot()[FILES.notifications]).toContain('"defaultDisposition": "bundle"');

    const history = await dispatchTool(session, { name: "list_revisions", arguments: {} });
    expect(history.ok).toBe(true);
    if (!history.ok) return;
    expect(history.data as unknown[]).toHaveLength(3);
  });

  it("refuses to roll back to a revision that was never applied", async () => {
    const { session } = setup();
    const result = await dispatchTool(session, { name: "propose_rollback", arguments: { revision: 7 } });
    expect(errorOf(result).message).toContain("no applied revision 7");
  });
});

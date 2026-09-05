import type { NormalConfig } from "@normal/schema";

export type FileSet = Readonly<Record<string, string>>;

export const CONFIG_ROOT = "/etc/normal";

export const FILES = {
  metadata: `${CONFIG_ROOT}/metadata.json`,
  launcher: `${CONFIG_ROOT}/launcher.json`,
  apps: `${CONFIG_ROOT}/apps.json`,
  notifications: `${CONFIG_ROOT}/notifications.json`,
  attention: `${CONFIG_ROOT}/attention.json`,
  webviewShim: `${CONFIG_ROOT}/generated/webview-shim.json`,
} as const;

export type ConfigFile = (typeof FILES)[keyof typeof FILES];

const sortValue = (value: unknown): unknown => {
  if (Array.isArray(value)) return value.map(sortValue);
  if (typeof value === "object" && value !== null) {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => a.localeCompare(b));
    return Object.fromEntries(entries.map(([k, v]) => [k, sortValue(v)]));
  }
  return value;
};

export const stableStringify = (value: unknown): string => `${JSON.stringify(sortValue(value), null, 2)}\n`;

const renderWebviewShim = (config: NormalConfig): unknown => {
  const policy = config.spec.attention.infiniteScroll;
  const exemptPackages = policy.exemptions.map((exemption) => exemption.package);
  const perApp = config.spec.apps.entries
    .filter((entry) => entry.attention !== undefined)
    .map((entry) => ({
      package: entry.package,
      enforcement: entry.attention?.enforcement ?? policy.enforcement,
      pageSize: entry.attention?.pageSize ?? policy.pageSize,
    }));
  return {
    enforcement: policy.enforcement,
    pageSize: policy.pageSize,
    maxAutoLoads: policy.maxAutoLoads,
    continuation: policy.continuation,
    continuationDelaySeconds: policy.continuationDelaySeconds,
    interceptIntersectionObserver: policy.webview.interceptIntersectionObserver,
    interceptHistoryScrollRestoration: policy.webview.interceptHistoryScrollRestoration,
    maxDocumentHeightMultiplier: policy.webview.maxDocumentHeightMultiplier,
    urlPatterns: policy.detectors.filter((d) => d.kind === "url-pattern").map((d) => d.pattern),
    domSignals: policy.detectors.flatMap((d) => (d.kind === "dom-heuristic" ? d.signals : [])),
    exemptPackages,
    perApp,
  };
};

export const renderConfig = (config: NormalConfig): FileSet => ({
  [FILES.metadata]: stableStringify({
    apiVersion: config.apiVersion,
    kind: config.kind,
    ...config.metadata,
  }),
  [FILES.launcher]: stableStringify(config.spec.launcher),
  [FILES.apps]: stableStringify(config.spec.apps),
  [FILES.notifications]: stableStringify(config.spec.notifications),
  [FILES.attention]: stableStringify(config.spec.attention),
  [FILES.webviewShim]: stableStringify(renderWebviewShim(config)),
});

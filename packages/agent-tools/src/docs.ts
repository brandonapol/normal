import { LIMITS } from "@normal/schema";

export type SchemaDoc = {
  readonly path: string;
  readonly summary: string;
  readonly writable: boolean;
  readonly notes?: string;
};

export const SCHEMA_DOCS: readonly SchemaDoc[] = [
  {
    path: "/metadata",
    summary: "Identity and revision of this configuration.",
    writable: false,
    notes: "Only /metadata/description and /metadata/labels accept agent writes; revision is engine-managed.",
  },
  {
    path: "/spec/launcher",
    summary: "Home screen layout: pages, items, dock, app drawer, gestures.",
    writable: true,
    notes: `columns is ${LIMITS.minColumns}-${LIMITS.maxColumns}; a page holds at most maxItemsPerPage items.`,
  },
  {
    path: "/spec/apps",
    summary: "Installed apps with their source, state, network policy, and permissions.",
    writable: true,
    notes: "Entries are keyed by package id. Permission and network changes always need user approval.",
  },
  {
    path: "/spec/notifications",
    summary: "Default disposition, bundling windows, quiet hours, and match rules.",
    writable: true,
    notes: "Rules are evaluated highest priority first; a rule must constrain at least one field.",
  },
  {
    path: "/spec/attention/infiniteScroll",
    summary: "The no-infinite-scroll policy: enforcement mode, page size, detectors, exemptions.",
    writable: true,
    notes:
      "There is no 'off' enforcement mode. Detectors cannot be emptied and the webview shim cannot be " +
      `disabled. At most ${LIMITS.maxExemptions} exemptions, each needing a stated reason and an expiry ` +
      `within ${LIMITS.maxExemptionDays} days. Every change here needs user approval.`,
  },
  {
    path: "/spec/attention/sessionBudgets",
    summary: "Per-app, per-domain, or system-wide time budgets and what happens when they run out.",
    writable: true,
    notes: "Raising a budget is treated as weakening policy and needs user approval.",
  },
];

export const docsFor = (path?: string): readonly SchemaDoc[] =>
  path === undefined
    ? SCHEMA_DOCS
    : SCHEMA_DOCS.filter((doc) => doc.path === path || doc.path.startsWith(`${path}/`) || path.startsWith(`${doc.path}/`));

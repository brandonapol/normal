import type { AppEntry, NormalConfig } from "./types.js";
import { API_VERSION, KIND } from "./version.js";

const app = (
  pkg: string,
  label: string,
  source: AppEntry["source"],
  overrides: Partial<AppEntry> = {},
): AppEntry => ({
  package: pkg,
  label,
  source,
  state: "installed",
  network: "allow",
  permissions: {},
  ...overrides,
});

export const BASELINE_APPS: readonly AppEntry[] = [
  app("os.normal.phone", "Phone", "system", { permissions: { contacts: "allow", notifications: "allow" } }),
  app("os.normal.messages", "Messages", "system", {
    permissions: { contacts: "allow", notifications: "allow" },
  }),
  app("os.normal.camera", "Camera", "system", { permissions: { camera: "allow", storage: "allow" } }),
  app("os.normal.browser", "Browser", "system", { permissions: { location: "ask" } }),
  app("com.spotify.music", "Spotify", "play-compat", {
    permissions: { notifications: "allow", storage: "allow", "background-activity": "allow" },
  }),
  app("com.google.android.apps.maps", "Maps", "play-compat", {
    permissions: { location: "ask", notifications: "deny", "background-activity": "deny" },
  }),
];

export const BASELINE_CONFIG: NormalConfig = {
  apiVersion: API_VERSION,
  kind: KIND,
  metadata: {
    name: "normal-baseline",
    revision: 0,
    description: "Default Normal phone configuration",
  },
  spec: {
    launcher: {
      layout: "list",
      columns: 1,
      maxItemsPerPage: 8,
      pages: [
        {
          id: "home",
          title: "Home",
          items: [
            { kind: "app", id: "home-phone", package: "os.normal.phone" },
            { kind: "app", id: "home-messages", package: "os.normal.messages" },
            { kind: "app", id: "home-maps", package: "com.google.android.apps.maps" },
            { kind: "app", id: "home-spotify", package: "com.spotify.music" },
          ],
        },
      ],
      dock: ["os.normal.phone", "os.normal.messages", "os.normal.camera", "os.normal.browser"],
      appDrawer: { enabled: true, sort: "alphabetical", search: true },
      gestures: {
        "swipe-up": { kind: "open-drawer" },
        "swipe-down": { kind: "open-notifications" },
        "long-press-home": { kind: "toggle", target: "grayscale" },
      },
    },
    apps: {
      policy: "allowlist",
      entries: BASELINE_APPS,
    },
    notifications: {
      defaultDisposition: "bundle",
      bundling: { enabled: true, deliveryWindows: ["09:00", "13:00", "18:00"] },
      quietHours: [
        {
          id: "overnight",
          start: "22:00",
          end: "07:00",
          days: ["mon", "tue", "wed", "thu", "fri", "sat", "sun"],
          breakthrough: ["os.normal.phone"],
        },
      ],
      rules: [
        {
          id: "calls-always",
          priority: 100,
          match: { package: "os.normal.phone" },
          disposition: "allow",
        },
        {
          id: "messages-allow",
          priority: 90,
          match: { package: "os.normal.messages" },
          disposition: "allow",
        },
        {
          id: "no-ongoing-noise",
          priority: 10,
          match: { ongoing: true },
          disposition: "silence",
        },
      ],
    },
    attention: {
      infiniteScroll: {
        enforcement: "paginate",
        pageSize: 20,
        maxAutoLoads: 0,
        continuation: "tap-with-delay",
        continuationDelaySeconds: 3,
        detectors: [
          {
            kind: "dom-heuristic",
            id: "dom-default",
            signals: [
              "append-on-scroll-mutation",
              "sentinel-intersection-observer",
              "unbounded-scroll-height",
              "absent-pagination-controls",
            ],
            minConfidence: 0.7,
          },
          {
            kind: "accessibility-heuristic",
            id: "a11y-default",
            signals: ["unbounded-collection-item-count", "recycler-view-refill"],
            minConfidence: 0.75,
          },
        ],
        exemptions: [],
        webview: {
          injectShim: true,
          interceptIntersectionObserver: true,
          interceptHistoryScrollRestoration: true,
          maxDocumentHeightMultiplier: 2,
        },
      },
      sessionBudgets: [
        {
          id: "browser-daily",
          scope: { kind: "app", package: "os.normal.browser" },
          dailyMinutes: 60,
          sessionMinutes: 15,
          cooldownMinutes: 30,
          onExhausted: "grayscale",
        },
      ],
    },
  },
};

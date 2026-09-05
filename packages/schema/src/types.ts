import type { ApiVersion, Kind } from "./version.js";

export type PackageId = string;
export type ResourceId = string;
export type TimeOfDay = string;
export type IsoTimestamp = string;

export type NormalConfig = {
  readonly apiVersion: ApiVersion;
  readonly kind: Kind;
  readonly metadata: Metadata;
  readonly spec: Spec;
};

export type Metadata = {
  readonly name: string;
  readonly revision: number;
  readonly description?: string;
  readonly labels?: Readonly<Record<string, string>>;
};

export type Spec = {
  readonly launcher: LauncherSpec;
  readonly apps: AppsSpec;
  readonly notifications: NotificationsSpec;
  readonly attention: AttentionSpec;
};

export type LauncherSpec = {
  readonly layout: LauncherLayout;
  readonly columns: number;
  readonly maxItemsPerPage: number;
  readonly pages: readonly LauncherPage[];
  readonly dock: readonly PackageId[];
  readonly appDrawer: AppDrawerSpec;
  readonly gestures: GestureBindings;
};

export type LauncherLayout = "grid" | "list";

export type LauncherPage = {
  readonly id: ResourceId;
  readonly title?: string;
  readonly items: readonly LauncherItem[];
};

export type LauncherItem =
  | { readonly kind: "app"; readonly id: ResourceId; readonly package: PackageId; readonly label?: string }
  | { readonly kind: "shortcut"; readonly id: ResourceId; readonly label: string; readonly action: ShortcutAction }
  | { readonly kind: "widget"; readonly id: ResourceId; readonly provider: string; readonly size: WidgetSize };

export type ShortcutAction =
  | { readonly kind: "open-app"; readonly package: PackageId }
  | { readonly kind: "open-url"; readonly url: string }
  | { readonly kind: "toggle"; readonly target: ToggleTarget };

export type ToggleTarget = "flashlight" | "do-not-disturb" | "wifi" | "airplane-mode" | "grayscale";

export type WidgetSize = "1x1" | "2x1" | "2x2" | "4x1" | "4x2";

export type AppDrawerSpec = {
  readonly enabled: boolean;
  readonly sort: "alphabetical" | "manual";
  readonly search: boolean;
};

export type Gesture = "swipe-up" | "swipe-down" | "swipe-left" | "swipe-right" | "double-tap" | "long-press-home";

export type GestureAction =
  | { readonly kind: "none" }
  | { readonly kind: "open-drawer" }
  | { readonly kind: "open-notifications" }
  | { readonly kind: "open-app"; readonly package: PackageId }
  | { readonly kind: "toggle"; readonly target: ToggleTarget };

export type GestureBindings = Readonly<Partial<Record<Gesture, GestureAction>>>;

export type AppsSpec = {
  readonly policy: "allowlist" | "denylist";
  readonly entries: readonly AppEntry[];
};

export type AppSource = "system" | "fdroid" | "play-compat" | "sideload";

export type AppState = "installed" | "blocked" | "absent";

export type NetworkPolicy = "allow" | "wifi-only" | "deny";

export type PermissionName =
  | "location"
  | "camera"
  | "microphone"
  | "contacts"
  | "storage"
  | "notifications"
  | "background-activity"
  | "nearby-devices";

export type PermissionDecision = "allow" | "ask" | "deny";

export type AppEntry = {
  readonly package: PackageId;
  readonly label?: string;
  readonly source: AppSource;
  readonly state: AppState;
  readonly network: NetworkPolicy;
  readonly permissions: Readonly<Partial<Record<PermissionName, PermissionDecision>>>;
  readonly sandboxProfile?: string;
  readonly attention?: AppAttentionOverride;
};

export type AppAttentionOverride = {
  readonly enforcement?: EnforcementMode;
  readonly pageSize?: number;
};

export type NotificationsSpec = {
  readonly defaultDisposition: NotificationDisposition;
  readonly bundling: BundlingSpec;
  readonly quietHours: readonly QuietHoursWindow[];
  readonly rules: readonly NotificationRule[];
};

export type NotificationDisposition = "allow" | "silence" | "bundle" | "block";

export type BundlingSpec = {
  readonly enabled: boolean;
  readonly deliveryWindows: readonly TimeOfDay[];
};

export type Weekday = "mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun";

export type QuietHoursWindow = {
  readonly id: ResourceId;
  readonly start: TimeOfDay;
  readonly end: TimeOfDay;
  readonly days: readonly Weekday[];
  readonly breakthrough: readonly PackageId[];
};

export type NotificationRule = {
  readonly id: ResourceId;
  readonly priority: number;
  readonly match: NotificationMatch;
  readonly disposition: NotificationDisposition;
};

export type NotificationMatch = {
  readonly package?: PackageId;
  readonly channel?: string;
  readonly titleContains?: string;
  readonly ongoing?: boolean;
};

export type AttentionSpec = {
  readonly infiniteScroll: InfiniteScrollPolicy;
  readonly sessionBudgets: readonly SessionBudget[];
};

export type EnforcementMode = "warn" | "paginate" | "block";

export type ContinuationGate = "tap" | "tap-with-delay" | "hold" | "passphrase";

export type InfiniteScrollPolicy = {
  readonly enforcement: EnforcementMode;
  readonly pageSize: number;
  readonly maxAutoLoads: number;
  readonly continuation: ContinuationGate;
  readonly continuationDelaySeconds: number;
  readonly detectors: readonly ScrollDetector[];
  readonly exemptions: readonly ScrollExemption[];
  readonly webview: WebViewEnforcement;
};

export type DomSignal =
  | "append-on-scroll-mutation"
  | "sentinel-intersection-observer"
  | "unbounded-scroll-height"
  | "recycled-virtual-list"
  | "absent-pagination-controls";

export type AccessibilitySignal =
  | "unbounded-collection-item-count"
  | "scroll-event-without-page-boundary"
  | "recycler-view-refill";

export type ScrollDetector =
  | {
      readonly kind: "dom-heuristic";
      readonly id: ResourceId;
      readonly signals: readonly DomSignal[];
      readonly minConfidence: number;
    }
  | {
      readonly kind: "accessibility-heuristic";
      readonly id: ResourceId;
      readonly signals: readonly AccessibilitySignal[];
      readonly minConfidence: number;
    }
  | {
      readonly kind: "url-pattern";
      readonly id: ResourceId;
      readonly pattern: string;
    }
  | {
      readonly kind: "app-surface";
      readonly id: ResourceId;
      readonly package: PackageId;
      readonly surface: string;
    };

export type ScrollExemption = {
  readonly id: ResourceId;
  readonly package: PackageId;
  readonly reason: string;
  readonly expiresAt: IsoTimestamp;
};

export type WebViewEnforcement = {
  readonly injectShim: boolean;
  readonly interceptIntersectionObserver: boolean;
  readonly interceptHistoryScrollRestoration: boolean;
  readonly maxDocumentHeightMultiplier: number;
};

export type BudgetScope =
  | { readonly kind: "app"; readonly package: PackageId }
  | { readonly kind: "domain"; readonly domain: string }
  | { readonly kind: "system-wide" };

export type BudgetExhaustedAction = "warn" | "grayscale" | "lock";

export type SessionBudget = {
  readonly id: ResourceId;
  readonly scope: BudgetScope;
  readonly dailyMinutes: number;
  readonly sessionMinutes: number;
  readonly cooldownMinutes: number;
  readonly onExhausted: BudgetExhaustedAction;
};

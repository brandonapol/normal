import { LIMITS } from "./limits.js";
import { err, ok, type Result } from "./result.js";
import type {
  AppEntry,
  IsoTimestamp,
  NormalConfig,
} from "./types.js";
import { API_VERSION, KIND } from "./version.js";

export type ValidationIssue = {
  readonly path: string;
  readonly code: string;
  readonly message: string;
};

export type ValidateOptions = {
  readonly now: IsoTimestamp;
};

type Push = (path: string, code: string, message: string) => void;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const isString = (value: unknown): value is string => typeof value === "string";

const isFiniteNumber = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value);

const TIME_OF_DAY = /^([01]\d|2[0-3]):[0-5]\d$/;
const PACKAGE_ID = /^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$/i;
const RESOURCE_ID = /^[a-z0-9][a-z0-9-]{0,63}$/;

const WEEKDAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"] as const;
const DISPOSITIONS = ["allow", "silence", "bundle", "block"] as const;
const ENFORCEMENT_MODES = ["warn", "paginate", "block"] as const;
const CONTINUATION_GATES = ["tap", "tap-with-delay", "hold", "passphrase"] as const;
const APP_SOURCES = ["system", "fdroid", "play-compat", "sideload"] as const;
const APP_STATES = ["installed", "blocked", "absent"] as const;
const NETWORK_POLICIES = ["allow", "wifi-only", "deny"] as const;
const PERMISSION_NAMES = [
  "location",
  "camera",
  "microphone",
  "contacts",
  "storage",
  "notifications",
  "background-activity",
  "nearby-devices",
] as const;
const PERMISSION_DECISIONS = ["allow", "ask", "deny"] as const;
const TOGGLE_TARGETS = ["flashlight", "do-not-disturb", "wifi", "airplane-mode", "grayscale"] as const;
const WIDGET_SIZES = ["1x1", "2x1", "2x2", "4x1", "4x2"] as const;
const GESTURES = [
  "swipe-up",
  "swipe-down",
  "swipe-left",
  "swipe-right",
  "double-tap",
  "long-press-home",
] as const;
const BUDGET_ACTIONS = ["warn", "grayscale", "lock"] as const;
const DOM_SIGNALS = [
  "append-on-scroll-mutation",
  "sentinel-intersection-observer",
  "unbounded-scroll-height",
  "recycled-virtual-list",
  "absent-pagination-controls",
] as const;
const A11Y_SIGNALS = [
  "unbounded-collection-item-count",
  "scroll-event-without-page-boundary",
  "recycler-view-refill",
] as const;

const enumOf = <T extends string>(allowed: readonly T[], value: unknown): value is T =>
  isString(value) && (allowed as readonly string[]).includes(value);

const requireEnum = <T extends string>(
  push: Push,
  path: string,
  allowed: readonly T[],
  value: unknown,
): value is T => {
  if (enumOf(allowed, value)) return true;
  push(path, "invalid-enum", `expected one of ${allowed.join(", ")}`);
  return false;
};

const requireString = (push: Push, path: string, value: unknown): value is string => {
  if (isString(value)) return true;
  push(path, "invalid-type", "expected a string");
  return false;
};

const requireBoolean = (push: Push, path: string, value: unknown): value is boolean => {
  if (typeof value === "boolean") return true;
  push(path, "invalid-type", "expected a boolean");
  return false;
};

const requireIntInRange = (
  push: Push,
  path: string,
  value: unknown,
  min: number,
  max: number,
): value is number => {
  if (!isFiniteNumber(value) || !Number.isInteger(value)) {
    push(path, "invalid-type", "expected an integer");
    return false;
  }
  if (value < min || value > max) {
    push(path, "out-of-range", `expected an integer between ${min} and ${max}`);
    return false;
  }
  return true;
};

const requireArray = (push: Push, path: string, value: unknown): value is readonly unknown[] => {
  if (Array.isArray(value)) return true;
  push(path, "invalid-type", "expected an array");
  return false;
};

const requireRecord = (
  push: Push,
  path: string,
  value: unknown,
): value is Record<string, unknown> => {
  if (isRecord(value)) return true;
  push(path, "invalid-type", "expected an object");
  return false;
};

const requirePackageId = (push: Push, path: string, value: unknown): value is string => {
  if (!requireString(push, path, value)) return false;
  if (!PACKAGE_ID.test(value)) {
    push(path, "invalid-format", "expected a reverse-DNS package id");
    return false;
  }
  return true;
};

const requireResourceId = (push: Push, path: string, value: unknown): value is string => {
  if (!requireString(push, path, value)) return false;
  if (!RESOURCE_ID.test(value)) {
    push(path, "invalid-format", "expected a kebab-case id of 1-64 characters");
    return false;
  }
  return true;
};

const requireUnique = (
  push: Push,
  path: string,
  ids: readonly string[],
  code: string,
): void => {
  const seen = new Set<string>();
  ids.forEach((id, index) => {
    if (seen.has(id)) push(`${path}/${index}`, code, `duplicate id '${id}'`);
    seen.add(id);
  });
};

const validateMetadata = (push: Push, value: unknown): void => {
  if (!requireRecord(push, "/metadata", value)) return;
  requireString(push, "/metadata/name", value["name"]);
  requireIntInRange(push, "/metadata/revision", value["revision"], 0, Number.MAX_SAFE_INTEGER);
  if (value["description"] !== undefined) requireString(push, "/metadata/description", value["description"]);
  if (value["labels"] !== undefined && requireRecord(push, "/metadata/labels", value["labels"])) {
    const entries = Object.entries(value["labels"]);
    if (entries.length > LIMITS.maxLabelCount) {
      push("/metadata/labels", "too-many", `at most ${LIMITS.maxLabelCount} labels`);
    }
    for (const [key, label] of entries) requireString(push, `/metadata/labels/${key}`, label);
  }
};

const validateShortcutAction = (push: Push, path: string, value: unknown): void => {
  if (!requireRecord(push, path, value)) return;
  const kind = value["kind"];
  if (!requireEnum(push, `${path}/kind`, ["open-app", "open-url", "toggle"] as const, kind)) return;
  if (kind === "open-app") requirePackageId(push, `${path}/package`, value["package"]);
  if (kind === "open-url") requireString(push, `${path}/url`, value["url"]);
  if (kind === "toggle") requireEnum(push, `${path}/target`, TOGGLE_TARGETS, value["target"]);
};

const validateLauncherItem = (push: Push, path: string, value: unknown): void => {
  if (!requireRecord(push, path, value)) return;
  requireResourceId(push, `${path}/id`, value["id"]);
  const kind = value["kind"];
  if (!requireEnum(push, `${path}/kind`, ["app", "shortcut", "widget"] as const, kind)) return;
  if (kind === "app") {
    requirePackageId(push, `${path}/package`, value["package"]);
    if (value["label"] !== undefined) requireString(push, `${path}/label`, value["label"]);
  }
  if (kind === "shortcut") {
    requireString(push, `${path}/label`, value["label"]);
    validateShortcutAction(push, `${path}/action`, value["action"]);
  }
  if (kind === "widget") {
    requireString(push, `${path}/provider`, value["provider"]);
    requireEnum(push, `${path}/size`, WIDGET_SIZES, value["size"]);
  }
};

const validateLauncher = (push: Push, value: unknown): void => {
  const base = "/spec/launcher";
  if (!requireRecord(push, base, value)) return;
  requireEnum(push, `${base}/layout`, ["grid", "list"] as const, value["layout"]);
  requireIntInRange(push, `${base}/columns`, value["columns"], LIMITS.minColumns, LIMITS.maxColumns);
  requireIntInRange(push, `${base}/maxItemsPerPage`, value["maxItemsPerPage"], 1, LIMITS.maxItemsPerPage);

  if (requireArray(push, `${base}/pages`, value["pages"])) {
    const pages = value["pages"];
    if (pages.length > LIMITS.maxPages) {
      push(`${base}/pages`, "too-many", `at most ${LIMITS.maxPages} pages`);
    }
    const pageIds: string[] = [];
    pages.forEach((page, index) => {
      const path = `${base}/pages/${index}`;
      if (!requireRecord(push, path, page)) return;
      if (requireResourceId(push, `${path}/id`, page["id"])) pageIds.push(page["id"] as string);
      if (page["title"] !== undefined) requireString(push, `${path}/title`, page["title"]);
      if (!requireArray(push, `${path}/items`, page["items"])) return;
      const items = page["items"];
      const declaredMax = value["maxItemsPerPage"];
      if (isFiniteNumber(declaredMax) && items.length > declaredMax) {
        push(`${path}/items`, "too-many", `page holds ${items.length} items, maxItemsPerPage is ${declaredMax}`);
      }
      const itemIds: string[] = [];
      items.forEach((item, itemIndex) => {
        validateLauncherItem(push, `${path}/items/${itemIndex}`, item);
        if (isRecord(item) && isString(item["id"])) itemIds.push(item["id"]);
      });
      requireUnique(push, `${path}/items`, itemIds, "duplicate-item-id");
    });
    requireUnique(push, `${base}/pages`, pageIds, "duplicate-page-id");
  }

  if (requireArray(push, `${base}/dock`, value["dock"])) {
    const dock = value["dock"];
    if (dock.length > LIMITS.maxDockItems) {
      push(`${base}/dock`, "too-many", `at most ${LIMITS.maxDockItems} dock entries`);
    }
    dock.forEach((pkg, index) => requirePackageId(push, `${base}/dock/${index}`, pkg));
  }

  if (requireRecord(push, `${base}/appDrawer`, value["appDrawer"])) {
    const drawer = value["appDrawer"];
    requireBoolean(push, `${base}/appDrawer/enabled`, drawer["enabled"]);
    requireEnum(push, `${base}/appDrawer/sort`, ["alphabetical", "manual"] as const, drawer["sort"]);
    requireBoolean(push, `${base}/appDrawer/search`, drawer["search"]);
  }

  if (requireRecord(push, `${base}/gestures`, value["gestures"])) {
    for (const [gesture, action] of Object.entries(value["gestures"])) {
      const path = `${base}/gestures/${gesture}`;
      if (!enumOf(GESTURES, gesture)) {
        push(path, "unknown-gesture", `unknown gesture '${gesture}'`);
        continue;
      }
      if (!requireRecord(push, path, action)) continue;
      const kind = action["kind"];
      if (
        !requireEnum(
          push,
          `${path}/kind`,
          ["none", "open-drawer", "open-notifications", "open-app", "toggle"] as const,
          kind,
        )
      ) {
        continue;
      }
      if (kind === "open-app") requirePackageId(push, `${path}/package`, action["package"]);
      if (kind === "toggle") requireEnum(push, `${path}/target`, TOGGLE_TARGETS, action["target"]);
    }
  }
};

const validateApps = (push: Push, value: unknown): void => {
  const base = "/spec/apps";
  if (!requireRecord(push, base, value)) return;
  requireEnum(push, `${base}/policy`, ["allowlist", "denylist"] as const, value["policy"]);
  if (!requireArray(push, `${base}/entries`, value["entries"])) return;
  const packages: string[] = [];
  value["entries"].forEach((entry, index) => {
    const path = `${base}/entries/${index}`;
    if (!requireRecord(push, path, entry)) return;
    if (requirePackageId(push, `${path}/package`, entry["package"])) {
      packages.push(entry["package"] as string);
    }
    if (entry["label"] !== undefined) requireString(push, `${path}/label`, entry["label"]);
    requireEnum(push, `${path}/source`, APP_SOURCES, entry["source"]);
    requireEnum(push, `${path}/state`, APP_STATES, entry["state"]);
    requireEnum(push, `${path}/network`, NETWORK_POLICIES, entry["network"]);
    if (entry["sandboxProfile"] !== undefined) {
      requireString(push, `${path}/sandboxProfile`, entry["sandboxProfile"]);
    }
    if (requireRecord(push, `${path}/permissions`, entry["permissions"])) {
      for (const [name, decision] of Object.entries(entry["permissions"])) {
        const permPath = `${path}/permissions/${name}`;
        if (!enumOf(PERMISSION_NAMES, name)) {
          push(permPath, "unknown-permission", `unknown permission '${name}'`);
          continue;
        }
        requireEnum(push, permPath, PERMISSION_DECISIONS, decision);
      }
    }
    if (entry["attention"] !== undefined && requireRecord(push, `${path}/attention`, entry["attention"])) {
      const override = entry["attention"];
      if (override["enforcement"] !== undefined) {
        requireEnum(push, `${path}/attention/enforcement`, ENFORCEMENT_MODES, override["enforcement"]);
      }
      if (override["pageSize"] !== undefined) {
        requireIntInRange(
          push,
          `${path}/attention/pageSize`,
          override["pageSize"],
          LIMITS.minPageSize,
          LIMITS.maxPageSize,
        );
      }
    }
  });
  requireUnique(push, `${base}/entries`, packages, "duplicate-package");
};

const validateNotifications = (push: Push, value: unknown): void => {
  const base = "/spec/notifications";
  if (!requireRecord(push, base, value)) return;
  requireEnum(push, `${base}/defaultDisposition`, DISPOSITIONS, value["defaultDisposition"]);

  if (requireRecord(push, `${base}/bundling`, value["bundling"])) {
    const bundling = value["bundling"];
    requireBoolean(push, `${base}/bundling/enabled`, bundling["enabled"]);
    if (requireArray(push, `${base}/bundling/deliveryWindows`, bundling["deliveryWindows"])) {
      const windows = bundling["deliveryWindows"];
      if (bundling["enabled"] === true && windows.length === 0) {
        push(
          `${base}/bundling/deliveryWindows`,
          "empty",
          "bundling is enabled but no delivery windows are declared",
        );
      }
      windows.forEach((time, index) => {
        const path = `${base}/bundling/deliveryWindows/${index}`;
        if (requireString(push, path, time) && !TIME_OF_DAY.test(time)) {
          push(path, "invalid-format", "expected HH:MM");
        }
      });
    }
  }

  if (requireArray(push, `${base}/quietHours`, value["quietHours"])) {
    const windows = value["quietHours"];
    if (windows.length > LIMITS.maxQuietHoursWindows) {
      push(`${base}/quietHours`, "too-many", `at most ${LIMITS.maxQuietHoursWindows} quiet-hours windows`);
    }
    const ids: string[] = [];
    windows.forEach((window, index) => {
      const path = `${base}/quietHours/${index}`;
      if (!requireRecord(push, path, window)) return;
      if (requireResourceId(push, `${path}/id`, window["id"])) ids.push(window["id"] as string);
      for (const field of ["start", "end"] as const) {
        const fieldPath = `${path}/${field}`;
        const time = window[field];
        if (requireString(push, fieldPath, time) && !TIME_OF_DAY.test(time)) {
          push(fieldPath, "invalid-format", "expected HH:MM");
        }
      }
      if (requireArray(push, `${path}/days`, window["days"])) {
        window["days"].forEach((day, dayIndex) =>
          requireEnum(push, `${path}/days/${dayIndex}`, WEEKDAYS, day),
        );
      }
      if (requireArray(push, `${path}/breakthrough`, window["breakthrough"])) {
        window["breakthrough"].forEach((pkg, pkgIndex) =>
          requirePackageId(push, `${path}/breakthrough/${pkgIndex}`, pkg),
        );
      }
    });
    requireUnique(push, `${base}/quietHours`, ids, "duplicate-window-id");
  }

  if (requireArray(push, `${base}/rules`, value["rules"])) {
    const rules = value["rules"];
    if (rules.length > LIMITS.maxNotificationRules) {
      push(`${base}/rules`, "too-many", `at most ${LIMITS.maxNotificationRules} rules`);
    }
    const ids: string[] = [];
    rules.forEach((rule, index) => {
      const path = `${base}/rules/${index}`;
      if (!requireRecord(push, path, rule)) return;
      if (requireResourceId(push, `${path}/id`, rule["id"])) ids.push(rule["id"] as string);
      requireIntInRange(push, `${path}/priority`, rule["priority"], 0, 1000);
      requireEnum(push, `${path}/disposition`, DISPOSITIONS, rule["disposition"]);
      if (!requireRecord(push, `${path}/match`, rule["match"])) return;
      const match = rule["match"];
      if (Object.keys(match).length === 0) {
        push(`${path}/match`, "empty", "a rule must constrain at least one field");
      }
      if (match["package"] !== undefined) requirePackageId(push, `${path}/match/package`, match["package"]);
      if (match["channel"] !== undefined) requireString(push, `${path}/match/channel`, match["channel"]);
      if (match["titleContains"] !== undefined) {
        requireString(push, `${path}/match/titleContains`, match["titleContains"]);
      }
      if (match["ongoing"] !== undefined) requireBoolean(push, `${path}/match/ongoing`, match["ongoing"]);
    });
    requireUnique(push, `${base}/rules`, ids, "duplicate-rule-id");
  }
};

const validateDetector = (push: Push, path: string, value: unknown): string | undefined => {
  if (!requireRecord(push, path, value)) return undefined;
  const id = requireResourceId(push, `${path}/id`, value["id"]) ? (value["id"] as string) : undefined;
  const kind = value["kind"];
  if (
    !requireEnum(
      push,
      `${path}/kind`,
      ["dom-heuristic", "accessibility-heuristic", "url-pattern", "app-surface"] as const,
      kind,
    )
  ) {
    return id;
  }
  if (kind === "dom-heuristic" || kind === "accessibility-heuristic") {
    const allowed = kind === "dom-heuristic" ? DOM_SIGNALS : A11Y_SIGNALS;
    if (requireArray(push, `${path}/signals`, value["signals"])) {
      const signals = value["signals"];
      if (signals.length === 0) push(`${path}/signals`, "empty", "a heuristic needs at least one signal");
      signals.forEach((signal, index) =>
        requireEnum(push, `${path}/signals/${index}`, allowed, signal),
      );
    }
    const confidence = value["minConfidence"];
    if (!isFiniteNumber(confidence) || confidence <= 0 || confidence > 1) {
      push(`${path}/minConfidence`, "out-of-range", "expected a number in (0, 1]");
    }
  }
  if (kind === "url-pattern") {
    if (requireString(push, `${path}/pattern`, value["pattern"])) {
      try {
        new RegExp(value["pattern"] as string);
      } catch {
        push(`${path}/pattern`, "invalid-format", "expected a valid regular expression");
      }
    }
  }
  if (kind === "app-surface") {
    requirePackageId(push, `${path}/package`, value["package"]);
    requireString(push, `${path}/surface`, value["surface"]);
  }
  return id;
};

const validateAttention = (push: Push, value: unknown, options: ValidateOptions): void => {
  const base = "/spec/attention";
  if (!requireRecord(push, base, value)) return;

  const scrollPath = `${base}/infiniteScroll`;
  if (requireRecord(push, scrollPath, value["infiniteScroll"])) {
    const policy = value["infiniteScroll"];
    requireEnum(push, `${scrollPath}/enforcement`, ENFORCEMENT_MODES, policy["enforcement"]);
    requireIntInRange(
      push,
      `${scrollPath}/pageSize`,
      policy["pageSize"],
      LIMITS.minPageSize,
      LIMITS.maxPageSize,
    );
    requireIntInRange(push, `${scrollPath}/maxAutoLoads`, policy["maxAutoLoads"], 0, LIMITS.maxAutoLoads);
    requireEnum(push, `${scrollPath}/continuation`, CONTINUATION_GATES, policy["continuation"]);
    requireIntInRange(
      push,
      `${scrollPath}/continuationDelaySeconds`,
      policy["continuationDelaySeconds"],
      LIMITS.minContinuationDelaySeconds,
      LIMITS.maxContinuationDelaySeconds,
    );

    if (requireArray(push, `${scrollPath}/detectors`, policy["detectors"])) {
      const detectors = policy["detectors"];
      if (detectors.length === 0) {
        push(
          `${scrollPath}/detectors`,
          "empty",
          "at least one detector is required; enforcement cannot be disabled by emptying this list",
        );
      }
      if (detectors.length > LIMITS.maxDetectors) {
        push(`${scrollPath}/detectors`, "too-many", `at most ${LIMITS.maxDetectors} detectors`);
      }
      const ids: string[] = [];
      detectors.forEach((detector, index) => {
        const id = validateDetector(push, `${scrollPath}/detectors/${index}`, detector);
        if (id !== undefined) ids.push(id);
      });
      requireUnique(push, `${scrollPath}/detectors`, ids, "duplicate-detector-id");
    }

    if (requireArray(push, `${scrollPath}/exemptions`, policy["exemptions"])) {
      const exemptions = policy["exemptions"];
      if (exemptions.length > LIMITS.maxExemptions) {
        push(
          `${scrollPath}/exemptions`,
          "too-many",
          `at most ${LIMITS.maxExemptions} exemptions may be active at once`,
        );
      }
      const nowMs = Date.parse(options.now);
      const ids: string[] = [];
      exemptions.forEach((exemption, index) => {
        const path = `${scrollPath}/exemptions/${index}`;
        if (!requireRecord(push, path, exemption)) return;
        if (requireResourceId(push, `${path}/id`, exemption["id"])) ids.push(exemption["id"] as string);
        requirePackageId(push, `${path}/package`, exemption["package"]);
        if (requireString(push, `${path}/reason`, exemption["reason"])) {
          const reason = exemption["reason"] as string;
          if (reason.trim().length < LIMITS.minExemptionReasonLength) {
            push(
              `${path}/reason`,
              "too-short",
              `an exemption needs a stated reason of at least ${LIMITS.minExemptionReasonLength} characters`,
            );
          }
        }
        if (requireString(push, `${path}/expiresAt`, exemption["expiresAt"])) {
          const expiresMs = Date.parse(exemption["expiresAt"] as string);
          if (Number.isNaN(expiresMs)) {
            push(`${path}/expiresAt`, "invalid-format", "expected an ISO-8601 timestamp");
          } else if (Number.isNaN(nowMs)) {
            push("/", "invalid-options", "validation options carry an unparseable 'now'");
          } else if (expiresMs <= nowMs) {
            push(`${path}/expiresAt`, "expired", "an exemption must expire in the future");
          } else if (expiresMs - nowMs > LIMITS.maxExemptionDays * 86_400_000) {
            push(
              `${path}/expiresAt`,
              "too-distant",
              `an exemption may not run longer than ${LIMITS.maxExemptionDays} days`,
            );
          }
        }
      });
      requireUnique(push, `${scrollPath}/exemptions`, ids, "duplicate-exemption-id");
    }

    const webviewPath = `${scrollPath}/webview`;
    if (requireRecord(push, webviewPath, policy["webview"])) {
      const webview = policy["webview"];
      requireBoolean(push, `${webviewPath}/injectShim`, webview["injectShim"]);
      requireBoolean(
        push,
        `${webviewPath}/interceptIntersectionObserver`,
        webview["interceptIntersectionObserver"],
      );
      requireBoolean(
        push,
        `${webviewPath}/interceptHistoryScrollRestoration`,
        webview["interceptHistoryScrollRestoration"],
      );
      const multiplier = webview["maxDocumentHeightMultiplier"];
      if (!isFiniteNumber(multiplier) || multiplier < 1 || multiplier > LIMITS.maxDocumentHeightMultiplier) {
        push(
          `${webviewPath}/maxDocumentHeightMultiplier`,
          "out-of-range",
          `expected a number between 1 and ${LIMITS.maxDocumentHeightMultiplier}`,
        );
      }
      if (webview["injectShim"] === false) {
        push(
          `${webviewPath}/injectShim`,
          "policy-violation",
          "the webview shim is the only enforcement point for web feeds and cannot be disabled",
        );
      }
    }
  }

  if (requireArray(push, `${base}/sessionBudgets`, value["sessionBudgets"])) {
    const budgets = value["sessionBudgets"];
    if (budgets.length > LIMITS.maxSessionBudgets) {
      push(`${base}/sessionBudgets`, "too-many", `at most ${LIMITS.maxSessionBudgets} budgets`);
    }
    const ids: string[] = [];
    budgets.forEach((budget, index) => {
      const path = `${base}/sessionBudgets/${index}`;
      if (!requireRecord(push, path, budget)) return;
      if (requireResourceId(push, `${path}/id`, budget["id"])) ids.push(budget["id"] as string);
      requireIntInRange(push, `${path}/dailyMinutes`, budget["dailyMinutes"], 0, 1440);
      requireIntInRange(push, `${path}/sessionMinutes`, budget["sessionMinutes"], 1, 1440);
      requireIntInRange(push, `${path}/cooldownMinutes`, budget["cooldownMinutes"], 0, 1440);
      requireEnum(push, `${path}/onExhausted`, BUDGET_ACTIONS, budget["onExhausted"]);
      if (
        isFiniteNumber(budget["dailyMinutes"]) &&
        isFiniteNumber(budget["sessionMinutes"]) &&
        budget["sessionMinutes"] > budget["dailyMinutes"]
      ) {
        push(
          `${path}/sessionMinutes`,
          "inconsistent",
          "sessionMinutes cannot exceed dailyMinutes",
        );
      }
      if (!requireRecord(push, `${path}/scope`, budget["scope"])) return;
      const scope = budget["scope"];
      const kind = scope["kind"];
      if (!requireEnum(push, `${path}/scope/kind`, ["app", "domain", "system-wide"] as const, kind)) return;
      if (kind === "app") requirePackageId(push, `${path}/scope/package`, scope["package"]);
      if (kind === "domain") requireString(push, `${path}/scope/domain`, scope["domain"]);
    });
    requireUnique(push, `${base}/sessionBudgets`, ids, "duplicate-budget-id");
  }
};

const installedPackages = (spec: unknown): ReadonlySet<string> => {
  if (!isRecord(spec)) return new Set();
  const apps = spec["apps"];
  if (!isRecord(apps) || !Array.isArray(apps["entries"])) return new Set();
  const installed = (apps["entries"] as unknown[]).filter(
    (entry): entry is AppEntry =>
      isRecord(entry) && entry["state"] === "installed" && isString(entry["package"]),
  );
  return new Set(installed.map((entry) => entry.package));
};

const validateReferences = (push: Push, spec: unknown): void => {
  if (!isRecord(spec)) return;
  const installed = installedPackages(spec);
  const requireInstalled = (path: string, pkg: unknown): void => {
    if (!isString(pkg)) return;
    if (!installed.has(pkg)) {
      push(path, "dangling-reference", `'${pkg}' is not an installed app in /spec/apps/entries`);
    }
  };

  const launcher = spec["launcher"];
  if (isRecord(launcher)) {
    if (Array.isArray(launcher["dock"])) {
      (launcher["dock"] as unknown[]).forEach((pkg, index) =>
        requireInstalled(`/spec/launcher/dock/${index}`, pkg),
      );
    }
    if (Array.isArray(launcher["pages"])) {
      (launcher["pages"] as unknown[]).forEach((page, pageIndex) => {
        if (!isRecord(page) || !Array.isArray(page["items"])) return;
        (page["items"] as unknown[]).forEach((item, itemIndex) => {
          if (!isRecord(item)) return;
          const path = `/spec/launcher/pages/${pageIndex}/items/${itemIndex}`;
          if (item["kind"] === "app") requireInstalled(`${path}/package`, item["package"]);
          if (item["kind"] === "shortcut" && isRecord(item["action"]) && item["action"]["kind"] === "open-app") {
            requireInstalled(`${path}/action/package`, item["action"]["package"]);
          }
        });
      });
    }
    if (isRecord(launcher["gestures"])) {
      for (const [gesture, action] of Object.entries(launcher["gestures"])) {
        if (isRecord(action) && action["kind"] === "open-app") {
          requireInstalled(`/spec/launcher/gestures/${gesture}/package`, action["package"]);
        }
      }
    }
  }

  const notifications = spec["notifications"];
  if (isRecord(notifications)) {
    if (Array.isArray(notifications["quietHours"])) {
      (notifications["quietHours"] as unknown[]).forEach((window, index) => {
        if (!isRecord(window) || !Array.isArray(window["breakthrough"])) return;
        (window["breakthrough"] as unknown[]).forEach((pkg, pkgIndex) =>
          requireInstalled(`/spec/notifications/quietHours/${index}/breakthrough/${pkgIndex}`, pkg),
        );
      });
    }
    if (Array.isArray(notifications["rules"])) {
      (notifications["rules"] as unknown[]).forEach((rule, index) => {
        if (!isRecord(rule) || !isRecord(rule["match"])) return;
        if (rule["match"]["package"] !== undefined) {
          requireInstalled(`/spec/notifications/rules/${index}/match/package`, rule["match"]["package"]);
        }
      });
    }
  }

  const attention = spec["attention"];
  if (isRecord(attention)) {
    const policy = attention["infiniteScroll"];
    if (isRecord(policy)) {
      if (Array.isArray(policy["exemptions"])) {
        (policy["exemptions"] as unknown[]).forEach((exemption, index) => {
          if (!isRecord(exemption)) return;
          requireInstalled(
            `/spec/attention/infiniteScroll/exemptions/${index}/package`,
            exemption["package"],
          );
        });
      }
      if (Array.isArray(policy["detectors"])) {
        (policy["detectors"] as unknown[]).forEach((detector, index) => {
          if (!isRecord(detector) || detector["kind"] !== "app-surface") return;
          requireInstalled(`/spec/attention/infiniteScroll/detectors/${index}/package`, detector["package"]);
        });
      }
    }
    if (Array.isArray(attention["sessionBudgets"])) {
      (attention["sessionBudgets"] as unknown[]).forEach((budget, index) => {
        if (!isRecord(budget) || !isRecord(budget["scope"])) return;
        if (budget["scope"]["kind"] === "app") {
          requireInstalled(`/spec/attention/sessionBudgets/${index}/scope/package`, budget["scope"]["package"]);
        }
      });
    }
  }
};

export const validateConfig = (
  input: unknown,
  options: ValidateOptions,
): Result<NormalConfig, ValidationIssue[]> => {
  const issues: ValidationIssue[] = [];
  const push: Push = (path, code, message) => {
    issues.push({ path, code, message });
  };

  if (!requireRecord(push, "", input)) return err(issues);

  if (input["apiVersion"] !== API_VERSION) {
    push("/apiVersion", "unsupported-version", `expected apiVersion '${API_VERSION}'`);
  }
  if (input["kind"] !== KIND) {
    push("/kind", "unsupported-kind", `expected kind '${KIND}'`);
  }

  validateMetadata(push, input["metadata"]);

  if (requireRecord(push, "/spec", input["spec"])) {
    const spec = input["spec"];
    validateLauncher(push, spec["launcher"]);
    validateApps(push, spec["apps"]);
    validateNotifications(push, spec["notifications"]);
    validateAttention(push, spec["attention"], options);
    validateReferences(push, spec);
  }

  return issues.length > 0 ? err(issues) : ok(input as unknown as NormalConfig);
};

export const isValidConfig = (input: unknown, options: ValidateOptions): input is NormalConfig =>
  validateConfig(input, options).ok;

export const LIMITS = {
  minColumns: 1,
  maxColumns: 8,
  maxPages: 8,
  maxItemsPerPage: 24,
  maxDockItems: 5,
  minPageSize: 5,
  maxPageSize: 100,
  maxAutoLoads: 3,
  minContinuationDelaySeconds: 0,
  maxContinuationDelaySeconds: 60,
  maxDocumentHeightMultiplier: 4,
  maxExemptions: 3,
  maxExemptionDays: 30,
  minExemptionReasonLength: 12,
  maxNotificationRules: 64,
  maxQuietHoursWindows: 8,
  maxSessionBudgets: 32,
  maxDetectors: 32,
  maxLabelCount: 16,
} as const;

export type Limits = typeof LIMITS;

export const KEYED_COLLECTIONS: Readonly<Record<string, string>> = {
  "/spec/launcher/pages": "id",
  "/spec/launcher/pages/*/items": "id",
  "/spec/apps/entries": "package",
  "/spec/notifications/quietHours": "id",
  "/spec/notifications/rules": "id",
  "/spec/attention/infiniteScroll/detectors": "id",
  "/spec/attention/infiniteScroll/exemptions": "id",
  "/spec/attention/sessionBudgets": "id",
};

export const keyFieldFor = (pattern: string): string | undefined => KEYED_COLLECTIONS[pattern];

export const isKeyedCollection = (pattern: string): boolean => pattern in KEYED_COLLECTIONS;

import { FILES } from "./render.js";

export const SERVICES = {
  launcher: "normal-launcher",
  appd: "normal-appd",
  notifyd: "normal-notifyd",
  attentiond: "normal-attentiond",
  webviewShim: "normal-webview-shim",
} as const;

export type ServiceName = (typeof SERVICES)[keyof typeof SERVICES];

export type FileOwnership = {
  readonly file: string;
  readonly sources: readonly string[];
  readonly readers: readonly ServiceName[];
};

export const FILE_OWNERSHIP: readonly FileOwnership[] = [
  {
    file: FILES.metadata,
    sources: ["/apiVersion", "/kind", "/metadata"],
    readers: [],
  },
  {
    file: FILES.launcher,
    sources: ["/spec/launcher"],
    readers: [SERVICES.launcher],
  },
  {
    file: FILES.apps,
    sources: ["/spec/apps"],
    readers: [SERVICES.appd, SERVICES.launcher],
  },
  {
    file: FILES.notifications,
    sources: ["/spec/notifications"],
    readers: [SERVICES.notifyd],
  },
  {
    file: FILES.attention,
    sources: ["/spec/attention"],
    readers: [SERVICES.attentiond],
  },
  {
    file: FILES.webviewShim,
    sources: ["/spec/attention", "/spec/apps"],
    readers: [SERVICES.webviewShim],
  },
];

const covers = (source: string, path: string): boolean =>
  path === source || path.startsWith(`${source}/`);

export const filesFor = (path: string): readonly string[] =>
  FILE_OWNERSHIP.filter((entry) => entry.sources.some((source) => covers(source, path))).map(
    (entry) => entry.file,
  );

export const isOwnedPath = (path: string): boolean => filesFor(path).length > 0;

export const readersOf = (file: string): readonly ServiceName[] =>
  FILE_OWNERSHIP.find((entry) => entry.file === file)?.readers ?? [];

export const servicesForFiles = (files: readonly string[]): readonly ServiceName[] =>
  [...new Set(files.flatMap(readersOf))].sort();

export const servicesFor = (paths: readonly string[]): readonly ServiceName[] =>
  servicesForFiles(paths.flatMap(filesFor));

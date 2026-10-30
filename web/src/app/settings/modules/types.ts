import type { PlatformComponents } from "@erpora/platform-contracts";

type Schemas = PlatformComponents["schemas"];

/** Bentuk respons API modul (dari kontrak hasil generate; satu sumber dengan backend). */
export type ModuleView = Schemas["Module"];
export type ModuleStatus = Schemas["ModuleStatus"];
export type OperationView = Schemas["Operation"];
export type EventView = Schemas["Event"];
export type StepView = Schemas["Step"];

export const MODULE_OPS = [
  "install",
  "enable",
  "disable",
  "upgrade",
  "uninstall",
  "retry",
] as const;
export type ModuleOp = (typeof MODULE_OPS)[number];

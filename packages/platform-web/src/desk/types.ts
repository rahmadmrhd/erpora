import type { PlatformComponents } from "@erpora/platform-contracts";

/**
 * Tipe Desk di klien (design M3a "Komponen 2"): diturunkan dari kontrak terbangkit (`platform.gen.ts`, skema
 * `Desk`/`DeskMenuItem`/`DeskMenuGroup`/`DesksEnvelope`) agar tidak menyimpang dari OpenAPI.
 */
type Schemas = PlatformComponents["schemas"];

/** Rujukan kunci i18n (namespace + kunci); teks tidak pernah datang dari server. */
export type MessageRef = Schemas["MessageRef"];

export type DeskMenuItem = Schemas["DeskMenuItem"];

export type DeskMenuGroup = Schemas["DeskMenuGroup"];

export type Desk = Schemas["Desk"];

export type DeskList = Schemas["DesksEnvelope"]["data"];

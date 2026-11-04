// Dibangkitkan oleh `erpora contracts:generate` dari contracts/module.schema.json. JANGAN DIEDIT.

/**
 * Kunci i18n bertitik pada katalog namespace modul (mis. nav.notes).
 */
export type MessageKey = string;
/**
 * Nama ikon dari peta ikon platform.
 */
export type IconName = string;
/**
 * Item menu atau grup satu tingkat (grup tidak boleh bersarang).
 */
export type DeskMenu = DeskItem | DeskGroup;
/**
 * Item menu: tepat satu dari permission atau access (PRD-000 Q43).
 */
export type DeskItem = DeskPermissionItem | DeskAccessItem;
/**
 * Id menu, unik dalam satu Desk (item, grup, dan anak grup).
 */
export type DeskMenuId = string;
/**
 * Kunci resource, dimensi, atau role: <slug>.<nama_snake> (awalan slug diperiksa registry).
 */
export type ResourceKey = string;
/**
 * Kunci aksi: aksi standar PRD-001 FR-110 atau aksi kustom ^[a-z_]+$ (keanggotaan diperiksa registry).
 */
export type ActionKey = string;
/**
 * Nama field resource (snake_case).
 */
export type FieldName = string;
/**
 * Kondisi scope: all, owner, assigned:<dimensi>, atau custom:<key> (PRD-001 FR-111).
 */
export type ScopeToken = string;

export interface ModuleManifest {
  schema_version: "1";
  key: string;
  slug: string;
  name: string;
  version: string;
  publisher: {
    id: string;
    name: string;
  };
  trust: "builtin" | "verified" | "untrusted";
  required?: boolean;
  compatibility: {
    platform: string;
    sdk: string;
  };
  dependencies: {
    module: string;
    version: string;
    optional: boolean;
  }[];
  capabilities: string[];
  provides?: string[];
  migrations: {
    strategy: "versioned" | "none";
  };
  offline: {
    mode: "online-only" | "offline-read" | "offline-draft" | "offline-write";
  };
  /**
   * PRD-000 FR-25: katalog terjemahan modul (namespace = slug). Opsional; modul tanpa bagian ini tidak punya namespace.
   */
  i18n?: {
    /**
     * @minItems 1
     * @maxItems 50
     */
    locales: [string, ...string[]];
    defaultLocale: string;
  };
  /**
   * Revisi M3a (Q40 butir 10-13): Desk modul. label/icon/menu dibaca server untuk menyusun daftar Desk per pengguna (GET /api/platform/desks); Dashboard dibangkitkan platform dan tidak boleh dideklarasikan. Opsional di schema; module:check mewajibkannya untuk modul berdirektori web/.
   */
  desk?: {
    label: MessageKey;
    icon: IconName;
    /**
     * Label dan ikon item Dashboard bawaan Desk; bila tidak diisi platform memakai label dan ikon bawaan.
     */
    dashboard?: {
      label?: MessageKey;
      icon?: IconName;
    };
    /**
     * @maxItems 100
     */
    menus: DeskMenu[];
    /**
     * Pengaturan Desk; isi diperketat kemudian.
     */
    settings?: {};
  };
  /**
   * Marketplace: lokasi artefak modul.
   */
  artifact?: string;
  /**
   * Marketplace: checksum artefak.
   */
  checksum?: string;
  /**
   * Marketplace: tanda tangan artefak.
   */
  signature?: string;
  /**
   * Marketplace: izin yang diminta modul.
   */
  requested_permissions?: string[];
  /**
   * Marketplace: lisensi modul.
   */
  license?: string;
  /**
   * PRD-001 FR-110: resource dan aksi permission modul. Permission diturunkan sebagai <key>.<aksi>; label berupa kunci i18n (Q40). Aturan lintas-field (aksi standar, grup default, dimensi) diperiksa registry dan module:check.
   */
  resources?: Resource[];
  /**
   * PRD-001 FR-110: dimensi scope modul (mis. gudang).
   */
  scope_dimensions?: ScopeDimension[];
  /**
   * PRD-001 FR-110, FR-113: role bawaan modul beserta matriks awalnya.
   */
  default_roles?: DefaultRole[];
  /**
   * PRD-001 FR-120: definisi setting modul. Isi diperketat kemudian.
   */
  settings?: {};
}
export interface DeskPermissionItem {
  id: DeskMenuId;
  label: MessageKey;
  icon?: IconName;
  /**
   * Path internal relatif base route modul (/notes); tanpa skema, host, //, query, atau fragmen.
   */
  to: string;
  /**
   * Salah satu capabilities modul; item disembunyikan bila pengguna tidak memilikinya.
   */
  permission: string;
}
export interface DeskAccessItem {
  id: DeskMenuId;
  label: MessageKey;
  icon?: IconName;
  /**
   * Path internal relatif base route modul (/notes); tanpa skema, host, //, query, atau fragmen.
   */
  to: string;
  /**
   * Pengganti eksplisit permission: item terlihat bagi setiap pengguna yang login (PRD-000 Q43). Tidak boleh bersama permission.
   */
  access: "authenticated";
}
export interface DeskGroup {
  id: DeskMenuId;
  label: MessageKey;
  icon?: IconName;
  /**
   * @minItems 1
   * @maxItems 100
   */
  children: [DeskItem, ...DeskItem[]];
}
export interface Resource {
  key: ResourceKey;
  label: MessageKey;
  actions: ActionKey[];
  custom_actions?: CustomAction[];
  owner_field?: FieldName;
  /**
   * Dimensi scope (kunci dimensi) menuju nama field resource.
   */
  scope_fields?: {
    [k: string]: FieldName;
  };
  /**
   * @minItems 1
   */
  field_groups: [FieldGroup, ...FieldGroup[]];
  /**
   * PRD-001 FR-131: hanya disimpan.
   */
  audit?: {
    data_changes?: boolean;
  };
}
export interface CustomAction {
  key: ActionKey;
  label: MessageKey;
}
export interface FieldGroup {
  key: string;
  label: MessageKey;
  fields?: FieldName[];
  /**
   * Klasifikasi data field group (security-guidelines §2, PRD-001 FR-110).
   */
  classification?: "public" | "internal" | "confidential" | "highly_confidential";
  default?: boolean;
  /**
   * PRD-001 FR-131: hanya disimpan.
   */
  sensitive?: boolean;
}
export interface ScopeDimension {
  key: ResourceKey;
  label: MessageKey;
  /**
   * Kunci resource, dimensi, atau role: <slug>.<nama_snake> (awalan slug diperiksa registry).
   */
  source?: string;
}
export interface DefaultRole {
  key: ResourceKey;
  label: MessageKey;
  permissions: RoleRow[];
  field_access?: RoleFieldAccess[];
}
export interface RoleRow {
  resource: ResourceKey;
  /**
   * @minItems 1
   */
  actions: [ActionKey, ...ActionKey[]];
  /**
   * Daftar kondisi digabung AND; daftar kosong tidak sah (PRD-001 FR-111).
   *
   * @minItems 1
   */
  scope: [ScopeToken, ...ScopeToken[]];
}
export interface RoleFieldAccess {
  resource: ResourceKey;
  field_group: string;
  access: "none" | "read" | "write";
}

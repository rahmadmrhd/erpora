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
   * PRD-001 FR-110: resource dan aksi permission modul. Hanya field_groups[].classification yang divalidasi (security-guidelines §2); isi lain diperketat kemudian.
   */
  resources?: {
    field_groups?: {
      key: string;
      fields?: string[];
      /**
       * Klasifikasi data field group (security-guidelines §2, PRD-001 FR-110).
       */
      classification?: "public" | "internal" | "confidential" | "highly_confidential";
      [k: string]: unknown;
    }[];
    [k: string]: unknown;
  }[];
  /**
   * PRD-001 FR-110: dimensi scope modul. Isi diperketat kemudian.
   */
  scope_dimensions?: unknown[];
  /**
   * PRD-001 FR-110: role bawaan modul. Isi diperketat kemudian.
   */
  default_roles?: unknown[];
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

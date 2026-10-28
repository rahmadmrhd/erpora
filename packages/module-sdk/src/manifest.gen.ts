// Dibangkitkan oleh `erpora contracts:generate` dari contracts/module.schema.json. JANGAN DIEDIT.

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

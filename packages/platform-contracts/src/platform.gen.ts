// Dibangkitkan oleh `erpora contracts:generate` dari contracts/openapi/platform.yaml. JANGAN DIEDIT.

export type paths = {
  "/api/bootstrap": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Manifest bootstrap untuk frontend
     * @description Daftar modul terkompilasi (tanpa platform.core) dengan status efektif. Status efektif dihitung lifecycle dari status tersimpan di store dan kompatibilitas binary (Effective), lalu dipublikasikan ke runtime snapshot yang dibaca endpoint ini. Memuat juga bahasa yang didukung dan revisi katalog terjemahan per namespace aktif (i18n). ETag = revision snapshot (ditambah penanda revisi katalog bila ada).
     */
    get: operations["getBootstrap"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/i18n/{locale}/{namespace}": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Katalog terjemahan satu namespace dan bahasa
     * @description Katalog efektif (pesan ICU MessageFormat) namespace `ui`, `platform`, atau slug modul aktif pada bahasa yang diminta, dengan rantai fallback bahasa sudah digabung (PRD-000 FR-25). `locale` divalidasi terhadap registri bahasa dan `namespace` terhadap daftar tertutup (ui, platform, modul aktif); selain itu 404 not_found tanpa membedakan penyebabnya. Publik pada fase ini (hanya teks terjemahan). ETag berubah bila isi berubah; selalu direvalidasi (Cache-Control private, no-cache).
     */
    get: operations["getI18nCatalog"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/i18n/overrides": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Daftar override istilah terjemahan tenant
     * @description Override teks katalog milik tenant ini (PRD-000 FR-25, Q34), berpagination halaman dan dapat difilter per namespace dan bahasa. Klasifikasi data internal.
     */
    get: operations["listI18nOverrides"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/i18n/overrides/{namespace}/{locale}/{key}": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    /**
     * Simpan (upsert) satu override istilah
     * @description Mengganti teks bawaan untuk (namespace, locale, key) pada tenant ini. Validasi: namespace aktif dan locale terdaftar; key ada di katalog bawaan bahasa default namespace itu; pesan lolos sintaks ICU dalam batas ukuran, tanpa HTML, dan memiliki argumen yang sama dengan pesan bawaan. Idempoten: nilai yang sama tidak mengubah apa pun dan tidak menaikkan revisi. Hanya aktif bila ERP_ENV=development atau ERP_MODULES_ADMIN_API=true; selain itu 404 not_found sebelum permission. ETag respons (kuat) dapat dipakai pada If-Match. PUT dipakai (pengecualian api-guidelines §3, x-erp-allow-put) karena resource-nya hanya satu bidang `message` sehingga ganti penuh tidak berisiko menghapus field.
     */
    put: operations["putI18nOverride"];
    post?: never;
    /**
     * Hapus satu override istilah (kembali ke bawaan)
     * @description Menghapus override; revisi namespace dan bahasa naik hanya bila barisnya ada. Namespace atau bahasa tak dikenal, atau override yang tidak ada, menghasilkan 404 not_found. Gerbang dan pengamanan sama dengan PUT.
     */
    delete: operations["deleteI18nOverride"];
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/modules": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Daftar modul beserta status
     * @description Semua modul terkompilasi beserta status efektifnya untuk tenant ini, berpagination halaman.
     */
    get: operations["listModules"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/modules/{key}": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Detail satu modul
     * @description Status satu modul untuk tenant ini; key tidak dikenal menghasilkan 404 not_found.
     */
    get: operations["getModule"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/modules/{key}/{operation}": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Jalankan operasi lifecycle (install, enable, disable, upgrade, uninstall, retry)
     * @description Hanya aktif di development atau dengan ERP_MODULES_ADMIN_API=true (selain itu 404 not_found). Sinkron dengan batas tunggu 30 detik; bila lebih lama -> 202 + Location ke operations/{id} dan operasi berlanjut di server (berlaku juga untuk retry). Operasi yang berstatus failed tetap 200 dengan operations[].status = failed. Idempoten: operasi yang status targetnya sudah tercapai -> 200 dengan operations kosong. Idempotency-Key diterima tetapi tidak disimpan. Kode error lifecycle (FR-19) ada di error.details[].code.
     */
    post: operations["runModuleOperation"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/modules/{key}/history": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Riwayat event lifecycle modul (terbaru dulu)
     * @description Diimplementasi oleh route terdaftar GET /api/platform/modules/{key}/{sub} (sub = history) karena ServeMux menolak GET /modules/{key}/history bersama GET /modules/operations/{id} sebagai bentrok.
     */
    get: operations["listModuleHistory"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/platform/modules/operations/{id}": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Status satu operasi lifecycle (polling untuk respons 202)
     * @description Operasi milik tenant lain atau yang tidak ada -> 404 not_found (bukan 403). Diimplementasi oleh route terdaftar GET /api/platform/modules/{key}/{sub} dengan key = operations.
     */
    get: operations["getModuleOperation"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
};
export type webhooks = Record<string, never>;
export type components = {
  schemas: {
    ActionBody: {
      /** @description Aktifkan setelah install (hanya install). */
      enable?: boolean;
      /** @description Sertakan dependensi wajib (hanya install). */
      withDeps?: boolean;
    };
    BootstrapEnvelope: {
      data: {
        appVersion: string;
        /** @description Bendera kemampuan server untuk UI (additive). */
        features?: {
          /** @description API mutasi /api/platform/modules aktif (ERP_ENV=development atau ERP_MODULES_ADMIN_API=true). */
          modulesAdminApi: boolean;
        };
        /** @description Bahasa yang didukung dan revisi katalog per namespace aktif (PRD-000 FR-25); frontend memuat ulang katalog bila revisinya berubah. */
        i18n?: {
          defaultLocale: string;
          locales: {
            code: string;
            name: string;
          }[];
          revisions: {
            [key: string]: string;
          };
        };
        modules: {
          basePath: string;
          key: string;
          offline: boolean;
          slug: string;
          status: components["schemas"]["ModuleStatus"];
          version: string;
        }[];
        permissions: string[];
        /** Format: int64 */
        revision: number;
        tenantId: components["schemas"]["common.Uuid"];
      };
    };
    "common.Error": {
      error: {
        /** @description Kode stabil (api-guidelines section 8). */
        code: string;
        /** @description Array ErrorDetail, atau objek konteks (mis. pada 412 version_mismatch). */
        details?:
          | components["schemas"]["common.ErrorDetail"][]
          | {
              [key: string]: unknown;
            };
        message: string;
        requestId?: string;
        traceId?: string;
      };
    };
    /** @description Satu butir detail error. code stabil dan menjadi kontrak; message untuk manusia. */
    "common.ErrorDetail": {
      code: string;
      /** @description Path JSON field, mis. lines[2].itemId. */
      field?: string;
      message: string;
    } & {
      [key: string]: unknown;
    };
    "common.Page": {
      hasMore: boolean;
      page: number;
      pageSize: number;
      /** Format: int64 */
      total: number;
    };
    /**
     * Format: date-time
     * @description ISO 8601 UTC, mis. 2026-10-21T07:15:00Z.
     */
    "common.Timestamp": string;
    /**
     * Format: uuid
     * @description UUID (v7 untuk ID baru).
     */
    "common.Uuid": string;
    Dependency: {
      module: string;
      optional: boolean;
      version: string;
    };
    Event: {
      /** @description cli:<user>, api:dev, system:reconcile, atau system:migrate. */
      actor: string;
      details: {
        [key: string]: unknown;
      };
      /** @description installed, enabled, disabled, upgraded, uninstalled, operation_failed, interrupted, retried, reconciled. */
      event: string;
      id: components["schemas"]["common.Uuid"];
      moduleKey: string;
      occurredAt: components["schemas"]["common.Timestamp"];
      operationId: components["schemas"]["common.Uuid"] | null;
    };
    I18nCatalogEnvelope: {
      data: {
        locale: string;
        /** @description Kunci lengkap bertitik ke pesan ICU MessageFormat. */
        messages: {
          [key: string]: string;
        };
        namespace: string;
        /** @description Revisi namespace (sama dengan bootstrap.i18n.revisions[namespace]). */
        revision: string;
      };
    };
    I18nOverride: {
      /** @description ETag kuat baris (hash isi); dipakai pada If-Match. */
      etag: string;
      key: string;
      locale: string;
      message: string;
      namespace: string;
      updatedAt: components["schemas"]["common.Timestamp"];
      /** @description api:dev atau cli:<pengguna>. */
      updatedBy: string;
    };
    I18nOverrideBody: {
      /** @description Pesan ICU MessageFormat pengganti; argumen harus sama dengan pesan bawaan; tanpa HTML. */
      message: string;
    };
    /** @description Bentuk error.details pada 412 version_mismatch override (OverrideView terbaru; hilang bila baris belum ada). */
    I18nOverridePreconditionDetails: {
      current: components["schemas"]["I18nOverride"];
    };
    Module: {
      binaryVersion: string;
      compatible: boolean;
      dependencies: components["schemas"]["Dependency"][];
      dependents: string[];
      incompatibleCodes: string[];
      installedVersion: string | null;
      key: string;
      /** Format: uuid */
      lastOperationId: string | null;
      name: string;
      required: boolean;
      slug: string;
      status: components["schemas"]["ModuleStatus"];
    };
    /** @enum {string} */
    ModuleStatus:
      | "available"
      | "installed"
      | "enabled"
      | "disabled"
      | "upgrade_pending"
      | "failed"
      | "uninstalled"
      | "missing"
      | "incompatible";
    Operation: {
      actor: string;
      failure: {
        code: string;
        message: string;
        step?: string;
      } | null;
      finishedAt: components["schemas"]["common.Timestamp"] | null;
      fromVersion: string | null;
      id: components["schemas"]["common.Uuid"];
      moduleKey: string;
      /** @enum {string} */
      operation: "install" | "enable" | "disable" | "upgrade" | "uninstall" | "reconcile";
      startedAt: components["schemas"]["common.Timestamp"];
      /** @enum {string} */
      status: "running" | "succeeded" | "failed";
      steps: components["schemas"]["Step"][];
      toVersion: string | null;
    };
    Step: {
      /** Format: int64 */
      durationMs: number;
      error: string | null;
      name: string;
      startedAt: components["schemas"]["common.Timestamp"];
      /** @enum {string} */
      status: "running" | "succeeded" | "failed" | "skipped";
    };
  };
  responses: {
    /** @description Parameter atau body salah (bad_request). */
    "common.BadRequest": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description internal_error; tanpa detail internal. */
    "common.InternalError": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description Tidak ada, atau di luar scope (not_found). Record di luar scope selalu 404, bukan 403. */
    "common.NotFound": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description not_ready; layanan belum siap. */
    "common.NotReady": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description Tidak punya permission (permission_denied). */
    "common.PermissionDenied": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description If-Match tidak cocok (version_mismatch). */
    "common.PreconditionFailed": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
    /** @description validation_failed atau business_rule_violated; kode spesifik di error.details[].code. */
    "common.Unprocessable": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
  };
  parameters: {
    /** @description UUID dari klien. Diterima, tetapi untuk endpoint platform modul TIDAK disimpan (setiap operasi idempoten lewat aturan no-op); penyimpanan 24 jam menunggu infrastruktur idempotency. */
    "common.IdempotencyKey": string;
    /** @description Opsional. Revision yang diharapkan; tidak cocok -> 412 version_mismatch. */
    "common.IfMatch": string;
    /** @description Halaman, mulai 1. */
    "common.Page": number;
    /** @description Jumlah item per halaman. */
    "common.PageSize": number;
    /** @description Key modul, mis. acme.example. */
    Key: string;
    /** @description Kode bahasa terdaftar di registri bahasa (mis. id, en, en-GB). */
    Locale: string;
    /** @description ui, platform, atau slug modul aktif. */
    Namespace: string;
    /** @description Kunci lengkap bertitik pada katalog bawaan (mis. note.title). */
    OverrideKey: string;
    OverrideLocale: string;
    OverrideNamespace: string;
  };
  requestBodies: never;
  headers: {
    /** @description Versi resource (di endpoint modul platform = revision status modul tenant), mis. "7". */
    "common.ETag": string;
    /** @description URL resource atau operasi yang dibuat/berjalan (201/202). */
    "common.Location": string;
  };
  pathItems: never;
};
export type $defs = Record<string, never>;
export interface operations {
  getBootstrap: {
    parameters: {
      query?: never;
      header?: {
        "If-None-Match"?: string;
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Manifest. */
      200: {
        headers: {
          ETag: components["headers"]["common.ETag"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "appVersion": "0.1.0",
           *         "features": {
           *           "modulesAdminApi": true
           *         },
           *         "i18n": {
           *           "defaultLocale": "id",
           *           "locales": [
           *             {
           *               "code": "id",
           *               "name": "Bahasa Indonesia"
           *             },
           *             {
           *               "code": "en",
           *               "name": "English"
           *             }
           *           ],
           *           "revisions": {
           *             "example": "11aa22bb33cc44dd",
           *             "platform": "8c01d2e9a4b6f730",
           *             "ui": "3f9a1c20b7d84e55"
           *           }
           *         },
           *         "modules": [
           *           {
           *             "basePath": "/example",
           *             "key": "acme.example",
           *             "offline": false,
           *             "slug": "example",
           *             "status": "enabled",
           *             "version": "1.0.0"
           *           }
           *         ],
           *         "permissions": [],
           *         "revision": 5,
           *         "tenantId": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
           *       }
           *     }
           */
          "application/json": components["schemas"]["BootstrapEnvelope"];
        };
      };
      /** @description Tidak berubah (If-None-Match cocok dengan ETag). */
      304: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
    };
  };
  getI18nCatalog: {
    parameters: {
      query?: never;
      header?: {
        "If-None-Match"?: string;
      };
      path: {
        /** @description Kode bahasa terdaftar di registri bahasa (mis. id, en, en-GB). */
        locale: components["parameters"]["Locale"];
        /** @description ui, platform, atau slug modul aktif. */
        namespace: components["parameters"]["Namespace"];
      };
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Katalog efektif. ETag = hash isi `messages`. */
      200: {
        headers: {
          /** @description private, no-cache */
          "Cache-Control"?: string;
          ETag: components["headers"]["common.ETag"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "locale": "en",
           *         "messages": {
           *           "actions.save": "Save",
           *           "pagination.summary": "{total, plural, =0 {No data} one {# item} other {# items}}"
           *         },
           *         "namespace": "ui",
           *         "revision": "3f9a1c20b7d84e55"
           *       }
           *     }
           */
          "application/json": components["schemas"]["I18nCatalogEnvelope"];
        };
      };
      /** @description Tidak berubah (If-None-Match cocok dengan ETag). */
      304: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      404: components["responses"]["common.NotFound"];
    };
  };
  listI18nOverrides: {
    parameters: {
      query?: {
        locale?: string;
        namespace?: string;
        /** @description Halaman, mulai 1. */
        page?: components["parameters"]["common.Page"];
        /** @description Jumlah item per halaman. */
        pageSize?: components["parameters"]["common.PageSize"];
      };
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Daftar override terurut (namespace, bahasa, kunci). */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": [
           *         {
           *           "etag": "\"3f9a1c20b7d84e55\"",
           *           "key": "note.title",
           *           "locale": "id",
           *           "message": "Klien",
           *           "namespace": "example",
           *           "updatedAt": "2026-10-29T10:00:00Z",
           *           "updatedBy": "api:dev"
           *         }
           *       ],
           *       "page": {
           *         "hasMore": false,
           *         "page": 1,
           *         "pageSize": 25,
           *         "total": 1
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["I18nOverride"][];
            page: components["schemas"]["common.Page"];
          };
        };
      };
      400: components["responses"]["common.BadRequest"];
      403: components["responses"]["common.PermissionDenied"];
      503: components["responses"]["common.NotReady"];
    };
  };
  putI18nOverride: {
    parameters: {
      query?: never;
      header?: {
        /** @description Opsional. Revision yang diharapkan; tidak cocok -> 412 version_mismatch. */
        "If-Match"?: components["parameters"]["common.IfMatch"];
      };
      path: {
        /** @description Kunci lengkap bertitik pada katalog bawaan (mis. note.title). */
        key: components["parameters"]["OverrideKey"];
        locale: components["parameters"]["OverrideLocale"];
        namespace: components["parameters"]["OverrideNamespace"];
      };
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "message": "Klien"
         *     }
         */
        "application/json": components["schemas"]["I18nOverrideBody"];
      };
    };
    responses: {
      /** @description Override tersimpan (atau sudah bernilai sama). */
      200: {
        headers: {
          ETag: components["headers"]["common.ETag"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "etag": "\"3f9a1c20b7d84e55\"",
           *         "key": "note.title",
           *         "locale": "id",
           *         "message": "Klien",
           *         "namespace": "example",
           *         "updatedAt": "2026-10-29T10:00:00Z",
           *         "updatedBy": "api:dev"
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["I18nOverride"];
          };
        };
      };
      /** @description invalid_i18n_override dengan error.details[].code salah satu: unknown_key, invalid_syntax, param_mismatch, html_not_allowed, too_long; atau bad_request untuk kerusakan struktur body. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
      /** @description version_mismatch: If-Match tidak cocok dengan ETag override saat ini. Bila barisnya ada, error.details.current memuat override terbaru (skema I18nOverride; bentuk lengkap details: I18nOverridePreconditionDetails) sehingga klien dapat menyelesaikan bentrok tanpa permintaan tambahan (api-guidelines section 5); bila barisnya belum ada, details tidak dikirim. */
      412: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "error": {
           *         "code": "version_mismatch",
           *         "details": {
           *           "current": {
           *             "etag": "\"3f9a1c20b7d84e55\"",
           *             "key": "note.title",
           *             "locale": "id",
           *             "message": "Klien",
           *             "namespace": "example",
           *             "updatedAt": "2026-10-29T10:00:00Z",
           *             "updatedBy": "api:dev"
           *           }
           *         },
           *         "message": "Override telah berubah; muat ulang lalu coba lagi."
           *       }
           *     }
           */
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description unsupported_media_type: mutasi wajib Content-Type application/json (charset utf-8). */
      415: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      422: components["responses"]["common.Unprocessable"];
      503: components["responses"]["common.NotReady"];
    };
  };
  deleteI18nOverride: {
    parameters: {
      query?: never;
      header?: {
        /** @description Opsional. Revision yang diharapkan; tidak cocok -> 412 version_mismatch. */
        "If-Match"?: components["parameters"]["common.IfMatch"];
      };
      path: {
        /** @description Kunci lengkap bertitik pada katalog bawaan (mis. note.title). */
        key: components["parameters"]["OverrideKey"];
        locale: components["parameters"]["OverrideLocale"];
        namespace: components["parameters"]["OverrideNamespace"];
      };
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Override dihapus. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
      /** @description version_mismatch: If-Match tidak cocok dengan ETag override saat ini. Bila barisnya ada, error.details.current memuat override terbaru (skema I18nOverride; bentuk lengkap details: I18nOverridePreconditionDetails) sehingga klien dapat menyelesaikan bentrok tanpa permintaan tambahan (api-guidelines section 5); bila barisnya belum ada, details tidak dikirim. */
      412: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "error": {
           *         "code": "version_mismatch",
           *         "details": {
           *           "current": {
           *             "etag": "\"3f9a1c20b7d84e55\"",
           *             "key": "note.title",
           *             "locale": "id",
           *             "message": "Klien",
           *             "namespace": "example",
           *             "updatedAt": "2026-10-29T10:00:00Z",
           *             "updatedBy": "api:dev"
           *           }
           *         },
           *         "message": "Override telah berubah; muat ulang lalu coba lagi."
           *       }
           *     }
           */
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description unsupported_media_type: mutasi wajib Content-Type application/json (charset utf-8). */
      415: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      503: components["responses"]["common.NotReady"];
    };
  };
  listModules: {
    parameters: {
      query?: {
        /** @description Halaman, mulai 1. */
        page?: components["parameters"]["common.Page"];
        /** @description Jumlah item per halaman. */
        pageSize?: components["parameters"]["common.PageSize"];
      };
      header?: {
        "If-None-Match"?: string;
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Daftar modul (urut topologis; modul missing di akhir). ETag = revision. */
      200: {
        headers: {
          ETag: components["headers"]["common.ETag"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": [
           *         {
           *           "binaryVersion": "1.0.0",
           *           "compatible": true,
           *           "dependencies": [
           *             {
           *               "module": "platform.core",
           *               "optional": false,
           *               "version": "^0.1"
           *             }
           *           ],
           *           "dependents": [],
           *           "incompatibleCodes": [],
           *           "installedVersion": "1.0.0",
           *           "key": "acme.example",
           *           "lastOperationId": null,
           *           "name": "Example",
           *           "required": false,
           *           "slug": "example",
           *           "status": "enabled"
           *         }
           *       ],
           *       "page": {
           *         "hasMore": false,
           *         "page": 1,
           *         "pageSize": 25,
           *         "total": 1
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["Module"][];
            page: components["schemas"]["common.Page"];
          };
        };
      };
      /** @description Tidak berubah. */
      304: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      400: components["responses"]["common.BadRequest"];
      403: components["responses"]["common.PermissionDenied"];
      503: components["responses"]["common.NotReady"];
    };
  };
  getModule: {
    parameters: {
      query?: never;
      header?: {
        "If-None-Match"?: string;
      };
      path: {
        /** @description Key modul, mis. acme.example. */
        key: components["parameters"]["Key"];
      };
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Modul. ETag = revision. */
      200: {
        headers: {
          ETag: components["headers"]["common.ETag"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "binaryVersion": "1.0.0",
           *         "compatible": true,
           *         "dependencies": [
           *           {
           *             "module": "platform.core",
           *             "optional": false,
           *             "version": "^0.1"
           *           }
           *         ],
           *         "dependents": [],
           *         "incompatibleCodes": [],
           *         "installedVersion": "1.0.0",
           *         "key": "acme.example",
           *         "lastOperationId": null,
           *         "name": "Example",
           *         "required": false,
           *         "slug": "example",
           *         "status": "enabled"
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["Module"];
          };
        };
      };
      /** @description Tidak berubah. */
      304: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
    };
  };
  runModuleOperation: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima, tetapi untuk endpoint platform modul TIDAK disimpan (setiap operasi idempoten lewat aturan no-op); penyimpanan 24 jam menunggu infrastruktur idempotency. */
        "Idempotency-Key"?: components["parameters"]["common.IdempotencyKey"];
        /** @description Opsional. Revision yang diharapkan; tidak cocok -> 412 version_mismatch. */
        "If-Match"?: components["parameters"]["common.IfMatch"];
      };
      path: {
        /** @description Key modul, mis. acme.example. */
        key: components["parameters"]["Key"];
        operation: "install" | "enable" | "disable" | "upgrade" | "uninstall" | "retry";
      };
      cookie?: never;
    };
    /** @description Hanya untuk install; maksimal 4 KiB, Content-Type application/json, satu objek, field tak dikenal ditolak (422 unknown_field). Body kosong diterima. */
    requestBody?: {
      content: {
        /**
         * @example {
         *       "enable": true,
         *       "withDeps": true
         *     }
         */
        "application/json": components["schemas"]["ActionBody"];
      };
    };
    responses: {
      /** @description Operasi selesai (sukses, gagal, atau no-op). */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "module": {
           *           "binaryVersion": "1.0.0",
           *           "compatible": true,
           *           "dependencies": [
           *             {
           *               "module": "platform.core",
           *               "optional": false,
           *               "version": "^0.1"
           *             }
           *           ],
           *           "dependents": [],
           *           "incompatibleCodes": [],
           *           "installedVersion": "1.0.0",
           *           "key": "acme.example",
           *           "lastOperationId": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c",
           *           "name": "Example",
           *           "required": false,
           *           "slug": "example",
           *           "status": "enabled"
           *         },
           *         "operationIds": [],
           *         "operations": []
           *       }
           *     }
           */
          "application/json": {
            data: {
              module: components["schemas"]["Module"];
              /** @description ID semua operasi pada rangkaian (aditif; urut eksekusi, tanpa noop). */
              operationIds?: components["schemas"]["common.Uuid"][];
              operations: components["schemas"]["Operation"][];
            };
          };
        };
      };
      /** @description Belum selesai dalam 30 detik; pantau lewat Location. */
      202: {
        headers: {
          Location: components["headers"]["common.Location"];
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "operationId": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c",
           *         "operationIds": [
           *           "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"
           *         ],
           *         "status": "running"
           *       }
           *     }
           */
          "application/json": {
            data: {
              operationId: components["schemas"]["common.Uuid"];
              /** @description ID semua operasi rangkaian yang sudah dimulai saat respons dikirim (aditif); operationId dan Location menunjuk yang pertama. */
              operationIds: components["schemas"]["common.Uuid"][];
              /** @enum {string} */
              status: "running";
            };
          };
        };
      };
      /** @description bad_request dengan error.details[].code salah satu: unsupported_media_type, body_too_large, malformed_json, expected_object, trailing_data, too_deep, array_too_long, duplicate_key, atau invalid (withDeps/enable bukan untuk install). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
      /** @description invalid_state (detail invalid_transition) atau request_in_progress (detail operation_in_progress). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      412: components["responses"]["common.PreconditionFailed"];
      /** @description unsupported_media_type: mutasi wajib Content-Type application/json (charset utf-8); body kosong atau {} diperbolehkan. Ini mencegah request lintas-origin tanpa preflight CORS. */
      415: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description validation_failed dengan error.details[].code unknown_field atau invalid_type (body aksi), atau business_rule_violated dengan error.details[].code salah satu: module_required, dependency_not_ready, dependents_active, module_incompatible (satu detail per masalah), downgrade_not_supported. */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      500: components["responses"]["common.InternalError"];
      503: components["responses"]["common.NotReady"];
    };
  };
  listModuleHistory: {
    parameters: {
      query?: {
        /** @description Halaman, mulai 1. */
        page?: components["parameters"]["common.Page"];
        /** @description Jumlah item per halaman. */
        pageSize?: components["parameters"]["common.PageSize"];
      };
      header?: never;
      path: {
        /** @description Key modul, mis. acme.example. */
        key: components["parameters"]["Key"];
      };
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Event lifecycle tenant ini saja. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": [
           *         {
           *           "actor": "cli:admin",
           *           "details": {},
           *           "event": "enabled",
           *           "id": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b",
           *           "moduleKey": "acme.example",
           *           "occurredAt": "2026-10-21T07:15:00Z",
           *           "operationId": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"
           *         }
           *       ],
           *       "page": {
           *         "hasMore": false,
           *         "page": 1,
           *         "pageSize": 25,
           *         "total": 1
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["Event"][];
            page: components["schemas"]["common.Page"];
          };
        };
      };
      400: components["responses"]["common.BadRequest"];
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
    };
  };
  getModuleOperation: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        id: components["schemas"]["common.Uuid"];
      };
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Operasi. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "actor": "cli:admin",
           *         "failure": null,
           *         "finishedAt": "2026-10-21T07:15:01Z",
           *         "fromVersion": "1.0.0",
           *         "id": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c",
           *         "moduleKey": "acme.example",
           *         "operation": "enable",
           *         "startedAt": "2026-10-21T07:15:00Z",
           *         "status": "succeeded",
           *         "steps": [
           *           {
           *             "durationMs": 15,
           *             "error": null,
           *             "name": "hook:enable",
           *             "startedAt": "2026-10-21T07:15:00Z",
           *             "status": "succeeded"
           *           }
           *         ],
           *         "toVersion": "1.0.0"
           *       }
           *     }
           */
          "application/json": {
            data: components["schemas"]["Operation"];
          };
        };
      };
      400: components["responses"]["common.BadRequest"];
      403: components["responses"]["common.PermissionDenied"];
      404: components["responses"]["common.NotFound"];
    };
  };
}

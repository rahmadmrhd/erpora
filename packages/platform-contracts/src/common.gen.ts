// Dibangkitkan oleh `erpora contracts:generate` dari contracts/openapi/common.yaml. JANGAN DIEDIT.

export type paths = Record<string, never>;
export type webhooks = Record<string, never>;
export type components = {
  schemas: {
    /** @description Metadata pagination cursor (api-guidelines section 6); cursor bersifat opaque. */
    CursorPage: {
      hasMore: boolean;
      /** @description Cursor halaman berikutnya; null bila tidak ada. */
      nextCursor: string | null;
    };
    /** @description Amplop respons sukses; sebuah respons tidak pernah berisi data dan error sekaligus. */
    DataEnvelope: {
      data: unknown;
    };
    /**
     * Format: date
     * @description Tanggal kalender ISO 8601, mis. 2026-10-21.
     */
    Date: string;
    /**
     * @description Uang dan kuantitas sebagai string desimal, tidak pernah float (api-guidelines section 4).
     * @example 1250000.50
     */
    Decimal: string;
    Error: {
      error: {
        /** @description Kode stabil (api-guidelines section 8). */
        code: string;
        /** @description Array ErrorDetail, atau objek konteks (mis. pada 412 version_mismatch). */
        details?:
          | components["schemas"]["ErrorDetail"][]
          | {
              [key: string]: unknown;
            };
        message: string;
        requestId?: string;
        traceId?: string;
      };
    };
    /** @description Satu butir detail error. code stabil dan menjadi kontrak; message untuk manusia. */
    ErrorDetail: {
      code: string;
      /** @description Path JSON field, mis. lines[2].itemId. */
      field?: string;
      message: string;
    } & {
      [key: string]: unknown;
    };
    Page: {
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
    Timestamp: string;
    /**
     * Format: uuid
     * @description UUID (v7 untuk ID baru).
     */
    Uuid: string;
  };
  responses: {
    /** @description Parameter atau body salah (bad_request). */
    BadRequest: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description invalid_state atau request_in_progress. */
    Conflict: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description internal_error; tanpa detail internal. */
    InternalError: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description Tidak ada, atau di luar scope (not_found). Record di luar scope selalu 404, bukan 403. */
    NotFound: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description not_implemented; endpoint terdaftar tetapi belum disambungkan ke use case (kerangka make:handler). */
    NotImplemented: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description not_ready; layanan belum siap. */
    NotReady: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description Tidak punya permission (permission_denied). */
    PermissionDenied: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description If-Match tidak cocok (version_mismatch). */
    PreconditionFailed: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
    /** @description validation_failed atau business_rule_violated; kode spesifik di error.details[].code. */
    Unprocessable: {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["Error"];
      };
    };
  };
  parameters: {
    /** @description Cursor opaque dari respons sebelumnya; klien tidak boleh membentuk atau mengurainya. */
    Cursor: string;
    /** @description UUID dari klien. Diterima, tetapi untuk endpoint platform modul TIDAK disimpan (setiap operasi idempoten lewat aturan no-op); penyimpanan 24 jam menunggu infrastruktur idempotency. */
    IdempotencyKey: string;
    /** @description Opsional. Revision yang diharapkan; tidak cocok -> 412 version_mismatch. */
    IfMatch: string;
    /** @description Jumlah item per permintaan (maksimal 1000). */
    Limit: number;
    /** @description Halaman, mulai 1. */
    Page: number;
    /** @description Jumlah item per halaman. */
    PageSize: number;
  };
  requestBodies: never;
  headers: {
    /** @description Versi resource (di endpoint modul platform = revision status modul tenant), mis. "7". */
    ETag: string;
    /** @description URL resource atau operasi yang dibuat/berjalan (201/202). */
    Location: string;
    /** @description ID request; selalu ada pada respons. */
    XRequestId: string;
  };
  pathItems: never;
};
export type $defs = Record<string, never>;
export type operations = Record<string, never>;

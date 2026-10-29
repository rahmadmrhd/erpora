// Dibangkitkan oleh `erpora contracts:generate` dari modules/example/openapi.yaml. JANGAN DIEDIT.
import { createClient } from "@erpora/platform-contracts";

export type paths = {
  "/api/example/notes": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Daftar notes
     * @description Pagination halaman (api-guidelines section 6). page atau pageSize di luar rentang menghasilkan 400 bad_request.
     */
    get: operations["listExampleNotes"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/example/ping": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Cek modul example hidup
     * @description Resource tunggal tanpa koleksi; dipakai untuk memvalidasi routing dan permission modul.
     */
    get: operations["getExamplePing"];
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
    Note: {
      createdAt: components["schemas"]["common.Timestamp"];
      id: components["schemas"]["common.Uuid"];
      title: string;
      updatedAt: components["schemas"]["common.Timestamp"];
      /** Format: int64 */
      version: number;
    };
    /**
     * Format: int64
     * @description API publik untuk modul lain (contract.Notes.Count); jumlah notes tenant aktif.
     */
    NoteCount: number;
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
    /** @description Tidak punya permission (permission_denied). */
    "common.PermissionDenied": {
      headers: {
        [name: string]: unknown;
      };
      content: {
        "application/json": components["schemas"]["common.Error"];
      };
    };
  };
  parameters: {
    /** @description Halaman, mulai 1. */
    "common.Page": number;
    /** @description Jumlah item per halaman. */
    "common.PageSize": number;
  };
  requestBodies: never;
  headers: never;
  pathItems: never;
};
export type $defs = Record<string, never>;
export interface operations {
  listExampleNotes: {
    parameters: {
      query?: {
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
      /** @description Notes tenant ini. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": [
           *         {
           *           "createdAt": "2026-10-21T07:15:00Z",
           *           "id": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b",
           *           "title": "Catatan pertama",
           *           "updatedAt": "2026-10-21T07:15:00Z",
           *           "version": 1
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
            data: components["schemas"]["Note"][];
            page: components["schemas"]["common.Page"];
          };
        };
      };
      400: components["responses"]["common.BadRequest"];
      403: components["responses"]["common.PermissionDenied"];
    };
  };
  getExamplePing: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Modul menjawab. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "pong": true
           *       }
           *     }
           */
          "application/json": {
            data: {
              pong: boolean;
            };
          };
        };
      };
      403: components["responses"]["common.PermissionDenied"];
    };
  };
}

export const client = createClient<paths>();

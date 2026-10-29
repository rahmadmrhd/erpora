// Dibangkitkan oleh `erpora contracts:generate` dari modules/exampleaddon/openapi.yaml. JANGAN DIEDIT.
import { createClient } from "@erpora/platform-contracts";

export type paths = {
  "/api/example-addon/summary": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /**
     * Ringkasan jumlah notes
     * @description Resource tunggal yang memakai kontrak publik modul example (contract.Notes).
     */
    get: operations["getExampleAddonSummary"];
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
    /**
     * Format: int64
     * @description API publik untuk modul lain (contract.Notes.Count); jumlah notes tenant aktif.
     */
    "example.NoteCount": number;
  };
  responses: {
    /** @description internal_error; tanpa detail internal. */
    "common.InternalError": {
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
  parameters: never;
  requestBodies: never;
  headers: never;
  pathItems: never;
};
export type $defs = Record<string, never>;
export interface operations {
  getExampleAddonSummary: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Ringkasan. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "noteCount": 3
           *       }
           *     }
           */
          "application/json": {
            data: {
              noteCount: components["schemas"]["example.NoteCount"];
            };
          };
        };
      };
      403: components["responses"]["common.PermissionDenied"];
      500: components["responses"]["common.InternalError"];
    };
  };
}

export const client = createClient<paths>();

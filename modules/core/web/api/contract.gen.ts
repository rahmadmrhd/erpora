// Dibangkitkan oleh `erpora contracts:generate` dari modules/core/openapi.yaml. JANGAN DIEDIT.
import { createClient } from "@erpora/platform-contracts";

export type paths = {
  "/api/core/auth/login": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Login dengan email dan sandi
     * @description Memverifikasi kredensial. Sukses mengembalikan access token (JWT, 10 menit) di body dan menulis refresh token HANYA lewat cookie `erp_refresh` (HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh). `mfaState` menunjukkan langkah MFA yang tersisa: `none` (akses penuh), `enroll_required` (administrator belum mendaftarkan TOTP), atau `verify_required` (kode TOTP harus diverifikasi); token pada dua keadaan terakhir hanya berlaku untuk route auth yang diizinkan (ADR-0013 section 4). Kegagalan tidak membedakan email tak dikenal, sandi salah, akun terkunci, atau akun nonaktif (401 `invalid_credentials`). Dibatasi laju per IP dan per email (429 `rate_limited`). Idempotency-Key diterima tetapi tidak pernah disimpan (respons memuat kredensial).
     */
    post: operations["login"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/logout": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Keluar: mencabut sesi Principal sendiri
     * @description Mencabut sesi milik Principal pada token dan menghapus cookie refresh. Tanpa parameter sesi atau pengguna: hanya sesi sendiri yang dapat dicabut. Boleh dipanggil sesi yang menunggu MFA. Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["logout"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/mfa/confirm": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Mengonfirmasi pendaftaran TOTP dengan kode pertama
     * @description Mengaktifkan faktor, menaikkan sesi ke `otp`, dan MEROTASI refresh token (PRD-001 Q27): cookie `erp_refresh` ditulis ulang dan token pra-MFA mati. Respons memuat 10 recovery code (tampil sekali) dan, bila penaikan sesi berhasil, access token baru di `session`; klien tidak perlu memanggil refresh setelahnya. `session` tidak ada bila penaikan gagal setelah faktor aktif: recovery code tetap diserahkan dan klien memakai `mfa/verify`. Kode salah: 422 `validation_failed` (`details[].code` = `invalid_code`). Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["confirmMfa"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/mfa/enroll": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Memulai pendaftaran TOTP
     * @description Membuat faktor TOTP yang menunggu konfirmasi untuk Principal sendiri. `secret` (base32) dan `otpauthUri` (untuk QR code) hanya ditampilkan sekali. Boleh dipanggil administrator yang belum punya faktor. Faktor yang sudah aktif: 409 `conflict`. Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["enrollMfa"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/mfa/recovery": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Memverifikasi recovery code pada sesi yang menunggu MFA
     * @description Sama dengan `mfa/verify` tetapi memakai recovery code sekali pakai sebagai pengganti kode TOTP. Menaikkan sesi ke `otp` dan merotasi refresh token (Q27). Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["recoverMfa"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/mfa/recovery-codes": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Membuat ulang recovery code
     * @description Menghanguskan recovery code lama dan membuat 10 yang baru (tampil sekali). Menuntut sesi yang sudah terverifikasi MFA (amr memuat `otp`; selain itu 403 `mfa_required`). Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["regenerateRecoveryCodes"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/mfa/verify": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Memverifikasi kode TOTP pada sesi yang menunggu MFA
     * @description Menaikkan sesi dari `pwd` ke `pwd+otp` dan merotasi refresh token (Q27): cookie `erp_refresh` ditulis ulang. Hanya untuk Principal pada token. Kode salah: 422 `validation_failed` (`details[].code` = `invalid_code`, seragam untuk semua sebab); dibatasi laju dan lockout seperti login. Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["verifyMfa"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/password/change": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Mengganti sandi Principal sendiri
     * @description Memverifikasi sandi saat ini (dengan batas laju dan lockout yang sama dengan login), menyimpan sandi baru, dan mencabut semua sesi LAIN (sesi ini tetap hidup). Hanya untuk Principal pada token. Sandi saat ini salah: 422 `validation_failed` (`details[].code` = `wrong_current_password`); sandi baru melanggar kebijakan: 422 `password_policy_violation`. Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["changePassword"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/password/forgot": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Meminta tautan reset sandi lewat email
     * @description Selalu 204 untuk email yang bentuknya sah, tanpa membedakan akun ada, tidak ada, terkunci, atau nonaktif; waktu respons seragam. Email (bila akun berhak) dikirim di latar dengan bahasa dari Accept-Language atau preferensi akun. Dibatasi laju per IP dan per email (429 `rate_limited`). Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["forgotPassword"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/password/reset": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Menyelesaikan reset sandi dengan token dari email
     * @description Token sekali pakai (berlaku 30 menit). Sukses mencabut SEMUA sesi pengguna dan membuka akun `locked`. Token tidak dikenal, kedaluwarsa, sudah terpakai, atau milik akun nonaktif dijawab seragam 422 `validation_failed` (`details[].code` = `invalid_token`). Sandi baru yang melanggar kebijakan: 422 `password_policy_violation` dengan kode aturan di `details[].code` (tautan tidak terpakai). Idempotency-Key diterima tetapi tidak disimpan.
     */
    post: operations["resetPassword"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/core/auth/refresh": {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    get?: never;
    put?: never;
    /**
     * Menukar refresh token (cookie) dengan access token baru
     * @description Refresh token dibaca dari cookie `erp_refresh`, bukan dari body. Header `Origin` wajib sama dengan origin URL publik aplikasi (pemeriksaan CSRF; selain itu 403 `permission_denied`). Token dirotasi tiap pakai: cookie ditulis ulang dengan token baru. Token tidak dikenal, kedaluwarsa, terpakai ulang, atau sesinya tercabut dijawab 401 `unauthenticated` generik dan cookie dihapus; pemakaian ulang token lama mencabut seluruh family. Dibatasi laju per IP dan per family refresh token (429 `rate_limited` + Retry-After; cookie TIDAK dihapus dan token tidak terpakai pada 429, sehingga klien dapat mencoba lagi setelah jeda). Idempotency-Key diterima tetapi tidak pernah disimpan.
     */
    post: operations["refreshSession"];
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
    ChangeRequest: {
      currentPassword: string;
      newPassword: string;
    };
    CodeRequest: {
      /** @description Kode TOTP 6 digit atau recovery code. */
      code: string;
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
    ConfirmEnvelope: {
      data: components["schemas"]["ConfirmResult"];
    };
    ConfirmResult: {
      /** @description 10 recovery code sekali pakai, tampil sekali. */
      codes: string[];
      session?: components["schemas"]["Session"];
    };
    Enrollment: {
      /** @description URI otpauth untuk QR code; memuat secret, tampil sekali. */
      otpauthUri: string;
      /** @description Rahasia TOTP (base32), tampil sekali. */
      secret: string;
    };
    EnrollmentEnvelope: {
      data: components["schemas"]["Enrollment"];
    };
    ForgotRequest: {
      /** Format: email */
      email: string;
    };
    LoginRequest: {
      /** Format: email */
      email: string;
      password: string;
    };
    /**
     * @description none: akses penuh. enroll_required: administrator belum mendaftarkan TOTP. verify_required: kode TOTP harus diverifikasi.
     * @enum {string}
     */
    MfaState: "none" | "enroll_required" | "verify_required";
    RecoveryCodes: {
      /** @description 10 recovery code baru, tampil sekali. */
      codes: string[];
    };
    RecoveryCodesEnvelope: {
      data: components["schemas"]["RecoveryCodes"];
    };
    ResetRequest: {
      newPassword: string;
      /** @description Token sekali pakai dari tautan email. */
      token: string;
    };
    /** @description Hasil login, refresh, atau verifikasi MFA. Tidak pernah memuat refresh token (hanya cookie). */
    Session: {
      /** @description JWT EdDSA berumur 10 menit; hanya disimpan di memori klien. */
      accessToken: string;
      /** @description Detik sampai access token kedaluwarsa. */
      expiresIn: number;
      mfaState: components["schemas"]["MfaState"];
    };
    SessionEnvelope: {
      data: components["schemas"]["Session"];
    };
  };
  responses: never;
  parameters: {
    /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
    IdempotencyKey: string;
  };
  requestBodies: never;
  headers: never;
  pathItems: never;
};
export type $defs = Record<string, never>;
export interface operations {
  login: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "email": "ani@example.test",
         *       "password": "kata-sandi-yang-panjang-2026"
         *     }
         */
        "application/json": components["schemas"]["LoginRequest"];
      };
    };
    responses: {
      /** @description Login berhasil. Cookie refresh ditulis; respons tidak pernah memuat refresh token. */
      200: {
        headers: {
          /** @description erp_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh */
          "Set-Cookie"?: string;
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "accessToken": "eyJhbGciOiJFZERTQSJ9.contoh.tanda-tangan",
           *         "expiresIn": 600,
           *         "mfaState": "none"
           *       }
           *     }
           */
          "application/json": components["schemas"]["SessionEnvelope"];
        };
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Kredensial salah (invalid_credentials, seragam). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Field tak dikenal atau tipe salah (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  logout: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Sesi dicabut; cookie refresh dihapus. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Permintaan salah bentuk (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Parameter tidak valid (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  confirmMfa: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "code": "123456"
         *     }
         */
        "application/json": components["schemas"]["CodeRequest"];
      };
    };
    responses: {
      /** @description Faktor aktif; recovery code (tampil sekali) dan sesi otp. Cookie refresh ditulis ulang. */
      200: {
        headers: {
          /** @description erp_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh (ditulis ulang bila sesi dinaikkan) */
          "Set-Cookie"?: string;
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "codes": [
           *           "a1b2-c3d4-e5f6",
           *           "g7h8-i9j0-k1l2"
           *         ],
           *         "session": {
           *           "accessToken": "eyJhbGciOiJFZERTQSJ9.contoh.tanda-tangan",
           *           "expiresIn": 600,
           *           "mfaState": "none"
           *         }
           *       }
           *     }
           */
          "application/json": components["schemas"]["ConfirmEnvelope"];
        };
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Aturan MFA belum terpenuhi (mfa_required) atau akun terkunci (account_locked). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tidak ada faktor yang menunggu konfirmasi (invalid_state). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Kode salah (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  enrollMfa: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Bahan pendaftaran TOTP (tampil sekali). */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "otpauthUri": "otpauth://totp/ERPora:ani@example.test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=ERPora",
           *         "secret": "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
           *       }
           *     }
           */
          "application/json": components["schemas"]["EnrollmentEnvelope"];
        };
      };
      /** @description Permintaan salah bentuk (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Aturan MFA belum terpenuhi (mfa_required). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Faktor MFA sudah aktif (conflict). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Parameter tidak valid (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  recoverMfa: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "code": "a1b2-c3d4-e5f6"
         *     }
         */
        "application/json": components["schemas"]["CodeRequest"];
      };
    };
    responses: {
      /** @description Sesi otp. Cookie refresh ditulis ulang. */
      200: {
        headers: {
          /** @description erp_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh */
          "Set-Cookie"?: string;
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "accessToken": "eyJhbGciOiJFZERTQSJ9.contoh.tanda-tangan",
           *         "expiresIn": 600,
           *         "mfaState": "none"
           *       }
           *     }
           */
          "application/json": components["schemas"]["SessionEnvelope"];
        };
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Akun terkunci (account_locked). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Faktor MFA belum didaftarkan (invalid_state). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Recovery code salah atau sudah terpakai (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  regenerateRecoveryCodes: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Recovery code baru (tampil sekali). */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "codes": [
           *           "a1b2-c3d4-e5f6",
           *           "g7h8-i9j0-k1l2"
           *         ]
           *       }
           *     }
           */
          "application/json": components["schemas"]["RecoveryCodesEnvelope"];
        };
      };
      /** @description Permintaan salah bentuk (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Sesi belum terverifikasi MFA (mfa_required). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Faktor MFA belum didaftarkan (invalid_state). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Parameter tidak valid (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  verifyMfa: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "code": "123456"
         *     }
         */
        "application/json": components["schemas"]["CodeRequest"];
      };
    };
    responses: {
      /** @description Sesi otp. Cookie refresh ditulis ulang. */
      200: {
        headers: {
          /** @description erp_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh */
          "Set-Cookie"?: string;
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "accessToken": "eyJhbGciOiJFZERTQSJ9.contoh.tanda-tangan",
           *         "expiresIn": 600,
           *         "mfaState": "none"
           *       }
           *     }
           */
          "application/json": components["schemas"]["SessionEnvelope"];
        };
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Akun terkunci (account_locked). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Faktor MFA belum didaftarkan (invalid_state). */
      409: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Kode salah (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  changePassword: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "currentPassword": "kata-sandi-lama-2026",
         *       "newPassword": "kata-sandi-baru-yang-panjang-2026"
         *     }
         */
        "application/json": components["schemas"]["ChangeRequest"];
      };
    };
    responses: {
      /** @description Sandi diganti; sesi lain dicabut. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Tanpa token atau token tidak valid (unauthenticated, token_expired). */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Aturan MFA belum terpenuhi (mfa_required) atau akun terkunci (account_locked). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Sandi saat ini salah (validation_failed) atau sandi baru melanggar kebijakan (password_policy_violation). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  forgotPassword: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "email": "ani@example.test"
         *     }
         */
        "application/json": components["schemas"]["ForgotRequest"];
      };
    };
    responses: {
      /** @description Permintaan diterima (selalu, tanpa membedakan akun). */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Field tak dikenal atau tipe salah (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  resetPassword: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        /**
         * @example {
         *       "newPassword": "kata-sandi-baru-yang-panjang-2026",
         *       "token": "dG9rZW4tdGF1dGFuLWRhcmktZW1haWw"
         *     }
         */
        "application/json": components["schemas"]["ResetRequest"];
      };
    };
    responses: {
      /** @description Sandi diganti; semua sesi dicabut. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Body bukan JSON yang sah (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Token tidak valid (validation_failed) atau sandi melanggar kebijakan (password_policy_violation). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
  refreshSession: {
    parameters: {
      query?: never;
      header?: {
        /** @description UUID dari klien. Diterima tetapi TIDAK PERNAH disimpan pada endpoint auth modul core (PRD-001 Q31): me-replay respons yang memuat access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. */
        "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
      };
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Token baru. Cookie refresh ditulis ulang; respons tidak pernah memuat refresh token. */
      200: {
        headers: {
          /** @description erp_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh */
          "Set-Cookie"?: string;
          [name: string]: unknown;
        };
        content: {
          /**
           * @example {
           *       "data": {
           *         "accessToken": "eyJhbGciOiJFZERTQSJ9.contoh.tanda-tangan",
           *         "expiresIn": 600,
           *         "mfaState": "none"
           *       }
           *     }
           */
          "application/json": components["schemas"]["SessionEnvelope"];
        };
      };
      /** @description Permintaan salah bentuk (bad_request). */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Refresh token tidak valid (unauthenticated); cookie dihapus. */
      401: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Origin tidak diizinkan (permission_denied). */
      403: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Parameter tidak valid (validation_failed). */
      422: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
      /** @description Dibatasi laju (rate_limited), dengan Retry-After dalam detik. Tidak membedakan sebab (IP, identitas sasaran, jeda lockout, pembatas hash). */
      429: {
        headers: {
          /** @description Detik sampai boleh mencoba lagi (bilangan bulat, minimal 1). */
          "Retry-After"?: number;
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["common.Error"];
        };
      };
    };
  };
}

export const client = createClient<paths>();

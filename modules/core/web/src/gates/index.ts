import { lazy } from "react";

export type { AuthScreensProps } from "./auth-screens";
export type { GateNavigate } from "./nav";

/**
 * Gerbang autentikasi dimuat MALAS: kode layar auth, katalog yang dibundel, dan (di layar pendaftaran MFA) pustaka QR
 * tidak masuk JS awal aplikasi (anggaran bundle, PRD-001 Q33 butir 2).
 */
export const AuthScreens = lazy(() =>
  import("./auth-screens").then((m) => ({ default: m.AuthScreens })),
);

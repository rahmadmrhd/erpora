import { safeInternalHref } from "../sources/href";

const MAX_LENGTH = 2048;
/** Jalur gerbang auth: tidak pernah dipakai sebagai tujuan setelah login (akan berputar). */
const GATE_PATH = /^\/(login|forgot-password|reset-password)(?:[/?#]|$)/;

/**
 * Tujuan setelah login (`returnTo`): hanya path relatif internal (diawali tepat satu "/", tanpa skema, host, backslash,
 * spasi, atau karakter kontrol; memakai `safeInternalHref` yang sama dengan notifikasi) dan bukan jalur gerbang auth
 * itu sendiri. Selain itu `/`. Total untuk masukan apa pun: tidak pernah melempar (Q33 butir 4).
 */
export function safeReturnTo(raw: unknown): string {
  const href = safeInternalHref(raw);
  if (href === null || href.length > MAX_LENGTH || GATE_PATH.test(href)) return "/";
  return href;
}

/** URL gerbang login dengan `returnTo` tersandi; tujuan bawaan ("/") tidak disertakan. */
export function loginHref(returnTo: unknown): string {
  const target = safeReturnTo(returnTo);
  return target === "/" ? "/login" : `/login?returnTo=${encodeURIComponent(target)}`;
}

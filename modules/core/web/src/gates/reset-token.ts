/**
 * Token reset sandi dari fragmen URL (`/reset-password#token=...`, tautan email ADR-0013 K-C1.6). Fragmen tidak
 * dikirim ke server dan tidak masuk log akses atau header Referer, tetapi tetap tidak boleh tinggal di bilah alamat
 * dan riwayat: `scrubResetToken` menghapusnya segera lewat `history.replaceState`.
 */
export function readResetToken(hash: string): string | null {
  const params = new URLSearchParams(hash.startsWith("#") ? hash.slice(1) : hash);
  const token = params.get("token");
  return token !== null && token.trim() !== "" ? token.trim() : null;
}

/** Menghapus fragmen dari URL tanpa membuat entri riwayat baru dan tanpa memuat ulang halaman. */
export function scrubResetToken(win: Pick<Window, "history" | "location"> = window): void {
  const { pathname, search } = win.location;
  win.history.replaceState(win.history.state, "", `${pathname}${search}`);
}

import { useCallback } from "react";
import { useTimezone } from "../prefs/hooks";
import { formatDateTime } from "./time";

/**
 * Pemformat tanggal dan jam yang mengikuti zona waktu Settings: komponen yang memakainya dirender ulang seketika saat
 * pengguna mengganti zona (tanpa muat ulang). Identitas fungsi berubah bersama zona sehingga aman dipakai di dependensi
 * `useMemo`/`useEffect`. Tanpa pilihan (otomatis) dipakai zona peramban.
 */
export function useDateTimeFormatter(): (iso: string, locale: string) => string {
  const timezone = useTimezone();
  return useCallback(
    (iso: string, locale: string) => formatDateTime(iso, locale, timezone ?? undefined),
    [timezone],
  );
}

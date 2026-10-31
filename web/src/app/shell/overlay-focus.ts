import { useCallback, useLayoutEffect, useRef } from "react";

/**
 * Pengembalian fokus untuk overlay Radix yang dikendalikan dari luar (tanpa `*Trigger` di dalam komponen): Radix hanya
 * mengembalikan fokus ke pemicu yang ia kenal, sehingga elemen yang fokus saat overlay dibuka dicatat di sini
 * (layout effect berjalan sebelum Radix memindahkan fokus ke isi) dan difokuskan lagi saat overlay tertutup.
 *
 * - `onCloseAutoFocus`: pasang pada `*Content`; mengembalikan fokus ke pemicu kecuali overlay ditutup lewat interaksi di
 *   luar (fokus mengikuti elemen yang diklik pengguna) atau pemicu sudah tidak ada di dokumen.
 * - `onInteractOutside`: pasang pada `*Content` non-modal; interaksi pada pemicu sendiri dibatalkan (pemicu yang
 *   membalik `open`, jangan tutup lalu buka ulang) dan interaksi lain menandai penutupan dari luar.
 */
export function useReturnFocus(open: boolean) {
  const opener = useRef<HTMLElement | null>(null);
  const dismissedOutside = useRef(false);

  useLayoutEffect(() => {
    if (!open) return;
    dismissedOutside.current = false;
    const active = document.activeElement;
    opener.current = active instanceof HTMLElement && active !== document.body ? active : null;
  }, [open]);

  const onCloseAutoFocus = useCallback((event: Event) => {
    event.preventDefault();
    const target = opener.current;
    if (!dismissedOutside.current && target?.isConnected) target.focus();
  }, []);

  const onInteractOutside = useCallback((event: Event) => {
    if (event.target instanceof Node && opener.current?.contains(event.target)) {
      event.preventDefault();
      return;
    }
    dismissedOutside.current = true;
  }, []);

  return { onCloseAutoFocus, onInteractOutside };
}

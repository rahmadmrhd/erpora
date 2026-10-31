import type * as React from "react";
import { Button } from "../ui/button";
import { Spinner } from "../ui/spinner";

/**
 * Pembungkus `Button` shadcn + `Spinner` (R1.4): shadcn tidak punya keadaan memuat pada tombol. Saat `loading`,
 * tombol dinonaktifkan, ditandai `aria-busy`, dan menampilkan `Spinner` (berlabel `status.loading` lewat UiLabels).
 * Berkas `ui/button.tsx` tidak diubah.
 */
export function LoadingButton({
  loading = false,
  disabled,
  children,
  ...props
}: React.ComponentProps<typeof Button> & { loading?: boolean }) {
  return (
    <Button {...props} disabled={disabled || loading} aria-busy={loading ? true : undefined}>
      {loading ? <Spinner /> : null}
      {children}
    </Button>
  );
}

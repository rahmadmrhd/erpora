import type * as React from "react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";

export type StatusTone = "primary" | "success" | "warning" | "error" | "secondary";

// Peta nada ke token status-* (kelas ditulis utuh agar terdeteksi pemindai Tailwind). Badge shadcn tidak punya varian
// bernada status (audit 1.4), sehingga pemetaan ada di lapisan pembungkus; berkas `ui/badge.tsx` tidak diubah.
// Peta 44 kode status dokumen ditunda ke M3c; pemanggil menerjemahkan label sendiri (status tidak boleh bergantung pada warna saja).
const toneClass: Record<StatusTone, string> = {
  primary: "bg-status-primary-bg text-status-primary-fg",
  success: "bg-status-success-bg text-status-success-fg",
  warning: "bg-status-warning-bg text-status-warning-fg",
  error: "bg-status-error-bg text-status-error-fg",
  secondary: "bg-status-secondary-bg text-status-secondary-fg",
};

/** Pil status: `Badge` shadcn dengan nada token status dan label teks dari `children`. */
export function StatusBadge({
  tone,
  className,
  children,
  ...props
}: { tone: StatusTone } & Omit<React.ComponentProps<typeof Badge>, "variant">) {
  return (
    <Badge data-tone={tone} className={cn(toneClass[tone], className)} {...props}>
      {children}
    </Badge>
  );
}

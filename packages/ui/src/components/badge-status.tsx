import { cn } from "@erpora/ui/lib/utils";
import type * as React from "react";

export type StatusTone = "primary" | "success" | "warning" | "error" | "secondary";

// Peta nada ke token status-* (kelas ditulis utuh agar terdeteksi pemindai Tailwind). Peta 44 kode status dokumen
// ditunda ke M3c; pemanggil menerjemahkan label sendiri (status tidak boleh bergantung pada warna saja).
const toneClass: Record<StatusTone, string> = {
  primary: "bg-status-primary-bg text-status-primary-fg",
  success: "bg-status-success-bg text-status-success-fg",
  warning: "bg-status-warning-bg text-status-warning-fg",
  error: "bg-status-error-bg text-status-error-fg",
  secondary: "bg-status-secondary-bg text-status-secondary-fg",
};

/** Komposit DS BadgeStatus: pil status dengan nada token dan label teks dari `children`. */
export function BadgeStatus({
  tone,
  className,
  children,
  ...props
}: { tone: StatusTone } & Omit<React.ComponentProps<"span">, "children"> & {
    children: React.ReactNode;
  }) {
  return (
    <span
      data-slot="badge-status"
      data-tone={tone}
      className={cn(
        "inline-flex w-fit items-center rounded-full px-2.5 py-0.5 text-sm font-medium whitespace-nowrap shadow-sm",
        toneClass[tone],
        className,
      )}
      {...props}
    >
      {children}
    </span>
  );
}

import { cn } from "@erpora/ui/lib/utils";
import type * as React from "react";
import { type IconName, resolveIcon } from "../icons";

/**
 * Komposit baru (celah DS, dicatat di K-24): ikon opsional, judul tebal, deskripsi `muted-foreground`, dan aksi.
 * Dipakai sidebar kosong, notifikasi kosong, dashboard, dan daftar modul kosong. Teks berasal dari props (i18n pemanggil).
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
  ...props
}: {
  icon?: IconName;
  title: string;
  description?: string;
  action?: React.ReactNode;
} & Omit<React.ComponentProps<"div">, "title">) {
  const Icon = resolveIcon(icon);
  return (
    <div
      data-slot="empty-state"
      className={cn("flex flex-col items-center gap-2 p-6 text-center", className)}
      {...props}
    >
      {Icon ? <Icon aria-hidden="true" className="size-8 text-muted-foreground" /> : null}
      <p className="text-sm font-semibold">{title}</p>
      {description ? <p className="max-w-sm text-sm text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

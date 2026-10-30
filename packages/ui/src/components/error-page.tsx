import { cn } from "@erpora/ui/lib/utils";
import type * as React from "react";

/**
 * Halaman galat (kartu DS ErrorPage; teks Inggris tertanam DS diganti props sehingga lewat i18n pemanggil): kolom tengah,
 * judul besar semibold dan satu kalimat keterangan. `status` (403/404/500/503) hanya penanda `data-status`.
 */
export function ErrorPage({
  title,
  description,
  actions,
  status,
  className,
  ...props
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  status?: 403 | 404 | 500 | 503;
} & Omit<React.ComponentProps<"div">, "title">) {
  return (
    <div
      data-slot="error-page"
      data-status={status}
      className={cn(
        "flex min-h-[60vh] flex-col items-center justify-center gap-3 px-4 text-center",
        className,
      )}
      {...props}
    >
      <h1 className="text-4xl font-semibold text-foreground">{title}</h1>
      {description ? <p className="max-w-xl text-sm text-muted-foreground">{description}</p> : null}
      {actions ? <div className="mt-2 flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}

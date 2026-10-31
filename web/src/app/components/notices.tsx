import {
  cn,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  type IconName,
  resolveIcon,
} from "@erpora/ui";
import type * as React from "react";

/** Keadaan kosong: komposisi `Empty` shadcn (menggantikan komposit `EmptyState` lama). Teks dari pemanggil (i18n). */
export function EmptyNotice({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: IconName;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}) {
  const Icon = resolveIcon(icon);
  return (
    <Empty className={className}>
      <EmptyHeader>
        {Icon ? (
          <EmptyMedia variant="icon">
            <Icon aria-hidden="true" />
          </EmptyMedia>
        ) : null}
        <EmptyTitle>{title}</EmptyTitle>
        {description ? <EmptyDescription>{description}</EmptyDescription> : null}
      </EmptyHeader>
      {action ? <EmptyContent>{action}</EmptyContent> : null}
    </Empty>
  );
}

/**
 * Halaman galat (403/404/500/503): komposisi `Empty` dengan judul `<h1>` (menggantikan komposit `ErrorPage` lama).
 * `status` hanya penanda `data-status`.
 */
export function ErrorNotice({
  title,
  description,
  actions,
  status,
  className,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  status?: 403 | 404 | 500 | 503;
  className?: string;
}) {
  return (
    <Empty data-status={status} className={cn("min-h-[60vh] justify-center", className)}>
      <EmptyHeader>
        <h1 className="text-4xl font-semibold text-foreground">{title}</h1>
        {description ? <EmptyDescription>{description}</EmptyDescription> : null}
      </EmptyHeader>
      {actions ? <EmptyContent>{actions}</EmptyContent> : null}
    </Empty>
  );
}

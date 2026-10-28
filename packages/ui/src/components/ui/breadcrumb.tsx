import { cn } from "@erpora/ui/lib/utils";
import { ChevronRight, Ellipsis } from "lucide-react";
import { Slot } from "radix-ui";
import type * as React from "react";
import { useUiLabel } from "../../labels";

// Hasil `shadcn add breadcrumb`, diadaptasi ke kartu DS Breadcrumb dan mockup (K-24, K-25): `<nav>` bernama dari UiLabels
// (`breadcrumb.label`), tautan bergaya teks `muted-foreground` (bukan kotak bertepi DS), halaman aktif `aria-current="page"`, elipsis
// bernama `breadcrumb.more` (dipakai sebagai pemicu menu jalur yang dilipat; lihat collapseCrumbs).
function Breadcrumb({ ...props }: React.ComponentProps<"nav">) {
  const t = useUiLabel();
  return <nav aria-label={t("breadcrumb.label")} data-slot="breadcrumb" {...props} />;
}

function BreadcrumbList({ className, ...props }: React.ComponentProps<"ol">) {
  return (
    <ol
      data-slot="breadcrumb-list"
      className={cn(
        "flex flex-wrap items-center gap-1.5 text-sm break-words text-muted-foreground sm:gap-2.5",
        className,
      )}
      {...props}
    />
  );
}

function BreadcrumbItem({ className, ...props }: React.ComponentProps<"li">) {
  return (
    <li
      data-slot="breadcrumb-item"
      className={cn("inline-flex items-center gap-1.5", className)}
      {...props}
    />
  );
}

function BreadcrumbLink({
  asChild,
  className,
  ...props
}: React.ComponentProps<"a"> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "a";
  return (
    <Comp
      data-slot="breadcrumb-link"
      className={cn("text-muted-foreground transition-colors hover:text-foreground", className)}
      {...props}
    />
  );
}

function BreadcrumbPage({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="breadcrumb-page"
      aria-current="page"
      className={cn("font-medium text-foreground", className)}
      {...props}
    />
  );
}

function BreadcrumbSeparator({ children, className, ...props }: React.ComponentProps<"li">) {
  return (
    <li
      data-slot="breadcrumb-separator"
      role="presentation"
      aria-hidden="true"
      className={cn("[&>svg]:size-3.5", className)}
      {...props}
    >
      {children ?? <ChevronRight />}
    </li>
  );
}

function BreadcrumbEllipsis({ className, ...props }: React.ComponentProps<"span">) {
  const t = useUiLabel();
  return (
    <span
      data-slot="breadcrumb-ellipsis"
      className={cn("flex size-9 items-center justify-center", className)}
      {...props}
    >
      <Ellipsis className="size-4" aria-hidden="true" />
      <span className="sr-only">{t("breadcrumb.more")}</span>
    </span>
  );
}

export {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
};

import { Link } from "@erpora/module-sdk";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  type FitPlan,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  useUiLabel,
} from "@erpora/ui";
import { Fragment, lazy, type ReactNode, Suspense, useState } from "react";

/**
 * Satu langkah jejak halaman. Hanya langkah yang bukan halaman saat ini yang dapat diaktifkan: `to` (tautan
 * router, untuk langkah halaman) atau `onSelect` (Home dan Desk, yang menyetel state navigasi sebelum berpindah).
 */
export interface BreadcrumbSegment {
  key: string;
  kind: "home" | "desk" | "crumb";
  /** Teks polos (menu, tooltip, nama aksesibel). */
  text: string;
  /** Isi tampil inline; Home berupa ikon. */
  label: ReactNode;
  to?: string;
  onSelect?: () => void;
}

// Sheet lembar bawah dimuat malas dan hanya dipasang setelah pertama kali dibuka (R7.5, anggaran bundle).
const PathSheet = lazy(() => import("./breadcrumb-path-sheet"));

function SegmentContent({ segment, current }: { segment: BreadcrumbSegment; current: boolean }) {
  if (current) return null;
  if (segment.to !== undefined) {
    return (
      <BreadcrumbLink asChild>
        <Link to={segment.to}>{segment.label}</Link>
      </BreadcrumbLink>
    );
  }
  if (segment.onSelect !== undefined) {
    return (
      <BreadcrumbLink asChild>
        <button
          type="button"
          onClick={segment.onSelect}
          aria-label={segment.kind === "home" ? segment.text : undefined}
          className="inline-flex items-center"
        >
          {segment.label}
        </button>
      </BreadcrumbLink>
    );
  }
  return <span>{segment.label}</span>;
}

function CurrentPage({
  segment,
  maxWidth,
}: {
  segment: BreadcrumbSegment;
  maxWidth: number | null;
}) {
  const truncated = maxWidth !== null;
  const page = (
    <BreadcrumbPage
      className={truncated ? "block truncate" : undefined}
      style={truncated ? { maxWidth } : undefined}
    >
      {segment.label}
    </BreadcrumbPage>
  );
  if (!truncated) return page;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{page}</TooltipTrigger>
      <TooltipContent>{segment.text}</TooltipContent>
    </Tooltip>
  );
}

/** Menu "…" pada layar lebar: jalur lengkap dengan tautan setiap langkah (R7.5). */
function PathDropdown({ segments }: { segments: readonly BreadcrumbSegment[] }) {
  const ui = useUiLabel();
  const lastIndex = segments.length - 1;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={ui("breadcrumb.more")}
          className="flex items-center rounded-sm hover:text-foreground"
        >
          <BreadcrumbEllipsis className="size-6" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {segments.map((s, i) => {
          if (i === lastIndex) {
            return (
              <DropdownMenuItem key={s.key} disabled aria-current="page">
                {s.text}
              </DropdownMenuItem>
            );
          }
          if (s.to !== undefined) {
            return (
              <DropdownMenuItem key={s.key} asChild>
                <Link to={s.to}>{s.text}</Link>
              </DropdownMenuItem>
            );
          }
          if (s.onSelect !== undefined) {
            return (
              <DropdownMenuItem key={s.key} onSelect={s.onSelect}>
                {s.text}
              </DropdownMenuItem>
            );
          }
          return (
            <DropdownMenuItem key={s.key} disabled>
              {s.text}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Pemicu "…" pada navbar sempit: membuka lembar bawah berisi jalur lengkap. */
function PathSheetTrigger({ segments }: { segments: readonly BreadcrumbSegment[] }) {
  const ui = useUiLabel();
  const [open, setOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  return (
    <>
      <button
        type="button"
        aria-label={ui("breadcrumb.more")}
        aria-haspopup="dialog"
        aria-expanded={open}
        className="flex items-center rounded-sm hover:text-foreground"
        onClick={() => {
          setMounted(true);
          setOpen(true);
        }}
      >
        <BreadcrumbEllipsis className="size-6" />
      </button>
      {mounted ? (
        <Suspense fallback={null}>
          <PathSheet open={open} onOpenChange={setOpen} segments={segments} />
        </Suspense>
      ) : null}
    </>
  );
}

export interface BreadcrumbNavProps {
  /** `[Home, Desk?, ...langkah]`; yang terakhir adalah halaman saat ini. */
  segments: readonly BreadcrumbSegment[];
  /** Rencana peringkasan dari `fitBreadcrumb` untuk `segments` yang sama. */
  plan: FitPlan;
  /** Navbar sempit: "…" membuka lembar bawah, bukan dropdown. */
  narrow: boolean;
  className?: string;
}

/**
 * Breadcrumb bertahap (R7): menggambar `plan` dari `fitBreadcrumb`. Langkah terakhir (halaman saat ini) bukan tautan;
 * tahap terpotong memberi elipsis teks dan tooltip judul penuh. Pengukuran dan pemilihan tahap dilakukan `Navbar`.
 */
export function BreadcrumbNav({ segments, plan, narrow, className }: BreadcrumbNavProps) {
  if (segments.length === 0) return null;
  const lastIndex = segments.length - 1;
  return (
    <Breadcrumb className={className}>
      <BreadcrumbList className="flex-nowrap whitespace-nowrap">
        {plan.slots.map((slot, i) => {
          const key = slot.kind === "segment" ? (segments[slot.index]?.key ?? i) : "ellipsis";
          return (
            <Fragment key={key}>
              {i > 0 ? <BreadcrumbSeparator /> : null}
              <BreadcrumbItem className="min-w-0">
                {slot.kind === "ellipsis" ? (
                  narrow ? (
                    <PathSheetTrigger segments={segments} />
                  ) : (
                    <PathDropdown segments={segments} />
                  )
                ) : slot.index === lastIndex ? (
                  <CurrentPage
                    segment={segments[slot.index] as BreadcrumbSegment}
                    maxWidth={plan.truncateLast ? plan.lastMaxWidth : null}
                  />
                ) : (
                  <SegmentContent
                    segment={segments[slot.index] as BreadcrumbSegment}
                    current={false}
                  />
                )}
              </BreadcrumbItem>
            </Fragment>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}

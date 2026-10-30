import { Link } from "@erpora/module-sdk";
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
  collapseCrumbs,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@erpora/ui";
import { Fragment, type ReactNode } from "react";

/** Satu tingkat jejak halaman yang sudah siap dirender; `to` hanya ada bila tingkat itu punya halaman sendiri. */
export interface ResolvedCrumb {
  key: string;
  label: ReactNode;
  /** Teks tampil bila sudah diketahui (crumb berupa teks data); crumb berpesan memakai teks hasil probe. */
  text?: string;
  to?: string;
}

/**
 * Membuang crumb yang teksnya sama dengan crumb sesudahnya (mis. crumb dasar modul "Catatan" dan halaman "Catatan"):
 * yang lebih dalam (halaman) dipertahankan. `texts` memuat teks terjemahan crumb berpesan menurut `key`; teks yang belum
 * diketahui tidak dianggap sama.
 */
export function dedupeCrumbs(
  crumbs: readonly ResolvedCrumb[],
  texts: Readonly<Record<string, string>>,
): ResolvedCrumb[] {
  const textOf = (c: ResolvedCrumb): string | undefined => c.text ?? texts[c.key];
  return crumbs.filter((c, i) => {
    const next = crumbs[i + 1];
    if (next === undefined) return true;
    const a = textOf(c);
    return a === undefined || a !== textOf(next);
  });
}

function CrumbContent({ crumb, current }: { crumb: ResolvedCrumb; current: boolean }) {
  if (current) return <BreadcrumbPage>{crumb.label}</BreadcrumbPage>;
  if (crumb.to) {
    return (
      <BreadcrumbLink asChild>
        <Link to={crumb.to}>{crumb.label}</Link>
      </BreadcrumbLink>
    );
  }
  return <span>{crumb.label}</span>;
}

/**
 * Jejak halaman dari definisi route. Lebih dari tiga tingkat atau layar kecil: tingkat tengah dilipat ke menu elipsis;
 * halaman aktif selalu teks (`aria-current="page"`), bukan tautan (P4, K-25).
 */
export function Breadcrumbs({
  crumbs,
  compact,
}: {
  crumbs: readonly ResolvedCrumb[];
  compact: boolean;
}) {
  const { visible, folded } = collapseCrumbs(crumbs, { compact });
  if (visible.length === 0) return null;
  return (
    <Breadcrumb className="min-w-0">
      <BreadcrumbList className="flex-nowrap">
        {visible.map((slot, i) => (
          <Fragment key={slot.kind === "crumb" ? slot.crumb.key : `ellipsis-${i}`}>
            {i > 0 ? <BreadcrumbSeparator /> : null}
            <BreadcrumbItem>
              {slot.kind === "ellipsis" ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      className="flex items-center rounded-sm hover:text-foreground"
                    >
                      <BreadcrumbEllipsis />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    {folded.map((c) =>
                      c.to ? (
                        <DropdownMenuItem key={c.key} asChild>
                          <Link to={c.to}>{c.label}</Link>
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem key={c.key} disabled>
                          {c.label}
                        </DropdownMenuItem>
                      ),
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : (
                <CrumbContent crumb={slot.crumb} current={slot.current} />
              )}
            </BreadcrumbItem>
          </Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  );
}

import { Link } from "@erpora/module-sdk";
import { useTranslation } from "@erpora/platform-web";
import {
  Button,
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@erpora/ui";
import type { BreadcrumbSegment } from "./breadcrumb-nav";
import { useReturnFocus } from "./overlay-focus";

export interface BreadcrumbPathSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Jalur lengkap `[Home, Desk?, ...langkah]`; yang terakhir adalah halaman saat ini. */
  segments: readonly BreadcrumbSegment[];
}

/**
 * Lembar bawah jalur lengkap untuk navbar sempit (R7.5, `Sheet side="bottom"`): tiap langkah adalah tautan
 * (menutup lembar), halaman saat ini memakai `aria-current="page"` dan memuat judul penuh. Dimuat malas.
 */
export default function BreadcrumbPathSheet({
  open,
  onOpenChange,
  segments,
}: BreadcrumbPathSheetProps) {
  const { t } = useTranslation("platform");
  const lastIndex = segments.length - 1;
  // Sheet dikendalikan dari luar (tanpa SheetTrigger): fokus dikembalikan ke pemicu "…" secara manual.
  const { onCloseAutoFocus } = useReturnFocus(open);
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="max-h-[80svh]" onCloseAutoFocus={onCloseAutoFocus}>
        <SheetHeader>
          <SheetTitle>{t("breadcrumb.pathTitle")}</SheetTitle>
          <SheetDescription>{t("breadcrumb.pathDescription")}</SheetDescription>
        </SheetHeader>
        <ol className="flex flex-col gap-1 overflow-y-auto px-4 pb-4">
          {segments.map((s, i) => (
            <li key={s.key}>
              {i === lastIndex ? (
                <span
                  aria-current="page"
                  className="block px-3 py-2 text-sm font-medium break-words text-foreground"
                >
                  {s.text}
                </span>
              ) : s.to !== undefined ? (
                <SheetClose asChild>
                  <Button asChild variant="ghost" className="w-full justify-start">
                    <Link to={s.to}>{s.text}</Link>
                  </Button>
                </SheetClose>
              ) : s.onSelect !== undefined ? (
                <SheetClose asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="w-full justify-start"
                    onClick={s.onSelect}
                  >
                    {s.text}
                  </Button>
                </SheetClose>
              ) : (
                <span className="block px-3 py-2 text-sm text-muted-foreground">{s.text}</span>
              )}
            </li>
          ))}
        </ol>
      </SheetContent>
    </Sheet>
  );
}

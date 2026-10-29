import {
  Kbd,
  KbdGroup,
  Separator,
  SidebarTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  useIsMobile,
  useUiLabel,
} from "@erpora/ui";
import { Breadcrumbs } from "./breadcrumbs";
import { PageActionsView } from "./page-actions-view";
import { useRouteCrumbs } from "./route-crumbs";

/** Navbar 48 px lengket di atas area konten: tombol collapse (dengan petunjuk Ctrl B), jejak halaman, aksi halaman. */
export function Navbar() {
  const t = useUiLabel();
  const compact = useIsMobile();
  const { crumbs } = useRouteCrumbs();
  return (
    <header className="sticky top-0 z-10 flex h-(--navbar-height) shrink-0 items-center gap-2.5 border-b bg-background px-3.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <SidebarTrigger aria-label={t("sidebar.toggle")} />
        </TooltipTrigger>
        <TooltipContent>
          <span className="flex items-center gap-2">
            {t("sidebar.toggle")}
            <KbdGroup>
              <Kbd>Ctrl</Kbd>
              <Kbd>B</Kbd>
            </KbdGroup>
          </span>
        </TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" className="h-4" />
      <Breadcrumbs crumbs={crumbs} compact={compact} />
      <PageActionsView />
    </header>
  );
}

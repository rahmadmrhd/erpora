import type { MessageRef } from "@erpora/module-sdk";
import { useMessage } from "@erpora/platform-web";
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
import { useRouterState } from "@tanstack/react-router";
import { useCallback, useLayoutEffect, useState } from "react";
import { Breadcrumbs, dedupeCrumbs } from "./breadcrumbs";
import { PageActionsView } from "./page-actions-view";
import { useRouteCrumbs } from "./route-crumbs";

/** Navbar 48 px lengket di atas area konten: tombol collapse (dengan petunjuk Ctrl B), jejak halaman, aksi halaman. */
/** Melaporkan teks terjemahan satu crumb berpesan (komponen per crumb sehingga jumlah hook tetap per komponen). */
function CrumbTextProbe({
  id,
  message,
  onText,
}: {
  id: string;
  message: MessageRef;
  onText: (id: string, text: string) => void;
}) {
  const text = useMessage(message);
  useLayoutEffect(() => onText(id, text), [id, text, onText]);
  return null;
}

export function Navbar() {
  const t = useUiLabel();
  const compact = useIsMobile();
  const { crumbs: raw, refs } = useRouteCrumbs();
  const [texts, setTexts] = useState<Record<string, string>>({});
  const onText = useCallback((id: string, text: string) => {
    setTexts((prev) => (prev[id] === text ? prev : { ...prev, [id]: text }));
  }, []);
  const crumbs = dedupeCrumbs(raw, texts);
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
      {refs.map((r) => (
        <CrumbTextProbe key={r.key} id={r.key} message={r.ref} onText={onText} />
      ))}
      <Breadcrumbs crumbs={crumbs} compact={compact} />
      <PageActionsView />
    </header>
  );
}

/**
 * Cadangan Navbar saat memuat atau galat: tombol collapse tetap ada (di layar kecil sidebar hanya dibuka lewat tombol ini)
 * dan jejak minimal berupa path.
 */
export function NavbarFallback() {
  const t = useUiLabel();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  return (
    <header className="sticky top-0 z-10 flex h-(--navbar-height) shrink-0 items-center gap-2.5 border-b bg-background px-3.5">
      <SidebarTrigger aria-label={t("sidebar.toggle")} />
      <Separator orientation="vertical" className="h-4" />
      <span className="truncate text-sm text-muted-foreground">{pathname}</span>
    </header>
  );
}

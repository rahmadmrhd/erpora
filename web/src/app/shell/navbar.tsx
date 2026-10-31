import type { MessageRef } from "@erpora/module-sdk";
import {
  useDesks,
  useDeskValidation,
  useMessage,
  usePageChrome,
  useTranslation,
} from "@erpora/platform-web";
import {
  Kbd,
  KbdGroup,
  resolveIcon,
  Separator,
  SidebarTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  useUiLabel,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { type ReactNode, useCallback, useLayoutEffect, useMemo, useState } from "react";
import { BreadcrumbNav, type BreadcrumbSegment } from "./breadcrumb-nav";
import { dedupeCrumbs } from "./crumbs";
import { useDeskNavigation } from "./desk-navigation";
import { planNavbar } from "./navbar-layout";
import {
  ACTION_GAP,
  ELLIPSIS_WIDTH,
  HOME_ICON_WIDTH,
  ICON_BUTTON_WIDTH,
  SEPARATOR_ICON_WIDTH,
  useNavbarBox,
  useTextWidths,
} from "./navbar-measure";
import { actionKey, MORE_ACTION_KEY, PageActionsView, splitPageActions } from "./page-actions-view";
import { useRouteCrumbs } from "./route-crumbs";

const HEADER_CLASS =
  "sticky top-0 z-10 flex h-(--navbar-height) shrink-0 items-center gap-2.5 border-b bg-background px-3.5";

/**
 * Tinggi pembatas dipasang pada varian orientasi agar menggantikan `h-full` bawaan shadcn (`h-4` polos kalah
 * spesifisitas, dan `h-full` di dalam flex berinduk tak berukuran runtuh menjadi 0 px: K5).
 */
const SEPARATOR_CLASS = "data-[orientation=vertical]:h-5";

const NO_DESK_LABEL: MessageRef = { ns: "platform", key: "app.name" };

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

/** Tombol collapse bawaan (SidebarTrigger shadcn) dengan petunjuk Ctrl B dan pemisah. */
function DefaultLeading() {
  const t = useUiLabel();
  return (
    <>
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
      <Separator orientation="vertical" className={SEPARATOR_CLASS} />
    </>
  );
}

function HomeIcon() {
  const Icon = resolveIcon("home");
  return Icon ? <Icon className="size-4" aria-hidden="true" /> : null;
}

export interface NavbarProps {
  /**
   * Isi paling kiri navbar (tombol collapse atau `☰`). `undefined` (bawaan): `SidebarTrigger` shadcn dengan petunjuk
   * Ctrl B dan pemisah, sehingga navbar tetap berfungsi di dalam `SidebarProvider`. `null`: tanpa isi leading.
   * Lebarnya diukur dan dicadangkan sebelum breadcrumb dan aksi dihitung.
   */
  leading?: ReactNode;
}

/**
 * Navbar 48 px lengket di atas area konten (R7, R8): `leading`, breadcrumb bertahap `Home › Desk › halaman`, dan aksi
 * halaman di kanan. Lebar navbar dan blok aksi diukur dengan `ResizeObserver`; aksi sekunder masuk menu "⋯" lebih dulu,
 * baru breadcrumb diringkas (`planNavbar`). Desk berasal dari penentuan Desk (bukan dari rute); halaman yang tidak
 * masuk Desk mana pun menampilkan `Home › halaman` tanpa segmen Desk (R9.6).
 */
export function Navbar({ leading }: NavbarProps) {
  const { t } = useTranslation("platform");
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { deskId } = useDeskValidation(pathname);
  const { desks } = useDesks();
  const desk = deskId === null ? null : (desks.find((d) => d.id === deskId) ?? null);
  const deskText = useMessage(desk?.label ?? NO_DESK_LABEL);
  const { openDesk, goToDesks } = useDeskNavigation();
  const { crumbs: raw, refs } = useRouteCrumbs();
  const [texts, setTexts] = useState<Record<string, string>>({});
  const onText = useCallback((id: string, text: string) => {
    setTexts((prev) => (prev[id] === text ? prev : { ...prev, [id]: text }));
  }, []);
  const crumbs = dedupeCrumbs(raw, texts);
  const homeText = t("breadcrumb.home");

  const { actions } = usePageChrome();
  const split = useMemo(() => splitPageActions(actions), [actions]);

  const [header, setHeader] = useState<HTMLElement | null>(null);
  const [leadingEl, setLeadingEl] = useState<HTMLElement | null>(null);
  const box = useNavbarBox(header, leadingEl);

  const segments = useMemo<BreadcrumbSegment[]>(() => {
    const out: BreadcrumbSegment[] = [
      {
        key: "home",
        kind: "home",
        text: homeText,
        label: <HomeIcon />,
        onSelect: goToDesks,
      },
    ];
    if (desk !== null) {
      out.push({
        key: `desk:${desk.id}`,
        kind: "desk",
        text: deskText,
        label: deskText,
        onSelect: () => openDesk(desk),
      });
    }
    for (const c of crumbs) {
      out.push({
        key: c.key,
        kind: "crumb",
        text: c.text ?? texts[c.key] ?? "",
        label: c.label,
        ...(c.to === undefined ? {} : { to: c.to }),
      });
    }
    return out;
  }, [homeText, goToDesks, desk, deskText, openDesk, raw, texts]);

  const measured = useTextWidths(
    header,
    segments.map((s) => s.text),
  );
  const widths = segments.map((s, i) => (s.kind === "home" ? HOME_ICON_WIDTH : (measured[i] ?? 0)));

  // Lebar alami tombol aksi (dari pengukuran PageActionsView); kunci berubah bila id atau label berubah.
  const [actionWidths, setActionWidths] = useState<Record<string, number>>({});
  const onMeasureActions = useCallback((next: Record<string, number>) => {
    setActionWidths((prev) => {
      let changed = false;
      const merged = { ...prev };
      for (const [key, width] of Object.entries(next)) {
        if (merged[key] === undefined || Math.abs(merged[key] - width) > 0.5) {
          merged[key] = width;
          changed = true;
        }
      }
      return changed ? merged : prev;
    });
  }, []);

  const primaryEntries = [...split.nodes, ...split.primary];
  const widthOf = (key: string): number => actionWidths[key] ?? 0;
  const gaps = (n: number): number => ACTION_GAP * Math.max(0, n - 1);
  const primaryNatural =
    primaryEntries.reduce((sum, a) => sum + widthOf(actionKey(a)), 0) + gaps(primaryEntries.length);
  const iconTotal = split.primary.length * ICON_BUTTON_WIDTH + gaps(split.primary.length);

  const layout = planNavbar({
    navWidth: box.navWidth,
    inset: box.inset,
    gap: box.gap,
    leadingWidth: box.leadingWidth,
    breadcrumb: {
      segments: segments.map((s) => s.key),
      widths,
      separatorWidth: SEPARATOR_ICON_WIDTH + 2 * box.listGap,
      ellipsisWidth: ELLIPSIS_WIDTH,
      pinned: desk === null ? 1 : 2,
    },
    actions: {
      primaryWidth: primaryEntries.length > 0 ? Math.max(primaryNatural, 1) : 0,
      iconWidth: iconTotal,
      secondaryWidths: split.secondary.map((a) => widthOf(actionKey(a))),
      moreWidth: actionWidths[MORE_ACTION_KEY] ?? ICON_BUTTON_WIDTH,
      gap: ACTION_GAP,
    },
  });

  // Tombol belum pernah diukur (aksi baru atau label berubah): satu putaran dengan semua aksi inline agar lebar
  // alaminya terukur sebelum cat; sesudahnya perencana melipat sesuai ruang.
  const unmeasured =
    !layout.narrow &&
    [...primaryEntries, ...split.secondary].some((a) => actionWidths[actionKey(a)] === undefined);
  const collapsed = unmeasured ? 0 : layout.collapsedActions;

  const leadingNode = leading === undefined ? <DefaultLeading /> : leading;

  return (
    <header ref={setHeader} data-slot="navbar" className={HEADER_CLASS}>
      {leadingNode === null ? null : (
        <div
          ref={setLeadingEl}
          data-slot="navbar-leading"
          className="flex shrink-0 items-center gap-2.5"
        >
          {leadingNode}
        </div>
      )}
      {refs.map((r) => (
        <CrumbTextProbe key={r.key} id={r.key} message={r.ref} onText={onText} />
      ))}
      <BreadcrumbNav
        className="min-w-0 flex-1"
        segments={segments}
        plan={layout.plan}
        narrow={layout.narrow}
      />
      <PageActionsView
        split={split}
        collapsed={collapsed}
        narrow={layout.narrow}
        onMeasure={onMeasureActions}
      />
    </header>
  );
}

/**
 * Cadangan Navbar saat memuat atau galat: tombol collapse tetap ada (di layar kecil panel hanya dibuka lewat tombol ini)
 * dan jejak minimal berupa path.
 */
export function NavbarFallback({ leading }: NavbarProps) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const leadingNode = leading === undefined ? <DefaultFallbackLeading /> : leading;
  return (
    <header className={HEADER_CLASS}>
      {leadingNode}
      <span className="truncate text-sm text-muted-foreground">{pathname}</span>
    </header>
  );
}

function DefaultFallbackLeading() {
  const t = useUiLabel();
  return (
    <>
      <SidebarTrigger aria-label={t("sidebar.toggle")} />
      <Separator orientation="vertical" className={SEPARATOR_CLASS} />
    </>
  );
}

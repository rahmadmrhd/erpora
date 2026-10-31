import {
  type Desk,
  useCurrentDesk,
  useDesks,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Separator,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@erpora/ui";
import {
  type KeyboardEvent,
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useDeskNavigation } from "./desk-navigation";
import { LogoPopover } from "./logo-popover";
import { NavIcon } from "./nav-icon";
import { UserMenu } from "./user-menu";

/** Ukuran tombol ikon Dock (`size-10`) dan jarak antartombol (`gap-1`), dalam px. */
export const DOCK_ITEM_SIZE = 40;
export const DOCK_GAP = 4;

/**
 * Jumlah ikon Desk yang ditampilkan dari `total` pada tinggi `availableHeight`. Bila semua muat, semuanya; bila
 * tidak, satu slot disisihkan untuk tombol "+N" (R4.3). Tinggi belum terukur (0 atau tak berhingga/NaN) berarti
 * tampilkan semua (tidak menyembunyikan Desk karena pengukuran belum ada).
 */
export function fitDockCount(input: {
  total: number;
  availableHeight: number;
  itemSize: number;
  gap: number;
}): number {
  const { total, availableHeight, itemSize, gap } = input;
  if (total <= 0) return 0;
  if (!Number.isFinite(availableHeight) || availableHeight <= 0) return total;
  const slots = Math.floor((availableHeight + gap) / (itemSize + gap));
  if (slots >= total) return total;
  return Math.max(0, slots - 1);
}

/** Tinggi konten elemen (diamati lewat `ResizeObserver`); 0 sebelum terukur atau tanpa `ResizeObserver`. */
function useElementHeight(ref: RefObject<HTMLElement | null>): number {
  const [height, setHeight] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setHeight(el.clientHeight);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) setHeight(entry.contentRect.height);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref]);
  return height;
}

function DeskIconButton({
  desk,
  active,
  tabIndex,
  onSelect,
  onFocus,
}: {
  desk: Desk;
  active: boolean;
  tabIndex: number;
  onSelect: (desk: Desk) => void;
  onFocus: () => void;
}) {
  const text = useMessage(desk.label);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          data-dock-item=""
          data-active={active ? "true" : undefined}
          aria-current={active ? "true" : undefined}
          aria-label={text}
          tabIndex={tabIndex}
          onFocus={onFocus}
          onClick={() => onSelect(desk)}
          className="size-10 rounded-lg text-sidebar-foreground data-[active=true]:bg-sidebar-accent data-[active=true]:text-sidebar-accent-foreground [&_svg]:size-5"
        >
          <NavIcon name={desk.icon} label={text} />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="right">{text}</TooltipContent>
    </Tooltip>
  );
}

function OverflowItem({ desk, onSelect }: { desk: Desk; onSelect: (desk: Desk) => void }) {
  const text = useMessage(desk.label);
  return (
    <DropdownMenuItem onSelect={() => onSelect(desk)}>
      <NavIcon name={desk.icon} label={text} />
      {text}
    </DropdownMenuItem>
  );
}

/**
 * Daftar ikon Desk yang diizinkan (R4.1 sampai R4.3, R4.7, R4.8): urutan dari server, Desk saat ini ditandai
 * (`aria-current`), `+N` membuka daftar sisanya, panah atas/bawah, Home, dan End memindahkan fokus (satu ikon yang
 * dapat difokus pada satu waktu). Dipakai Dock Pinned dan pil Floating.
 */
export function DockDeskList({
  onSelected,
  initialFocusRef,
}: {
  /** Dipanggil setelah Desk dipilih (pil Floating menutup diri). */
  onSelected?: () => void;
  /** Diisi dengan fungsi yang memfokuskan ikon Desk saat ini (atau pertama). */
  initialFocusRef?: RefObject<(() => void) | null>;
}) {
  const { t } = useTranslation("platform");
  const { desks } = useDesks();
  const { deskId } = useCurrentDesk();
  const { openDesk } = useDeskNavigation();
  const listRef = useRef<HTMLUListElement>(null);
  const height = useElementHeight(listRef);
  const visible = fitDockCount({
    total: desks.length,
    availableHeight: height,
    itemSize: DOCK_ITEM_SIZE,
    gap: DOCK_GAP,
  });
  const shown = desks.slice(0, visible);
  const hidden = desks.slice(visible);
  const [focusIndex, setFocusIndex] = useState(0);
  const slots = shown.length + (hidden.length > 0 ? 1 : 0);
  const focusable = Math.min(focusIndex, Math.max(0, slots - 1));

  const select = useCallback(
    (desk: Desk) => {
      openDesk(desk);
      onSelected?.();
    },
    [openDesk, onSelected],
  );

  const items = () =>
    Array.from(listRef.current?.querySelectorAll<HTMLElement>("[data-dock-item]") ?? []);

  useEffect(() => {
    if (!initialFocusRef) return;
    initialFocusRef.current = () => {
      const list = items();
      const active = list.find((el) => el.getAttribute("aria-current") === "true");
      (active ?? list[0])?.focus();
    };
    return () => {
      initialFocusRef.current = null;
    };
  });

  const onKeyDown = (event: KeyboardEvent<HTMLUListElement>) => {
    const list = items();
    const index = list.findIndex((el) => el === document.activeElement);
    if (index < 0) return;
    let next = index;
    if (event.key === "ArrowDown") next = Math.min(list.length - 1, index + 1);
    else if (event.key === "ArrowUp") next = Math.max(0, index - 1);
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = list.length - 1;
    else return;
    event.preventDefault();
    list[next]?.focus();
  };

  return (
    <ul
      ref={listRef}
      onKeyDown={onKeyDown}
      className="flex min-h-0 flex-1 flex-col items-center gap-1 overflow-hidden"
    >
      {shown.map((desk, i) => (
        <li key={desk.id}>
          <DeskIconButton
            desk={desk}
            active={desk.id === deskId}
            tabIndex={i === focusable ? 0 : -1}
            onSelect={select}
            onFocus={() => setFocusIndex(i)}
          />
        </li>
      ))}
      {hidden.length > 0 ? (
        <li>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                data-dock-item=""
                aria-label={t("dock.more", { count: hidden.length })}
                tabIndex={shown.length === focusable ? 0 : -1}
                onFocus={() => setFocusIndex(shown.length)}
                className="size-10 rounded-lg text-xs font-medium"
              >
                {`+${hidden.length}`}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent side="right" align="start">
              {hidden.map((d) => (
                <OverflowItem key={d.id} desk={d} onSelect={select} />
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </li>
      ) : null}
    </ul>
  );
}

/**
 * Dock Pinned (R4.4): rel ikon berdiri di kiri bersama panel menu. Dari atas: logo (popover), pemisah, ikon Desk,
 * lalu avatar pengguna di dasar (R16.1). Di bawah 1024 px Dock tidak dirender (panel menjadi Sheet, R4.6).
 */
export function Dock() {
  const { t } = useTranslation("platform");
  return (
    <nav
      aria-label={t("dock.label")}
      data-slot="dock"
      className="flex h-full w-(--dock-width) shrink-0 flex-col items-center gap-2 border-r border-sidebar-border bg-sidebar py-2 text-sidebar-foreground"
    >
      <LogoPopover variant="dock" />
      <Separator className="w-8" />
      <DockDeskList />
      <UserMenu variant="dock" />
    </nav>
  );
}

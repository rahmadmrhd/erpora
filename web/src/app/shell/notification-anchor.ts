import { useLayoutEffect, useState } from "react";

/**
 * Penempatan panel Notifikasi:
 * - `sidebar`: lembar setinggi layar yang dimulai tepat di tepi kanan Dock/panel menu;
 * - `desks`: kartu mengambang di kanan atas, di bawah navbar (halaman Desks tidak punya Dock maupun panel menu).
 */
export type NotificationPlacement = "sidebar" | "desks";

export interface NotificationAnchor {
  placement: NotificationPlacement;
  /** Offset kiri (px) untuk `sidebar`; 0 bila tidak ada Dock/panel menu (mis. layar ponsel). */
  left: number;
  /** Jarak atas (px) untuk `desks`: tepi bawah navbar ditambah celah. */
  top: number;
}

/** Container panel menu (sidebar shadcn), Dock Pinned, navbar, dan penanda kerangka halaman Desks. */
const SIDEBAR = '[data-slot="sidebar-container"]';
const SIDEBAR_ROOT = '[data-slot="sidebar"]';
const DOCK = '[data-slot="dock"]';
const NAVBAR = '[data-slot="navbar"]';
const DESKS_CHROME = '[data-shell-chrome="desks"]';

/** Celah antara navbar dan kartu (px) dan tinggi navbar cadangan bila navbar tidak ditemukan. */
const CARD_GAP = 8;
const FALLBACK_NAVBAR = 48;

/**
 * Lama (ms) pengukuran ulang berkelanjutan setelah panel menu berubah keadaan (transisi sidebar 200 ms): offset
 * mengikuti tepi panel menu yang bergerak, bukan melompat setelah transisi selesai.
 */
const FOLLOW_MS = 320;

/**
 * Tepi kanan terjauh dari Dock dan container panel menu yang terlihat: Pinned terbuka = tepi kanan container; Pinned
 * diciutkan (container offcanvas di luar layar, tepi kanannya <= tepi Dock) = tepi kanan Dock; Floating = tepi kanan
 * strip ikon. Tidak ada keduanya = 0.
 */
export function measureSidebarOffset(): number {
  let edge = 0;
  for (const selector of [DOCK, SIDEBAR]) {
    const el = document.querySelector(selector);
    if (el !== null) edge = Math.max(edge, Math.round(el.getBoundingClientRect().right));
  }
  return Math.max(0, edge);
}

function measureCardTop(): number {
  const navbar = document.querySelector(NAVBAR);
  const bottom =
    navbar === null ? FALLBACK_NAVBAR : Math.round(navbar.getBoundingClientRect().bottom);
  return Math.max(0, bottom) + CARD_GAP;
}

function resolve(requested: NotificationPlacement | "auto"): NotificationPlacement {
  if (requested !== "auto") return requested;
  return document.querySelector(DESKS_CHROME) === null ? "sidebar" : "desks";
}

function measure(requested: NotificationPlacement | "auto"): NotificationAnchor {
  const placement = resolve(requested);
  return placement === "desks"
    ? { placement, left: 0, top: measureCardTop() }
    : { placement, left: measureSidebarOffset(), top: 0 };
}

function same(a: NotificationAnchor, b: NotificationAnchor): boolean {
  return a.placement === b.placement && a.left === b.left && a.top === b.top;
}

/**
 * Posisi panel Notifikasi, diukur langsung dari kerangka (panel dirender di portal, di luar `SidebarProvider`, sehingga
 * tidak mewarisi `--sidebar-width`). Diukur sinkron sebelum dilukis saat dibuka (tanpa lompatan), lalu diikuti saat
 * jendela atau panel menu berubah ukuran/keadaan (observer ukuran, perubahan atribut container, dan pengukuran per
 * frame selama transisi sidebar).
 */
export function useNotificationAnchor(
  open: boolean,
  requested: NotificationPlacement | "auto" = "auto",
): NotificationAnchor {
  const [anchor, setAnchor] = useState<NotificationAnchor>(() =>
    typeof document === "undefined"
      ? { placement: "sidebar", left: 0, top: 0 }
      : measure(requested),
  );

  useLayoutEffect(() => {
    if (!open) return;
    const update = () => {
      const next = measure(requested);
      setAnchor((prev) => (same(prev, next) ? prev : next));
    };
    update();

    let frame = 0;
    let until = 0;
    const follow = () => {
      update();
      frame = performance.now() < until ? requestAnimationFrame(follow) : 0;
    };
    const startFollowing = () => {
      until = performance.now() + FOLLOW_MS;
      if (frame === 0) frame = requestAnimationFrame(follow);
    };

    window.addEventListener("resize", update);
    const watched = [DOCK, SIDEBAR, NAVBAR]
      .map((selector) => document.querySelector(selector))
      .filter((el): el is Element => el !== null);
    const resize =
      typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(() => update());
    for (const el of watched) resize?.observe(el);
    // Keadaan ciut/buka ada di akar sidebar (`data-state`/`data-collapsible`), induk container.
    const sidebarRoot = document.querySelector(SIDEBAR)?.closest(SIDEBAR_ROOT) ?? null;
    const mutation =
      typeof MutationObserver === "undefined" ? undefined : new MutationObserver(startFollowing);
    for (const el of sidebarRoot === null ? watched : [...watched, sidebarRoot]) {
      mutation?.observe(el, {
        attributes: true,
        attributeFilter: ["data-state", "data-collapsible", "class", "style"],
      });
    }
    return () => {
      window.removeEventListener("resize", update);
      resize?.disconnect();
      mutation?.disconnect();
      if (frame !== 0) cancelAnimationFrame(frame);
    };
  }, [open, requested]);

  return anchor;
}

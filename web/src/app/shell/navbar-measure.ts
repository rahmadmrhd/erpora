import { useEffect, useLayoutEffect, useState } from "react";

/** Lebar ikon Home (`size-4`) pada breadcrumb. */
export const HOME_ICON_WIDTH = 16;
/** Lebar pemicu elipsis breadcrumb (`size-6`). */
export const ELLIPSIS_WIDTH = 24;
/** Lebar ikon pemisah breadcrumb (`size-3.5`). */
export const SEPARATOR_ICON_WIDTH = 14;
/** Jarak antartombol aksi di navbar (`gap-2`). */
export const ACTION_GAP = 8;
/** Lebar tombol ikon (`size-9`). */
export const ICON_BUTTON_WIDTH = 36;

export interface NavbarBox {
  /** Lebar header navbar (px); 0 bila belum terukur. */
  navWidth: number;
  /** Padding horizontal header (kiri + kanan). */
  inset: number;
  /** Jarak antarblok di header. */
  gap: number;
  /** Lebar blok leading; 0 bila tidak ada. */
  leadingWidth: number;
  /** Jarak antaritem daftar breadcrumb. */
  listGap: number;
}

const EMPTY_BOX: NavbarBox = { navWidth: 0, inset: 0, gap: 10, leadingWidth: 0, listGap: 10 };

const px = (v: string | undefined): number => {
  const n = Number.parseFloat(v ?? "");
  return Number.isFinite(n) ? n : 0;
};

function readBox(header: HTMLElement, leading: HTMLElement | null): NavbarBox {
  const style = getComputedStyle(header);
  const list = header.querySelector('[data-slot="breadcrumb-list"]');
  return {
    navWidth: header.getBoundingClientRect().width,
    inset: px(style.paddingLeft) + px(style.paddingRight),
    gap: px(style.columnGap) || EMPTY_BOX.gap,
    leadingWidth: leading?.getBoundingClientRect().width ?? 0,
    listGap: (list ? px(getComputedStyle(list).columnGap) : 0) || EMPTY_BOX.listGap,
  };
}

const sameBox = (a: NavbarBox, b: NavbarBox): boolean =>
  a.navWidth === b.navWidth &&
  a.inset === b.inset &&
  a.gap === b.gap &&
  a.leadingWidth === b.leadingWidth &&
  a.listGap === b.listGap;

/**
 * Mengukur header navbar dan blok leading dengan `ResizeObserver` (R7.2: pengukuran kontainer, bukan media query).
 * Pengukuran awal terjadi sebelum cat pertama (layout effect).
 */
export function useNavbarBox(header: HTMLElement | null, leading: HTMLElement | null): NavbarBox {
  const [box, setBox] = useState<NavbarBox>(EMPTY_BOX);
  useLayoutEffect(() => {
    if (header === null) return;
    const measure = () => {
      const next = readBox(header, leading);
      setBox((prev) => (sameBox(prev, next) ? prev : next));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(header);
    if (leading !== null) observer.observe(leading);
    return () => observer.disconnect();
  }, [header, leading]);
  return box;
}

/**
 * Huruf halaman saat ini (breadcrumb terakhir, satu-satunya judul visual: K9). Dipakai `BreadcrumbNav` dan probe
 * pengukuran sehingga lebar yang direncanakan `fitBreadcrumb` sama dengan lebar yang digambar.
 */
export const CURRENT_PAGE_CLASS = "text-base font-semibold";

const PROBE_BASE_CLASS = "invisible pointer-events-none absolute top-0 left-0 whitespace-nowrap";
const PROBE_STEP_CLASS = "text-sm font-normal";

/**
 * Mengukur lebar teks dengan elemen sementara di dalam `container` (mewarisi huruf navbar) lalu menghapusnya. Teks
 * terakhir adalah halaman saat ini dan diukur dengan `CURRENT_PAGE_CLASS`; yang lain dengan huruf langkah biasa.
 */
export function measureTexts(container: HTMLElement, texts: readonly string[]): number[] {
  const probe = document.createElement("span");
  probe.setAttribute("aria-hidden", "true");
  probe.dataset.fitProbe = "";
  container.append(probe);
  const last = texts.length - 1;
  try {
    return texts.map((text, i) => {
      probe.className = `${PROBE_BASE_CLASS} ${i === last ? CURRENT_PAGE_CLASS : PROBE_STEP_CLASS}`;
      probe.textContent = text;
      return probe.getBoundingClientRect().width;
    });
  } finally {
    probe.remove();
  }
}

/**
 * Lebar teks tiap segmen breadcrumb (sejajar dengan `texts`). Diukur ulang saat teks berubah dan setelah huruf
 * web selesai dimuat (lebar teks bergantung pada huruf).
 */
export function useTextWidths(container: HTMLElement | null, texts: readonly string[]): number[] {
  const [widths, setWidths] = useState<number[]>([]);
  const [fontsTick, setFontsTick] = useState(0);
  const signature = texts.join("\u0000");

  useEffect(() => {
    const fonts = (document as Document & { fonts?: FontFaceSet }).fonts;
    if (fonts === undefined) return;
    let active = true;
    const bump = () => {
      if (active) setFontsTick((n) => n + 1);
    };
    void fonts.ready.then(bump);
    fonts.addEventListener?.("loadingdone", bump);
    return () => {
      active = false;
      fonts.removeEventListener?.("loadingdone", bump);
    };
  }, []);

  useLayoutEffect(() => {
    if (container === null) return;
    const next = measureTexts(container, texts);
    setWidths((prev) =>
      prev.length === next.length && prev.every((w, i) => w === next[i]) ? prev : next,
    );
  }, [container, signature, fontsTick]);

  return texts.map((_, i) => widths[i] ?? 0);
}

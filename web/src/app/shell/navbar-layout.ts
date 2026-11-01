import { type FitPlan, fitBreadcrumb } from "@erpora/ui";

/** Navbar lebih sempit dari ini dianggap ponsel: "…" jadi lembar bawah, aksi utama jadi ikon, breadcrumb terakhir saja. */
export const NARROW_NAVBAR_WIDTH = 480;

export interface NavbarLayoutInput {
  /** Lebar header navbar (px); `<= 0` berarti belum terukur (tanpa pembatasan). */
  navWidth: number;
  /** Jumlah padding horizontal header (kiri + kanan). */
  inset: number;
  /** Jarak antarblok di header. */
  gap: number;
  /** Lebar blok leading (tombol collapse/☰); 0 bila tidak ada. */
  leadingWidth: number;
  breadcrumb: {
    segments: readonly string[];
    widths: readonly number[];
    separatorWidth: number;
    ellipsisWidth: number;
    pinned: number;
  };
  actions: {
    /** Lebar alami aksi utama (dan simpul kustom) bila tampil penuh; 0 bila tidak ada. */
    primaryWidth: number;
    /** Lebar total aksi utama sebagai tombol ikon (ponsel). */
    iconWidth: number;
    /** Lebar alami tiap aksi sekunder berurutan (deklarasi); yang terlipat ke "⋯" paling belakang lebih dulu. */
    secondaryWidths: readonly number[];
    /** Lebar tombol "⋯". */
    moreWidth: number;
    /** Jarak antartombol aksi. */
    gap: number;
  };
}

export interface NavbarLayout {
  /** Navbar < 480 px. */
  narrow: boolean;
  /** Jumlah aksi sekunder (dari belakang) yang masuk menu "⋯". */
  collapsedActions: number;
  /** Rencana breadcrumb pada ruang yang tersisa. */
  plan: FitPlan;
}

function actionsWidth(a: NavbarLayoutInput["actions"], narrow: boolean, collapsed: number): number {
  const secondary = a.secondaryWidths.slice(0, a.secondaryWidths.length - collapsed);
  const widths: number[] = [];
  const primary = narrow ? (a.primaryWidth > 0 ? a.iconWidth : 0) : a.primaryWidth;
  if (primary > 0) widths.push(primary);
  widths.push(...secondary);
  if (collapsed > 0) widths.push(a.moreWidth);
  if (widths.length === 0) return 0;
  return widths.reduce((s, w) => s + w, 0) + a.gap * (widths.length - 1);
}

/**
 * Menentukan tata letak navbar (R7.2, R7.6, R8): aksi sekunder masuk "⋯" lebih dulu (dari yang terakhir) sebelum
 * breadcrumb diringkas lebih jauh; aksi utama tidak pernah hilang. Pada navbar sempit semua sekunder terlipat dan
 * breadcrumb menyisakan langkah terakhir. Fungsi murni: pengukuran DOM dilakukan pemanggil.
 */
export function planNavbar(input: NavbarLayoutInput): NavbarLayout {
  const { navWidth, breadcrumb: bc } = input;
  const measured = navWidth > 0;
  const narrow = measured && navWidth < NARROW_NAVBAR_WIDTH;
  const secondaryCount = input.actions.secondaryWidths.length;

  const fit = (collapsed: number): FitPlan => {
    let available = Number.POSITIVE_INFINITY;
    if (measured) {
      const actions = actionsWidth(input.actions, narrow, collapsed);
      const leading = input.leadingWidth > 0 ? input.leadingWidth + input.gap : 0;
      available = navWidth - input.inset - leading - (actions > 0 ? actions + input.gap : 0);
    }
    return fitBreadcrumb({
      segments: bc.segments,
      widths: bc.widths,
      available,
      mobile: narrow,
      separatorWidth: bc.separatorWidth,
      ellipsisWidth: bc.ellipsisWidth,
      pinned: bc.pinned,
    });
  };

  if (!measured) return { narrow, collapsedActions: 0, plan: fit(0) };
  if (narrow) return { narrow, collapsedActions: secondaryCount, plan: fit(secondaryCount) };

  for (let collapsed = 0; collapsed <= secondaryCount; collapsed++) {
    const plan = fit(collapsed);
    if (plan.level === 0) return { narrow, collapsedActions: collapsed, plan };
  }
  return { narrow, collapsedActions: secondaryCount, plan: fit(secondaryCount) };
}

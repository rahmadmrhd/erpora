import { deskContains, matchMenu } from "./match-menu";
import { isValidDeskId } from "./storage";
import type { Desk } from "./types";

export type ResolveStep = 1 | 2 | 3 | 4 | 5;

export interface ResolveInput {
  /** Desk yang diizinkan server (sudah disaring). */
  desks: readonly Desk[];
  /** Tahap 1: `sessionStorage`. Tidak tepercaya: tipe apa pun diterima dan divalidasi di sini. */
  tabDesk?: unknown;
  /** Tahap 2: `localStorage`. Tidak tepercaya. */
  rememberedDesk?: unknown;
  /** Tahap 3: `default_desk` pengguna (`null` sampai PRD-001). */
  defaultDesk?: unknown;
  /** Path yang dibuka; tahap 4-5 hanya berjalan bila ada, dan semua kandidat harus memuatnya. */
  page?: string | null;
}

export interface ResolveResult {
  deskId: string | null;
  step: ResolveStep | null;
}

const none: ResolveResult = { deskId: null, step: null };

/**
 * Penentuan Desk saat ini (M3a R9.1-R9.2), murni: kandidat berurutan memori tab, `localStorage`, `default_desk`,
 * Desk pemilik utama halaman, Desk pertama yang memuat halaman. Kandidat diterima bila berbentuk id valid, ada di
 * `desks`, dan (bila `page` ada) memuat `page`; kalau tidak, lanjut ke tahap berikutnya.
 */
export function resolveDesk(input: ResolveInput): ResolveResult {
  const { desks, page } = input;
  const hasPage = typeof page === "string" && page !== "";
  const accept = (id: string): boolean => {
    const desk = desks.find((d) => d.id === id);
    return desk !== undefined && (!hasPage || deskContains(desk, page));
  };

  const stored: [unknown, ResolveStep][] = [
    [input.tabDesk, 1],
    [input.rememberedDesk, 2],
    [input.defaultDesk, 3],
  ];
  for (const [candidate, step] of stored) {
    if (isValidDeskId(candidate) && accept(candidate)) return { deskId: candidate, step };
  }

  if (!hasPage) return none;

  const { owner } = matchMenu(page, desks);
  if (owner !== null && accept(owner)) return { deskId: owner, step: 4 };

  const first = desks.find((d) => deskContains(d, page));
  return first ? { deskId: first.id, step: 5 } : none;
}

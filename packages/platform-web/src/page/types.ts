import type { ReactNode } from "react";

/** Aksi halaman: tombol yang dideklarasikan halaman dan dirender shell di navbar (M3a K-8). */
export interface PageAction {
  readonly id: string;
  /** Sudah diterjemahkan oleh pemanggil. */
  readonly label: string;
  /** Nama ikon di registry `@erpora/ui`. */
  readonly icon?: string;
  readonly variant?: "primary" | "outline" | "ghost" | "destructive";
  readonly onSelect: () => void;
  readonly disabled?: boolean;
  readonly loading?: boolean;
  /** Tetap tampil sebagai tombol ikon di layar kecil (selain itu dilipat ke menu ⋯). */
  readonly pinned?: boolean;
}

/** Escape hatch simpul kustom: hanya tampil di layar >= md (di layar kecil tidak dilipat ke menu ⋯). */
export interface PageActionNode {
  readonly id: string;
  readonly node: ReactNode;
}

export type PageActionEntry = PageAction | PageActionNode;

export function isPageActionNode(entry: PageActionEntry): entry is PageActionNode {
  return "node" in entry;
}

export interface PageChrome {
  readonly actions: readonly PageActionEntry[];
  /** `false` mematikan gulir area konten (layout list-detail). */
  readonly scroll: boolean;
}

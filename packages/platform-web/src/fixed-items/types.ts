import type { MessageRef } from "../desk/types";

/**
 * Aksi item tetap: `command` dipetakan shell ke perintah (mis. `search` membuka palet, `notifications` membuka
 * panel); `link` menavigasi ke path internal. Registri tidak mengenal komponen UI sehingga item dapat ditambah
 * tanpa mengubah panel (M3a revisi R5.3).
 */
export type FixedItemAction = { kind: "command"; command: string } | { kind: "link"; to: string };

/** Masukan `registerFixedItem`. */
export interface FixedItemInput {
  /** Pengenal unik (huruf kecil, angka, tanda hubung). */
  id: string;
  label: MessageRef;
  /** Nama ikon registri `@erpora/ui`. */
  icon: string;
  /** Urutan naik; bawaan 10 (Cari), 20 (Notifikasi), 30 (ToDo). */
  order: number;
  action: FixedItemAction;
  /** Hook yang mengembalikan jumlah untuk lencana (0 = tanpa lencana); dipanggil di komponen baris item. */
  useBadge?: () => number;
  /** Tombol pintasan yang ditampilkan sebagai petunjuk (mis. `["Ctrl", "K"]`). */
  shortcut?: readonly string[];
}

/** Item terdaftar; `useBadge` selalu ada sehingga baris dapat memanggilnya tanpa kondisi. */
export type FixedItem = Omit<FixedItemInput, "useBadge"> & { useBadge: () => number };

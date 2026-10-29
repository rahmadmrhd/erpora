import type { ReactNode } from "react";

/**
 * Bentuk katalog terjemahan: `{ [namespace]: { [kunci lengkap]: bentuk argumen } }`. Kosong di sini dan diperluas
 * oleh `keys.gen.ts` tiap paket lewat augmentasi (`declare module "@erpora/platform-web" { interface I18nCatalogs {...} }`)
 * dari katalog bahasa default (design spec i18n-platform §8). Contoh nilai: `{}` (tanpa argumen),
 * `{ count: number }`, `{ state: "draft" | "approved" }`, dan `{ b: RichTag }` untuk tag rich.
 */
// biome-ignore lint/suspicious/noEmptyInterface: sengaja kosong, diperluas lewat penggabungan interface
export interface I18nCatalogs {}

/** Penanda tag rich pada bentuk argumen (`<b>…</b>` menjadi properti `b: RichTag`). */
export type RichTag = (chunks: ReactNode) => ReactNode;

type TagKeys<V> = { [K in keyof V]-?: V[K] extends RichTag ? K : never }[keyof V];

/** Argumen teks polos: bentuk argumen tanpa tag rich. */
export type PlainArgs<V> = Omit<V, TagKeys<V>>;

/** Sisa parameter `t(key, ...args)`: wajib bila ada argumen, dihilangkan bila tidak. */
export type ArgsOf<V> = [keyof PlainArgs<V>] extends [never]
  ? [args?: PlainArgs<V>]
  : [args: PlainArgs<V>];

/** Sisa parameter `rich(key, ...args)`: argumen polos dan fungsi tag. */
export type RichArgsOf<V> = [keyof V] extends [never] ? [args?: V] : [args: V];

/** Hasil `useTranslation(ns)` untuk bentuk katalog namespace `K`. */
export interface Translator<K> {
  /** Teks polos (tag rich dilucuti); kunci tidak ada atau pesan rusak mengembalikan kunci mentah. */
  t<Key extends keyof K & string>(key: Key, ...args: ArgsOf<K[Key]>): string;
  /** Teks dengan tag rich dirender sebagai simpul React. */
  rich<Key extends keyof K & string>(key: Key, ...args: RichArgsOf<K[Key]>): ReactNode;
  readonly locale: string;
  /** Mengganti bahasa (divalidasi terhadap daftar bahasa; yang tak terdaftar diabaikan). */
  setLocale(locale: string): void;
}

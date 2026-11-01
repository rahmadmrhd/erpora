import { type ComponentProps, type ComponentType, lazy } from "react";

// biome-ignore lint/suspicious/noExplicitAny: parameter props komponen apa pun (sama dengan tipe `React.lazy`)
type AnyComponent = ComponentType<any>;

/**
 * `React.lazy` yang chunk-nya dapat dicoba lagi. `React.lazy` menyimpan promise yang ditolak secara permanen, sehingga
 * merender ulang komponen lazy yang sama tidak pernah memuat ulang chunk. Di sini komponen lazy dibuat ulang oleh
 * `reset()` (dipanggil `onRetry` batas galat); pembungkus membaca komponen terbaru saat dirender ulang, jadi percobaan
 * berikutnya memanggil `load` lagi. Bila peramban sendiri menyimpan kegagalan impor dinamis, `load` tetap gagal dan
 * galat kembali ke batas; pemulihannya muat ulang halaman.
 */
export function retryableLazy<C extends AnyComponent>(
  load: () => Promise<{ default: C }>,
): ((props: ComponentProps<C>) => React.JSX.Element) & { reset(force?: boolean): void } {
  // Percobaan terakhir ditolak? Hanya dalam keadaan itu `reset()` membuat komponen lazy baru: bila percobaan terakhir
  // berhasil (atau sedang berjalan), reset tidak berbuat apa pun agar bagian yang sudah pulih tidak ter-remount
  // (fokus dan popover terbuka tetap) oleh batas galat lain yang ikut mencoba lagi.
  let failed = false;
  const create = (): AnyComponent =>
    lazy(() =>
      load().catch((error: unknown) => {
        failed = true;
        throw error;
      }),
    );
  let Current = create();
  const Wrapper = (props: ComponentProps<C>) => {
    const Component = Current;
    return <Component {...props} />;
  };
  // `force` (uji) membuat ulang komponen walau percobaan terakhir tidak gagal.
  Wrapper.reset = (force = false): void => {
    if (!failed && !force) return;
    failed = false;
    Current = create();
  };
  return Wrapper;
}

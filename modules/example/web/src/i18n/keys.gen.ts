// Kunci namespace "example" (bentuk argumen per kunci, dari katalog bahasa default modules/example/i18n/id.json).
// DITULIS TANGAN sementara: generator `i18n:generate` belum ada (spec i18n-platform); setelah ada, berkas ini dibangkitkan ulang
// dan diperiksa `i18n:generate --check`. Memperluas `I18nCatalogs` lewat augmentasi sehingga `msg()`/`useTranslation` menolak
// kunci dan argumen yang salah ketik.
export {};

declare module "@erpora/platform-web" {
  interface I18nCatalogs {
    example: {
      "nav.notes": {};
      "page.notes.title": {};
      "page.notes.description": {};
      "page.notes.empty": {};
      "page.notes.count": { count: number };
      "page.notes.error": {};
      "page.notes.columns.title": {};
      "page.notes.columns.version": {};
      "page.notes.columns.updated": {};
      "actions.refresh": {};
    };
  }
}

// Kunci namespace "example-addon" (bentuk argumen per kunci, dari katalog bahasa default modules/exampleaddon/i18n/id.json).
// DITULIS TANGAN sementara: generator `i18n:generate` belum ada (spec i18n-platform); setelah ada, berkas ini dibangkitkan ulang
// dan diperiksa `i18n:generate --check`. Memperluas `I18nCatalogs` lewat augmentasi sehingga `msg()`/`useTranslation` menolak
// kunci dan argumen yang salah ketik.
export {};

declare module "@erpora/platform-web" {
  interface I18nCatalogs {
    "example-addon": {
      "nav.summary": {};
      "page.summary.title": {};
      "page.summary.description": {};
      "page.summary.note_count": { count: number };
      "page.summary.error": {};
      "page.summary.view_notes": {};
      "actions.refresh": {};
    };
  }
}

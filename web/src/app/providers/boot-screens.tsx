import { Button, ErrorPage, LoadingIcon } from "@erpora/ui";

// Layar boot memakai kamus bawaan tiga kalimat id/en, BUKAN kunci i18n (PRD-000 Q36, PK-4): runtime i18n bergantung pada
// bootstrap (tenantId, daftar bahasa), sehingga saat bootstrap gagal tidak ada katalog yang dapat dimuat.
export const BOOT_DICTIONARY = {
  id: {
    title: "Aplikasi Tidak Dapat Dimuat",
    description: "Periksa koneksi Anda, lalu coba lagi.",
    retry: "Coba Lagi",
    loading: "Memuat…",
  },
  en: {
    title: "The Application Could Not Be Loaded",
    description: "Check your connection and try again.",
    retry: "Try Again",
    loading: "Loading…",
  },
} as const;

/** Memilih kamus bawaan dari `navigator.languages`: "id*" memakai Indonesia, bahasa lain Inggris, kosong Indonesia. */
export function pickBootLanguage(languages: readonly string[]): "id" | "en" {
  const first = languages.find((l) => l.trim() !== "");
  if (first === undefined) return "id";
  return first.toLowerCase().startsWith("id") ? "id" : "en";
}

function bootLanguage(): "id" | "en" {
  return pickBootLanguage(typeof navigator === "undefined" ? [] : (navigator.languages ?? []));
}

/** Kerangka saat bootstrap dan katalog awal dimuat. */
export function BootLoading() {
  const words = BOOT_DICTIONARY[bootLanguage()];
  return (
    <div
      role="status"
      aria-busy="true"
      className="flex min-h-svh items-center justify-center bg-background text-muted-foreground"
    >
      <LoadingIcon aria-label={words.loading} className="size-8" />
    </div>
  );
}

/** Layar galat bootstrap dengan tombol "Coba Lagi" (kamus bawaan id/en). */
export function BootErrorScreen({ onRetry }: { onRetry: () => void }) {
  const words = BOOT_DICTIONARY[bootLanguage()];
  return (
    <div className="min-h-svh bg-background">
      <ErrorPage
        status={503}
        title={words.title}
        description={words.description}
        actions={
          <Button type="button" onClick={onRetry}>
            {words.retry}
          </Button>
        }
      />
    </div>
  );
}

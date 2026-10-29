import { Message, msg } from "@erpora/platform-web";
import "../i18n/keys.gen";

/** Halaman awal modul `inventory`; ganti dengan halaman Anda (teks selalu dari katalog i18n modul). */
export function HomePage() {
  return (
    <div className="flex flex-col gap-4 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">
          <Message message={msg("inventory", "page.home.title")} />
        </h1>
        <p className="text-sm text-muted-foreground">
          <Message message={msg("inventory", "page.home.description")} />
        </p>
      </header>
    </div>
  );
}

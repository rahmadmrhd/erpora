import { Message, msg } from "@erpora/platform-web";
import "../i18n/keys.gen";

/**
 * Halaman contoh modul `inventory` (`/inventory/items`); ganti dengan halaman Anda (teks selalu dari katalog i18n
 * modul). Tanpa judul halaman: judul visual dan satu-satunya judul level 1 adalah breadcrumb terakhir dari `crumb` rute (K9).
 */
export function ItemsPage() {
  return (
    <div className="flex flex-col gap-4 p-6">
      <p className="text-sm text-muted-foreground">
        <Message message={msg("inventory", "page.items.description")} />
      </p>
    </div>
  );
}

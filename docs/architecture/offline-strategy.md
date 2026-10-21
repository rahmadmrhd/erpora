# Strategi Offline & Sinkronisasi

| Atribut | Nilai |
| --- | --- |
| Status | **Draft desain** (diekstrak dari dokumen awal `arsitektur.md` §5–§6). Akan diformalkan di **PRD Offline & Sync** |
| Terkait | PRD-000 FR-06 (`offline` di manifest), FR-09 (`OfflinePolicy` UI), FR-10 (PWA), FR-11 (`OutboxOperation`); api-guidelines §6–§7 |

## 1. Prinsip

**Offline diatur secara eksplisit.** Kemampuan offline bukan otomatis untuk setiap modul; setiap modul mendeklarasikan kebijakannya, dan backend tetap menjadi otoritas.

**Izin melihat data saat online ≠ izin menyimpan salinannya di browser.** Data karyawan, keuangan, atau pelanggan dapat membutuhkan kebijakan retensi dan keamanan tersendiri sebelum boleh disimpan di IndexedDB.

## 2. Empat mode offline

| Mode | Perilaku | Contoh |
| --- | --- | --- |
| `online-only` | Butuh koneksi untuk menampilkan data dan menjalankan operasi | Approval finansial yang harus selalu tervalidasi server |
| `offline-read` | UI dan data yang pernah disimpan dapat dibaca tanpa internet | Katalog produk, laporan historis yang diizinkan |
| `offline-draft` | Draft dibuat lokal; transaksi baru sah setelah dikirim dan divalidasi server | Purchase Request, draft Sales Order |
| `offline-write` | Operasi lokal masuk antrean sinkronisasi dengan aturan konflik dan idempotensi | Pencatatan aktivitas lapangan nonkritis |

### Contoh kebijakan per modul

| Modul | UI offline | Data offline | Operasi offline |
| --- | --- | --- | --- |
| Dashboard | Ya | Cache terakhir | Tidak |
| Inventory | Ya | Produk, gudang sesuai scope | Draft penyesuaian |
| CRM | Ya | Kontak yang diizinkan | Catatan aktivitas |
| Accounting | Ya | Data tertentu | Draft jurnal saja |
| HRM | Ya | Data terbatas | Bergantung kebijakan |
| Settings | Ya | Konfigurasi UI aman | Umumnya online |

## 3. Deskriptor kebijakan (`OfflinePolicy`)

Kontrak rancangan platform (bukan fitur bawaan TanStack). Sisi backend (`manifest.offline`) adalah otoritas; sisi frontend (`@erp/module-sdk`) memakai bentuk yang sama untuk UX.

```ts
type OfflinePolicy = {
  mode: "online-only" | "offline-read" | "offline-draft" | "offline-write";
  assets: { strategy: "on-demand" | "precache" };
  data: {
    collections: string[];      // resource yang boleh disalin ke perangkat
    maxAgeSeconds: number;      // umur maksimum salinan sebelum wajib diperbarui
    maxRecords?: number;        // batas jumlah record per collection
  };
  sync: {
    enabled: boolean;
    conflict: "server-wins" | "reject" | "custom";
  };
};

// Contoh: Inventory
const inventoryOffline: OfflinePolicy = {
  mode: "offline-draft",
  assets: { strategy: "on-demand" },
  data: { collections: ["products", "warehouses"], maxAgeSeconds: 86400, maxRecords: 5000 },
  sync: { enabled: true, conflict: "reject" },
};
```

Workbox dipakai dengan strategi berbeda untuk aset (precache app shell, on-demand untuk chunk modul) dan data.

## 4. Transactional outbox di klien

Operasi offline **tidak** ditulis langsung ke IndexedDB lalu dianggap tersimpan di server. Setiap operasi dicatat sebagai entri outbox:

```json
{
  "operationId": "uuid-unik",
  "module": "inventory",
  "action": "create-adjustment-draft",
  "tenantId": "…",
  "entityId": "draft-123",
  "baseVersion": 4,
  "payload": { "warehouseId": "WH-01", "productId": "PRD-001", "quantity": "10" },
  "status": "pending"
}
```

Ketika koneksi tersedia:

1. Klien mengirim operasi dengan `operationId` sebagai **`Idempotency-Key`** dan `baseVersion` sebagai **`If-Match`** (api-guidelines §7).
2. Server memvalidasi sesi, tenant, permission, dan aturan bisnis **saat itu juga** (bukan saat operasi dibuat).
3. Server memeriksa apakah operasi sudah pernah diproses dan apakah versi data masih sesuai.
4. Satu transaksi database menyimpan hasil dan status idempotensi secara atomik.
5. Klien menerima hasil, lalu memperbarui IndexedDB.

Operasi yang gagal validasi **tidak diulang tanpa batas**: tampilkan status konflik dan minta pengguna memperbaiki draft.

TanStack Query mendukung paused mutations dan persistence, tetapi fungsi mutation harus diregistrasi ulang dengan benar setelah reload. Untuk transaksi bisnis yang butuh audit dan idempotensi kuat, **outbox eksplisit di IndexedDB** lebih dipilih.

## 5. Bootstrap saat offline

Frontend boleh memakai **snapshot bootstrap terakhir** untuk menampilkan UI sesuai kebijakan, tetapi snapshot itu **bukan otorisasi** untuk operasi server. Hak akses divalidasi ulang ketika koneksi kembali. `revision` pada bootstrap dipakai untuk invalidasi saat role atau aktivasi modul berubah.

## 6. Pertanyaan untuk PRD Offline & Sync

- Enkripsi data IndexedDB dan penghapusan saat logout/pergantian user.
- Kebijakan retensi per resource (berkaitan dengan field group sensitif PRD-001).
- Bentuk endpoint sync (cursor per collection, api-guidelines §6) dan penanganan record yang dihapus (tombstone).
- Resolusi konflik `custom` per modul.

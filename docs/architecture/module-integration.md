# Integrasi Antarmodul

| Atribut | Nilai |
| --- | --- |
| Status | **Draft desain** (diekstrak dari dokumen awal `Blueprint.md` §10). Akan diformalkan di **PRD Events & Jobs** |
| Terkait | PRD-000 §4.2 (D1, D8), FR-05 (`contract/`), FR-17.4 (propagasi trace) |

## 1. Prinsip

Modul tidak mengubah tabel milik modul lain secara langsung. Buat **kontrak yang menyatakan maksud bisnis** di `modules/<x>/contract`, dan foreign key hanya dipakai di dalam batas kepemilikan data yang jelas. Data milik modul lain diakses lewat kontrak atau referensi dokumen yang stabil.

## 2. Contoh: Purchasing menerima barang ke Inventory

**Application port** (`modules/inventory/contract`):

```go
type ReceiveStock interface {
    Execute(ctx context.Context, cmd ReceiveStockCommand) (ReceiveStockResult, error)
}

type ReceiveStockCommand struct {
    GoodsReceiptRef string        // referensi dokumen sumber (unik → idempotensi)
    Lines           []ReceiveLine // item, gudang, kuantitas (desimal), satuan
}
```

- Tenant, perusahaan, dan actor diambil dari **context tervalidasi**, bukan dari payload (`tenant_id` di payload mentah tidak dipercaya).
- Implementasi kontrak **menegakkan otorisasinya sendiri** terhadap principal asli di context (`authz.Check`/`authz.Filter`); hak sistem hanya lewat `authz.AsSystem(ctx, reason)` yang teraudit (PRD-001 FR-119, pencegahan IDOR/confused deputy).

**Use case Inventory** (sketsa):

```go
func (h *ReceiveStockHandler) Execute(ctx context.Context, cmd contract.ReceiveStockCommand) (contract.ReceiveStockResult, error) {
    if err := h.policy.AssertCanReceive(ctx, cmd); err != nil { return contract.ReceiveStockResult{}, err }
    db, err := data.DB(ctx)
    if err != nil { return contract.ReceiveStockResult{}, err }
    var res contract.ReceiveStockResult
    err = db.Transaction(func(tx *gorm.DB) error {
        // validasi idempotensi & kunci record terkait (FOR UPDATE)
        // terapkan aturan inventory
        // simpan stock ledger & status receipt
        // catat event outbox DI TRANSAKSI YANG SAMA
        return nil
    })
    return res, err
}
```

Implementasi produksi wajib menangani receipt bersamaan, reservasi stok, validasi kuantitas, referensi sumber unik, idempotensi, dan penulisan outbox dalam transaksi yang sama.

## 3. Kapan sinkron, kapan event?

| Kondisi | Mekanisme |
| --- | --- |
| Goods Receipt harus ditolak bila aturan stok tidak terpenuhi | Panggilan sinkron lewat `contract/` |
| Mengirim notifikasi setelah penerimaan barang | Event / listener asinkron |
| Memperbarui dashboard statistik | Event + read projection |
| Integrasi warehouse eksternal | Integration event atau API |
| Posting jurnal harus berhasil bersama transaksi tertentu | Desain transaksi eksplisit, **bukan** event fire-and-forget |

Event bukan pengganti semua pemanggilan method: gunakan kontrak langsung ketika pemanggil butuh hasil atau kegagalan segera.

## 4. Transactional outbox (server)

Tabel `outbox_messages` di tenant database. Event disimpan **dalam transaksi bisnis yang sama**, lalu worker mempublikasikannya dan menandai status pengiriman. Konsumen **wajib idempoten** karena pesan bisa terkirim lebih dari sekali. Trace context ikut disimpan (PRD-000 FR-17.4) agar alur asinkron tetap tersambung di tracing.

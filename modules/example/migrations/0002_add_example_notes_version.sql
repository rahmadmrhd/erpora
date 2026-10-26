-- +goose Up
-- version: optimistic concurrency (api-guidelines §4.3/§7.1); baris lama mulai dari 1.
-- updated_at: baris lama diisi created_at agar konsisten (bukan waktu migrasi).
ALTER TABLE example_notes
    ADD COLUMN version    bigint      NOT NULL DEFAULT 1 CHECK (version >= 1),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

-- Catatan: backfill satu UPDATE ini hanya pantas untuk tabel kecil (mengunci dan menulis ulang semua
-- baris dalam satu transaksi). Untuk tabel besar, backfill secara bertahap (per batch) di migrasi
-- tanpa transaksi (anotasi goose NO TRANSACTION) agar tidak menahan kunci lama.
-- Jangan menulis penanda anotasi goose (tanda plus diikuti kata goose) di komentar: goose mem-parse-nya.
UPDATE example_notes SET updated_at = created_at;

-- +goose Down
ALTER TABLE example_notes
    DROP COLUMN updated_at,
    DROP COLUMN version;

-- +goose Up
-- created_by dan updated_by: jejak audit pengguna (sdk:audit). Kolom nullable (expand, aditif): baris lama, seed,
-- dan tindakan sistem tanpa principal bernilai NULL; tidak ada backfill. Tanpa foreign key (id pengguna milik
-- modul lain; relasi lintas modul hanya menyimpan id).
ALTER TABLE example_notes
    ADD COLUMN created_by uuid,
    ADD COLUMN updated_by uuid;

-- +goose Down
-- Menghapus kolom menghilangkan jejak pengguna yang sudah tercatat (tidak dapat dibalik tanpa kehilangan data).
ALTER TABLE example_notes
    DROP COLUMN updated_by,
    DROP COLUMN created_by;

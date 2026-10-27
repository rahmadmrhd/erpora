-- +goose Up
-- Migrasi modul example: add_tags. Tanpa tenant_id: isolasi tenant ditangani lapisan database (ADR-004).
-- Ubah skema secara aditif (expand, migrate, contract); jangan menulis penanda anotasi goose di komentar.

-- +goose Down
-- Balikkan perubahan Up di atas.

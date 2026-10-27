-- +goose Up
ALTER TABLE fx_items ADD COLUMN note text NULL;
ALTER TABLE fx_items ADD COLUMN fixed text NULL;

-- +goose Down
ALTER TABLE fx_items DROP COLUMN fixed;
ALTER TABLE fx_items DROP COLUMN note;

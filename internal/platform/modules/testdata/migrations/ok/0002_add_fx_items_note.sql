-- +goose Up
ALTER TABLE fx_items ADD COLUMN note text NULL;

-- +goose Down
ALTER TABLE fx_items DROP COLUMN note;

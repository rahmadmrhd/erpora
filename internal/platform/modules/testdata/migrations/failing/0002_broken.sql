-- +goose Up
ALTER TABLE fx_items ADD COLUMN note text NULL;
ALTER TABLE fx_items ADD COLUMN broken nonexistent_type;

-- +goose Down
ALTER TABLE fx_items DROP COLUMN note;

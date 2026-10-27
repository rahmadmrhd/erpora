-- +goose Up
ALTER TABLE example_x ADD COLUMN note text;

-- +goose Down
ALTER TABLE example_x DROP COLUMN note;

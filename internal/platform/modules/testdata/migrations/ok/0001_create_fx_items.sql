-- +goose Up
CREATE TABLE fx_items (
    id   uuid PRIMARY KEY,
    name text NOT NULL
);

-- +goose Down
DROP TABLE fx_items;

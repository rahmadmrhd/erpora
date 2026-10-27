-- +goose Up
CREATE TABLE fx_items (
    id   uuid PRIMARY KEY,
    name text NOT NULL
);
CREATE TABLE fx_counter (n int NOT NULL);
INSERT INTO fx_counter (n) VALUES (1);

-- +goose Down
DROP TABLE fx_counter;
DROP TABLE fx_items;

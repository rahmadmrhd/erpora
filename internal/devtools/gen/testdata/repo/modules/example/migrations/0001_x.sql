-- +goose Up
CREATE TABLE example_x (id uuid PRIMARY KEY);

-- +goose Down
DROP TABLE example_x;

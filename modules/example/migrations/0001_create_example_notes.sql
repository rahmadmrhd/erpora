-- +goose Up
-- Tanpa tenant_id: isolasi tenant ditangani lapisan database (ADR-004).
CREATE TABLE example_notes (
    id         uuid PRIMARY KEY,
    title      text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX example_notes_created_at_idx ON example_notes (created_at);

-- +goose Down
DROP TABLE example_notes;

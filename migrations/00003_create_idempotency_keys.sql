-- +goose Up
CREATE TABLE idempotency_keys (
    key          UUID PRIMARY KEY,
    request_hash TEXT NOT NULL,
    trip_id      UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE idempotency_keys;

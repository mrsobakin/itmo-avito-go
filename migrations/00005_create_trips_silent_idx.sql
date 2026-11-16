-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY trips_active_silent_since_idx
    ON trips ((GREATEST(last_position_at, started_at)), id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY trips_active_silent_since_idx;

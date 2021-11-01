-- +goose Up
-- +goose StatementBegin
DROP INDEX proxy_requests_timestamp_idx;

CREATE INDEX proxy_requests_truncated_timestamp_idx
ON proxy_requests (date_trunc('minute', timestamp));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX proxy_requests_truncated_timestamp_idx;

CREATE INDEX proxy_requests_timestamp_idx
ON proxy_requests (timestamp);
-- +goose StatementEnd

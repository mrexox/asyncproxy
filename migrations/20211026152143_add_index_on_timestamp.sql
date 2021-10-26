-- +goose Up
-- +goose StatementBegin
CREATE INDEX proxy_requests_timestamp_idx
ON proxy_requests (timestamp);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX proxy_requests_timestamp_idx;
-- +goose StatementEnd

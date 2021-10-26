-- +goose Up
-- +goose StatementBegin
ALTER TABLE proxy_requests DROP COLUMN processed;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE proxy_requests ADD COLUMN processed boolean;
-- +goose StatementEnd

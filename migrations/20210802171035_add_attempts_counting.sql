-- +goose Up
-- +goose StatementBegin
ALTER TABLE proxy_requests ADD COLUMN attempt SMALLINT NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE proxy_requests DROP COLUMN attempt;
-- +goose StatementEnd

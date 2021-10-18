-- TODO: Here the column becomes unused. Drop 'processed' column next release.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE proxy_requests ALTER COLUMN processed DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE proxy_requests ALTER COLUMN processed SET NOT NULL;
-- +goose StatementEnd

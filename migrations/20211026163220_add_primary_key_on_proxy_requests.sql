-- +goose Up
-- +goose StatementBegin
ALTER TABLE proxy_requests ADD PRIMARY KEY (id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE proxy_requests DROP CONSTRAINT proxy_requests_pkey;
-- +goose StatementEnd

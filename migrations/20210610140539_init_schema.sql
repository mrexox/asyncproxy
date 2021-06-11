-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS proxy_requests (
 id varchar NOT NULL,
 timestamp timestamp without time zone NOT NULL,
 processed boolean NOT NULL,
 method varchar NOT NULL,
 header varchar NOT NULL,
 body text,
 origin_url varchar NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE proxy_requests;
-- +goose StatementEnd

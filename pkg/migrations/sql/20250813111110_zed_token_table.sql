-- +goose Up
-- +goose StatementBegin
CREATE TABLE zed_token (
    id SMALLINT PRIMARY KEY,
    token TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE zed_token;
-- +goose StatementEnd

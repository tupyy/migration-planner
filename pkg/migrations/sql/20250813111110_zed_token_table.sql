-- +goose Up
-- +goose StatementBegin
CREATE TABLE zed_token (
    token TEXT
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE zed_token;
-- +goose StatementEnd

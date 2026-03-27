-- +goose Up
-- +goose StatementBegin
CREATE TABLE organizations (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP,
    name TEXT NOT NULL,
    description TEXT,
    kind VARCHAR(50) NOT NULL,
    icon BYTEA,
    company VARCHAR(200) NOT NULL,
    UNIQUE (company, name),
    parent_id VARCHAR(255) REFERENCES organizations(id)
);

CREATE TABLE users (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    phone VARCHAR(50) NOT NULL,
    location VARCHAR(10) NOT NULL,
    title VARCHAR(200),
    bio TEXT,
    organization_id VARCHAR(255) NOT NULL REFERENCES organizations(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
DROP TABLE organizations;
-- +goose StatementEnd

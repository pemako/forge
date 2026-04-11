-- +goose Up
CREATE TABLE test_users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);

-- +goose Down
DROP TABLE test_users;

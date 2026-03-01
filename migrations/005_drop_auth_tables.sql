-- +goose Up
DROP TABLE IF EXISTS user_secrets;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

-- +goose Down
-- Auth tables are not recreated on rollback.

DROP TABLE IF EXISTS user_secrets;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

---- create above / drop below ----
-- Auth tables are not recreated on rollback.

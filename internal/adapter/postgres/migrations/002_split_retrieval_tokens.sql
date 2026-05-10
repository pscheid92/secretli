ALTER TABLE secrets RENAME COLUMN retrieval_token TO metadata_token;
ALTER TABLE secrets ADD COLUMN blob_token TEXT NOT NULL DEFAULT '';
UPDATE secrets SET blob_token = metadata_token WHERE blob_token = '';
ALTER TABLE secrets ALTER COLUMN blob_token DROP DEFAULT;

---- create above / drop below ----
ALTER TABLE secrets DROP COLUMN blob_token;
ALTER TABLE secrets RENAME COLUMN metadata_token TO retrieval_token;

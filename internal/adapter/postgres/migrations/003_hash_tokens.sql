ALTER TABLE secrets RENAME COLUMN metadata_token TO metadata_token_hash;
ALTER TABLE secrets RENAME COLUMN blob_token TO blob_token_hash;
ALTER TABLE secrets RENAME COLUMN deletion_token TO deletion_token_hash;

UPDATE secrets
SET metadata_token_hash = encode(sha256(convert_to(metadata_token_hash, 'UTF8')), 'hex'),
    blob_token_hash = encode(sha256(convert_to(blob_token_hash, 'UTF8')), 'hex'),
    deletion_token_hash = encode(sha256(convert_to(deletion_token_hash, 'UTF8')), 'hex');

ALTER TABLE secrets ADD CONSTRAINT secrets_metadata_token_hash_length CHECK (length(metadata_token_hash) = 64);
ALTER TABLE secrets ADD CONSTRAINT secrets_blob_token_hash_length CHECK (length(blob_token_hash) = 64);
ALTER TABLE secrets ADD CONSTRAINT secrets_deletion_token_hash_length CHECK (length(deletion_token_hash) = 64);

---- create above / drop below ----
-- Raw token values are not recoverable; downgrade keeps the hash values under the legacy names.
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_deletion_token_hash_length;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_blob_token_hash_length;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_metadata_token_hash_length;

ALTER TABLE secrets RENAME COLUMN deletion_token_hash TO deletion_token;
ALTER TABLE secrets RENAME COLUMN blob_token_hash TO blob_token;
ALTER TABLE secrets RENAME COLUMN metadata_token_hash TO metadata_token;

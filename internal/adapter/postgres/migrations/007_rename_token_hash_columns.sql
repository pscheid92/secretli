ALTER TABLE secrets RENAME COLUMN retrieval_token_hash TO retrieval_token;
ALTER TABLE secrets RENAME COLUMN deletion_token_hash TO deletion_token;

---- create above / drop below ----
ALTER TABLE secrets RENAME COLUMN retrieval_token TO retrieval_token_hash;
ALTER TABLE secrets RENAME COLUMN deletion_token TO deletion_token_hash;

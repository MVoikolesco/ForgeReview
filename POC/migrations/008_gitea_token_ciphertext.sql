ALTER TABLE gitea_instances
ADD COLUMN token_ciphertext TEXT NOT NULL DEFAULT '';

-- Token hashes carry an explicit algorithm version ("sha256:<hex>") so the
-- hashing scheme can be replaced in the future without invalidating existing
-- tokens. Existing bare-hex hashes adopt the prefix; they are the same
-- SHA-256 digests.
UPDATE api_tokens
SET token_hash = 'sha256:' || token_hash
WHERE token_hash NOT LIKE 'sha256:%';

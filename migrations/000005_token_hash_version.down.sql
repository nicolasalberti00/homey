UPDATE api_tokens
SET token_hash = substr(token_hash, 8)
WHERE token_hash LIKE 'sha256:%';

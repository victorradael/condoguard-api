-- name: InsertRefreshToken :exec
INSERT INTO refresh_tokens (condominium_id, user_id, family_id, token_hash, expires_at)
VALUES (@condominium_id, @user_id, @family_id, @token_hash, @expires_at);

-- ConsumeRefreshToken revoga o token se ele ainda vale e devolve a sessão.
-- É atômico: dois pedidos com o mesmo token não conseguem ambos trocá-lo.
-- name: ConsumeRefreshToken :one
UPDATE refresh_tokens
SET revoked_at = now()
WHERE condominium_id = @condominium_id
  AND token_hash = @token_hash
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING user_id, family_id;

-- name: GetRefreshTokenFamily :one
SELECT family_id
FROM refresh_tokens
WHERE condominium_id = @condominium_id AND token_hash = @token_hash;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE condominium_id = @condominium_id AND family_id = @family_id AND revoked_at IS NULL;

-- name: RevokeUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE condominium_id = @condominium_id AND user_id = @user_id AND revoked_at IS NULL;

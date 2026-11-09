-- Todas as consultas recebem condominium_id: nenhuma leitura ou escrita sem tenant.
-- O e-mail chega já normalizado (minúsculo, sem espaços) e é único na base toda.

-- name: InsertUser :one
INSERT INTO users (condominium_id, username, email, password_hash, roles)
VALUES (@condominium_id, @username, @email, @password_hash, @roles)
RETURNING id;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE condominium_id = @condominium_id AND id = @id;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE condominium_id = @condominium_id AND email = @email;

-- name: ListUsers :many
SELECT *
FROM users
WHERE condominium_id = @condominium_id
ORDER BY created_at, id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUsers :one
SELECT count(*)
FROM users
WHERE condominium_id = @condominium_id;

-- name: UpdateUser :execrows
UPDATE users
SET username = @username, email = @email, password_hash = @password_hash, roles = @roles
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteUser :execrows
DELETE FROM users
WHERE condominium_id = @condominium_id AND id = @id;

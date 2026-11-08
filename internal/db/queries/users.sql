-- Todas as consultas recebem condominium_id: nenhuma leitura ou escrita sem tenant.

-- name: InsertUser :one
INSERT INTO users (condominium_id, username, email, password_hash, roles)
VALUES (@condominium_id, @username, @email, @password_hash, @roles)
RETURNING id;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE condominium_id = @condominium_id AND id = @id;

-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE condominium_id = @condominium_id AND username = @username
ORDER BY created_at, id
LIMIT 1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE condominium_id = @condominium_id AND email = @email;

-- name: ListUsers :many
SELECT *
FROM users
WHERE condominium_id = @condominium_id
ORDER BY created_at, id;

-- name: UpdateUser :execrows
UPDATE users
SET username = @username, email = @email, password_hash = @password_hash, roles = @roles
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteUser :execrows
DELETE FROM users
WHERE condominium_id = @condominium_id AND id = @id;

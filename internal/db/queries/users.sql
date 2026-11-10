-- A conta (users) é global; o papel é do vínculo (memberships) com o condomínio.
-- Todas as consultas recebem condominium_id: nenhuma leitura ou escrita sem tenant.
-- O e-mail chega já normalizado (minúsculo, sem espaços) e é único na base toda.

-- O id é gerado pela API: o RETURNING exigiria ler a linha antes de ela ter vínculo.
-- name: InsertUser :exec
INSERT INTO users (id, username, email, password_hash)
VALUES (@id, @username, @email, @password_hash);

-- name: InsertMembership :exec
INSERT INTO memberships (user_id, condominium_id, roles)
VALUES (@user_id, @condominium_id, @roles);

-- name: GetUserByID :one
SELECT u.id, u.username, u.email, u.password_hash, u.created_at, m.condominium_id, m.roles
FROM users u
JOIN memberships m ON m.user_id = u.id
WHERE m.condominium_id = @condominium_id AND u.id = @id;

-- name: ListUsers :many
SELECT u.id, u.username, u.email, u.password_hash, u.created_at, m.condominium_id, m.roles
FROM users u
JOIN memberships m ON m.user_id = u.id
WHERE m.condominium_id = @condominium_id
ORDER BY u.created_at, u.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUsers :one
SELECT count(*)
FROM memberships
WHERE condominium_id = @condominium_id;

-- name: UpdateUser :execrows
UPDATE users u
SET username = @username, email = @email, password_hash = @password_hash
WHERE u.id = @id
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.condominium_id = @condominium_id);

-- name: UpdateMembershipRoles :execrows
UPDATE memberships
SET roles = @roles
WHERE condominium_id = @condominium_id AND user_id = @user_id;

-- Remove o vínculo do usuário com o condomínio atual (e a conta, se era o último).
-- name: RemoveMembership :one
SELECT remove_membership(@user_id)::int AS removed;

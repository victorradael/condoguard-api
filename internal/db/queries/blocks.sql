-- name: InsertBlock :one
INSERT INTO blocks (id, condominium_id, name, is_demo)
VALUES (@id, @condominium_id, @name, @is_demo)
RETURNING id;

-- name: GetBlock :one
SELECT b.id, b.name, b.created_at, (SELECT count(*) FROM residents r WHERE r.condominium_id = b.condominium_id AND r.block_id = b.id) AS unit_count
FROM blocks b
WHERE b.condominium_id = @condominium_id AND b.id = @id;

-- name: ListBlocks :many
SELECT b.id, b.name, b.created_at, (SELECT count(*) FROM residents r WHERE r.condominium_id = b.condominium_id AND r.block_id = b.id) AS unit_count
FROM blocks b
WHERE b.condominium_id = @condominium_id
ORDER BY b.name, b.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBlocks :one
SELECT count(*) FROM blocks WHERE condominium_id = @condominium_id;

-- name: UpdateBlock :execrows
UPDATE blocks SET name = @name WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteBlock :execrows
DELETE FROM blocks WHERE condominium_id = @condominium_id AND id = @id;

-- name: FindBlockByName :one
SELECT id FROM blocks WHERE condominium_id = @condominium_id AND name = @name;

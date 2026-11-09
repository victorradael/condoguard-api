-- name: InsertShopOwner :one
INSERT INTO shop_owners (condominium_id, shop_name, cnpj, floor, owner_id)
VALUES (@condominium_id, @shop_name, @cnpj, @floor, @owner_id)
RETURNING id;

-- name: GetShopOwnerByID :one
SELECT *
FROM shop_owners
WHERE condominium_id = @condominium_id AND id = @id;

-- owner_id NULL lista todas as lojas do condomínio (administração).
-- name: ListShopOwners :many
SELECT *
FROM shop_owners
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(owner_id)::text IS NULL OR owner_id = sqlc.narg(owner_id)::text)
ORDER BY created_at, id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountShopOwners :one
SELECT count(*)
FROM shop_owners
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(owner_id)::text IS NULL OR owner_id = sqlc.narg(owner_id)::text);

-- name: ListShopOwnerIDsByOwner :many
SELECT id
FROM shop_owners
WHERE condominium_id = @condominium_id AND owner_id = @owner_id;

-- name: UpdateShopOwner :execrows
UPDATE shop_owners
SET shop_name = @shop_name, cnpj = @cnpj, floor = @floor, owner_id = @owner_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteShopOwner :execrows
DELETE FROM shop_owners
WHERE condominium_id = @condominium_id AND id = @id;

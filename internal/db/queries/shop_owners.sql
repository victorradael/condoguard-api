-- name: InsertShopOwner :one
INSERT INTO shop_owners (condominium_id, shop_name, cnpj, floor, owner_id)
VALUES (@condominium_id, @shop_name, @cnpj, @floor, @owner_id)
RETURNING id;

-- name: GetShopOwnerByID :one
SELECT *
FROM shop_owners
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ListShopOwners :many
SELECT *
FROM shop_owners
WHERE condominium_id = @condominium_id
ORDER BY created_at, id;

-- name: UpdateShopOwner :execrows
UPDATE shop_owners
SET shop_name = @shop_name, cnpj = @cnpj, floor = @floor, owner_id = @owner_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteShopOwner :execrows
DELETE FROM shop_owners
WHERE condominium_id = @condominium_id AND id = @id;

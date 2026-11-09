-- name: InsertResident :one
INSERT INTO residents (condominium_id, condominium_ref, unit_number, floor, owner_id)
VALUES (@condominium_id, @condominium_ref, @unit_number, @floor, @owner_id)
RETURNING id;

-- name: GetResidentByID :one
SELECT *
FROM residents
WHERE condominium_id = @condominium_id AND id = @id;

-- owner_id NULL lista todas as unidades do condomínio (administração).
-- name: ListResidents :many
SELECT *
FROM residents
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(owner_id)::text IS NULL OR owner_id = sqlc.narg(owner_id)::text)
ORDER BY created_at, id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountResidents :one
SELECT count(*)
FROM residents
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(owner_id)::text IS NULL OR owner_id = sqlc.narg(owner_id)::text);

-- name: ListResidentIDsByOwner :many
SELECT id
FROM residents
WHERE condominium_id = @condominium_id AND owner_id = @owner_id;

-- name: UpdateResident :execrows
UPDATE residents
SET condominium_ref = @condominium_ref, unit_number = @unit_number, floor = @floor, owner_id = @owner_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteResident :execrows
DELETE FROM residents
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ResidentUnitExists :one
SELECT EXISTS (
    SELECT 1
    FROM residents
    WHERE condominium_id = @condominium_id
      AND condominium_ref = @condominium_ref
      AND unit_number = @unit_number
      AND (sqlc.narg(exclude_id)::uuid IS NULL OR id <> sqlc.narg(exclude_id)::uuid)
);

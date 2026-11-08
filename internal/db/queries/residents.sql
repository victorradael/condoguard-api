-- name: InsertResident :one
INSERT INTO residents (condominium_id, condominium_ref, unit_number, floor, owner_id)
VALUES (@condominium_id, @condominium_ref, @unit_number, @floor, @owner_id)
RETURNING id;

-- name: GetResidentByID :one
SELECT *
FROM residents
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ListResidents :many
SELECT *
FROM residents
WHERE condominium_id = @condominium_id
ORDER BY created_at, id;

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

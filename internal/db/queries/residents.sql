-- A unidade (tabela residents) e o seu bloco. Quem não vê tudo (viewer) enxerga as
-- unidades em que consta como responsável antigo (owner_id) ou tem vínculo vigente que
-- dá acesso aos dados da unidade (todos menos dependente).

-- name: InsertResident :one
INSERT INTO residents (id, condominium_id, condominium_ref, unit_number, floor, owner_id, block_id, is_demo)
VALUES (@id, @condominium_id, @condominium_ref, @unit_number, @floor, @owner_id, @block_id, @is_demo)
RETURNING id;

-- name: GetResidentByID :one
SELECT r.id, r.condominium_ref, r.unit_number, r.floor, r.owner_id, r.block_id, b.name AS block_name
FROM residents r
LEFT JOIN blocks b ON b.condominium_id = r.condominium_id AND b.id = r.block_id
WHERE r.condominium_id = @condominium_id AND r.id = @id;

-- viewer NULL lista todas as unidades do condomínio (administração).
-- name: ListResidents :many
SELECT r.id, r.condominium_ref, r.unit_number, r.floor, r.owner_id, r.block_id, b.name AS block_name
FROM residents r
LEFT JOIN blocks b ON b.condominium_id = r.condominium_id AND b.id = r.block_id
WHERE r.condominium_id = @condominium_id
  AND (sqlc.narg(viewer)::text IS NULL
       OR r.owner_id = sqlc.narg(viewer)::text
       OR EXISTS (SELECT 1 FROM unit_links l JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
                  WHERE l.condominium_id = r.condominium_id AND l.unit_id = r.id AND p.user_id::text = sqlc.narg(viewer)::text
                    AND l.kind <> 'dependente' AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))
  AND (sqlc.narg(block_id)::uuid IS NULL OR r.block_id = sqlc.narg(block_id)::uuid)
ORDER BY (CASE WHEN @by_number::boolean THEN b.name END) NULLS FIRST,
         (CASE WHEN @by_number::boolean THEN length(r.unit_number) END),
         (CASE WHEN @by_number::boolean THEN r.unit_number END),
         r.created_at, r.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountResidents :one
SELECT count(*)
FROM residents r
WHERE r.condominium_id = @condominium_id
  AND (sqlc.narg(viewer)::text IS NULL
       OR r.owner_id = sqlc.narg(viewer)::text
       OR EXISTS (SELECT 1 FROM unit_links l JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
                  WHERE l.condominium_id = r.condominium_id AND l.unit_id = r.id AND p.user_id::text = sqlc.narg(viewer)::text
                    AND l.kind <> 'dependente' AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))
  AND (sqlc.narg(block_id)::uuid IS NULL OR r.block_id = sqlc.narg(block_id)::uuid);

-- name: ListResidentIDsByViewer :many
SELECT r.id
FROM residents r
WHERE r.condominium_id = @condominium_id
  AND (r.owner_id = @viewer::text
       OR EXISTS (SELECT 1 FROM unit_links l JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
                  WHERE l.condominium_id = r.condominium_id AND l.unit_id = r.id AND p.user_id::text = @viewer::text
                    AND l.kind <> 'dependente' AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())));

-- name: UpdateResident :execrows
UPDATE residents
SET condominium_ref = @condominium_ref, unit_number = @unit_number, floor = @floor, owner_id = @owner_id, block_id = @block_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteResident :execrows
DELETE FROM residents
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ResidentUnitExists :one
SELECT EXISTS (
    SELECT 1
    FROM residents
    WHERE condominium_id = @condominium_id
      AND block_id IS NOT DISTINCT FROM sqlc.narg(block_id)::uuid
      AND unit_number = @unit_number
      AND (sqlc.narg(exclude_id)::uuid IS NULL OR id <> sqlc.narg(exclude_id)::uuid)
);

-- name: FindResidentByNumber :one
SELECT id FROM residents
WHERE condominium_id = @condominium_id
  AND block_id IS NOT DISTINCT FROM sqlc.narg(block_id)::uuid
  AND unit_number = @unit_number;

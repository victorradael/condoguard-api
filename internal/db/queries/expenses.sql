-- name: InsertExpense :one
INSERT INTO expenses (condominium_id, description, amount_cents, due_date, resident_id, shop_owner_id)
VALUES (@condominium_id, @description, @amount_cents, @due_date, @resident_id, @shop_owner_id)
RETURNING id;

-- name: GetExpenseByID :one
SELECT *
FROM expenses
WHERE condominium_id = @condominium_id AND id = @id;

-- restricted = false lista tudo (administração); true só as despesas das
-- unidades e lojas informadas (as do próprio usuário).
-- name: ListExpenses :many
SELECT *
FROM expenses
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR due_date >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR due_date <= sqlc.narg(due_to)::timestamptz)
  AND (NOT @restricted::boolean
       OR resident_id = ANY(@resident_ids::text[])
       OR shop_owner_id = ANY(@shop_owner_ids::text[]))
ORDER BY due_date, id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountExpenses :one
SELECT count(*)
FROM expenses
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR due_date >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR due_date <= sqlc.narg(due_to)::timestamptz)
  AND (NOT @restricted::boolean
       OR resident_id = ANY(@resident_ids::text[])
       OR shop_owner_id = ANY(@shop_owner_ids::text[]));

-- name: UpdateExpense :execrows
UPDATE expenses
SET description = @description, amount_cents = @amount_cents, due_date = @due_date,
    resident_id = @resident_id, shop_owner_id = @shop_owner_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteExpense :execrows
DELETE FROM expenses
WHERE condominium_id = @condominium_id AND id = @id;

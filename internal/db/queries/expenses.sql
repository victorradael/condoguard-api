-- name: InsertExpense :one
INSERT INTO expenses (condominium_id, description, amount_cents, due_date, resident_id, shop_owner_id)
VALUES (@condominium_id, @description, @amount_cents, @due_date, @resident_id, @shop_owner_id)
RETURNING id;

-- name: GetExpenseByID :one
SELECT *
FROM expenses
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ListExpenses :many
SELECT *
FROM expenses
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(due_from)::timestamptz IS NULL OR due_date >= sqlc.narg(due_from)::timestamptz)
  AND (sqlc.narg(due_to)::timestamptz IS NULL OR due_date <= sqlc.narg(due_to)::timestamptz)
ORDER BY due_date, id;

-- name: UpdateExpense :execrows
UPDATE expenses
SET description = @description, amount_cents = @amount_cents, due_date = @due_date,
    resident_id = @resident_id, shop_owner_id = @shop_owner_id
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteExpense :execrows
DELETE FROM expenses
WHERE condominium_id = @condominium_id AND id = @id;

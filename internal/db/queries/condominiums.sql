-- O id do condomínio é gerado pela API e a transação já nasce definida para ele:
-- só assim o RLS deixa o INSERT passar (e nada de RETURNING antes do vínculo).
-- name: InsertCondominium :exec
INSERT INTO condominiums (id, administrator_id, name)
VALUES (@id, @administrator_id, @name);

-- name: InsertAdministrator :exec
INSERT INTO administrators (id, name)
VALUES (@id, @name);

-- name: GetCondominium :one
SELECT c.id, c.name, c.administrator_id, c.created_at, a.name AS administrator_name
FROM condominiums c
LEFT JOIN administrators a ON a.id = c.administrator_id
WHERE c.id = @id;

-- name: InsertAudit :exec
INSERT INTO audit_log (condominium_id, actor_user_id, actor_roles, action, resource, resource_id, ip, user_agent, request_id, before, after)
VALUES (@condominium_id, @actor_user_id, @actor_roles, @action, @resource, @resource_id, @ip, @user_agent, @request_id, @before, @after);

-- name: ListAudit :many
SELECT *
FROM audit_log
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource)::text)
  AND (sqlc.narg(resource_id)::text IS NULL OR resource_id = sqlc.narg(resource_id)::text)
ORDER BY id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAudit :one
SELECT count(*)
FROM audit_log
WHERE condominium_id = @condominium_id
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource)::text)
  AND (sqlc.narg(resource_id)::text IS NULL OR resource_id = sqlc.narg(resource_id)::text);

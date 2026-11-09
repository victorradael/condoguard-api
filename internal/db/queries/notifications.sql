-- name: InsertNotification :one
INSERT INTO notifications (condominium_id, message, created_by_id, created_at, resident_ids, shop_owner_ids)
VALUES (@condominium_id, @message, @created_by_id, @created_at, @resident_ids, @shop_owner_ids)
RETURNING id;

-- read_at é a leitura do usuário informado (NULL = não lida por ele).
-- name: GetNotificationByID :one
SELECT n.id, n.condominium_id, n.message, n.created_by_id, n.created_at, n.resident_ids, n.shop_owner_ids,
       r.read_at
FROM notifications n
LEFT JOIN notification_reads r
       ON r.notification_id = n.id AND r.user_id = sqlc.narg(reader_id)::uuid
WHERE n.condominium_id = @condominium_id AND n.id = @id;

-- restricted = false lista tudo (administração); true só as endereçadas às
-- unidades e lojas informadas (as do próprio usuário).
-- name: ListNotifications :many
SELECT n.id, n.condominium_id, n.message, n.created_by_id, n.created_at, n.resident_ids, n.shop_owner_ids,
       r.read_at
FROM notifications n
LEFT JOIN notification_reads r
       ON r.notification_id = n.id AND r.user_id = sqlc.narg(reader_id)::uuid
WHERE n.condominium_id = @condominium_id
  AND (NOT @restricted::boolean
       OR n.resident_ids && @resident_ids::text[]
       OR n.shop_owner_ids && @shop_owner_ids::text[])
ORDER BY n.created_at DESC, n.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountNotifications :one
SELECT count(*)
FROM notifications n
WHERE n.condominium_id = @condominium_id
  AND (NOT @restricted::boolean
       OR n.resident_ids && @resident_ids::text[]
       OR n.shop_owner_ids && @shop_owner_ids::text[]);

-- name: UpdateNotification :execrows
UPDATE notifications
SET message = @message, resident_ids = @resident_ids, shop_owner_ids = @shop_owner_ids
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteNotification :execrows
DELETE FROM notifications
WHERE condominium_id = @condominium_id AND id = @id;

-- Idempotente: a primeira leitura vale; as seguintes não mudam read_at.
-- name: MarkNotificationRead :one
WITH ins AS (
    INSERT INTO notification_reads (notification_id, user_id, condominium_id)
    SELECT n.id, @user_id, n.condominium_id
    FROM notifications n
    WHERE n.condominium_id = @condominium_id AND n.id = @notification_id
    ON CONFLICT (notification_id, user_id) DO NOTHING
    RETURNING read_at
)
SELECT read_at FROM ins
UNION ALL
SELECT r.read_at FROM notification_reads r
WHERE r.notification_id = @notification_id AND r.user_id = @user_id AND r.condominium_id = @condominium_id
LIMIT 1;

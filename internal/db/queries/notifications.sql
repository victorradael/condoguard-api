-- name: InsertNotification :one
INSERT INTO notifications (condominium_id, message, created_by_id, created_at, read, read_at, resident_ids, shop_owner_ids)
VALUES (@condominium_id, @message, @created_by_id, @created_at, @read, @read_at, @resident_ids, @shop_owner_ids)
RETURNING id;

-- name: GetNotificationByID :one
SELECT *
FROM notifications
WHERE condominium_id = @condominium_id AND id = @id;

-- name: ListNotifications :many
SELECT *
FROM notifications
WHERE condominium_id = @condominium_id
ORDER BY created_at DESC, id;

-- name: UpdateNotification :execrows
UPDATE notifications
SET message = @message, created_by_id = @created_by_id, read = @read, read_at = @read_at,
    resident_ids = @resident_ids, shop_owner_ids = @shop_owner_ids
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeleteNotification :execrows
DELETE FROM notifications
WHERE condominium_id = @condominium_id AND id = @id;

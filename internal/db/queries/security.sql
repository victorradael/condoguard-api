-- Segurança da conta: 2FA e códigos de recuperação. Rodam no escopo do
-- condomínio e do usuário da sessão (o RLS só mostra a conta a quem é membro).

-- name: GetAccountSecurity :one
SELECT u.id, u.username, u.email, u.email_verified_at, u.totp_secret, u.totp_enabled_at, u.totp_last_step
FROM users u
JOIN memberships m ON m.user_id = u.id
WHERE m.condominium_id = @condominium_id AND u.id = @id;

-- A semente nova fica pendente (totp_enabled_at nulo) até o primeiro código válido.
-- name: SetPendingTotp :execrows
UPDATE users u
SET totp_secret = @totp_secret, totp_enabled_at = NULL, totp_last_step = 0
WHERE u.id = @id AND u.totp_enabled_at IS NULL
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.condominium_id = @condominium_id);

-- name: EnableTotp :execrows
UPDATE users u
SET totp_enabled_at = now(), totp_last_step = @step
WHERE u.id = @id AND u.totp_enabled_at IS NULL AND u.totp_secret IS NOT NULL
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.condominium_id = @condominium_id);

-- Um código só vale uma vez: o passo precisa ser maior que o último aceito.
-- name: AdvanceTotpStep :execrows
UPDATE users u
SET totp_last_step = @step
WHERE u.id = @id AND u.totp_enabled_at IS NOT NULL AND u.totp_last_step < @step
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.condominium_id = @condominium_id);

-- name: ClearTotp :execrows
UPDATE users u
SET totp_secret = NULL, totp_enabled_at = NULL, totp_last_step = 0
WHERE u.id = @id
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.condominium_id = @condominium_id);

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes WHERE user_id = @user_id;

-- name: InsertRecoveryCode :exec
INSERT INTO recovery_codes (user_id, code_hash) VALUES (@user_id, @code_hash);

-- name: UseRecoveryCode :execrows
UPDATE recovery_codes SET used_at = now()
WHERE user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: CountRecoveryCodes :one
SELECT count(*) FROM recovery_codes WHERE user_id = @user_id AND used_at IS NULL;

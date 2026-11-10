-- +goose Up
-- Segurança da conta (1b, FAB-9): verificação de e-mail, recuperação de senha,
-- 2FA (TOTP) com códigos de recuperação e revogação de sessões.
--
-- Os links de recuperação/verificação e a troca de senha acontecem antes de
-- haver condomínio ou usuário na sessão, então ficam em funções SECURITY
-- DEFINER restritas; a tabela de tokens não tem política (o papel da API não a
-- lê nem escreve direto).

ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz,
    -- Semente do TOTP cifrada pela API (AES-256-GCM); nunca em claro.
    ADD COLUMN totp_secret       bytea,
    -- Só preenchido depois que o usuário provou ter o app configurado.
    ADD COLUMN totp_enabled_at   timestamptz,
    -- Último passo de 30 s aceito: um código vale uma vez só.
    ADD COLUMN totp_last_step    bigint NOT NULL DEFAULT 0;

-- Quem já tinha conta antes da verificação existir continua entrando.
UPDATE users SET email_verified_at = now();

CREATE TABLE account_tokens (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('password_reset', 'email_verify')),
    -- SHA-256 do token; o valor em claro só existe no e-mail ou no link entregue ao usuário.
    token_hash bytea       NOT NULL UNIQUE,
    -- Quem gerou o link para repassar (síndico); nulo quando o próprio usuário pediu.
    issued_by  uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE INDEX account_tokens_user_idx ON account_tokens (user_id, purpose);
ALTER TABLE account_tokens ENABLE ROW LEVEL SECURITY;

CREATE TABLE recovery_codes (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz,
    CONSTRAINT recovery_codes_key UNIQUE (user_id, code_hash)
);

ALTER TABLE recovery_codes ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_codes ON recovery_codes
    USING (user_id = app_user()) WITH CHECK (user_id = app_user());
-- O síndico que zera o 2FA de quem perdeu o celular também apaga os códigos dessa conta.
CREATE POLICY member_codes_delete ON recovery_codes FOR DELETE
    USING (EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = recovery_codes.user_id AND m.condominium_id = app_condominium()));

-- +goose StatementBegin
-- Gera um token para a conta (pedido pelo próprio usuário: "esqueci a senha",
-- "reenviar verificação"). Os anteriores ainda não usados do mesmo tipo deixam de valer.
CREATE FUNCTION account_token_issue(p_user uuid, p_purpose text, p_hash bytea, p_expires timestamptz) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS $$
    UPDATE account_tokens SET expires_at = now()
    WHERE user_id = p_user AND purpose = p_purpose AND used_at IS NULL AND expires_at > now();
    INSERT INTO account_tokens (user_id, purpose, token_hash, expires_at) VALUES (p_user, p_purpose, p_hash, p_expires);
$$;

-- Gera um token para um membro do condomínio atual, a pedido de quem o administra
-- (o link ou código que o síndico repassa por WhatsApp). Devolve false se a conta
-- não for membro deste condomínio.
CREATE FUNCTION account_token_issue_for_member(p_user uuid, p_purpose text, p_hash bytea, p_expires timestamptz, p_issuer uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
    IF app_condominium() IS NULL OR NOT EXISTS (
        SELECT 1 FROM memberships WHERE user_id = p_user AND condominium_id = app_condominium()
    ) THEN
        RETURN false;
    END IF;
    UPDATE account_tokens SET expires_at = now()
    WHERE user_id = p_user AND purpose = p_purpose AND used_at IS NULL AND expires_at > now();
    INSERT INTO account_tokens (user_id, purpose, token_hash, expires_at, issued_by)
    VALUES (p_user, p_purpose, p_hash, p_expires, p_issuer);
    RETURN true;
END
$$;

-- Diz de quem é o token sem gastá-lo (para validar a senha nova com os dados da conta
-- antes de consumir o link).
CREATE FUNCTION account_token_peek(p_hash bytea, p_purpose text) RETURNS TABLE (user_id uuid, email text, username text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT u.id, u.email, u.username
    FROM account_tokens t JOIN users u ON u.id = t.user_id
    WHERE t.token_hash = p_hash AND t.purpose = p_purpose AND t.used_at IS NULL AND t.expires_at > now()
$$;

-- Confirma o e-mail com o token (uso único). Devolve a conta, ou nada se o token
-- é desconhecido, já foi usado ou expirou.
CREATE FUNCTION account_verify_email(p_hash bytea) RETURNS uuid
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS $$
    WITH t AS (
        UPDATE account_tokens SET used_at = now()
        WHERE token_hash = p_hash AND purpose = 'email_verify' AND used_at IS NULL AND expires_at > now()
        RETURNING user_id
    )
    UPDATE users SET email_verified_at = coalesce(email_verified_at, now())
    FROM t WHERE users.id = t.user_id
    RETURNING users.id
$$;

-- Troca a senha com o token (uso único), confirma o e-mail (quem recebeu o link é
-- dono dele) e revoga todas as sessões da conta, em todos os condomínios.
CREATE FUNCTION account_reset_password(p_hash bytea, p_password_hash text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    v_user uuid;
BEGIN
    UPDATE account_tokens SET used_at = now()
    WHERE token_hash = p_hash AND purpose = 'password_reset' AND used_at IS NULL AND expires_at > now()
    RETURNING user_id INTO v_user;
    IF v_user IS NULL THEN
        RETURN NULL;
    END IF;
    UPDATE users SET password_hash = p_password_hash, email_verified_at = coalesce(email_verified_at, now())
    WHERE id = v_user;
    UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = v_user AND revoked_at IS NULL;
    RETURN v_user;
END
$$;

-- Revoga os refresh tokens da conta em todos os condomínios. Vale para a própria
-- conta ou para um membro do condomínio atual; devolve quantos revogou.
CREATE FUNCTION revoke_user_sessions(p_user uuid) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    v_rows integer;
BEGIN
    IF NOT (p_user = app_user() OR EXISTS (
        SELECT 1 FROM memberships WHERE user_id = p_user AND condominium_id = app_condominium()
    )) THEN
        RETURN 0;
    END IF;
    UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = p_user AND revoked_at IS NULL;
    GET DIAGNOSTICS v_rows = ROW_COUNT;
    RETURN v_rows;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION account_token_issue(uuid, text, bytea, timestamptz),
    account_token_issue_for_member(uuid, text, bytea, timestamptz, uuid),
    account_token_peek(bytea, text), account_verify_email(bytea), account_reset_password(bytea, text), revoke_user_sessions(uuid) FROM PUBLIC;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'condoguard_app') THEN
        GRANT EXECUTE ON FUNCTION account_token_issue(uuid, text, bytea, timestamptz),
            account_token_issue_for_member(uuid, text, bytea, timestamptz, uuid),
            account_token_peek(bytea, text), account_verify_email(bytea), account_reset_password(bytea, text), revoke_user_sessions(uuid) TO condoguard_app;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION revoke_user_sessions(uuid);
DROP FUNCTION account_reset_password(bytea, text);
DROP FUNCTION account_verify_email(bytea);
DROP FUNCTION account_token_peek(bytea, text);
DROP FUNCTION account_token_issue_for_member(uuid, text, bytea, timestamptz, uuid);
DROP FUNCTION account_token_issue(uuid, text, bytea, timestamptz);
DROP TABLE recovery_codes;
DROP TABLE account_tokens;
ALTER TABLE users DROP COLUMN totp_last_step, DROP COLUMN totp_enabled_at, DROP COLUMN totp_secret, DROP COLUMN email_verified_at;

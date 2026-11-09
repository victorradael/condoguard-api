-- +goose Up
-- Segurança imediata (1a, FAB-8).
--
-- 1. Login por e-mail: o e-mail passa a ser único em toda a base (não só no
--    condomínio) e é guardado normalizado (sem espaços, minúsculo). Assim o
--    login na 1b, já com vários condomínios, continua sem ambiguidade. O
--    username vira só nome de exibição e pode repetir.
-- 2. Refresh tokens rotativos com revogação (sessão do web por cookie e do app).
-- 3. "Lido" por destinatário: cada usuário marca a própria leitura.

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM users GROUP BY lower(btrim(email)) HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'users: há e-mails repetidos (ignorando maiúsculas e espaços); unifique as contas antes de migrar';
    END IF;
END
$$;
-- +goose StatementEnd

UPDATE users SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

ALTER TABLE users DROP CONSTRAINT users_condominium_email_key;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
ALTER TABLE users ADD CONSTRAINT users_email_normalized_check CHECK (email = lower(btrim(email)) AND email <> '');

CREATE TABLE refresh_tokens (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id text        NOT NULL CHECK (condominium_id <> ''),
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Todos os tokens de uma mesma sessão (login → refresh → refresh...).
    -- Reusar um token já trocado revoga a família inteira.
    family_id      uuid        NOT NULL,
    -- SHA-256 do token; o valor em claro nunca é guardado.
    token_hash     bytea       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    CONSTRAINT refresh_tokens_hash_key UNIQUE (token_hash)
);

CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

ALTER TABLE notifications DROP COLUMN read, DROP COLUMN read_at;

CREATE TABLE notification_reads (
    notification_id uuid        NOT NULL REFERENCES notifications (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    condominium_id  text        NOT NULL CHECK (condominium_id <> ''),
    read_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (notification_id, user_id)
);

CREATE INDEX notification_reads_user_idx ON notification_reads (condominium_id, user_id);

-- +goose Down
DROP TABLE notification_reads;
ALTER TABLE notifications ADD COLUMN read boolean NOT NULL DEFAULT false, ADD COLUMN read_at timestamptz;
DROP TABLE refresh_tokens;
ALTER TABLE users DROP CONSTRAINT users_email_normalized_check;
ALTER TABLE users DROP CONSTRAINT users_email_key;
ALTER TABLE users ADD CONSTRAINT users_condominium_email_key UNIQUE (condominium_id, email);

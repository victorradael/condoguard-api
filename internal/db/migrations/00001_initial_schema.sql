-- +goose Up
-- Esquema inicial (1a-db): as mesmas entidades que a API já expunha.
--
-- Toda tabela tem condominium_id obrigatório. Ainda não há a entidade
-- condomínio (vem na 1b, junto com RLS), então a coluna é texto sem FK.
-- Referências entre registros (ownerId, residentId, createdById...) também
-- ficam como texto sem FK para manter o comportamento atual da API, que aceita
-- qualquer valor; as chaves estrangeiras entram com o novo modelo na 1b/1c.

CREATE TABLE users (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id text        NOT NULL CHECK (condominium_id <> ''),
    username       text        NOT NULL,
    email          text        NOT NULL,
    password_hash  text        NOT NULL,
    roles          text[]      NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_condominium_email_key UNIQUE (condominium_id, email)
);

CREATE INDEX users_condominium_username_idx ON users (condominium_id, username);

CREATE TABLE residents (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id  text        NOT NULL CHECK (condominium_id <> ''),
    -- Valor livre do campo "condominiumId" da API, guardado como chegou.
    -- Na 1b ele passa a ser o próprio condominium_id (FK para condominiums).
    condominium_ref text        NOT NULL,
    unit_number     text        NOT NULL,
    floor           integer     NOT NULL DEFAULT 0,
    owner_id        text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT residents_unit_key UNIQUE (condominium_id, condominium_ref, unit_number)
);

CREATE INDEX residents_condominium_owner_idx ON residents (condominium_id, owner_id);

CREATE TABLE shop_owners (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id text        NOT NULL CHECK (condominium_id <> ''),
    shop_name      text        NOT NULL,
    cnpj           text        NOT NULL,
    floor          integer     NOT NULL DEFAULT 0,
    owner_id       text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_owners_cnpj_key UNIQUE (condominium_id, cnpj)
);

CREATE INDEX shop_owners_condominium_owner_idx ON shop_owners (condominium_id, owner_id);

CREATE TABLE expenses (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id text        NOT NULL CHECK (condominium_id <> ''),
    description    text        NOT NULL,
    amount_cents   bigint      NOT NULL CHECK (amount_cents >= 0),
    due_date       timestamptz NOT NULL,
    -- '' quando a despesa não é de uma residência/loja (a API devolve "").
    resident_id    text        NOT NULL DEFAULT '',
    shop_owner_id  text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX expenses_condominium_due_date_idx ON expenses (condominium_id, due_date);
CREATE INDEX expenses_condominium_resident_idx ON expenses (condominium_id, resident_id) WHERE resident_id <> '';
CREATE INDEX expenses_condominium_shop_owner_idx ON expenses (condominium_id, shop_owner_id) WHERE shop_owner_id <> '';

CREATE TABLE notifications (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id text        NOT NULL CHECK (condominium_id <> ''),
    message        text        NOT NULL,
    created_by_id  text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    read           boolean     NOT NULL DEFAULT false,
    read_at        timestamptz,
    resident_ids   text[]      NOT NULL DEFAULT '{}',
    shop_owner_ids text[]      NOT NULL DEFAULT '{}'
);

CREATE INDEX notifications_condominium_created_at_idx ON notifications (condominium_id, created_at DESC);
CREATE INDEX notifications_condominium_created_by_idx ON notifications (condominium_id, created_by_id);

-- +goose Down
DROP TABLE notifications;
DROP TABLE expenses;
DROP TABLE shop_owners;
DROP TABLE residents;
DROP TABLE users;

-- +goose Up
-- Blocos, unidades, pessoas e vínculos com histórico, convites e marca de dados
-- fictícios (1c, FAB-10).
--
-- 1. `residents` continua sendo a unidade (a API /residents e /units); ganha o
--    bloco. A unicidade passa a ser (condomínio, bloco, número): o mesmo número
--    pode existir em blocos diferentes.
-- 2. `people` são as pessoas (CPF, e-mail e telefone); `unit_links` liga a pessoa
--    à unidade como proprietário, coproprietário, inquilino, morador ou
--    dependente, com início e fim. O fim é exclusivo (primeiro dia sem o
--    vínculo), então a troca no mesmo dia não se sobrepõe. Uma unidade nunca tem
--    dois proprietários principais no mesmo período (EXCLUDE no banco).
-- 3. `invitations`: convite de ativação de conta (só o hash do token é guardado).
-- 4. `is_demo`: todo registro do gerador de dados fictícios é marcado, para o
--    reset-demo apagar só eles.
--
-- Todas as tabelas novas têm RLS pelo condomínio, como as demais.

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Data de hoje no fuso do condomínio (vínculos são por dia, não por instante).
-- +goose StatementBegin
CREATE FUNCTION civil_today() RETURNS date
LANGUAGE sql STABLE AS $$ SELECT (now() AT TIME ZONE 'America/Sao_Paulo')::date $$;
-- +goose StatementEnd

-- ── marca de dados fictícios ────────────────────────────────────────────────
ALTER TABLE administrators ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE condominiums   ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE users          ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE memberships    ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE residents      ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE shop_owners    ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE expenses       ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE notifications  ADD COLUMN is_demo boolean NOT NULL DEFAULT false;
ALTER TABLE audit_log      ADD COLUMN is_demo boolean NOT NULL DEFAULT false;

-- ── blocos ──────────────────────────────────────────────────────────────────
CREATE TABLE blocks (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id uuid        NOT NULL REFERENCES condominiums (id) ON DELETE CASCADE,
    name           text        NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 80),
    is_demo        boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT blocks_name_key UNIQUE (condominium_id, name),
    -- Alvo das chaves compostas: o bloco da unidade é sempre do mesmo condomínio.
    CONSTRAINT blocks_condominium_id_key UNIQUE (condominium_id, id)
);

ALTER TABLE residents
    ADD COLUMN block_id uuid,
    ADD CONSTRAINT residents_block_fkey FOREIGN KEY (condominium_id, block_id)
        REFERENCES blocks (condominium_id, id),
    ADD CONSTRAINT residents_condominium_id_key UNIQUE (condominium_id, id);

-- O texto livre "condominiumId" da API antiga fazia o papel de bloco (a unicidade do
-- número era por esse texto): cada valor distinto vira um bloco e as unidades passam a
-- apontar para ele. Quem usava o id do próprio condomínio fica sem bloco.
INSERT INTO blocks (condominium_id, name)
SELECT DISTINCT condominium_id, left(btrim(condominium_ref), 80)
FROM residents
WHERE btrim(condominium_ref) <> '' AND condominium_ref <> condominium_id::text;

UPDATE residents r SET block_id = b.id
FROM blocks b
WHERE b.condominium_id = r.condominium_id AND b.name = left(btrim(r.condominium_ref), 80)
  AND r.condominium_ref <> r.condominium_id::text;

ALTER TABLE residents DROP CONSTRAINT residents_unit_key;
ALTER TABLE residents ADD CONSTRAINT residents_unit_key
    UNIQUE NULLS NOT DISTINCT (condominium_id, block_id, unit_number);
-- O dono da unidade passa a ser deduzido dos vínculos; o campo antigo fica como
-- "responsável pela conta" opcional.
ALTER TABLE residents ALTER COLUMN owner_id SET DEFAULT '';

-- ── pessoas ─────────────────────────────────────────────────────────────────
CREATE TABLE people (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id uuid        NOT NULL REFERENCES condominiums (id) ON DELETE CASCADE,
    full_name      text        NOT NULL CHECK (btrim(full_name) <> '' AND length(full_name) <= 200),
    -- Só dígitos (11); validado pela API, inclusive os dígitos verificadores.
    cpf            text        CHECK (cpf ~ '^[0-9]{11}$'),
    email          text        CHECK (email = lower(btrim(email)) AND email <> '' AND length(email) <= 254),
    -- Só dígitos, com DDI opcional.
    phone          text        CHECK (phone ~ '^[0-9]{10,13}$'),
    -- Conta de acesso (preenchida ao aceitar o convite).
    user_id        uuid        REFERENCES users (id) ON DELETE SET NULL,
    is_demo        boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT people_condominium_id_key UNIQUE (condominium_id, id),
    CONSTRAINT people_cpf_key UNIQUE (condominium_id, cpf)
);

CREATE UNIQUE INDEX people_user_key ON people (condominium_id, user_id) WHERE user_id IS NOT NULL;
CREATE INDEX people_name_idx ON people (condominium_id, lower(full_name), id);
CREATE INDEX people_email_idx ON people (condominium_id, email) WHERE email IS NOT NULL;

-- ── vínculos pessoa ↔ unidade ───────────────────────────────────────────────
CREATE TABLE unit_links (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id uuid        NOT NULL REFERENCES condominiums (id) ON DELETE CASCADE,
    unit_id        uuid        NOT NULL,
    person_id      uuid        NOT NULL,
    kind           text        NOT NULL
        CHECK (kind IN ('proprietario', 'coproprietario', 'inquilino', 'morador', 'dependente')),
    starts_on      date        NOT NULL,
    -- Exclusivo: primeiro dia em que a pessoa já não está vinculada. Nulo = vigente.
    ends_on        date,
    is_demo        boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT unit_links_period_check CHECK (ends_on IS NULL OR ends_on > starts_on),
    CONSTRAINT unit_links_unit_fkey FOREIGN KEY (condominium_id, unit_id)
        REFERENCES residents (condominium_id, id) ON DELETE CASCADE,
    -- Uma pessoa com histórico não é apagada; o vínculo é encerrado.
    CONSTRAINT unit_links_person_fkey FOREIGN KEY (condominium_id, person_id)
        REFERENCES people (condominium_id, id),
    -- Dois proprietários principais no mesmo período: nunca.
    CONSTRAINT unit_links_one_owner EXCLUDE USING gist (
        unit_id WITH =, daterange(starts_on, ends_on) WITH &&
    ) WHERE (kind = 'proprietario'),
    -- A mesma pessoa não aparece duas vezes com o mesmo papel no mesmo período.
    CONSTRAINT unit_links_no_repeat EXCLUDE USING gist (
        unit_id WITH =, person_id WITH =, kind WITH =, daterange(starts_on, ends_on) WITH &&
    )
);

CREATE INDEX unit_links_unit_idx ON unit_links (condominium_id, unit_id, starts_on DESC);
CREATE INDEX unit_links_person_idx ON unit_links (condominium_id, person_id);
CREATE INDEX unit_links_open_idx ON unit_links (condominium_id, unit_id) WHERE ends_on IS NULL;

-- ── convites ────────────────────────────────────────────────────────────────
CREATE TABLE invitations (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    condominium_id uuid        NOT NULL REFERENCES condominiums (id) ON DELETE CASCADE,
    person_id      uuid        NOT NULL,
    -- E-mail da conta a criar (cópia do e-mail da pessoa no momento do convite).
    email          text        NOT NULL CHECK (email = lower(btrim(email)) AND email <> ''),
    -- SHA-256 do token; o valor em claro só existe no e-mail ou na resposta ao síndico.
    token_hash     bytea       NOT NULL UNIQUE,
    -- 'email': o token só foi enviado ao e-mail da pessoa (aceitar prova o e-mail).
    -- 'link': o síndico recebeu o token para repassar (aceitar não prova o e-mail).
    channel        text        NOT NULL CHECK (channel IN ('email', 'link')),
    created_by     uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    accepted_at    timestamptz,
    revoked_at     timestamptz,
    is_demo        boolean     NOT NULL DEFAULT false,
    CONSTRAINT invitations_person_fkey FOREIGN KEY (condominium_id, person_id)
        REFERENCES people (condominium_id, id) ON DELETE CASCADE
);

CREATE INDEX invitations_person_idx ON invitations (condominium_id, person_id, created_at DESC);

-- ── RLS ─────────────────────────────────────────────────────────────────────
ALTER TABLE blocks ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON blocks
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE people ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON people
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE unit_links ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON unit_links
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invitations
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());

-- ── auditoria: linhas de condomínio fictício podem ser apagadas no reset-demo ──
-- Continua só de inserção para todo o resto: apagar exige a linha ser fictícia
-- E a sessão declarar o reset (app.demo_reset), o que o papel da API não consegue
-- (não tem DELETE na tabela).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.is_demo AND current_setting('app.demo_reset', true) = 'on' THEN
            RETURN OLD;
        END IF;
    END IF;
    RAISE EXCEPTION 'audit_log is append-only' USING ERRCODE = 'insufficient_privilege';
END
$$;

-- A entrada herda a marca do condomínio.
CREATE FUNCTION audit_log_mark_demo() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
    NEW.is_demo := coalesce((SELECT is_demo FROM condominiums WHERE id = NEW.condominium_id), false);
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_log_demo BEFORE INSERT ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_mark_demo();

-- ── funções restritas (antes de haver condomínio na sessão) ─────────────────
-- +goose StatementBegin
-- Aceite do convite: de quem é o token (sem gastá-lo). Nada se o token é
-- desconhecido, foi usado, revogado ou expirou.
CREATE FUNCTION invitation_peek(p_hash bytea)
RETURNS TABLE (id uuid, condominium_id uuid, condominium_name text, person_id uuid, person_name text, email text, channel text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT i.id, i.condominium_id, c.name, i.person_id, p.full_name, i.email, i.channel
    FROM invitations i
    JOIN condominiums c ON c.id = i.condominium_id
    JOIN people p ON p.condominium_id = i.condominium_id AND p.id = i.person_id
    WHERE i.token_hash = p_hash AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > now()
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION invitation_peek(bytea), audit_log_mark_demo() FROM PUBLIC;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'condoguard_app') THEN
        GRANT EXECUTE ON FUNCTION invitation_peek(bytea) TO condoguard_app;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION invitation_peek(bytea);
DROP TRIGGER audit_log_demo ON audit_log;
DROP FUNCTION audit_log_mark_demo();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only' USING ERRCODE = 'insufficient_privilege';
END
$$;
-- +goose StatementEnd

DROP TABLE invitations;
DROP TABLE unit_links;
DROP TABLE people;

ALTER TABLE residents DROP CONSTRAINT residents_unit_key;
-- Voltar à unicidade antiga exige um número por condomínio.
ALTER TABLE residents ADD CONSTRAINT residents_unit_key UNIQUE (condominium_id, condominium_ref, unit_number);
ALTER TABLE residents DROP CONSTRAINT residents_block_fkey;
ALTER TABLE residents DROP CONSTRAINT residents_condominium_id_key;
ALTER TABLE residents DROP COLUMN block_id;
ALTER TABLE residents ALTER COLUMN owner_id DROP DEFAULT;
DROP TABLE blocks;

ALTER TABLE audit_log      DROP COLUMN is_demo;
ALTER TABLE notifications  DROP COLUMN is_demo;
ALTER TABLE expenses       DROP COLUMN is_demo;
ALTER TABLE shop_owners    DROP COLUMN is_demo;
ALTER TABLE residents      DROP COLUMN is_demo;
ALTER TABLE memberships    DROP COLUMN is_demo;
ALTER TABLE users          DROP COLUMN is_demo;
ALTER TABLE condominiums   DROP COLUMN is_demo;
ALTER TABLE administrators DROP COLUMN is_demo;
DROP FUNCTION civil_today();

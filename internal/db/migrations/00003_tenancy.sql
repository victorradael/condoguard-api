-- +goose Up
-- Fundação multi-tenant (1b, FAB-9).
--
-- 1. Entidades administradora e condomínio; condominium_id vira uuid com FK em
--    todas as tabelas de dados.
-- 2. Usuário passa a ser uma identidade global (e-mail único); o papel é do
--    vínculo (memberships) entre usuário e condomínio.
-- 3. Row-Level Security: a API define o condomínio por transação
--    (set_config('app.condominium_id', ..., true)); o papel do banco da API não
--    é dono das tabelas, então as políticas valem para ele. Sem condomínio
--    definido, nenhuma linha é visível (falha fechada).
-- 4. Trilha de auditoria só de inserção.
--
-- Papéis (por condomínio): administradora, sindico, conselho, porteiro, morador.
-- Os papéis antigos viram ROLE_ADMIN → sindico e ROLE_USER → morador.

CREATE TABLE administrators (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL CHECK (name <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE condominiums (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    administrator_id uuid        REFERENCES administrators (id),
    name             text        NOT NULL CHECK (name <> ''),
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX condominiums_administrator_idx ON condominiums (administrator_id);

-- ── condomínios existentes (valores de texto da 1a) ─────────────────────────
CREATE TEMP TABLE condo_map (old text PRIMARY KEY, new uuid NOT NULL DEFAULT gen_random_uuid()) ON COMMIT DROP;

INSERT INTO condo_map (old)
SELECT condominium_id FROM users
UNION SELECT condominium_id FROM residents
UNION SELECT condominium_id FROM shop_owners
UNION SELECT condominium_id FROM expenses
UNION SELECT condominium_id FROM notifications
UNION SELECT condominium_id FROM refresh_tokens
UNION SELECT condominium_id FROM notification_reads;

INSERT INTO condominiums (id, name) SELECT new, old FROM condo_map;

-- ── usuários: identidade global + vínculos ──────────────────────────────────
CREATE TABLE memberships (
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    condominium_id uuid        NOT NULL REFERENCES condominiums (id) ON DELETE CASCADE,
    roles          text[]      NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, condominium_id),
    CONSTRAINT memberships_roles_check CHECK (
        cardinality(roles) > 0
        AND roles <@ ARRAY['administradora', 'sindico', 'conselho', 'porteiro', 'morador']::text[]
    )
);

CREATE INDEX memberships_condominium_idx ON memberships (condominium_id);

INSERT INTO memberships (user_id, condominium_id, roles, created_at)
SELECT u.id,
       m.new,
       CASE WHEN cardinality(mapped.roles) = 0 THEN ARRAY['morador']::text[] ELSE mapped.roles END,
       u.created_at
FROM users u
JOIN condo_map m ON m.old = u.condominium_id
CROSS JOIN LATERAL (
    SELECT coalesce(array_agg(DISTINCT CASE r WHEN 'ROLE_ADMIN' THEN 'sindico' ELSE 'morador' END), '{}') AS roles
    FROM unnest(u.roles) AS r
) mapped;

DROP INDEX users_condominium_username_idx;
ALTER TABLE users DROP COLUMN condominium_id, DROP COLUMN roles;

-- ── demais tabelas: condominium_id text → uuid com FK ───────────────────────
-- Soltar a coluna leva junto índices e unicidades que a usam; recriamos abaixo.
ALTER TABLE residents ADD COLUMN cid uuid;
UPDATE residents t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE residents DROP COLUMN condominium_id;
ALTER TABLE residents RENAME COLUMN cid TO condominium_id;
ALTER TABLE residents ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE residents ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;
ALTER TABLE residents ADD CONSTRAINT residents_unit_key UNIQUE (condominium_id, condominium_ref, unit_number);
CREATE INDEX residents_condominium_owner_idx ON residents (condominium_id, owner_id);

ALTER TABLE shop_owners ADD COLUMN cid uuid;
UPDATE shop_owners t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE shop_owners DROP COLUMN condominium_id;
ALTER TABLE shop_owners RENAME COLUMN cid TO condominium_id;
ALTER TABLE shop_owners ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE shop_owners ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;
ALTER TABLE shop_owners ADD CONSTRAINT shop_owners_cnpj_key UNIQUE (condominium_id, cnpj);
CREATE INDEX shop_owners_condominium_owner_idx ON shop_owners (condominium_id, owner_id);

ALTER TABLE expenses ADD COLUMN cid uuid;
UPDATE expenses t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE expenses DROP COLUMN condominium_id;
ALTER TABLE expenses RENAME COLUMN cid TO condominium_id;
ALTER TABLE expenses ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE expenses ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;
CREATE INDEX expenses_condominium_due_date_idx ON expenses (condominium_id, due_date);
CREATE INDEX expenses_condominium_resident_idx ON expenses (condominium_id, resident_id) WHERE resident_id <> '';
CREATE INDEX expenses_condominium_shop_owner_idx ON expenses (condominium_id, shop_owner_id) WHERE shop_owner_id <> '';

ALTER TABLE notifications ADD COLUMN cid uuid;
UPDATE notifications t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE notifications DROP COLUMN condominium_id;
ALTER TABLE notifications RENAME COLUMN cid TO condominium_id;
ALTER TABLE notifications ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE notifications ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;
CREATE INDEX notifications_condominium_created_at_idx ON notifications (condominium_id, created_at DESC);
CREATE INDEX notifications_condominium_created_by_idx ON notifications (condominium_id, created_by_id);

ALTER TABLE refresh_tokens ADD COLUMN cid uuid;
UPDATE refresh_tokens t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE refresh_tokens DROP COLUMN condominium_id;
ALTER TABLE refresh_tokens RENAME COLUMN cid TO condominium_id;
ALTER TABLE refresh_tokens ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE refresh_tokens ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;

ALTER TABLE notification_reads ADD COLUMN cid uuid;
UPDATE notification_reads t SET cid = m.new FROM condo_map m WHERE m.old = t.condominium_id;
ALTER TABLE notification_reads DROP COLUMN condominium_id;
ALTER TABLE notification_reads RENAME COLUMN cid TO condominium_id;
ALTER TABLE notification_reads ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE notification_reads ADD FOREIGN KEY (condominium_id) REFERENCES condominiums (id) ON DELETE CASCADE;
CREATE INDEX notification_reads_user_idx ON notification_reads (condominium_id, user_id);

-- ── trilha de auditoria (só inserção) ───────────────────────────────────────
CREATE TABLE audit_log (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    condominium_id uuid        NOT NULL REFERENCES condominiums (id),
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    actor_user_id  uuid,
    actor_roles    text[]      NOT NULL DEFAULT '{}',
    action         text        NOT NULL CHECK (action <> ''),
    resource       text        NOT NULL CHECK (resource <> ''),
    resource_id    text        NOT NULL DEFAULT '',
    ip             text        NOT NULL DEFAULT '',
    user_agent     text        NOT NULL DEFAULT '',
    request_id     text        NOT NULL DEFAULT '',
    before         jsonb,
    after          jsonb
);

CREATE INDEX audit_log_condominium_time_idx ON audit_log (condominium_id, occurred_at DESC);
CREATE INDEX audit_log_resource_idx ON audit_log (condominium_id, resource, resource_id);

-- +goose StatementBegin
CREATE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only' USING ERRCODE = 'insufficient_privilege';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_change BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

-- ── Row-Level Security ──────────────────────────────────────────────────────
-- O dono das tabelas (papel das migrações) ignora as políticas; o papel da API
-- não é dono e não tem BYPASSRLS, então está sempre sujeito a elas.

-- Um valor que não é uuid vale como "sem condomínio/usuário": nenhuma linha
-- aparece, sem erro (falha fechada).
-- +goose StatementBegin
CREATE FUNCTION app_condominium() RETURNS uuid
LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN current_setting('app.condominium_id', true) ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN current_setting('app.condominium_id', true)::uuid END
$$;

CREATE FUNCTION app_user() RETURNS uuid
LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN current_setting('app.user_id', true) ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                THEN current_setting('app.user_id', true)::uuid END
$$;
-- +goose StatementEnd

ALTER TABLE condominiums ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON condominiums
    USING (id = app_condominium()) WITH CHECK (id = app_condominium());

-- A administradora é visível pelos condomínios que administra; criá-la só
-- exige uma transação com condomínio definido (o vínculo vem logo depois).
ALTER TABLE administrators ENABLE ROW LEVEL SECURITY;
CREATE POLICY administrators_select ON administrators FOR SELECT USING (
    EXISTS (SELECT 1 FROM condominiums c WHERE c.administrator_id = administrators.id AND c.id = app_condominium())
);
CREATE POLICY administrators_insert ON administrators FOR INSERT WITH CHECK (app_condominium() IS NOT NULL);

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());

-- Um usuário é visível no condomínio onde tem vínculo (ou para ele mesmo).
-- A criação só exige que ainda não haja linha: o vínculo vem logo depois, na
-- mesma transação.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY users_select ON users FOR SELECT USING (
    id = app_user()
    OR EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND m.condominium_id = app_condominium())
);
CREATE POLICY users_insert ON users FOR INSERT WITH CHECK (app_condominium() IS NOT NULL);
CREATE POLICY users_update ON users FOR UPDATE USING (
    id = app_user()
    OR EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND m.condominium_id = app_condominium())
);
CREATE POLICY users_delete ON users FOR DELETE USING (
    EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = users.id AND m.condominium_id = app_condominium())
);

ALTER TABLE residents ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON residents
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE shop_owners ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON shop_owners
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE expenses ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON expenses
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notifications
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE notification_reads ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_reads
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());
ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON refresh_tokens
    USING (condominium_id = app_condominium()) WITH CHECK (condominium_id = app_condominium());

-- Auditoria: lê e insere só no próprio condomínio; não há política de
-- UPDATE/DELETE (e o gatilho acima barra até o dono).
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_select ON audit_log FOR SELECT USING (condominium_id = app_condominium());
CREATE POLICY audit_insert ON audit_log FOR INSERT WITH CHECK (condominium_id = app_condominium());

-- ── funções restritas para o que acontece antes de haver condomínio ─────────
-- Rodam com os direitos do dono (ignoram o RLS) e fazem uma coisa só.

-- +goose StatementBegin
-- Login: acha a conta pelo e-mail (único na base toda).
CREATE FUNCTION auth_user_by_email(p_email text) RETURNS SETOF users
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT * FROM users WHERE email = p_email
$$;

-- Login: condomínios (e papéis) onde a conta tem vínculo.
CREATE FUNCTION auth_memberships(p_user uuid)
RETURNS TABLE (condominium_id uuid, name text, roles text[])
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT m.condominium_id, c.name, m.roles
    FROM memberships m JOIN condominiums c ON c.id = m.condominium_id
    WHERE m.user_id = p_user
    ORDER BY c.name, c.id
$$;

-- Refresh: de qual condomínio é o refresh token (o cookie não diz).
CREATE FUNCTION auth_refresh_condominium(p_hash bytea) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT condominium_id FROM refresh_tokens WHERE token_hash = p_hash
$$;

-- Configuração: resolve DEFAULT_CONDOMINIUM_ID, que pode ser o id ou o nome.
CREATE FUNCTION condominium_lookup(p_ref text) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT id FROM condominiums
    WHERE id::text = lower(p_ref) OR name = p_ref
    ORDER BY (id::text = lower(p_ref)) DESC, created_at
    LIMIT 1
$$;

-- Remove o vínculo do usuário com o condomínio atual; sem outros vínculos, a
-- conta some junto. Devolve quantos vínculos removeu (0 ou 1).
CREATE FUNCTION remove_membership(p_user uuid) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    v_condo uuid := app_condominium();
    v_rows  integer;
BEGIN
    IF v_condo IS NULL THEN
        RETURN 0;
    END IF;
    DELETE FROM memberships WHERE user_id = p_user AND condominium_id = v_condo;
    GET DIAGNOSTICS v_rows = ROW_COUNT;
    IF v_rows > 0 AND NOT EXISTS (SELECT 1 FROM memberships WHERE user_id = p_user) THEN
        DELETE FROM users WHERE id = p_user;
    END IF;
    RETURN v_rows;
END
$$;
-- +goose StatementEnd

-- A API só chama as funções acima; ninguém mais precisa delas.
REVOKE ALL ON FUNCTION auth_user_by_email(text), auth_memberships(uuid), auth_refresh_condominium(bytea),
    condominium_lookup(text), remove_membership(uuid) FROM PUBLIC;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'condoguard_app') THEN
        GRANT EXECUTE ON FUNCTION auth_user_by_email(text), auth_memberships(uuid), auth_refresh_condominium(bytea),
            condominium_lookup(text), remove_membership(uuid) TO condoguard_app;
        REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM condoguard_app;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- A descida não recompõe os dados: volta ao esquema da 1a com condominium_id em
-- texto (o uuid do condomínio) e papéis ROLE_*. Vínculos extras (mais de um
-- condomínio por conta) não cabem no modelo antigo: fica o mais antigo.
DROP FUNCTION remove_membership(uuid);
DROP FUNCTION condominium_lookup(text);
DROP FUNCTION auth_refresh_condominium(bytea);
DROP FUNCTION auth_memberships(uuid);
DROP FUNCTION auth_user_by_email(text);

DROP TABLE audit_log;
DROP FUNCTION audit_log_append_only();

DROP POLICY administrators_insert ON administrators;
DROP POLICY administrators_select ON administrators;
DROP POLICY tenant_isolation ON refresh_tokens;
DROP POLICY tenant_isolation ON notification_reads;
DROP POLICY tenant_isolation ON notifications;
DROP POLICY tenant_isolation ON expenses;
DROP POLICY tenant_isolation ON shop_owners;
DROP POLICY tenant_isolation ON residents;
DROP POLICY users_delete ON users;
DROP POLICY users_update ON users;
DROP POLICY users_insert ON users;
DROP POLICY users_select ON users;
ALTER TABLE refresh_tokens DISABLE ROW LEVEL SECURITY;
ALTER TABLE notification_reads DISABLE ROW LEVEL SECURITY;
ALTER TABLE notifications DISABLE ROW LEVEL SECURITY;
ALTER TABLE expenses DISABLE ROW LEVEL SECURITY;
ALTER TABLE shop_owners DISABLE ROW LEVEL SECURITY;
ALTER TABLE residents DISABLE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;

ALTER TABLE users ADD COLUMN condominium_id text, ADD COLUMN roles text[] NOT NULL DEFAULT '{}';
UPDATE users u SET condominium_id = x.condominium_id::text,
    roles = ARRAY[CASE WHEN 'sindico' = ANY (x.roles) OR 'administradora' = ANY (x.roles) THEN 'ROLE_ADMIN' ELSE 'ROLE_USER' END]
FROM (SELECT DISTINCT ON (user_id) user_id, condominium_id, roles FROM memberships ORDER BY user_id, created_at) x
WHERE x.user_id = u.id;
DELETE FROM users WHERE condominium_id IS NULL;
ALTER TABLE users ALTER COLUMN condominium_id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_condominium_id_check CHECK (condominium_id <> '');
CREATE INDEX users_condominium_username_idx ON users (condominium_id, username);

ALTER TABLE residents DROP CONSTRAINT residents_condominium_id_fkey;
ALTER TABLE shop_owners DROP CONSTRAINT shop_owners_condominium_id_fkey;
ALTER TABLE expenses DROP CONSTRAINT expenses_condominium_id_fkey;
ALTER TABLE notifications DROP CONSTRAINT notifications_condominium_id_fkey;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_condominium_id_fkey;
ALTER TABLE notification_reads DROP CONSTRAINT notification_reads_condominium_id_fkey;
ALTER TABLE residents DROP CONSTRAINT residents_unit_key;
ALTER TABLE shop_owners DROP CONSTRAINT shop_owners_cnpj_key;
ALTER TABLE residents ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE shop_owners ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE expenses ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE notifications ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE refresh_tokens ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE notification_reads ALTER COLUMN condominium_id TYPE text USING condominium_id::text;
ALTER TABLE residents ADD CONSTRAINT residents_unit_key UNIQUE (condominium_id, condominium_ref, unit_number);
ALTER TABLE shop_owners ADD CONSTRAINT shop_owners_cnpj_key UNIQUE (condominium_id, cnpj);

DROP TABLE memberships;
DROP TABLE condominiums;
DROP TABLE administrators;
DROP FUNCTION app_condominium();
DROP FUNCTION app_user();

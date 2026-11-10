-- Pessoas, vínculos com unidades e convites. Todas as consultas recebem o condomínio.

-- name: InsertPerson :exec
INSERT INTO people (id, condominium_id, full_name, cpf, email, phone, is_demo)
VALUES (@id, @condominium_id, @full_name, @cpf, @email, @phone, @is_demo);

-- name: GetPerson :one
SELECT * FROM people WHERE condominium_id = @condominium_id AND id = @id;

-- name: GetPersonByUser :one
SELECT * FROM people WHERE condominium_id = @condominium_id AND user_id = @user_id;

-- name: FindPersonByCPF :one
SELECT * FROM people WHERE condominium_id = @condominium_id AND cpf = @cpf;

-- Filtros: texto no nome, unidade (vínculo vigente ou qualquer) e restrição às unidades de
-- quem só vê a própria casa (visible_units, só vínculos vigentes).
-- name: ListPeople :many
SELECT p.*
FROM people p
WHERE p.condominium_id = @condominium_id
  AND (sqlc.narg(q)::text IS NULL OR p.full_name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(unit_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM unit_links l
        WHERE l.condominium_id = p.condominium_id AND l.person_id = p.id AND l.unit_id = sqlc.narg(unit_id)::uuid
          AND (NOT @active_only::boolean OR (l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))))
  AND (sqlc.narg(visible_units)::uuid[] IS NULL OR EXISTS (
        SELECT 1 FROM unit_links l
        WHERE l.condominium_id = p.condominium_id AND l.person_id = p.id AND l.unit_id = ANY (sqlc.narg(visible_units)::uuid[])
          AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))
ORDER BY lower(p.full_name), p.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPeople :one
SELECT count(*)
FROM people p
WHERE p.condominium_id = @condominium_id
  AND (sqlc.narg(q)::text IS NULL OR p.full_name ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(unit_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM unit_links l
        WHERE l.condominium_id = p.condominium_id AND l.person_id = p.id AND l.unit_id = sqlc.narg(unit_id)::uuid
          AND (NOT @active_only::boolean OR (l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))))
  AND (sqlc.narg(visible_units)::uuid[] IS NULL OR EXISTS (
        SELECT 1 FROM unit_links l
        WHERE l.condominium_id = p.condominium_id AND l.person_id = p.id AND l.unit_id = ANY (sqlc.narg(visible_units)::uuid[])
          AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())));

-- name: UpdatePerson :execrows
UPDATE people
SET full_name = @full_name, cpf = @cpf, email = @email, phone = @phone, updated_at = now()
WHERE condominium_id = @condominium_id AND id = @id;

-- name: DeletePerson :execrows
DELETE FROM people WHERE condominium_id = @condominium_id AND id = @id;

-- name: SetPersonUser :execrows
UPDATE people SET user_id = @user_id, updated_at = now()
WHERE condominium_id = @condominium_id AND id = @id AND user_id IS NULL;

-- ── vínculos ────────────────────────────────────────────────────────────────

-- name: InsertLink :exec
INSERT INTO unit_links (id, condominium_id, unit_id, person_id, kind, starts_on, ends_on, is_demo)
VALUES (@id, @condominium_id, @unit_id, @person_id, @kind, @starts_on, @ends_on, @is_demo);

-- name: GetLink :one
SELECT * FROM unit_links WHERE condominium_id = @condominium_id AND id = @id;

-- Histórico da unidade: mais recente primeiro.
-- name: ListLinksByUnit :many
SELECT l.id, l.unit_id, l.person_id, p.full_name AS person_name, l.kind, l.starts_on, l.ends_on, l.created_at
FROM unit_links l
JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
WHERE l.condominium_id = @condominium_id AND l.unit_id = @unit_id
  AND (NOT @active_only::boolean OR (l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())))
ORDER BY l.starts_on DESC, l.created_at DESC, l.id;

-- Vínculos vigentes de várias pessoas (resumo na listagem de pessoas).
-- name: ListActiveLinksByPeople :many
SELECT l.id, l.unit_id, l.person_id, l.kind, l.starts_on, l.ends_on, r.unit_number, b.name AS block_name
FROM unit_links l
JOIN residents r ON r.condominium_id = l.condominium_id AND r.id = l.unit_id
LEFT JOIN blocks b ON b.condominium_id = r.condominium_id AND b.id = r.block_id
WHERE l.condominium_id = @condominium_id AND l.person_id = ANY (@person_ids::uuid[])
  AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())
ORDER BY l.person_id, l.starts_on, l.id;

-- Todos os vínculos de uma pessoa (ficha da pessoa).
-- name: ListLinksByPerson :many
SELECT l.id, l.unit_id, l.person_id, l.kind, l.starts_on, l.ends_on, r.unit_number, b.name AS block_name
FROM unit_links l
JOIN residents r ON r.condominium_id = l.condominium_id AND r.id = l.unit_id
LEFT JOIN blocks b ON b.condominium_id = r.condominium_id AND b.id = r.block_id
WHERE l.condominium_id = @condominium_id AND l.person_id = @person_id
ORDER BY l.starts_on DESC, l.id;

-- Vínculos da conta nas unidades (app do morador): vigentes primeiro.
-- name: ListLinksByUser :many
SELECT l.id, l.unit_id, l.person_id, l.kind, l.starts_on, l.ends_on, r.unit_number, b.name AS block_name
FROM unit_links l
JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
JOIN residents r ON r.condominium_id = l.condominium_id AND r.id = l.unit_id
LEFT JOIN blocks b ON b.condominium_id = r.condominium_id AND b.id = r.block_id
WHERE l.condominium_id = @condominium_id AND p.user_id = @user_id
  AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())
ORDER BY b.name NULLS FIRST, r.unit_number, l.kind, l.id;

-- A conta é responsável pela unidade (proprietário, coproprietário ou inquilino vigente)?
-- name: UserIsResponsible :one
SELECT EXISTS (
    SELECT 1 FROM unit_links l
    JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
    WHERE l.condominium_id = @condominium_id AND l.unit_id = @unit_id AND p.user_id = @user_id
      AND l.kind IN ('proprietario', 'coproprietario', 'inquilino')
      AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())
);

-- A conta tem vínculo vigente (qualquer papel) com a unidade?
-- name: UserHasLink :one
SELECT EXISTS (
    SELECT 1 FROM unit_links l
    JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
    WHERE l.condominium_id = @condominium_id AND l.unit_id = @unit_id AND p.user_id = @user_id
      AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today())
);

-- Unidades em que a conta tem vínculo vigente (qualquer papel): o que o morador enxerga da casa.
-- name: ListUnitIDsByUserLinks :many
SELECT DISTINCT l.unit_id
FROM unit_links l
JOIN people p ON p.condominium_id = l.condominium_id AND p.id = l.person_id
WHERE l.condominium_id = @condominium_id AND p.user_id = @user_id
  AND l.starts_on <= civil_today() AND (l.ends_on IS NULL OR l.ends_on > civil_today());

-- A pessoa mora (vínculo vigente) em alguma unidade em que a conta também tem vínculo vigente?
-- name: PersonSharesUnitWithUser :one
SELECT EXISTS (
    SELECT 1 FROM unit_links a
    JOIN unit_links b ON b.condominium_id = a.condominium_id AND b.unit_id = a.unit_id
    JOIN people pu ON pu.condominium_id = b.condominium_id AND pu.id = b.person_id
    WHERE a.condominium_id = @condominium_id AND a.person_id = @person_id AND pu.user_id = @user_id
      AND a.starts_on <= civil_today() AND (a.ends_on IS NULL OR a.ends_on > civil_today())
      AND b.starts_on <= civil_today() AND (b.ends_on IS NULL OR b.ends_on > civil_today())
);

-- Vínculos dos tipos dados que valem na data (a trocar). Trava as linhas para a troca ser atômica.
-- name: ListOpenLinksForUpdate :many
SELECT * FROM unit_links
WHERE condominium_id = @condominium_id AND unit_id = @unit_id AND kind = ANY (@kinds::text[])
  AND starts_on < @on_day::date AND (ends_on IS NULL OR ends_on > @on_day::date)
ORDER BY starts_on, id
FOR UPDATE;

-- Vínculos dos tipos dados que ainda começam depois da data (a troca ficaria fora de ordem).
-- name: CountLinksStartingAfter :one
SELECT count(*) FROM unit_links
WHERE condominium_id = @condominium_id AND unit_id = @unit_id AND kind = ANY (@kinds::text[])
  AND starts_on >= @on_day::date;

-- name: EndLink :execrows
UPDATE unit_links SET ends_on = @ends_on
WHERE condominium_id = @condominium_id AND id = @id AND starts_on < @ends_on::date;

-- name: DeleteLink :execrows
DELETE FROM unit_links WHERE condominium_id = @condominium_id AND id = @id;

-- ── convites ────────────────────────────────────────────────────────────────

-- name: RevokePendingInvitations :exec
UPDATE invitations SET revoked_at = now()
WHERE condominium_id = @condominium_id AND person_id = @person_id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: InsertInvitation :exec
INSERT INTO invitations (id, condominium_id, person_id, email, token_hash, channel, created_by, expires_at, is_demo)
VALUES (@id, @condominium_id, @person_id, @email, @token_hash, @channel, @created_by, @expires_at, @is_demo);

-- name: ListInvitationsByPerson :many
SELECT id, person_id, email, channel, created_at, expires_at, accepted_at, revoked_at
FROM invitations
WHERE condominium_id = @condominium_id AND person_id = @person_id
ORDER BY created_at DESC, id
LIMIT 20;

-- Gasta o convite; 0 linhas = alguém já aceitou, ou ele foi revogado ou expirou.
-- name: AcceptInvitation :execrows
UPDATE invitations SET accepted_at = now()
WHERE condominium_id = @condominium_id AND id = @id
  AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now();

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified_at = coalesce(email_verified_at, now()) WHERE id = @id;

-- name: GetUserCredentialsByEmail :one
SELECT * FROM auth_user_by_email(@email);

-- name: MembershipExists :one
SELECT EXISTS (SELECT 1 FROM memberships WHERE user_id = @user_id AND condominium_id = @condominium_id);

-- name: FindPersonByEmailAndName :one
SELECT * FROM people
WHERE condominium_id = @condominium_id AND email = @email AND lower(full_name) = lower(@full_name)
ORDER BY created_at, id
LIMIT 1;

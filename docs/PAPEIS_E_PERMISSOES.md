# Papéis e permissões (1b)

## Modelo

- **Administradora** → **condomínio**: uma administradora (empresa) pode administrar vários condomínios.
- Uma **conta** é global (um e-mail, uma conta). O **papel pertence ao vínculo** (`memberships`) entre a conta e
  um condomínio: a mesma pessoa pode ser síndica de um condomínio e moradora de outro.
- O token de acesso é emitido **para um condomínio** (claim `cid`) e carrega os papéis dela ali. Quem tem mais de
  um vínculo informa `condominiumId` no login; sem isso a API responde `409` com a lista (só depois de conferir a senha).
- Papéis: `administradora`, `sindico`, `conselho`, `porteiro`, `morador`. Uma pessoa pode ter mais de um papel
  no mesmo condomínio; as permissões se somam.
- Os nomes antigos `ROLE_ADMIN` e `ROLE_USER` não valem mais. A migração `00003` converte `ROLE_ADMIN → sindico`
  e `ROLE_USER → morador`.

## Matriz

| Permissão | administradora | síndico | conselho | porteiro | morador |
|---|:-:|:-:|:-:|:-:|:-:|
| `condominium.manage` (papel `administradora`) | ✅ | | | | |
| `users.manage` (criar, alterar e remover usuários) | ✅ | ✅ | | | |
| `users.read` (listar e ler usuários) | ✅ | ✅ | ✅ | | |
| `units.write` (unidades e lojas) | ✅ | ✅ | | | |
| `units.read_all` | ✅ | ✅ | ✅ | ✅ | só as suas |
| `expenses.write` | ✅ | ✅ | | | |
| `expenses.read_all` | ✅ | ✅ | ✅ | | só as suas |
| `notifications.write` | ✅ | ✅ | | | |
| `notifications.read_all` | ✅ | ✅ | ✅ | | só as endereçadas às suas |
| `audit.read` (trilha de auditoria) | ✅ | ✅ | ✅ | | |
| `people.write` (pessoas, vínculos, trocas, convites, importação CSV) | ✅ | ✅ | | | só `morador`/`dependente` da própria unidade, se for responsável |
| `people.read_all` (nomes e unidades vigentes) | ✅ | ✅ | ✅ | ✅ | só a casa dele |
| `people.read_contact` (e-mail, telefone, CPF mascarado, histórico da unidade) | ✅ | ✅ | ✅ | | |
| `people.read_cpf` (CPF inteiro) | ✅ | ✅ | | | só o próprio |

Todo usuário autenticado lê a própria conta (`GET /users/me`) e o condomínio do token (`GET /condominium`).

**Autorização por dono do dado.** Quem não tem a permissão `*.read_all` do recurso só enxerga o que é das unidades
que possui (`ownerId`). O que não puder ver responde `404`, igual a um registro que não existe.

**Sem subir de nível.** Só quem tem `condominium.manage` cria, promove, altera ou remove uma `administradora`.
O síndico não promove ninguém (nem a si mesmo) acima do próprio nível.

A matriz está em `internal/authz/authz.go` e repetida, de propósito, em `internal/authz/authz_test.go`
(mudar uma permissão exige editar os dois) e testada por HTTP em `internal/parity/foundation_test.go`.

## Isolamento entre condomínios (duas camadas)

1. **Repositório**: toda consulta recebe o `condominium_id` do contexto (sem tenant, o repositório recusa).
2. **PostgreSQL Row-Level Security**: a API roda como `condoguard_app`, que não é dona das tabelas, não é
   superusuário e não tem `BYPASSRLS`. Cada instrução roda numa transação que antes faz
   `set_config('app.condominium_id', …, true)` e `set_config('app.user_id', …, true)` (`internal/db/scoped.go`);
   as políticas comparam `condominium_id` com esse valor. Sem condomínio definido (ou com valor inválido) nenhuma
   linha aparece.

Duas exceções, ambas funções `SECURITY DEFINER` restritas (migração `00003`): o login precisa achar a conta pelo e-mail
e listar os vínculos antes de saber o condomínio (`auth_user_by_email`, `auth_memberships`); o refresh precisa saber de
qual condomínio é o token (`auth_refresh_condominium`). `condominium_lookup` resolve `DEFAULT_CONDOMINIUM_ID`
(id ou nome) na partida e `remove_membership` tira o vínculo do condomínio atual (e a conta, se era o último).

A suíte `internal/isolation` roda instruções **sem** o filtro do repositório, como o papel restrito, e prova que só o
RLS já barra leitura, alteração, remoção e escrita cruzadas, que a falha é fechada e que o tenant não vaza entre
requisições que reaproveitam a mesma conexão.

## Trilha de auditoria

`audit_log`: condomínio, quando, quem (usuário e papéis), o quê (`create`/`update`/`delete` + recurso e id), de onde
(IP já resolvido pelos proxies confiáveis, User-Agent, id da requisição) e o valor anterior e novo (JSON, sem segredos).
Só inserção: sem política de UPDATE/DELETE e um gatilho que barra até o dono da tabela (inclusive `TRUNCATE`).
Cada mudança em cadastro grava a entrada **na mesma transação** da mudança (`audit.Trail.Atomic`): não há mudança sem
trilha nem trilha sem mudança. Leitura: `GET /audit-log?resource=&resourceId=&page=&pageSize=` (permissão `audit.read`).

## Segurança da conta (migração 00004)

| Rota | Quem | O quê |
|---|---|---|
| `POST /auth/email/verify`, `/auth/email/resend` | público (limite por IP) | confirma o e-mail com o token; reenvia o link. Mesma resposta (202) para qualquer e-mail |
| `POST /auth/password/forgot`, `/auth/password/reset` | público (limite por IP) | link de uso único (1 h); a troca revoga todas as sessões da conta e confirma o e-mail |
| `POST /auth/2fa/setup`, `/auth/2fa/enable` | logado, ou `mfaToken` de cadastro (corpo) | gera a semente, confirma com o primeiro código e mostra **uma vez** 10 códigos de recuperação |
| `POST /auth/2fa/verify` | `mfaToken` de verificação | conclui o login com o código TOTP ou um código de recuperação |
| `POST /auth/2fa/disable`, `/auth/2fa/recovery-codes` | logado + senha/código | desliga (não vale para quem tem papel que exige) / gera novos códigos |
| `DELETE /auth/sessions` | logado | encerra todas as sessões (refresh e access tokens) |
| `POST /users/{id}/account-links` `{purpose}` | `users.manage` | síndico gera link/código (`email_verify` ou `password_reset`, 24 h) para repassar por WhatsApp. Devolvido uma vez; a auditoria registra só a emissão |
| `DELETE /users/{id}/2fa` | `users.manage` | zera o 2FA de quem perdeu celular e códigos e encerra as sessões |

- **2FA obrigatório** para `administradora`, `sindico` e `conselho`: a senha sozinha devolve `{mfaRequired, stage: "enroll", mfaToken}` e
  nenhuma sessão até a conta cadastrar o app. Com 2FA ligado, em qualquer papel, o login devolve `stage: "verify"`.
- Um código TOTP vale uma vez (o passo precisa ser maior que o último aceito) e há limite de tentativas por conta.
- A semente é cifrada com AES-256-GCM (`TOTP_ENCRYPTION_KEY`, amarrada ao id do usuário); os códigos de recuperação e os
  tokens de link ficam só como hash SHA-256.
- Quem ajuda não sobe de nível: `account-links` e `2fa` seguem a regra do `mayGrant` (o síndico não mexe na conta de uma
  administradora).
- Revogação de access token: lista no Redis por `jti` (logout) e por usuário (sair de todos, troca de senha, papéis). Se a lista
  não puder ser lida, a requisição é recusada com 503, nunca aceita.
- `REQUIRE_VERIFIED_EMAIL` (padrão: ligado em produção) recusa o login até confirmar o e-mail (`403 email_not_verified`).
- Variáveis: `TOTP_ENCRYPTION_KEY` (Base64 de 32 bytes), `WEB_BASE_URL`, `SMTP_HOST`/`SMTP_PORT`/`SMTP_USERNAME`/`SMTP_PASSWORD`/`MAIL_FROM`,
  `TOTP_ISSUER`; as três primeiras são obrigatórias com `APP_ENV=production`.

## Pessoas, vínculos e convites (migração 00005)

Detalhes e rotas em [UNIDADES_E_MORADORES.md](UNIDADES_E_MORADORES.md). Em resumo:

- Quem é "dono" de uma unidade deixou de ser o campo `ownerId`: vem dos **vínculos** (`unit_links`) da pessoa ligada à conta.
  O morador enxerga as unidades onde tem vínculo vigente de qualquer papel menos `dependente` (despesas e avisos seguem a mesma regra).
- O **responsável** pela unidade (proprietário, coproprietário ou inquilino vigente) cadastra e encerra `morador` e `dependente` dela. Isso
  é checado no banco a cada chamada, não no token.
- Dados pessoais por necessidade: porteiro vê nome e unidade; conselho soma contato e CPF mascarado; síndico e administradora veem tudo;
  morador vê a própria ficha e o contato de quem mora com ele, nunca o CPF dos outros. O que o papel não vê responde 404.
- Aceitar convite cria conta com o papel `morador` apenas. Porteiro, conselho e síndico continuam sendo criados em `POST /users`.

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

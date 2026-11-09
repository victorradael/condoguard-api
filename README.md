<div style="text-align: center;">
    <img src="https://github.com/victorradael/condoguard/blob/main/assets/condoguard-logo.png?raw=true" alt="CondoguardLogo" height="200">
</div>

# CondoGuard API

REST API do CondoGuard — sistema de gestão financeira e administrativa de condomínios.

## Stack

| Camada | Tecnologia |
|--------|-----------|
| Linguagem | Go 1.26+ |
| HTTP | `net/http` (stdlib, Go 1.22+ method routing) |
| Banco de dados | PostgreSQL 17 (`pgx` + consultas tipadas com `sqlc`, migrações `goose`) |
| Cache / limites | Redis 7 (`go-redis`) — limite de tentativas de login |
| Autenticação | JWT HS256 de curta duração + refresh token rotativo (cookie httpOnly no web, bearer no app) |
| Senhas | bcrypt (`golang.org/x/crypto`) |
| Logging | `log/slog` (stdlib) |
| Métricas | `expvar` (stdlib) |

## Pré-requisitos

- Go 1.26+
- Docker e Docker Compose (PostgreSQL e Redis locais)
- [`sqlc`](https://sqlc.dev) só para quem altera as consultas em `internal/db/queries/`

## Configuração

Copie `.env.example` para `.env` e preencha os valores:

```bash
cp .env.example .env
```

Não há **segredos padrão** no código nem no compose: `POSTGRES_PASSWORD`,
`REDIS_PASSWORD` e `JWT_SECRET_KEY` precisam estar no `.env` (gere com
`openssl rand -base64 32`). A API **não sobe** sem `JWT_SECRET_KEY` em Base64
com pelo menos 32 bytes, nem com `CORS_ALLOWED_ORIGINS=*`.

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `DATABASE_URL` | URL do PostgreSQL (`postgres://...`). Sem ela a API usa repositórios em memória (só para desenvolvimento). Com o banco fora do host, use `sslmode=require` ou `verify-full` | — |
| `DATABASE_MIGRATIONS_URL` | URL do PostgreSQL com o papel dono do esquema, usada só para aplicar as migrações. Assim o papel de `DATABASE_URL` não precisa de permissão de DDL (no Postgres 15+, o `public` não aceita `CREATE` de quem não é dono) | `DATABASE_URL` |
| `DB_MIGRATE_ON_START` | Aplica as migrações pendentes ao subir | `true` |
| `REDIS_URL` | URL do Redis (`redis://:senha@host:6379/0`). Sem ela o limite de login fica em memória (uma instância só) | — |
| `DEFAULT_CONDOMINIUM_ID` | Condomínio (tenant) do cadastro e do login até a entidade condomínio existir (Fase 1b). Nas rotas autenticadas o tenant vem do token | `default` |
| `JWT_SECRET_KEY` | Segredo JWT em Base64, ≥ 32 bytes (**obrigatório**) | — |
| `ACCESS_TOKEN_TTL` | Validade do access token | `15m` |
| `REFRESH_TOKEN_TTL` | Validade do refresh token | `720h` |
| `LOGIN_MAX_ATTEMPTS` | Falhas de login por IP + e-mail antes do bloqueio | `5` |
| `LOGIN_MAX_ATTEMPTS_PER_IP` | Falhas de login por IP, somando todos os e-mails | `50` |
| `LOGIN_LOCKOUT_WINDOW` | Janela do bloqueio (duração Go, ex.: `15m`) | `15m` |
| `REGISTER_MAX_PER_IP` | Cadastros por IP dentro da janela (freia a enumeração de e-mails) | `10` |
| `REGISTER_WINDOW` | Janela do limite de cadastro | `1h` |
| `CORS_ALLOWED_ORIGINS` | Origens do web, separadas por vírgula (`*` é recusado) | `http://localhost:3000` |
| `COOKIE_SECURE` | Cookies com `Secure`; `false` só em HTTP puro fora de `localhost` | `true` |
| `COOKIE_DOMAIN` | Domínio dos cookies. Com web e API em subdomínios diferentes, use o domínio comum para o `middleware.ts` do web ver a sessão | host da API |
| `COOKIE_PATH_PREFIX` | Prefixo com que o navegador chega à API quando um proxy a monta num subcaminho (ex.: `/api`). O cookie de refresh fica em `<prefixo>/auth` | — (API na raiz) |
| `METRICS_TOKEN` | Bearer exigido em `/metrics` (≥ 32 caracteres). Vazio desliga `/metrics` | — |
| `PORT` | Porta do servidor HTTP | `8080` |

## Rodando localmente

```bash
# Tudo em container (API + PostgreSQL + Redis); as migrações rodam ao subir
make up

# Ou: só PostgreSQL e Redis em container, API no host
make dev-up
DATABASE_URL=... REDIS_URL=... JWT_SECRET_KEY=... make run
```

### Primeiro administrador

O cadastro público (`POST /auth/register`) cria **só moradores** (`ROLE_USER`);
papéis enviados pelo cliente são ignorados. Só um admin cria outro admin
(`POST /users`). O primeiro admin do condomínio é criado pela linha de comando:

```bash
# No host
ADMIN_EMAIL=sindico@exemplo.com ADMIN_USERNAME=sindico DATABASE_URL=... \
  go run ./cmd/createadmin < senha.txt

# No container
docker compose exec -T -e ADMIN_EMAIL=sindico@exemplo.com -e ADMIN_USERNAME=sindico \
  api condoguard-createadmin < senha.txt
```

### Banco de dados

- Migrações em `internal/db/migrations/` (goose, embutidas no binário).
  `make migrate-up`, `make migrate-down`, `make migrate-reset` e `make migrate-version` usam `DATABASE_MIGRATIONS_URL` (ou `DATABASE_URL`, se ela não estiver definida).
- Consultas em `internal/db/queries/*.sql`; o código Go é gerado em `internal/db/dbgen/` com `make sqlc` (o CI confere com `sqlc diff`).
- Toda tabela tem `condominium_id` obrigatório e toda consulta filtra por ele. O valor vem do token (rotas autenticadas) ou do `DEFAULT_CONDOMINIUM_ID` (cadastro e login); um repositório chamado sem tenant retorna erro. O Row-Level Security entra na Fase 1b.
- O e-mail é o login: único em toda a base, guardado normalizado (minúsculo, sem espaços). O `username` é só nome de exibição.
- IDs são UUID. Um ID mal formado em `/{id}` responde 404, como um ID inexistente.

## Testes

```bash
# Testes unitários (sem banco; os de PostgreSQL são pulados)
make test-unit

# Todos os testes, com PostgreSQL 17 e Redis 7 de verdade em container
make test-db-up        # postgres-test em 127.0.0.1:55432, redis-test em 127.0.0.1:56379
make test-integration  # usa TEST_DATABASE_URL e TEST_REDIS_URL
make test-db-down

# Cobertura
make test-cover
```

Cada pacote de teste cria um banco descartável a partir de `TEST_DATABASE_URL`
(o usuário precisa poder `CREATE DATABASE`), aplica as migrações e apaga o banco no fim.
`internal/parity` sobe a API inteira contra o PostgreSQL e prova, item a item,
que os ataques corrigidos na 1a falham (papéis no cadastro, token forjado,
acesso a dado de outro morador ou de outro condomínio, autor de notificação,
CORS, limite de login, cookies e refresh).

O CI roda também `golangci-lint`, `govulncheck` e o build da imagem Docker
(usuário sem root, healthcheck). As actions são fixadas por SHA e o Dependabot
propõe as atualizações.

## Endpoints

O contrato completo está em OpenAPI 3.1: `GET /openapi.json` e `GET /docs`
(Swagger UI) exigem `ROLE_ADMIN`; para gerar tipos no web e no app use
`go run ./cmd/openapi > openapi.json`. O Swagger UI vem do CDN numa versão
exata, com SRI e CSP próprios (`pkg/openapi/handler.go`); ao atualizar a
versão, troque os hashes juntos.

Toda listagem é paginada: `?page=1&pageSize=20` (máximo 100) e a resposta é
`{ "items": [...], "page": 1, "pageSize": 20, "total": 42 }`.

### Autorização

- `ROLE_ADMIN` (síndico/administração) gerencia tudo do próprio condomínio.
- `ROLE_USER` (morador ou lojista) só **lê** as unidades, lojas, despesas e
  notificações ligadas às unidades de que é dono (`ownerId`). O que ele não pode
  ver responde 404, como se não existisse.
- Dados de outro condomínio nunca aparecem (404).

### Autenticação (público)

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/auth/register` | Cadastro de morador (`username`, `email`, `password`). Senha: ≥ 10 caracteres, letras e números, não comum, sem o e-mail ou o username. Após `REGISTER_MAX_PER_IP` cadastros na janela responde `429` com `Retry-After` |
| `POST` | `/auth/login` | Login por **e-mail**. App: `{ accessToken, refreshToken, roles }`. Web (`"useCookies": true`): tokens só em cookies httpOnly, e só para uma origem de `CORS_ALLOWED_ORIGINS` (senão `403`). Após `LOGIN_MAX_ATTEMPTS` falhas responde `429` com `Retry-After` |
| `POST` | `/auth/refresh` | Troca o refresh token (corpo no app, cookie `cg_refresh` no web) por um novo par; o antigo deixa de valer. Reusar um refresh token já trocado revoga a sessão |
| `POST` | `/auth/logout` | Revoga o refresh token e apaga os cookies |

### Usuários

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/users/me` | Conta do usuário autenticado (qualquer papel) |
| `GET` | `/users` | Listar usuários (`ROLE_ADMIN`) |
| `GET` | `/users/{id}` | Obter usuário (`ROLE_ADMIN`) |
| `POST` | `/users` | Criar usuário, inclusive admin (`ROLE_ADMIN`) |
| `PUT` | `/users/{id}` | Atualizar usuário, e-mail imutável (`ROLE_ADMIN`). Mudar os `roles` revoga os refresh tokens do usuário |
| `DELETE` | `/users/{id}` | Remover usuário (`ROLE_ADMIN`) |

### Residências

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/residents` | Listar residências |
| `GET` | `/residents/{id}` | Obter residência |
| `POST` | `/residents` | Criar residência (`ROLE_ADMIN`) |
| `PUT` | `/residents/{id}` | Atualizar residência (`ROLE_ADMIN`) |
| `DELETE` | `/residents/{id}` | Remover residência (`ROLE_ADMIN`) |

> Regra de negócio: `unitNumber` deve ser único dentro do condomínio.

### Lojas

`/shop-owners` é a rota oficial; `/shopOwners` continua aceito como sinônimo.

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/shop-owners` | Listar lojas |
| `GET` | `/shop-owners/{id}` | Obter loja |
| `POST` | `/shop-owners` | Criar loja, CNPJ obrigatório e validado (`ROLE_ADMIN`) |
| `PUT` | `/shop-owners/{id}` | Atualizar loja, CNPJ imutável (`ROLE_ADMIN`) |
| `DELETE` | `/shop-owners/{id}` | Remover loja (`ROLE_ADMIN`) |

### Despesas

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/expenses` | Listar despesas (aceita `?from=&to=` em RFC3339) |
| `GET` | `/expenses/{id}` | Obter despesa |
| `POST` | `/expenses` | Criar despesa (`ROLE_ADMIN`) |
| `PUT` | `/expenses/{id}` | Atualizar despesa (`ROLE_ADMIN`) |
| `DELETE` | `/expenses/{id}` | Remover despesa (`ROLE_ADMIN`) |

> Regra de negócio: `amountCents` deve ser positivo (inteiro, em centavos).  
> `dueDate` é obrigatório (formato RFC3339). A despesa é de uma unidade (`residentId`) ou de uma loja (`shopOwnerId`).

### Notificações

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/notifications` | Listar notificações (o morador vê as endereçadas às suas unidades e lojas) |
| `GET` | `/notifications/{id}` | Obter notificação, com o estado de leitura do usuário |
| `POST` | `/notifications` | Criar notificação (`ROLE_ADMIN`); o autor (`createdById`) é sempre o usuário do token |
| `PUT` | `/notifications/{id}` | Atualizar notificação (`ROLE_ADMIN`). Uma lista de destinatários presente substitui a atual, mesmo vazia; deve sobrar ao menos um |
| `DELETE` | `/notifications/{id}` | Remover notificação (`ROLE_ADMIN`) |
| `PUT` | `/notifications/{id}/read` | Marcar como lida para o próprio usuário (idempotente; cada destinatário tem o seu) |

### Observabilidade

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/health` | Health check — `{ "status": "ok" }` |
| `GET` | `/metrics` | Métricas expvar; exige `Authorization: Bearer $METRICS_TOKEN` (desligado sem ele) |

## Estrutura do projeto

```
api/
├── cmd/server/          — entry point (main.go)
├── cmd/migrate/         — goose up/down/reset/version
├── cmd/createadmin/     — cria o primeiro admin do condomínio
├── cmd/openapi/         — imprime o contrato OpenAPI (geração de tipos do web e do app)
├── internal/
│   ├── app/             — configuração, pool pgx e cliente Redis únicos, wiring dos handlers
│   ├── db/              — pool, migrações (goose), consultas sqlc e helper de testes (dbtest)
│   ├── tenant/          — condomínio (tenant) no contexto da requisição
│   ├── authz/           — papéis, quem chama e de quais unidades é dono
│   ├── auth/            — cadastro, login, refresh rotativo e logout
│   ├── user/            — /users e /users/me
│   ├── resident/        — CRUD /residents
│   ├── shopowner/       — CRUD /shop-owners
│   ├── expense/         — CRUD /expenses
│   ├── notification/    — CRUD /notifications + leitura por destinatário
│   ├── middleware/      — auth (bearer e cookie), CORS, limite de login, cabeçalhos de segurança, request ID, logging, métricas
│   └── parity/          — testes end-to-end de contrato e de segurança
├── pkg/
│   ├── email/           — normalização de e-mail
│   ├── jwt/             — geração e validação de tokens
│   ├── page/            — paginação
│   ├── password/        — política de senha e hash bcrypt
│   ├── openapi/         — contrato OpenAPI 3.1 e Swagger UI
│   └── ratelimit/       — contador de falhas (Redis e memória)
├── specs/               — specs de domínio e plano de migração
├── sqlc.yaml
├── docker-compose.yml
├── Makefile
└── .env.example
```

## Autenticação

- **App (Expo):** envie `Authorization: Bearer <accessToken>`. Quando o access
  token expirar (`ACCESS_TOKEN_TTL`, 15 min por padrão), chame
  `POST /auth/refresh` com o `refreshToken` e guarde o novo par.
- **Web (Next.js):** faça login com `"useCookies": true`. Os tokens ficam em
  cookies httpOnly (`cg_access` em `/`, `cg_refresh` só em `/auth`, ou `<COOKIE_PATH_PREFIX>/auth` atrás de proxy), nunca no
  `localStorage`. Requisições que alteram dados com cookie precisam vir de uma
  origem de `CORS_ALLOWED_ORIGINS` (proteção contra CSRF).

## Licença

[GNU General Public License v3.0](LICENSE)

---

**CondoGuard** — Simplificando a gestão do seu condomínio.

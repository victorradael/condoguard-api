<div style="text-align: center;">
    <img src="https://github.com/victorradael/condoguard/blob/main/assets/condoguard-logo.png?raw=true" alt="CondoguardLogo" height="200">
</div>

# CondoGuard API

REST API do CondoGuard — sistema de gestão financeira e administrativa de condomínios.

## Stack

| Camada | Tecnologia |
|--------|-----------|
| Linguagem | Go 1.25+ |
| HTTP | `net/http` (stdlib, Go 1.22+ method routing) |
| Banco de dados | PostgreSQL 17 (`pgx` + consultas tipadas com `sqlc`, migrações `goose`) |
| Cache / limites | Redis 7 (`go-redis`) — limite de tentativas de login |
| Autenticação | JWT HS256 (`github.com/golang-jwt/jwt/v5`) |
| Senhas | bcrypt (`golang.org/x/crypto`) |
| Logging | `log/slog` (stdlib) |
| Métricas | `expvar` (stdlib) |

## Pré-requisitos

- Go 1.25+
- Docker e Docker Compose (PostgreSQL e Redis locais)
- [`sqlc`](https://sqlc.dev) só para quem altera as consultas em `internal/db/queries/`

## Configuração

Copie `.env.example` para `.env` e preencha os valores:

```bash
cp .env.example .env
```

O compose **não tem segredos padrão**: `POSTGRES_PASSWORD`, `REDIS_PASSWORD` e
`JWT_SECRET_KEY` precisam estar no `.env` (gere com `openssl rand -base64 32`).

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `DATABASE_URL` | URL do PostgreSQL (`postgres://...`). Sem ela a API usa repositórios em memória (só para desenvolvimento) | — |
| `DB_MIGRATE_ON_START` | Aplica as migrações pendentes ao subir | `true` |
| `REDIS_URL` | URL do Redis (`redis://:senha@host:6379/0`). Sem ela o limite de login fica em memória (uma instância só) | — |
| `DEFAULT_CONDOMINIUM_ID` | Condomínio (tenant) de todas as consultas até a entidade condomínio existir (Fase 1b) | `default` |
| `LOGIN_MAX_ATTEMPTS` | Falhas de login por IP + usuário antes do bloqueio | `5` |
| `LOGIN_LOCKOUT_WINDOW` | Janela do bloqueio (duração Go, ex.: `15m`) | `15m` |
| `JWT_SECRET_KEY` | Segredo JWT em Base64 | — |
| `PORT` | Porta do servidor HTTP | `8080` |
| `CORS_ALLOWED_ORIGINS` | Origens permitidas, separadas por vírgula | `http://localhost:3000` |

## Rodando localmente

```bash
# Tudo em container (API + PostgreSQL + Redis); as migrações rodam ao subir
make up

# Ou: só PostgreSQL e Redis em container, API no host
make dev-up
DATABASE_URL=... REDIS_URL=... JWT_SECRET_KEY=... make run
```

### Banco de dados

- Migrações em `internal/db/migrations/` (goose, embutidas no binário).
  `make migrate-up`, `make migrate-down`, `make migrate-reset` e `make migrate-version` usam `DATABASE_URL`.
- Consultas em `internal/db/queries/*.sql`; o código Go é gerado em `internal/db/dbgen/` com `make sqlc` (o CI confere com `sqlc diff`).
- Toda tabela tem `condominium_id` obrigatório e toda consulta filtra por ele. O valor vem do contexto da requisição (`internal/tenant`); um repositório chamado sem tenant retorna erro. O Row-Level Security entra na Fase 1b.
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

## Endpoints

### Autenticação (público)

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/auth/register` | Registrar usuário |
| `POST` | `/auth/login` | Login — retorna `{ token, roles }`. Após `LOGIN_MAX_ATTEMPTS` falhas do mesmo IP + usuário responde `429` com `Retry-After` |

### Usuários (requer `ROLE_ADMIN`)

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/users` | Listar usuários |
| `GET` | `/users/{id}` | Obter usuário |
| `POST` | `/users` | Criar usuário |
| `PUT` | `/users/{id}` | Atualizar usuário (e-mail imutável) |
| `DELETE` | `/users/{id}` | Remover usuário |

### Residências (requer autenticação)

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/residents` | Listar residências |
| `GET` | `/residents/{id}` | Obter residência |
| `POST` | `/residents` | Criar residência |
| `PUT` | `/residents/{id}` | Atualizar residência |
| `DELETE` | `/residents/{id}` | Remover residência |

> Regra de negócio: `unitNumber` deve ser único dentro do mesmo `condominiumId`.

### Lojas (requer autenticação)

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/shopOwners` | Listar lojas |
| `GET` | `/shopOwners/{id}` | Obter loja |
| `POST` | `/shopOwners` | Criar loja (CNPJ obrigatório e validado) |
| `PUT` | `/shopOwners/{id}` | Atualizar loja (CNPJ imutável) |
| `DELETE` | `/shopOwners/{id}` | Remover loja |

### Despesas (requer autenticação)

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/expenses` | Listar despesas (aceita `?from=&to=` em RFC3339) |
| `GET` | `/expenses/{id}` | Obter despesa |
| `POST` | `/expenses` | Criar despesa |
| `PUT` | `/expenses/{id}` | Atualizar despesa |
| `DELETE` | `/expenses/{id}` | Remover despesa |

> Regra de negócio: `amountCents` deve ser positivo (inteiro, em centavos).  
> `dueDate` é obrigatório (formato RFC3339).

### Notificações (requer autenticação)

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/notifications` | Listar notificações |
| `GET` | `/notifications/{id}` | Obter notificação |
| `POST` | `/notifications` | Criar notificação |
| `PUT` | `/notifications/{id}` | Atualizar notificação |
| `DELETE` | `/notifications/{id}` | Remover notificação |
| `PUT` | `/notifications/{id}/read` | Marcar como lida (idempotente) |

### Observabilidade

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/health` | Health check — `{ "status": "ok" }` |
| `GET` | `/metrics` | Métricas expvar (requests, erros 5xx, latência) |

## Estrutura do projeto

```
api/
├── cmd/server/          — entry point (main.go)
├── cmd/migrate/         — goose up/down/reset/version
├── internal/
│   ├── app/             — configuração, pool pgx e cliente Redis únicos, wiring dos handlers
│   ├── db/              — pool, migrações (goose), consultas sqlc e helper de testes (dbtest)
│   ├── tenant/          — condomínio (tenant) no contexto da requisição
│   ├── auth/            — POST /auth/register, POST /auth/login
│   ├── user/            — CRUD /users
│   ├── resident/        — CRUD /residents
│   ├── shopowner/       — CRUD /shopOwners
│   ├── expense/         — CRUD /expenses
│   ├── notification/    — CRUD /notifications + mark-as-read
│   ├── middleware/      — JWT auth, request ID, logging, métricas
│   └── parity/          — testes de paridade end-to-end
├── pkg/
│   ├── jwt/             — geração e validação de tokens
│   ├── password/        — hash bcrypt e verificação
│   └── ratelimit/       — contador de falhas (Redis e memória)
├── specs/               — specs de domínio e plano de migração
├── sqlc.yaml
├── docker-compose.yml
├── Makefile
└── .env.example
```

## Autenticação

Todas as rotas protegidas exigem o header:

```
Authorization: Bearer <token>
```

O token é obtido via `POST /auth/login` e expira em **10 horas**.

## Licença

[GNU General Public License v3.0](LICENSE)

---

**CondoGuard** — Simplificando a gestão do seu condomínio.

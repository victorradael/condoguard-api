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
| Banco de dados | MongoDB 7 |
| Autenticação | JWT HS256 (`github.com/golang-jwt/jwt/v5`) |
| Senhas | bcrypt (`golang.org/x/crypto`) |
| Logging | `log/slog` (stdlib) |
| Métricas | `expvar` (stdlib) |

## Pré-requisitos

- Go 1.22+
- Docker e Docker Compose (para MongoDB local)

## Configuração

Copie `.env.example` para `.env` e preencha os valores:

```bash
cp .env.example .env
```

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `MONGODB_URI` | URI de conexão com o MongoDB | — |
| `JWT_SECRET_KEY` | Segredo JWT em Base64 | — |
| `PORT` | Porta do servidor HTTP | `8080` |
| `MONGO_DB` | Nome do banco de dados | `condoguard` |

## Rodando localmente

```bash
# Subir MongoDB
make dev-up

# Rodar o servidor
make run
```

## Testes

```bash
# Testes unitários (sem banco)
make test-unit

# Todos os testes (unitários + integração com banco)
make test-db-up   # sobe MongoDB de teste na porta 27018
MONGODB_URI=mongodb://root:secret@localhost:27018/condoguard_test?authSource=admin \
JWT_SECRET_KEY=dGVzdA== \
make test

# Cobertura
make test-cover
```

## Endpoints

### Autenticação (público)

| Método | Rota | Descrição |
|--------|------|-----------|
| `POST` | `/auth/register` | Registrar usuário |
| `POST` | `/auth/login` | Login — retorna `{ token, roles }` |

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
├── internal/
│   ├── app/             — NewRouter (wiring de todos os handlers)
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
│   └── password/        — hash bcrypt e verificação
├── specs/               — specs de domínio e plano de migração
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

# CondoGuard API — Plano de Migração Java → Go

## Contexto

O backend atual é uma API REST em Spring Boot 3 / Java 17 com MongoDB e autenticação JWT. O objetivo é reescrever esse backend em Go, mantendo total paridade funcional, usando práticas modernas da linguagem e orientando toda a construção por testes.

---

## Princípios Guia

- **Test-First**: nenhuma funcionalidade é implementada sem testes escritos antes.
- **Paridade funcional**: o contrato HTTP (endpoints, payloads, códigos de status) deve ser idêntico ao da API Java.
- **Sem over-engineering**: usar a biblioteca padrão do Go ao máximo; dependências externas apenas quando justificadas.
- **Separação de responsabilidades**: handler → service → repository. Cada camada testável de forma isolada.
- **Configuração por ambiente**: via variáveis de ambiente, sem valores hardcoded.

---

## Fases da Migração

### Fase 0 — Fundação e Setup

**Objetivo:** estrutura do projeto, toolchain e pipeline de qualidade prontos antes de qualquer código de negócio.

Tarefas:
1. Inicializar módulo Go (`go mod init`).
2. Definir estrutura de diretórios do projeto.
3. Configurar linter (`golangci-lint`) e formatter (`gofmt`).
4. Configurar pipeline CI com execução de testes e lint.
5. Configurar `docker-compose` para MongoDB de desenvolvimento e testes.
6. Escrever testes de smoke (servidor sobe, rota `/health` responde 200).

Critério de aceite: `go test ./...` passa, lint sem erros, container MongoDB disponível.

---

### Fase 1 — Autenticação (Auth)

**Regras de negócio críticas:**
- Registro exige e-mail único, senha com hash (bcrypt).
- Login retorna JWT com claims de `user_id` e `role`.
- Token inválido ou expirado retorna 401.
- Rotas protegidas exigem token no header `Authorization: Bearer <token>`.

**Ordem de construção:**
1. Escrever testes unitários para geração e validação de JWT.
2. Escrever testes unitários para hash e verificação de senha.
3. Escrever testes de integração para `POST /auth/register`.
4. Escrever testes de integração para `POST /auth/login`.
5. Escrever testes do middleware de autenticação (token ausente, inválido, expirado).
6. Implementar os handlers, services e middleware para passar os testes.

Endpoints: `POST /auth/register`, `POST /auth/login`.

---

### Fase 2 — Usuários (Users)

**Regras de negócio críticas:**
- Apenas usuários autenticados acessam rotas de usuário.
- `DELETE` de usuário que não existe retorna 404.
- E-mail é imutável após criação.

**Ordem de construção:**
1. Escrever testes unitários para lógica de validação de usuário.
2. Escrever testes de integração para cada endpoint (incluindo casos de erro).
3. Implementar CRUD completo para passar os testes.

Endpoints: `GET /users`, `GET /users/:id`, `POST /users`, `PUT /users/:id`, `DELETE /users/:id`.

---

### Fase 3 — Residências (Residents)

**Regras de negócio críticas:**
- Uma residência pertence a exatamente um condomínio.
- Número de unidade deve ser único dentro do condomínio.

**Ordem de construção:**
1. Testes unitários para validação de unicidade de unidade.
2. Testes de integração para CRUD completo.
3. Implementação.

Endpoints: `GET /residents`, `GET /residents/:id`, `POST /residents`, `PUT /residents/:id`, `DELETE /residents/:id`.

---

### Fase 4 — Lojas (ShopOwners)

**Regras de negócio críticas:**
- Lojas têm atributos distintos de residências (CNPJ, razão social).
- Validação de formato de CNPJ deve ocorrer na camada de service.

**Ordem de construção:**
1. Testes unitários para validação de CNPJ.
2. Testes de integração para CRUD completo.
3. Implementação.

Endpoints: `GET /shopOwners`, `GET /shopOwners/:id`, `POST /shopOwners`, `PUT /shopOwners/:id`, `DELETE /shopOwners/:id`.

---

### Fase 5 — Despesas (Expenses)

**Regras de negócio críticas:**
- Despesa está vinculada a uma unidade (residente ou loja).
- Valor deve ser positivo e em centavos (inteiro) para evitar imprecisão de float.
- Data de vencimento é obrigatória.
- Filtro por período de data deve funcionar corretamente.

**Ordem de construção:**
1. Testes unitários para validação de valor e data.
2. Testes de integração para criação, listagem com filtro e atualização de status.
3. Implementação.

Endpoints: `GET /expenses`, `GET /expenses/:id`, `POST /expenses`, `PUT /expenses/:id`, `DELETE /expenses/:id`.

---

### Fase 6 — Notificações (Notifications)

**Regras de negócio críticas:**
- Notificação tem destinatário (usuário ou grupo) e status de leitura.
- Marcar como lida é idempotente.

**Ordem de construção:**
1. Testes unitários para transição de status (não lida → lida).
2. Testes de integração para CRUD e marcação de leitura.
3. Implementação.

Endpoints: `GET /notifications`, `GET /notifications/:id`, `POST /notifications`, `PUT /notifications/:id`, `DELETE /notifications/:id`.

---

### Fase 7 — Hardening e Observabilidade

Tarefas:
1. Structured logging com `slog` (stdlib Go 1.21+).
2. Middleware de request ID para rastreamento.
3. Métricas básicas (latência, contagem de erros) via `expvar` ou Prometheus.
4. Documentação OpenAPI gerada a partir de anotações ou spec manual.
5. Revisão de cobertura de testes — mínimo 80% nas camadas de service.
6. Testes de carga básicos para validar performance aceitável.

---

### Fase 8 — Validação de Paridade e Cutover

Tarefas:
1. Executar a collection Insomnia existente contra a nova API Go e validar todas as respostas.
2. Comparar schemas de resposta JSON com a API Java em produção.
3. Validar comportamento de erros (4xx, 5xx) para garantir compatibilidade com os clientes web e mobile.
4. Documentar breaking changes, se houver, e comunicar aos times de frontend.
5. Definir estratégia de cutover (feature flag, DNS switch, ou deploy paralelo).

---

## Estrutura de Diretórios Proposta

```
api/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── auth/
│   │   ├── handler.go
│   │   ├── handler_test.go
│   │   ├── service.go
│   │   └── service_test.go
│   ├── user/
│   ├── resident/
│   ├── shopowner/
│   ├── expense/
│   ├── notification/
│   └── middleware/
├── pkg/
│   ├── jwt/
│   ├── password/
│   └── mongodb/
├── specs/
├── .env.example
├── docker-compose.yml
├── go.mod
└── go.sum
```

---

## Dependências Externas Previstas

| Pacote | Finalidade | Justificativa |
|--------|-----------|---------------|
| `go.mongodb.org/mongo-driver` | Driver MongoDB | Necessário, sem alternativa nativa |
| `github.com/golang-jwt/jwt/v5` | JWT | Padrão da comunidade Go para JWT |
| `golang.org/x/crypto` | bcrypt | Hash seguro de senhas |

Router HTTP: avaliar `net/http` com `ServeMux` do Go 1.22+ (suporte a method routing nativo) antes de adotar um framework externo.

---

## Critérios de Qualidade

- `go vet ./...` sem erros em todo commit.
- `golangci-lint run` sem erros críticos.
- Cobertura de testes: mínimo 80% em `internal/`.
- Nenhum `panic` em código de produção; erros explícitos sempre.
- Nenhuma goroutine leak nos testes (verificar com `goleak`).
- Tempo de resposta p99 < 200ms para operações CRUD simples.

---

## Convenções de Código

- Erros sempre tratados explicitamente — proibido `_` para ignorar erros.
- Tipos de domínio definidos em cada pacote; sem structs genéricas compartilhadas entre domínios.
- Interfaces definidas no lado do consumidor (princípio Go idiomático).
- Testes de integração usam banco de dados real (MongoDB em container) — sem mocks de banco.
- Mocks apenas para dependências externas de terceiros (e.g., serviço de e-mail futuro).
- Contexto (`context.Context`) propagado em todas as chamadas de I/O.

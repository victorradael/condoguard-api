# CondoGuard API — Relatório de Paridade e Plano de Cutover

## Contexto

Este documento registra as diferenças entre a API Java/Spring Boot (origem) e a
API Go (destino), classifica cada breaking change por impacto nos clientes web e
mobile, e define a estratégia de cutover.

---

## 1. Paridade de Endpoints

Todos os endpoints da collection Insomnia estão implementados na API Go:

| Método | Rota | Java ✓ | Go ✓ |
|--------|------|--------|-------|
| POST | `/auth/register` | ✓ | ✓ |
| POST | `/auth/login` | ✓ | ✓ |
| GET/POST/PUT/DELETE | `/users/{id?}` | ✓ | ✓ |
| GET/POST/PUT/DELETE | `/residents/{id?}` | ✓ | ✓ |
| GET/POST/PUT/DELETE | `/shopOwners/{id?}` | ✓ | ✓ |
| GET/POST/PUT/DELETE | `/expenses/{id?}` | ✓ | ✓ |
| GET/POST/PUT/DELETE | `/notifications/{id?}` | ✓ | ✓ |
| PUT | `/notifications/{id}/read` | ✗ | ✓ (novo) |
| GET | `/health` | ✗ | ✓ (novo) |
| GET | `/metrics` | ✗ | ✓ (novo) |

---

## 2. Breaking Changes

### 2.1 Expenses — campo `amount` → `amountCents` (BREAKING)

| | Java | Go |
|---|---|---|
| Campo | `amount` (double) | `amountCents` (int64) |
| Valor | `100.0` | `10000` |
| Tipo JSON | number (float) | number (integer) |

**Impacto:** clientes que enviam `amount: 100.0` receberão `amountCents: 0` (campo ignorado).
**Ação necessária:** atualizar todos os payloads de criação/atualização de despesas nos clientes.

**Justificativa:** `double` acumula erros de ponto flutuante em valores monetários.
Centavos como `int64` é a prática correta para sistemas financeiros.

---

### 2.2 Expenses — campo `date` → `dueDate` (BREAKING)

| | Java | Go |
|---|---|---|
| Campo de criação | `date` | `dueDate` |
| Campo de resposta | `date` | `dueDate` |
| Formato | ISO 8601 (Date) | RFC 3339 (UTC) |

**Impacto:** o campo `date` é ignorado na API Go. Clientes devem usar `dueDate`.

---

### 2.3 Expenses — associação `resident`/`shopOwner` → `residentId`/`shopOwnerId` (BREAKING)

| | Java | Go |
|---|---|---|
| Payload criação | `{ "resident": { "id": "..." } }` | `{ "residentId": "..." }` |
| Payload criação | `{ "shopOwner": { "id": "..." } }` | `{ "shopOwnerId": "..." }` |
| Resposta | objeto `resident` embedded | string `residentId` |

**Impacto:** a API Go não expande objetos nested — retorna apenas IDs.
**Ação necessária:** adaptar payloads de criação e parsers de resposta.

---

### 2.4 Residents — campo `owner` → `ownerId` + campo novo `condominiumId` (BREAKING)

| | Java | Go |
|---|---|---|
| Payload criação | `{ "owner": { "id": "..." } }` | `{ "ownerId": "..." }` |
| Campo novo | ausente | `condominiumId` (obrigatório) |
| Resposta | objeto `owner` embedded | string `ownerId` |

**Impacto (owner):** adaptar payload de criação.
**Impacto (condominiumId):** campo obrigatório novo — clientes sem esse campo recebem 422.
**Ação necessária:** adicionar `condominiumId` em todas as criações de residentes.

---

### 2.5 ShopOwners — campo `owner` → `ownerId` + campo novo `cnpj` (BREAKING)

| | Java | Go |
|---|---|---|
| Payload criação | `{ "owner": { "id": "..." } }` | `{ "ownerId": "..." }` |
| Campo novo | ausente | `cnpj` (obrigatório, validado) |
| Resposta | objeto `owner` embedded | string `ownerId` |

**Impacto (owner):** adaptar payload.
**Impacto (cnpj):** campo obrigatório novo com validação — lojas sem CNPJ não podem ser criadas.

---

### 2.6 Notifications — campo `createdBy` → `createdById` + listas de IDs (BREAKING)

| | Java | Go |
|---|---|---|
| Payload | `{ "createdBy": { "id": "..." } }` | `{ "createdById": "..." }` |
| Destinatários | `residents: [{ "id": "..." }]` | `residentIds: ["..."]` |
| Destinatários | `shopOwners: [{ "id": "..." }]` | `shopOwnerIds: ["..."]` |
| Campo novo | ausente | `read` (bool), `readAt` (*time) |

**Impacto:** payloads precisam usar strings simples em vez de objetos nested.
**Novo recurso:** `PUT /notifications/{id}/read` — operação idempotente para marcar como lida.

---

### 2.7 Users — campo `roles` é array em vez de Set (não breaking)

Java serializa `Set<String>` — a ordem dos elementos não é garantida.
Go serializa `[]string` — semânticamente equivalente para clientes JSON.
Nenhuma ação necessária.

---

### 2.8 Autenticação — token usa claim `user_id` em vez de `sub` apenas

A API Go inclui `user_id` como claim customizado além do `subject` padrão.
A validação de token continua idêntica (HS256, mesma chave, 10h de expiração).
Não há impacto em clientes que apenas enviam o token no header.

---

### 2.9 Erros — formato padronizado `{ "error": "mensagem" }` (não breaking para maioria)

Java retorna corpo de erro dependente do Spring (`timestamp`, `status`, `message`, etc.).
Go retorna `{ "error": "descrição" }` em todos os casos de erro.
Clientes que parseiam o corpo de erro da API Java precisam adaptar o parser.

---

## 3. Novas Funcionalidades (aditivas, sem impacto em clientes existentes)

| Recurso | Descrição |
|---------|-----------|
| `PUT /notifications/{id}/read` | Marca notificação como lida (idempotente) |
| `GET /health` | Health check para load balancers |
| `GET /metrics` | Métricas via expvar (requests, erros, latência) |
| `X-Request-Id` header | Rastreamento de requisição ponta a ponta |
| `GET /expenses?from=&to=` | Filtro de despesas por período de vencimento |
| CNPJ validado | Validação do dígito verificador na criação de lojas |
| Unicidade de unidade por condomínio | Índice composto impedindo duplicatas |

---

## 4. Plano de Cutover

### Estratégia: Deploy Paralelo com DNS Switch

```
Fase A — Deploy paralelo
  ┌─────────────┐     ┌───────────────────┐
  │  Java :8080 │     │  Go :8081 (novo)  │
  └─────────────┘     └───────────────────┘
         ↑                      ↑
     clientes              testes de validação
     existentes             + monitoramento

Fase B — Traffic split (load balancer)
  10% → Go API    (monitorar erros, latência)
  90% → Java API

Fase C — Cutover total
  100% → Go API
  Java API em standby por 72h

Fase D — Descomissionamento
  Remover containers Java
  Arquivar código Spring Boot
```

### Checklist pré-cutover

- [ ] Todos os testes de paridade passando com `MONGODB_URI` real
- [ ] Clientes web atualizados para os novos campos de payload (ver seção 2)
- [ ] Clientes mobile atualizados para os novos campos de payload
- [ ] `condominiumId` adicionado em todas as criações de residente
- [ ] `cnpj` adicionado em todas as criações de loja
- [ ] `amountCents` substituindo `amount` nas despesas
- [ ] `dueDate` substituindo `date` nas despesas
- [ ] Parsers de resposta adaptados para campos de ID simples
- [ ] Variáveis de ambiente configuradas: `MONGODB_URI`, `JWT_SECRET_KEY`, `PORT`
- [ ] `JWT_SECRET_KEY` idêntica entre Java e Go (mesma chave — tokens existentes continuam válidos)
- [ ] Docker image do servidor Go publicada no registry
- [ ] Alertas de erro 5xx configurados no load balancer

### Rollback

Se erros 5xx > 1% em 5 minutos após cutover:
1. Reverter DNS/load balancer para Java API (< 60s)
2. Investigar logs via `X-Request-Id`
3. Corrigir, rodar suite de testes de paridade, repetir cutover

---

## 5. Compatibilidade de Dados (MongoDB)

A API Go usa as **mesmas coleções** que a API Java (`users`, `residents`, `shopOwners`,
`expenses`, `notifications`). Os documentos existentes são lidos sem migração para campos
como `id`, `username`, `email`, `password`, `roles`, `unitNumber`, `floor`, `message`,
`shopName`.

Campos novos introduzidos pela API Go (`condominiumId`, `cnpj`, `amountCents`, `dueDate`,
`residentIds`, `shopOwnerIds`, `createdById`, `read`, `readAt`) serão `null` ou ausentes
em documentos antigos — a API Go os tratará como zero-values e não retornará erros de leitura.

**Recomendação:** executar script de migração para preencher `condominiumId` com um valor
padrão nos documentos de `residents` existentes antes do cutover.

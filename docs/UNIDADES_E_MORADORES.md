# Blocos, unidades, pessoas e vínculos (1c)

Contrato da API para o web (síndico e administradora) e para o app (morador). Todas as rotas exigem
`Authorization: Bearer <accessToken>`, exceto `/auth/invitations/*`. Listas devolvem
`{items, page, pageSize, total}` (`?page=&pageSize=`); erros, `{error}`.

## Modelo

- **Bloco** (`blocks`): torre ou bloco do condomínio. O nome é único no condomínio.
- **Unidade** (`residents`, rotas `/units` e o alias `/residents`): `unitNumber` é único **dentro do bloco**
  (o mesmo "101" existe em blocos diferentes). Unidade sem bloco é permitida (um número por condomínio nesse caso).
  `condominiumId` e `ownerId` são campos antigos, opcionais; o dono vem dos vínculos.
- **Pessoa** (`people`): nome, CPF, e-mail, telefone. Pode ter uma conta (`hasAccount`), criada ao aceitar um convite.
- **Vínculo** (`unit_links`): pessoa ↔ unidade, com papel `proprietario | coproprietario | inquilino | morador | dependente`,
  `startsOn` e `endsOn`. **`endsOn` é exclusivo** (primeiro dia sem o vínculo): a troca no mesmo dia não se sobrepõe. Vigente =
  `startsOn <= hoje < endsOn` (dia no fuso de São Paulo). O banco garante (EXCLUDE) que a unidade nunca tem dois `proprietario`
  no mesmo período e que a mesma pessoa não repete o mesmo papel no mesmo período. Vários `coproprietario`, `inquilino`, `morador`
  e `dependente` são permitidos.
- **Responsável pela unidade**: quem tem vínculo vigente `proprietario`, `coproprietario` ou `inquilino`. Só o responsável cadastra
  `morador` e `dependente` pelo app.
- **Convite** (`invitations`): token de 160 bits (8 grupos de 4 caracteres), guardado só em hash, vale 7 dias, uso único.

## Rotas

### Blocos — `units.read_all` lê, `units.write` altera
| Rota | |
|---|---|
| `GET /blocks`, `GET /blocks/{id}` | `{id, name, unitCount, createdAt}` |
| `POST /blocks` `{name}` → 201; `PUT /blocks/{id}` `{name}` | 409 se o nome já existe; 422 se vazio ou > 80 |
| `DELETE /blocks/{id}` → 204 | 409 se ainda tem unidades |

### Unidades — `/units` (alias `/residents`)
`GET /units?blockId=&sort=number` · `POST /units {unitNumber, floor, blockId?}` · `GET|PUT|DELETE /units/{id}`.
`PUT` aceita `blockId` (`""` tira do bloco). A resposta traz `blockId` e `blockName`. `sort=number` ordena por bloco e número
(o padrão é a ordem de criação). Morador só enxerga as unidades onde tem vínculo vigente (menos `dependente`); as outras dão 404.

### Pessoas
| Rota | Quem | |
|---|---|---|
| `GET /people?q=&unitId=&active=true` | `people.read_all`; morador: só a casa dele | busca por trecho do nome |
| `POST /people` `{fullName, cpf?, email?, phone?}` → 201 | `people.write` | CPF com dígitos verificadores; 409 se o CPF já existe no condomínio |
| `GET /people/{id}` | idem | traz `units[]` (vínculos; o histórico completo só para `people.read_contact`) |
| `PUT /people/{id}`, `DELETE /people/{id}` | `people.write` | `DELETE` → 409 se a pessoa tem algum vínculo (o histórico fica) |

O que cada papel vê de uma pessoa: **porteiro** nome e unidades vigentes; **conselho** + e-mail, telefone, `hasAccount` e CPF mascarado
(`***.982.247-**`); **síndico/administradora** tudo, CPF inteiro; **morador** a própria ficha inteira e, dos outros moradores
das suas unidades, nome, e-mail e telefone (sem CPF). O que o papel não pode ver responde **404**.

### Vínculos e trocas
| Rota | Quem | |
|---|---|---|
| `GET /units/{id}/links?active=true` | `people.read_contact`: histórico todo; porteiro e quem mora lá: só os vigentes | `{items:[{id, unitId, personId, personName, kind, startsOn, endsOn?, active}]}`, mais novo primeiro |
| `POST /units/{id}/links` `{personId \| person:{fullName,cpf?,email?,phone?}, kind, startsOn?, endsOn?}` → 201 | `people.write`; **responsável** só `morador`/`dependente` e só com `person` novo | `startsOn` padrão = hoje. 409 se violar uma regra do banco. Um CPF já cadastrado: o síndico reaproveita a pessoa; o morador recebe 409 sem saber de quem é |
| `PUT /links/{id}/end` `{endsOn?}` | `people.write`; responsável só `morador`/`dependente` | `endsOn` > `startsOn` (padrão hoje) |
| `DELETE /links/{id}` → 204 | idem | só vínculo que ainda não começou antes de hoje (erro de cadastro); senão 409 |
| `POST /units/{id}/transfers` `{kind: "proprietario"\|"inquilino", effectiveDate?, personId \| person}` → 201 | `people.write` | encerra os vigentes (no caso do dono, também os coproprietários) em `effectiveDate` e cria o novo, numa transação: `{created, ended[]}`. 409 se há vínculo do mesmo tipo começando em/depois da data |

Quem não tem vínculo nenhum com a unidade e não lê unidades recebe 404; porteiro e conselho recebem 403 nas escritas.

### Convites
| Rota | Quem | |
|---|---|---|
| `POST /people/{id}/invitations` `{channel: "email"\|"link"}` → 201 | `people.write` | A pessoa precisa de e-mail e não ter conta. Cada convite novo revoga os pendentes. `email`: o link vai só para o e-mail da pessoa (Mailpit no ambiente local). `link`: a resposta traz `{token, link}` **uma vez**, para o síndico repassar (WhatsApp) |
| `GET /people/{id}/invitations` | `people.write` | últimos 20, com `status` (`pending`, `accepted`, `revoked`, `expired`) |
| `POST /auth/invitations/inspect` `{token}` | público (limite por IP) | `{condominiumName, personName, email, accountExists}`; 400 para token inválido, usado, revogado ou expirado |
| `POST /auth/invitations/accept` `{token, username?, password}` → 201 | público (limite por IP) | Conta nova: `password` segue a política de senhas. E-mail que já tem conta: `password` é a dela (401 se errada). Dá o papel `morador` no condomínio. `{condominiumId, email, verifyEmail}` |

Convite por `email` prova a posse do e-mail (a conta nasce verificada). Convite por `link` não prova: a API envia o e-mail de confirmação
(`verifyEmail: true`) e, com `REQUIRE_VERIFIED_EMAIL`, o login espera a confirmação. Link no e-mail: `WEB_BASE_URL/convite?token=…`; o mesmo
código vale para digitar no app.

### Importação por CSV — `units.write` e `people.write`
`POST /imports/preview` e `POST /imports`, corpo `text/csv` (até 1 MiB e 5000 linhas, UTF-8; separador `;` ou `,`).

```
bloco;unidade;andar;papel;nome;cpf;email;telefone;inicio
Torre A;101;1;proprietario;Ana Souza;529.982.247-25;ana@exemplo.test;(11) 90000-1111;2020-01-01
Torre A;101;1;morador;Bia Lima;;;;
Torre A;102;1;;;;;;
```

Uma linha = uma pessoa vinculada a uma unidade; linha só com bloco/unidade/andar cria a unidade. As colunas podem vir em qualquer ordem
(`unidade` é obrigatória; aceita os apelidos `torre`, `apto`, `vinculo`, `celular`, `desde`…); `inicio` aceita `AAAA-MM-DD` ou `DD/MM/AAAA`
(padrão: hoje). Pessoas com o mesmo CPF (ou mesmo e-mail e nome) são a mesma pessoa; reimportar o arquivo não duplica nada
(linhas já cadastradas voltam como `unchanged`).

Resposta (`preview` 200; `commit` 201): `{valid, rows, newBlocks, newUnits, newPeople, newLinks, unchanged, errorCount, errors:[{line,field,message}], preview:[{line,block,unit,kind,person,action}], truncated}`.
A importação é **tudo ou nada**: com erros, `POST /imports` responde 422 `{error, report}` e não grava nada. `errors` traz até 500 problemas e
`preview` as 200 primeiras linhas (`truncated: true` quando há mais).

### Perfil do morador (app)
`GET /me/profile` → `{person, units:[{linkId, unitId, unitNumber, blockName, kind, startsOn, active, canManageHousehold}]}`;
`GET /me/units` → `{items}` (as mesmas unidades; mais de uma = o app oferece a troca de unidade);
`PUT /me/profile {fullName, phone}` (e-mail é o login, CPF só o síndico muda). 404 se a conta não tem ficha de pessoa (ex.: o primeiro síndico).
O morador responsável (`canManageHousehold`) cadastra os demais com `POST /units/{id}/links` e `kind` `morador` ou `dependente`.

## Auditoria

Toda mudança grava em `audit_log` na mesma transação: `block`, `person` (CPF mascarado), `unit_link`, `unit_transfer`, `invitation`
(sem o token), `import` (só contagens). Leitura: `GET /audit-log?resource=`.

## Dados fictícios (`is_demo`)

Todas as tabelas de dados têm `is_demo`. O gerador (`cmd/seed`, `make seed|seed-poc|reset-demo`, ver `docs/DADOS_FICTICIOS.md`) marca o que cria e o `reset-demo`
apaga só condomínios marcados — nunca um condomínio real.

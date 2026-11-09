package openapi

// NewSpec constructs and returns the complete CondoGuard OpenAPI 3.1 spec.
// It is called once at startup and the result is cached for the lifetime
// of the process.
func NewSpec() *Spec {
	return &Spec{
		OpenAPI: "3.1.0",
		Info: Info{
			Title: "CondoGuard API",
			Description: "REST API para gestão financeira e administrativa de condomínios.\n\n" +
				"**Autenticação.** O app envia o access token em `Authorization: Bearer`; o web usa os cookies " +
				"httpOnly `cg_access`/`cg_refresh` (login com `useCookies: true`). Com cookie, requisições que " +
				"alteram dados precisam vir de uma origem permitida (cabeçalho `Origin`).\n\n" +
				"**Papéis.** `ROLE_ADMIN` administra o condomínio e vê tudo nele. `ROLE_USER` (morador/lojista) " +
				"só lê os registros das próprias unidades e lojas; o que ele não pode ver responde 404. " +
				"Nada de outro condomínio é visível.\n\n" +
				"**Listas** são paginadas: `?page=1&pageSize=20` (máx. 100) → `{items, page, pageSize, total}`.",
			Version: "1.1.0",
			License: &License{
				Name: "GNU GPL v3.0",
				URL:  "https://www.gnu.org/licenses/gpl-3.0.html",
			},
		},
		Servers: []Server{
			{URL: "/", Description: "Current server"},
		},
		Tags: []Tag{
			{Name: "Auth", Description: "Cadastro, login e sessão"},
			{Name: "Users", Description: "Gestão de usuários (ROLE_ADMIN), exceto /users/me"},
			{Name: "Residents", Description: "Unidades residenciais"},
			{Name: "ShopOwners", Description: "Lojas"},
			{Name: "Expenses", Description: "Despesas"},
			{Name: "Notifications", Description: "Notificações"},
			{Name: "System", Description: "Health check"},
		},
		Components: buildComponents(),
		Paths:      buildPaths(),
	}
}

// ── Components ────────────────────────────────────────────────────────────────

func buildComponents() Components {
	return Components{
		SecuritySchemes: map[string]*SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "Access token (JWT) de POST /auth/login ou /auth/refresh. Expira em 15 minutos.",
			},
			"cookieAuth": {
				Type:        "apiKey",
				In:          "cookie",
				Name:        "cg_access",
				Description: "Cookie httpOnly definido pelo login com useCookies=true (web).",
			},
		},
		Schemas: buildSchemas(),
	}
}

func buildSchemas() map[string]*Schema {
	str := func(desc string) *Schema { return &Schema{Type: "string", Description: desc} }
	strFmt := func(desc, format string) *Schema {
		return &Schema{Type: "string", Format: format, Description: desc}
	}
	intg := func(desc string) *Schema { return &Schema{Type: "integer", Description: desc} }
	int64s := func(desc string) *Schema { return &Schema{Type: "integer", Format: "int64", Description: desc} }
	boolean := func(desc string) *Schema { return &Schema{Type: "boolean", Description: desc} }
	strArr := func(desc string) *Schema {
		return &Schema{Type: "array", Items: &Schema{Type: "string"}, Description: desc}
	}
	roles := &Schema{Type: "array", Items: &Schema{Type: "string", Enum: []any{"ROLE_USER", "ROLE_ADMIN"}}, Description: "Papéis"}
	password := &Schema{Type: "string", MinLength: ptr(10), Description: "Senha: 10+ caracteres, até 72 bytes, com letras e números; não pode conter o e-mail ou o username"}

	return map[string]*Schema{
		// ── Generic ──────────────────────────────────────────────────────────
		"Error": {
			Type: "object",
			Properties: map[string]*Schema{
				"error": str("Mensagem de erro"),
			},
			Required: []string{"error"},
		},
		"Health": {
			Type: "object",
			Properties: map[string]*Schema{
				"status": {Type: "string", Example: "ok"},
			},
			Required: []string{"status"},
		},

		// ── Auth ─────────────────────────────────────────────────────────────
		"RegisterRequest": {
			Type:        "object",
			Description: "O cadastro público sempre cria um morador (ROLE_USER). Papéis enviados aqui são ignorados.",
			Properties: map[string]*Schema{
				"username": str("Nome de exibição"),
				"email":    strFmt("E-mail (login); único, sem diferenciar maiúsculas", "email"),
				"password": password,
			},
			Required: []string{"username", "email", "password"},
		},
		"RegisterResponse": {
			Type: "object",
			Properties: map[string]*Schema{
				"message": str("Confirmação de registro"),
			},
			Required: []string{"message"},
			Example:  map[string]any{"message": "User registered successfully!"},
		},
		"LoginRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"email":      strFmt("E-mail", "email"),
				"password":   str("Senha"),
				"useCookies": boolean("true no web: os tokens vão só para cookies httpOnly e não voltam no corpo"),
			},
			Required: []string{"email", "password"},
		},
		"RefreshRequest": {
			Type:        "object",
			Description: "No web o corpo pode ser vazio: o refresh token vai no cookie cg_refresh.",
			Properties: map[string]*Schema{
				"refreshToken": str("Refresh token recebido no login ou no último refresh"),
			},
		},
		"TokenPair": {
			Type:        "object",
			Description: "accessToken e refreshToken só vêm no fluxo do app; no fluxo com cookies ficam de fora.",
			Properties: map[string]*Schema{
				"accessToken":  str("JWT — enviar em Authorization: Bearer <token>"),
				"refreshToken": str("Opaco, de uso único: cada refresh devolve um novo e invalida o anterior"),
				"tokenType":    {Type: "string", Enum: []any{"Bearer"}},
				"expiresIn":    intg("Validade do access token, em segundos"),
				"roles":        roles,
			},
			Required: []string{"tokenType", "expiresIn", "roles"},
		},

		// ── User ─────────────────────────────────────────────────────────────
		"User": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":       str("ID único do usuário"),
				"username": str("Nome de exibição"),
				"email":    strFmt("E-mail do usuário", "email"),
				"roles":    roles,
			},
			Required: []string{"id", "username", "email", "roles"},
		},
		"CreateUserRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"username": str("Nome de exibição"),
				"email":    strFmt("E-mail", "email"),
				"password": password,
				"roles":    roles,
			},
			Required: []string{"username", "email", "password"},
		},
		"UpdateUserRequest": {
			Type:        "object",
			Description: "Apenas username e roles podem ser alterados. E-mail é imutável.",
			Properties: map[string]*Schema{
				"username": str("Novo nome de exibição"),
				"roles":    roles,
			},
		},

		// ── Resident ─────────────────────────────────────────────────────────
		"Resident": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":            str("ID único da residência"),
				"unitNumber":    str("Número da unidade (único por condomínio)"),
				"floor":         intg("Andar"),
				"condominiumId": str("Bloco/condomínio informado no cadastro"),
				"ownerId":       str("ID do usuário proprietário"),
			},
			Required: []string{"id", "unitNumber", "floor", "condominiumId", "ownerId"},
		},
		"CreateResidentRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"unitNumber":    str("Número da unidade"),
				"floor":         intg("Andar"),
				"condominiumId": str("Bloco/condomínio"),
				"ownerId":       str("ID do proprietário"),
			},
			Required: []string{"unitNumber", "condominiumId", "ownerId"},
		},
		"UpdateResidentRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"unitNumber": str("Novo número da unidade"),
				"floor":      intg("Novo andar"),
			},
		},

		// ── ShopOwner ─────────────────────────────────────────────────────────
		"ShopOwner": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":       str("ID único da loja"),
				"shopName": str("Razão social / nome fantasia"),
				"cnpj":     str("CNPJ no formato XX.XXX.XXX/XXXX-XX"),
				"floor":    intg("Andar"),
				"ownerId":  str("ID do usuário responsável"),
			},
			Required: []string{"id", "shopName", "cnpj", "floor", "ownerId"},
		},
		"CreateShopOwnerRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"shopName": str("Nome da loja"),
				"cnpj":     str("CNPJ — raw (14 dígitos) ou formatado; validado pelo servidor"),
				"floor":    intg("Andar"),
				"ownerId":  str("ID do responsável"),
			},
			Required: []string{"shopName", "cnpj", "ownerId"},
		},
		"UpdateShopOwnerRequest": {
			Type:        "object",
			Description: "CNPJ é imutável após criação.",
			Properties: map[string]*Schema{
				"shopName": str("Novo nome"),
				"floor":    intg("Novo andar"),
			},
		},

		// ── Expense ───────────────────────────────────────────────────────────
		"Expense": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":          str("ID único da despesa"),
				"description": str("Descrição"),
				"amountCents": int64s("Valor em centavos (inteiro, >= 0)"),
				"dueDate":     strFmt("Data de vencimento (RFC3339 UTC)", "date-time"),
				"residentId":  str("ID da residência vinculada; \"\" se for de loja"),
				"shopOwnerId": str("ID da loja vinculada; \"\" se for de residência"),
			},
			Required: []string{"id", "description", "amountCents", "dueDate", "residentId", "shopOwnerId"},
		},
		"CreateExpenseRequest": {
			Type:        "object",
			Description: "Exige residentId ou shopOwnerId (ao menos um).",
			Properties: map[string]*Schema{
				"description": str("Descrição da despesa"),
				"amountCents": int64s("Valor em centavos (>= 0)"),
				"dueDate":     strFmt("Data de vencimento (RFC3339)", "date-time"),
				"residentId":  str("ID da residência (opcional)"),
				"shopOwnerId": str("ID da loja (opcional)"),
			},
			Required: []string{"description", "amountCents", "dueDate"},
		},
		"UpdateExpenseRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"description": str("Nova descrição"),
				"amountCents": int64s("Novo valor em centavos (> 0)"),
				"dueDate":     strFmt("Nova data de vencimento (RFC3339)", "date-time"),
			},
		},

		// ── Notification ──────────────────────────────────────────────────────
		"Notification": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":           str("ID único da notificação"),
				"message":      str("Conteúdo da mensagem"),
				"createdById":  str("ID do usuário que criou (vem do token, nunca do corpo)"),
				"createdAt":    strFmt("Data de criação (RFC3339)", "date-time"),
				"read":         boolean("Se o usuário autenticado já leu"),
				"readAt":       {Type: "string", Format: "date-time", Nullable: true, Description: "Quando o usuário autenticado leu"},
				"residentIds":  strArr("IDs das residências destinatárias"),
				"shopOwnerIds": strArr("IDs das lojas destinatárias"),
			},
			Required: []string{"id", "message", "createdById", "createdAt", "read", "residentIds", "shopOwnerIds"},
		},
		"CreateNotificationRequest": {
			Type:        "object",
			Description: "Exige ao menos um destinatário (residentIds ou shopOwnerIds). O autor é o usuário autenticado.",
			Properties: map[string]*Schema{
				"message":      str("Conteúdo da mensagem"),
				"residentIds":  strArr("IDs das residências destinatárias"),
				"shopOwnerIds": strArr("IDs das lojas destinatárias"),
			},
			Required: []string{"message"},
		},
		"UpdateNotificationRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"message":      str("Nova mensagem"),
				"residentIds":  strArr("Novos destinatários (residências)"),
				"shopOwnerIds": strArr("Novos destinatários (lojas)"),
			},
		},

		// ── Pages ─────────────────────────────────────────────────────────────
		"UserPage":         pageSchema("User"),
		"ResidentPage":     pageSchema("Resident"),
		"ShopOwnerPage":    pageSchema("ShopOwner"),
		"ExpensePage":      pageSchema("Expense"),
		"NotificationPage": pageSchema("Notification"),
	}
}

// ── Paths ─────────────────────────────────────────────────────────────────────

// crud describes one resource for crudPaths.
type crud struct {
	tag, schema, noun string // e.g. "Residents", "Resident", "residência"
	opID              string // e.g. "Resident" → listResidents, createResident...
	createDesc        string
	conflict          string // 409 description on create/update; "" for none
	extraListParams   []Parameter
}

// crudPaths builds the collection and item paths of a resource.
func crudPaths(c crud) (collection, item PathItem) {
	idParam := pathParam("id", "ID do recurso")
	forbidden := errResp("ROLE_ADMIN necessário")
	notFound := errResp(c.noun + " não encontrada(o), ou de outro usuário/condomínio")
	unauth := errResp("Não autenticado")

	create := map[string]*Response{
		"201": jsonResp("Criado", c.schema),
		"401": unauth,
		"403": forbidden,
		"422": errResp("Dados inválidos"),
	}
	update := map[string]*Response{
		"200": jsonResp("Atualizado", c.schema),
		"401": unauth,
		"403": forbidden,
		"404": notFound,
		"422": errResp("Dados inválidos"),
	}
	if c.conflict != "" {
		create["409"] = errResp(c.conflict)
		update["409"] = errResp(c.conflict)
	}

	collection = PathItem{
		"get": {
			Tags:        []string{c.tag},
			Summary:     "Listar " + c.tag,
			Description: "Admin: todos do condomínio. Usuário: só os seus.",
			OperationID: "list" + c.opID + "s",
			Security:    bearerSecurity,
			Parameters:  append(append([]Parameter{}, pageParams...), c.extraListParams...),
			Responses: map[string]*Response{
				"200": jsonPageResp("Página de "+c.tag, c.schema),
				"400": errResp("Parâmetro inválido"),
				"401": unauth,
			},
		},
		"post": {
			Tags:        []string{c.tag},
			Summary:     "Criar (ROLE_ADMIN)",
			Description: c.createDesc,
			OperationID: "create" + c.opID,
			Security:    bearerSecurity,
			RequestBody: jsonBody("Create"+c.schema+"Request", true),
			Responses:   create,
		},
	}
	item = PathItem{
		"get": {
			Tags:        []string{c.tag},
			Summary:     "Obter por ID",
			OperationID: "get" + c.opID + "ByID",
			Security:    bearerSecurity,
			Parameters:  []Parameter{idParam},
			Responses: map[string]*Response{
				"200": jsonResp("Encontrado", c.schema),
				"401": unauth,
				"404": notFound,
			},
		},
		"put": {
			Tags:        []string{c.tag},
			Summary:     "Atualizar (ROLE_ADMIN)",
			OperationID: "update" + c.opID,
			Security:    bearerSecurity,
			Parameters:  []Parameter{idParam},
			RequestBody: jsonBody("Update"+c.schema+"Request", true),
			Responses:   update,
		},
		"delete": {
			Tags:        []string{c.tag},
			Summary:     "Remover (ROLE_ADMIN)",
			OperationID: "delete" + c.opID,
			Security:    bearerSecurity,
			Parameters:  []Parameter{idParam},
			Responses: map[string]*Response{
				"204": noContentResp(),
				"401": unauth,
				"403": forbidden,
				"404": notFound,
			},
		},
	}
	return collection, item
}

func buildPaths() map[string]PathItem {
	idParam := pathParam("id", "ID do recurso")
	public := []SecurityRequirement{} // explicit empty: no auth

	residents, resident := crudPaths(crud{
		tag: "Residents", schema: "Resident", noun: "Residência", opID: "Resident",
		createDesc: "unitNumber deve ser único dentro do mesmo condominiumId.",
		conflict:   "Número de unidade já existe neste condomínio",
	})
	shops, shop := crudPaths(crud{
		tag: "ShopOwners", schema: "ShopOwner", noun: "Loja", opID: "ShopOwner",
		createDesc: "CNPJ é validado (dígito verificador) e normalizado para XX.XXX.XXX/XXXX-XX. " +
			"/shopOwners continua aceito como sinônimo de /shop-owners.",
		conflict: "CNPJ já cadastrado",
	})
	expenses, expense := crudPaths(crud{
		tag: "Expenses", schema: "Expense", noun: "Despesa", opID: "Expense",
		createDesc: "amountCents >= 0. Exige residentId ou shopOwnerId.",
		extraListParams: []Parameter{
			{Name: "from", In: "query", Description: "Vencimento a partir de (RFC3339)", Schema: &Schema{Type: "string", Format: "date-time"}},
			{Name: "to", In: "query", Description: "Vencimento até (RFC3339)", Schema: &Schema{Type: "string", Format: "date-time"}},
		},
	})
	notifications, notification := crudPaths(crud{
		tag: "Notifications", schema: "Notification", noun: "Notificação", opID: "Notification",
		createDesc: "Exige ao menos um destinatário. createdById é o usuário autenticado. " +
			"Usuários só veem as notificações endereçadas às suas unidades e lojas.",
	})

	users, userItem := crudPaths(crud{
		tag: "Users", schema: "User", noun: "Usuário", opID: "User",
		conflict: "E-mail já cadastrado",
	})
	users["get"].Summary, users["get"].Description = "Listar usuários (ROLE_ADMIN)", ""
	users["get"].Responses["403"] = errResp("ROLE_ADMIN necessário")
	userItem["get"].Summary = "Obter usuário por ID (ROLE_ADMIN)"
	userItem["get"].Responses["403"] = errResp("ROLE_ADMIN necessário")

	return map[string]PathItem{
		// ── System ───────────────────────────────────────────────────────────
		"/health": {
			"get": {
				Tags:        []string{"System"},
				Summary:     "Health check",
				OperationID: "getHealth",
				Security:    public,
				Responses: map[string]*Response{
					"200": jsonResp("Servidor disponível", "Health"),
				},
			},
		},

		// ── Auth ─────────────────────────────────────────────────────────────
		"/auth/register": {
			"post": {
				Tags:        []string{"Auth"},
				Summary:     "Cadastrar morador",
				Description: "Cria sempre um ROLE_USER. Administradores são criados por um admin em POST /users.",
				OperationID: "authRegister",
				Security:    public,
				RequestBody: jsonBody("RegisterRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Usuário registrado", "RegisterResponse"),
					"400": errResp("JSON inválido"),
					"409": errResp("E-mail já cadastrado"),
					"413": errResp("Corpo maior que 1 MiB"),
					"422": errResp("Dados inválidos ou senha fora da política"),
				},
			},
		},
		"/auth/login": {
			"post": {
				Tags:        []string{"Auth"},
				Summary:     "Entrar (e-mail e senha)",
				Description: "Abre uma sessão. Com useCookies=true define os cookies httpOnly cg_access e cg_refresh.",
				OperationID: "authLogin",
				Security:    public,
				RequestBody: jsonBody("LoginRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Login bem-sucedido", "TokenPair"),
					"400": errResp("Corpo inválido: precisa ser um único objeto JSON de até 64 KiB"),
					"401": errResp("Credenciais inválidas"),
					"422": errResp("Dados inválidos"),
					"429": errResp("Muitas tentativas com falha (por e-mail e IP, ou só por IP); tente após o tempo do cabeçalho Retry-After"),
				},
			},
		},
		"/auth/refresh": {
			"post": {
				Tags:    []string{"Auth"},
				Summary: "Renovar a sessão (refresh rotativo)",
				Description: "Troca o refresh token por um par novo; o antigo deixa de valer. Reusar um refresh já trocado " +
					"revoga a sessão inteira. Sem refreshToken no corpo, usa o cookie cg_refresh (exige Origin permitido).",
				OperationID: "authRefresh",
				Security:    public,
				RequestBody: jsonBody("RefreshRequest", false),
				Responses: map[string]*Response{
					"200": jsonResp("Novo par", "TokenPair"),
					"401": errResp("Refresh inválido, expirado ou revogado"),
					"403": errResp("Cookie enviado de uma origem não permitida"),
				},
			},
		},
		"/auth/logout": {
			"post": {
				Tags:        []string{"Auth"},
				Summary:     "Sair",
				Description: "Revoga a sessão do refresh token (corpo ou cookie) e apaga os cookies. Sempre 204.",
				OperationID: "authLogout",
				Security:    public,
				RequestBody: jsonBody("RefreshRequest", false),
				Responses: map[string]*Response{
					"204": noContentResp(),
					"403": errResp("Cookie enviado de uma origem não permitida"),
				},
			},
		},

		// ── Users ─────────────────────────────────────────────────────────────
		"/users/me": {
			"get": {
				Tags:        []string{"Users"},
				Summary:     "Minha conta",
				Description: "Disponível para qualquer usuário autenticado.",
				OperationID: "getMe",
				Security:    bearerSecurity,
				Responses: map[string]*Response{
					"200": jsonResp("Usuário autenticado", "User"),
					"401": errResp("Não autenticado"),
				},
			},
		},
		"/users":      users,
		"/users/{id}": userItem,

		// ── Domain resources ──────────────────────────────────────────────────
		"/residents":          residents,
		"/residents/{id}":     resident,
		"/shop-owners":        shops,
		"/shop-owners/{id}":   shop,
		"/expenses":           expenses,
		"/expenses/{id}":      expense,
		"/notifications":      notifications,
		"/notifications/{id}": notification,
		"/notifications/{id}/read": {
			"put": {
				Tags:        []string{"Notifications"},
				Summary:     "Marcar como lida (para mim)",
				Description: "A leitura é por destinatário. Idempotente: chamar de novo não altera o readAt.",
				OperationID: "markNotificationAsRead",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Notificação com read=true para o usuário autenticado", "Notification"),
					"401": errResp("Não autenticado"),
					"404": errResp("Notificação não encontrada, ou não endereçada a você"),
				},
			},
		},
	}
}

package openapi

// NewSpec constructs and returns the complete CondoGuard OpenAPI 3.1 spec.
// It is called once at startup and the result is cached for the lifetime
// of the process.
func NewSpec() *Spec {
	return &Spec{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       "CondoGuard API",
			Description: "REST API para gestão financeira e administrativa de condomínios.",
			Version:     "1.0.0",
			License: &License{
				Name: "GNU GPL v3.0",
				URL:  "https://www.gnu.org/licenses/gpl-3.0.html",
			},
		},
		Servers: []Server{
			{URL: "/", Description: "Current server"},
		},
		Tags: []Tag{
			{Name: "Auth", Description: "Registro e autenticação"},
			{Name: "Users", Description: "Gestão de usuários (ROLE_ADMIN)"},
			{Name: "Residents", Description: "Gestão de residências"},
			{Name: "ShopOwners", Description: "Gestão de lojas"},
			{Name: "Expenses", Description: "Gestão de despesas"},
			{Name: "Notifications", Description: "Gestão de notificações"},
			{Name: "System", Description: "Health check e métricas"},
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
				Description:  "Token JWT obtido via POST /auth/login. Expira em 10 horas.",
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

	return map[string]*Schema{
		// ── Generic ──────────────────────────────────────────────────────────
		"Error": {
			Type: "object",
			Properties: map[string]*Schema{
				"error": str("Mensagem de erro"),
			},
			Required: []string{"error"},
		},

		// ── Auth ─────────────────────────────────────────────────────────────
		"RegisterRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"username": str("Nome de usuário único"),
				"email":    strFmt("Endereço de e-mail", "email"),
				"password": str("Senha em texto plano (será armazenada com bcrypt)"),
				"roles":    strArr("Lista de papéis (ex: ROLE_USER, ROLE_ADMIN)"),
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
				"username": str("Nome de usuário"),
				"password": str("Senha"),
			},
			Required: []string{"username", "password"},
		},
		"LoginResponse": {
			Type: "object",
			Properties: map[string]*Schema{
				"token": str("Token JWT — enviar em Authorization: Bearer <token>"),
				"roles": strArr("Papéis do usuário autenticado"),
			},
			Required: []string{"token", "roles"},
		},

		// ── User ─────────────────────────────────────────────────────────────
		"User": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":       str("ID único do usuário"),
				"username": str("Nome de usuário"),
				"email":    strFmt("E-mail do usuário", "email"),
				"roles":    strArr("Papéis do usuário"),
			},
			Required: []string{"id", "username", "email", "roles"},
		},
		"CreateUserRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"username": str("Nome de usuário único"),
				"email":    strFmt("E-mail", "email"),
				"password": str("Senha"),
				"roles":    strArr("Papéis"),
			},
			Required: []string{"username", "email", "password"},
		},
		"UpdateUserRequest": {
			Type:        "object",
			Description: "Apenas username e roles podem ser alterados. E-mail é imutável.",
			Properties: map[string]*Schema{
				"username": str("Novo nome de usuário"),
				"roles":    strArr("Novos papéis"),
			},
		},

		// ── Resident ─────────────────────────────────────────────────────────
		"Resident": {
			Type: "object",
			Properties: map[string]*Schema{
				"id":            str("ID único da residência"),
				"unitNumber":    str("Número da unidade (único por condomínio)"),
				"floor":         intg("Andar"),
				"condominiumId": str("ID do condomínio ao qual pertence"),
				"ownerId":       str("ID do usuário proprietário"),
			},
			Required: []string{"id", "unitNumber", "floor", "condominiumId", "ownerId"},
		},
		"CreateResidentRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"unitNumber":    str("Número da unidade"),
				"floor":         intg("Andar"),
				"condominiumId": str("ID do condomínio"),
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
				"cnpj":     {Type: "string", Format: "string", Description: "CNPJ no formato XX.XXX.XXX/XXXX-XX"},
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
				"amountCents": int64s("Valor em centavos (inteiro positivo)"),
				"dueDate":     strFmt("Data de vencimento (RFC3339 UTC)", "date-time"),
				"residentId":  {Type: "string", Nullable: true, Description: "ID da residência vinculada (opcional)"},
				"shopOwnerId": {Type: "string", Nullable: true, Description: "ID da loja vinculada (opcional)"},
			},
			Required: []string{"id", "description", "amountCents", "dueDate"},
		},
		"CreateExpenseRequest": {
			Type:        "object",
			Description: "Exige residentId ou shopOwnerId (ao menos um).",
			Properties: map[string]*Schema{
				"description": str("Descrição da despesa"),
				"amountCents": int64s("Valor em centavos (> 0)"),
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
				"createdById":  str("ID do usuário que criou"),
				"createdAt":    strFmt("Data de criação (RFC3339)", "date-time"),
				"read":         boolean("Se a notificação foi lida"),
				"readAt":       {Type: "string", Format: "date-time", Nullable: true, Description: "Momento em que foi marcada como lida"},
				"residentIds":  strArr("IDs das residências destinatárias"),
				"shopOwnerIds": strArr("IDs das lojas destinatárias"),
			},
			Required: []string{"id", "message", "createdById", "createdAt", "read"},
		},
		"CreateNotificationRequest": {
			Type:        "object",
			Description: "Exige ao menos um destinatário (residentIds ou shopOwnerIds).",
			Properties: map[string]*Schema{
				"message":      str("Conteúdo da mensagem"),
				"createdById":  str("ID do usuário que está criando"),
				"residentIds":  strArr("IDs das residências destinatárias"),
				"shopOwnerIds": strArr("IDs das lojas destinatárias"),
			},
			Required: []string{"message", "createdById"},
		},
		"UpdateNotificationRequest": {
			Type: "object",
			Properties: map[string]*Schema{
				"message":      str("Nova mensagem"),
				"residentIds":  strArr("Novos destinatários (residências)"),
				"shopOwnerIds": strArr("Novos destinatários (lojas)"),
			},
		},
	}
}

// ── Paths ─────────────────────────────────────────────────────────────────────

func buildPaths() map[string]PathItem {
	idParam := pathParam("id", "ID do recurso")

	return map[string]PathItem{
		// ── System ───────────────────────────────────────────────────────────
		"/health": {
			"get": {
				Tags:        []string{"System"},
				Summary:     "Health check",
				OperationID: "getHealth",
				Responses: map[string]*Response{
					"200": {
						Description: "Servidor disponível",
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"status": {Type: "string", Example: "ok"},
									},
								},
							},
						},
					},
				},
			},
		},

		// ── Auth ─────────────────────────────────────────────────────────────
		"/auth/register": {
			"post": {
				Tags:        []string{"Auth"},
				Summary:     "Registrar novo usuário",
				OperationID: "authRegister",
				Security:    []SecurityRequirement{}, // public — explicit empty
				RequestBody: jsonBody("RegisterRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Usuário registrado", "RegisterResponse"),
					"409": errResp("E-mail já cadastrado"),
					"422": errResp("Dados inválidos"),
				},
			},
		},
		"/auth/login": {
			"post": {
				Tags:        []string{"Auth"},
				Summary:     "Autenticar e obter token JWT",
				OperationID: "authLogin",
				Security:    []SecurityRequirement{},
				RequestBody: jsonBody("LoginRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Login bem-sucedido", "LoginResponse"),
					"401": errResp("Credenciais inválidas"),
					"422": errResp("Dados inválidos"),
				},
			},
		},

		// ── Users ─────────────────────────────────────────────────────────────
		"/users": {
			"get": {
				Tags:        []string{"Users"},
				Summary:     "Listar usuários",
				OperationID: "listUsers",
				Security:    bearerSecurity,
				Responses: map[string]*Response{
					"200": jsonArrayResp("Lista de usuários", "User"),
					"401": errResp("Não autenticado"),
					"403": errResp("ROLE_ADMIN necessário"),
				},
			},
			"post": {
				Tags:        []string{"Users"},
				Summary:     "Criar usuário",
				OperationID: "createUser",
				Security:    bearerSecurity,
				RequestBody: jsonBody("CreateUserRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Usuário criado", "User"),
					"401": errResp("Não autenticado"),
					"403": errResp("ROLE_ADMIN necessário"),
					"409": errResp("E-mail já cadastrado"),
					"422": errResp("Dados inválidos"),
				},
			},
		},
		"/users/{id}": {
			"get": {
				Tags:        []string{"Users"},
				Summary:     "Obter usuário por ID",
				OperationID: "getUserByID",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Usuário encontrado", "User"),
					"401": errResp("Não autenticado"),
					"403": errResp("ROLE_ADMIN necessário"),
					"404": errResp("Usuário não encontrado"),
				},
			},
			"put": {
				Tags:        []string{"Users"},
				Summary:     "Atualizar usuário",
				Description: "E-mail é imutável após criação.",
				OperationID: "updateUser",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				RequestBody: jsonBody("UpdateUserRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Usuário atualizado", "User"),
					"401": errResp("Não autenticado"),
					"403": errResp("ROLE_ADMIN necessário"),
					"404": errResp("Usuário não encontrado"),
				},
			},
			"delete": {
				Tags:        []string{"Users"},
				Summary:     "Remover usuário",
				OperationID: "deleteUser",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"204": noContentResp(),
					"401": errResp("Não autenticado"),
					"403": errResp("ROLE_ADMIN necessário"),
					"404": errResp("Usuário não encontrado"),
				},
			},
		},

		// ── Residents ─────────────────────────────────────────────────────────
		"/residents": {
			"get": {
				Tags:        []string{"Residents"},
				Summary:     "Listar residências",
				OperationID: "listResidents",
				Security:    bearerSecurity,
				Responses: map[string]*Response{
					"200": jsonArrayResp("Lista de residências", "Resident"),
					"401": errResp("Não autenticado"),
				},
			},
			"post": {
				Tags:        []string{"Residents"},
				Summary:     "Criar residência",
				Description: "unitNumber deve ser único dentro do mesmo condominiumId.",
				OperationID: "createResident",
				Security:    bearerSecurity,
				RequestBody: jsonBody("CreateResidentRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Residência criada", "Resident"),
					"401": errResp("Não autenticado"),
					"409": errResp("Número de unidade já existe neste condomínio"),
					"422": errResp("Dados inválidos"),
				},
			},
		},
		"/residents/{id}": {
			"get": {
				Tags:        []string{"Residents"},
				Summary:     "Obter residência por ID",
				OperationID: "getResidentByID",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Residência encontrada", "Resident"),
					"401": errResp("Não autenticado"),
					"404": errResp("Residência não encontrada"),
				},
			},
			"put": {
				Tags:        []string{"Residents"},
				Summary:     "Atualizar residência",
				OperationID: "updateResident",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				RequestBody: jsonBody("UpdateResidentRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Residência atualizada", "Resident"),
					"401": errResp("Não autenticado"),
					"404": errResp("Residência não encontrada"),
					"409": errResp("Número de unidade já existe neste condomínio"),
				},
			},
			"delete": {
				Tags:        []string{"Residents"},
				Summary:     "Remover residência",
				OperationID: "deleteResident",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"204": noContentResp(),
					"401": errResp("Não autenticado"),
					"404": errResp("Residência não encontrada"),
				},
			},
		},

		// ── ShopOwners ─────────────────────────────────────────────────────────
		"/shopOwners": {
			"get": {
				Tags:        []string{"ShopOwners"},
				Summary:     "Listar lojas",
				OperationID: "listShopOwners",
				Security:    bearerSecurity,
				Responses: map[string]*Response{
					"200": jsonArrayResp("Lista de lojas", "ShopOwner"),
					"401": errResp("Não autenticado"),
				},
			},
			"post": {
				Tags:        []string{"ShopOwners"},
				Summary:     "Criar loja",
				Description: "CNPJ é validado (dígito verificador) e normalizado para XX.XXX.XXX/XXXX-XX.",
				OperationID: "createShopOwner",
				Security:    bearerSecurity,
				RequestBody: jsonBody("CreateShopOwnerRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Loja criada", "ShopOwner"),
					"401": errResp("Não autenticado"),
					"409": errResp("CNPJ já cadastrado"),
					"422": errResp("CNPJ inválido ou dados ausentes"),
				},
			},
		},
		"/shopOwners/{id}": {
			"get": {
				Tags:        []string{"ShopOwners"},
				Summary:     "Obter loja por ID",
				OperationID: "getShopOwnerByID",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Loja encontrada", "ShopOwner"),
					"401": errResp("Não autenticado"),
					"404": errResp("Loja não encontrada"),
				},
			},
			"put": {
				Tags:        []string{"ShopOwners"},
				Summary:     "Atualizar loja",
				Description: "CNPJ é imutável após criação.",
				OperationID: "updateShopOwner",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				RequestBody: jsonBody("UpdateShopOwnerRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Loja atualizada", "ShopOwner"),
					"401": errResp("Não autenticado"),
					"404": errResp("Loja não encontrada"),
				},
			},
			"delete": {
				Tags:        []string{"ShopOwners"},
				Summary:     "Remover loja",
				OperationID: "deleteShopOwner",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"204": noContentResp(),
					"401": errResp("Não autenticado"),
					"404": errResp("Loja não encontrada"),
				},
			},
		},

		// ── Expenses ──────────────────────────────────────────────────────────
		"/expenses": {
			"get": {
				Tags:        []string{"Expenses"},
				Summary:     "Listar despesas",
				Description: "Suporta filtro por período: ?from=2026-01-01T00:00:00Z&to=2026-01-31T23:59:59Z",
				OperationID: "listExpenses",
				Security:    bearerSecurity,
				Parameters: []Parameter{
					{
						Name: "from", In: "query",
						Description: "Data de início do filtro (RFC3339)",
						Schema:      &Schema{Type: "string", Format: "date-time"},
					},
					{
						Name: "to", In: "query",
						Description: "Data de fim do filtro (RFC3339)",
						Schema:      &Schema{Type: "string", Format: "date-time"},
					},
				},
				Responses: map[string]*Response{
					"200": jsonArrayResp("Lista de despesas", "Expense"),
					"400": errResp("Parâmetro de data inválido"),
					"401": errResp("Não autenticado"),
				},
			},
			"post": {
				Tags:        []string{"Expenses"},
				Summary:     "Criar despesa",
				Description: "amountCents deve ser > 0. Exige residentId ou shopOwnerId.",
				OperationID: "createExpense",
				Security:    bearerSecurity,
				RequestBody: jsonBody("CreateExpenseRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Despesa criada", "Expense"),
					"401": errResp("Não autenticado"),
					"422": errResp("Dados inválidos (valor negativo, data ausente, sem unidade)"),
				},
			},
		},
		"/expenses/{id}": {
			"get": {
				Tags:        []string{"Expenses"},
				Summary:     "Obter despesa por ID",
				OperationID: "getExpenseByID",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Despesa encontrada", "Expense"),
					"401": errResp("Não autenticado"),
					"404": errResp("Despesa não encontrada"),
				},
			},
			"put": {
				Tags:        []string{"Expenses"},
				Summary:     "Atualizar despesa",
				OperationID: "updateExpense",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				RequestBody: jsonBody("UpdateExpenseRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Despesa atualizada", "Expense"),
					"401": errResp("Não autenticado"),
					"404": errResp("Despesa não encontrada"),
					"422": errResp("Dados inválidos"),
				},
			},
			"delete": {
				Tags:        []string{"Expenses"},
				Summary:     "Remover despesa",
				OperationID: "deleteExpense",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"204": noContentResp(),
					"401": errResp("Não autenticado"),
					"404": errResp("Despesa não encontrada"),
				},
			},
		},

		// ── Notifications ──────────────────────────────────────────────────────
		"/notifications": {
			"get": {
				Tags:        []string{"Notifications"},
				Summary:     "Listar notificações",
				OperationID: "listNotifications",
				Security:    bearerSecurity,
				Responses: map[string]*Response{
					"200": jsonArrayResp("Lista de notificações", "Notification"),
					"401": errResp("Não autenticado"),
				},
			},
			"post": {
				Tags:        []string{"Notifications"},
				Summary:     "Criar notificação",
				Description: "Exige ao menos um destinatário (residentIds ou shopOwnerIds).",
				OperationID: "createNotification",
				Security:    bearerSecurity,
				RequestBody: jsonBody("CreateNotificationRequest", true),
				Responses: map[string]*Response{
					"201": jsonResp("Notificação criada", "Notification"),
					"401": errResp("Não autenticado"),
					"422": errResp("Mensagem ausente ou sem destinatários"),
				},
			},
		},
		"/notifications/{id}": {
			"get": {
				Tags:        []string{"Notifications"},
				Summary:     "Obter notificação por ID",
				OperationID: "getNotificationByID",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Notificação encontrada", "Notification"),
					"401": errResp("Não autenticado"),
					"404": errResp("Notificação não encontrada"),
				},
			},
			"put": {
				Tags:        []string{"Notifications"},
				Summary:     "Atualizar notificação",
				OperationID: "updateNotification",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				RequestBody: jsonBody("UpdateNotificationRequest", true),
				Responses: map[string]*Response{
					"200": jsonResp("Notificação atualizada", "Notification"),
					"401": errResp("Não autenticado"),
					"404": errResp("Notificação não encontrada"),
				},
			},
			"delete": {
				Tags:        []string{"Notifications"},
				Summary:     "Remover notificação",
				OperationID: "deleteNotification",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"204": noContentResp(),
					"401": errResp("Não autenticado"),
					"404": errResp("Notificação não encontrada"),
				},
			},
		},
		"/notifications/{id}/read": {
			"put": {
				Tags:        []string{"Notifications"},
				Summary:     "Marcar notificação como lida",
				Description: "Operação idempotente. Chamar múltiplas vezes não altera o readAt.",
				OperationID: "markNotificationAsRead",
				Security:    bearerSecurity,
				Parameters:  []Parameter{idParam},
				Responses: map[string]*Response{
					"200": jsonResp("Estado atual da notificação (read=true)", "Notification"),
					"401": errResp("Não autenticado"),
					"404": errResp("Notificação não encontrada"),
				},
			},
		},
	}
}

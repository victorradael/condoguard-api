// Package openapi builds and serves an OpenAPI 3.1 specification for the
// CondoGuard API. The spec is constructed in Go code — no annotations,
// no code generation, no external dependencies.
package openapi

// Spec is the root OpenAPI 3.1 document.
type Spec struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Servers    []Server              `json:"servers,omitempty"`
	Paths      map[string]PathItem   `json:"paths"`
	Components Components            `json:"components"`
	Security   []SecurityRequirement `json:"security,omitempty"`
	Tags       []Tag                 `json:"tags,omitempty"`
}

type Info struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version"`
	License     *License `json:"license,omitempty"`
}

type License struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem maps HTTP methods (lowercase) to Operations.
type PathItem map[string]*Operation

type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses"`
	Security    []SecurityRequirement `json:"security,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // path | query | header | cookie
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Schema is a subset of JSON Schema used in OpenAPI 3.1.
type Schema struct {
	Ref         string             `json:"$ref,omitempty"`
	Type        string             `json:"type,omitempty"`
	Format      string             `json:"format,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Enum        []any              `json:"enum,omitempty"`
	Example     any                `json:"example,omitempty"`
	Nullable    bool               `json:"nullable,omitempty"`
	Minimum     *float64           `json:"minimum,omitempty"`
	Maximum     *float64           `json:"maximum,omitempty"`
	MinLength   *int               `json:"minLength,omitempty"`
	MaxLength   *int               `json:"maxLength,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	In           string `json:"in,omitempty"`   // apiKey only: header | query | cookie
	Name         string `json:"name,omitempty"` // apiKey only
	Description  string `json:"description,omitempty"`
}

// SecurityRequirement maps scheme name → scopes (empty for JWT).
type SecurityRequirement map[string][]string

// ── Helpers ───────────────────────────────────────────────────────────────────

// ref returns a $ref schema pointing to a named component schema.
func ref(name string) *Schema {
	return &Schema{Ref: "#/components/schemas/" + name}
}

// arrayOf returns an array schema whose items are a $ref.
func arrayOf(name string) *Schema {
	return &Schema{Type: "array", Items: ref(name)}
}

// jsonBody wraps a schema in an application/json request body.
func jsonBody(schemaName string, required bool) *RequestBody {
	return &RequestBody{
		Required: required,
		Content:  map[string]MediaType{"application/json": {Schema: ref(schemaName)}},
	}
}

// jsonResp wraps a schema in an application/json response.
func jsonResp(description, schemaName string) *Response {
	r := &Response{Description: description}
	if schemaName != "" {
		r.Content = map[string]MediaType{"application/json": {Schema: ref(schemaName)}}
	}
	return r
}

// jsonPageResp returns a paginated list response of the named schema
// (component "<schemaName>Page").
func jsonPageResp(description, schemaName string) *Response {
	return jsonResp(description, schemaName+"Page")
}

// errResp returns a response using the generic Error schema.
func errResp(description string) *Response {
	return jsonResp(description, "Error")
}

// noContentResp returns a 204 No Content response.
func noContentResp() *Response {
	return &Response{Description: "No content"}
}

// bearerSecurity is the security requirement for JWT-protected routes:
// the access token as a bearer header (app) or in the httpOnly cookie (web).
var bearerSecurity = []SecurityRequirement{{"bearerAuth": []string{}}, {"cookieAuth": []string{}}}

// pageParams are the pagination query parameters of every list endpoint.
var pageParams = []Parameter{
	{Name: "page", In: "query", Description: "Página (começa em 1)", Schema: &Schema{Type: "integer", Minimum: ptr(1.0)}},
	{Name: "pageSize", In: "query", Description: "Itens por página (1 a 100, padrão 20)", Schema: &Schema{Type: "integer", Minimum: ptr(1.0), Maximum: ptr(100.0)}},
}

func ptr[T any](v T) *T { return &v }

// pageSchema is the list envelope {items, page, pageSize, total} of itemSchema.
func pageSchema(itemSchema string) *Schema {
	return &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"items":    arrayOf(itemSchema),
			"page":     {Type: "integer", Description: "Página atual"},
			"pageSize": {Type: "integer", Description: "Itens por página"},
			"total":    {Type: "integer", Format: "int64", Description: "Total de itens em todas as páginas"},
		},
		Required: []string{"items", "page", "pageSize", "total"},
	}
}

// pathParam builds a required path parameter schema.
func pathParam(name, description string) Parameter {
	return Parameter{
		Name:        name,
		In:          "path",
		Description: description,
		Required:    true,
		Schema:      &Schema{Type: "string"},
	}
}

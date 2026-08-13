package model

type OpenAPIDoc struct {
	OpenAPI string                     `json:"openapi"`
	Info    OpenAPIInfo                `json:"info"`
	Servers []OpenAPIServer            `json:"servers"`
	Paths   map[string]OpenAPIPathItem `json:"paths"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// OpenAPIPathItem maps HTTP method → operation (e.g. "get" → OpenAPIOperation)
type OpenAPIPathItem map[string]*OpenAPIOperation

type OpenAPIOperation struct {
	OperationId string                      `json:"operationId,omitempty"`
	Tags        []string                    `json:"tags,omitempty"`
	Summary     string                      `json:"summary,omitempty"`
	Description string                      `json:"description,omitempty"`
	Parameters  []*OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*OpenAPIResponse `json:"responses"`
}

type OpenAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"` // query | path | header
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Schema      *OpenAPISchema `json:"schema,omitempty"`
}

type OpenAPIRequestBody struct {
	Description string                       `json:"description,omitempty"`
	Required    bool                         `json:"required,omitempty"`
	Content     map[string]*OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPIMediaType struct {
	Schema *OpenAPISchema `json:"schema,omitempty"`
}

type OpenAPISchema struct {
	Type        string                    `json:"type,omitempty"`
	Format      string                    `json:"format,omitempty"`
	Description string                    `json:"description,omitempty"`
	Example     any                       `json:"example,omitempty"`
	Enum        []any                     `json:"enum,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Properties  map[string]*OpenAPISchema `json:"properties,omitempty"`
	Items       *OpenAPISchema            `json:"items,omitempty"`
}

type OpenAPIResponse struct {
	Description string                       `json:"description"`
	Content     map[string]*OpenAPIMediaType `json:"content,omitempty"`
}

package model

import "encoding/json"

type TransformConfig struct {
	Rules []TransformRule `json:"rules"`
}

type TransformRule struct {
	Type       string               `json:"type"`
	Target     string               `json:"target"`
	Path       string               `json:"path,omitempty"`
	XPath      string               `json:"xpath,omitempty"`
	HeaderName string               `json:"header_name,omitempty"`
	Action     string               `json:"action"`
	Conditions []TransformCondition `json:"conditions,omitempty"`
	Params     TransformParams      `json:"params,omitempty"`
}

type TransformCondition struct {
	Field     string        `json:"field"`
	Equals    []interface{} `json:"equals,omitempty"`
	NotEquals []interface{} `json:"not_equals,omitempty"`
}

type TransformParams struct {
	Find      string      `json:"find,omitempty"`
	Replace   string      `json:"replace,omitempty"`
	Regex     string      `json:"regex,omitempty"`
	Value     interface{} `json:"value,omitempty"`
	Separator string      `json:"separator,omitempty"`
	From      string      `json:"from,omitempty"`
	To        string      `json:"to,omitempty"`
	Field     string      `json:"field,omitempty"`
}

type TransformContext struct {
	BaseURL   string            `json:"baseUrl"`
	AuthToken string            `json:"authToken"` // internal use only
	AuthType  string            `json:"authType"`  // "api_key" | "bearer"
	Variables map[string]string `json:"variables,omitempty"`
}

type TransformResult struct {
	Found   bool              `json:"found"`
	Body    json.RawMessage   `json:"body"`
	Headers map[string]string `json:"headers"`
	Status  int               `json:"status"`
	Error   string            `json:"error,omitempty"`
}

func (tc *TransformContext) TemplateVars() map[string]string {
	orcyaAuth := ""
	switch tc.AuthType {
	case "api_key":
		orcyaAuth = "api_key=" + tc.AuthToken
	case "bearer":
		orcyaAuth = "token=" + tc.AuthToken
	}

	vars := map[string]string{
		"oryca_gateway_url": tc.BaseURL,
		"oryca_auth":        orcyaAuth,
	}
	for k, v := range tc.Variables {
		vars[k] = v
	}
	return vars
}

func (tr *TransformRule) GetRuleType() string {
	if tr.Type == "" {
		return "json"
	}
	return tr.Type
}

func (tr *TransformRule) GetPathSelector() string {
	if tr.Path != "" {
		return tr.Path
	}
	return tr.XPath
}

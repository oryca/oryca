package transform

import (
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"strings"
)

// HybridEngine handles both JSON and XML
type HybridEngine struct {
	Config     *model.TransformConfig
	Context    *model.TransformContext
	jsonEngine *Engine
	xmlEngine  *XMLStdEngine
}

// NewHybridEngine builds a new hybrid engine
func NewHybridEngine(config *model.TransformConfig, context *model.TransformContext) *HybridEngine {
	return &HybridEngine{
		Config:     config,
		Context:    context,
		jsonEngine: NewEngine(config, context),
		xmlEngine:  NewXMLStdEngine(config, context),
	}
}

// Apply picks an engine by content type, then transforms
func (he *HybridEngine) Apply(body []byte) (*model.TransformResult, error) {

	if len(body) == 0 {
		fmt.Println("HybridEngine: input body is empty")
		return &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}, nil
	}

	// are there XML rules at all?
	hasXMLRules := false
	hasJSONRules := false

	for _, rule := range he.Config.Rules {
		ruleType := rule.GetRuleType()
		switch ruleType {
		case "xml":
			hasXMLRules = true
		case "json":
			hasJSONRules = true
		}
	}

	// no rule says which format it is, so look at the body
	if !hasXMLRules && !hasJSONRules {
		// guess from the first bytes
		contentType := he.detectContentType(body)
		if contentType == "xml" {
			hasXMLRules = true
		} else {
			hasJSONRules = true
		}
	}

	var result *model.TransformResult
	var err error

	// XML first, when there is any
	if hasXMLRules {
		result, err = he.xmlEngine.Apply(body)
		if err != nil {
			return nil, fmt.Errorf("XML transform failed: %w", err)
		}

		if len(result.Body) == 0 {
			fmt.Println("XML transform resulted in empty body")
			result.Body = body
		}

		body = result.Body // the JSON pass continues from the XML result
		result.Found = true
	}

	// then JSON, when there is any
	if hasJSONRules {
		jsonResult, err := he.jsonEngine.Apply(body)
		if err != nil {
			return nil, fmt.Errorf("JSON transform failed: %w", err)
		}

		if len(jsonResult.Body) == 0 {
			fmt.Println("JSON transform resulted in empty body")
			if len(body) > 0 {
				jsonResult.Body = body // keep the body as it was
			}
		}

		// merge the header changes from both passes
		if result != nil {
			for k, v := range result.Headers {
				jsonResult.Headers[k] = v
			}
		}

		result = jsonResult
		result.Found = true
	}

	// no rules matched at all
	if result == nil {
		result = &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}
	}

	return result, nil
}

// detectContentType guesses the format from the body
func (he *HybridEngine) detectContentType(body []byte) string {
	content := strings.TrimSpace(string(body))

	// XML
	if strings.HasPrefix(content, "<") && strings.HasSuffix(content, ">") {
		return "xml"
	}

	// XML with declaration
	if strings.HasPrefix(content, "<?xml") {
		return "xml"
	}

	// JSON
	if (strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}")) ||
		(strings.HasPrefix(content, "[") && strings.HasSuffix(content, "]")) {
		return "json"
	}

	// default to json
	return "json"
}

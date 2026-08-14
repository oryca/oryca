package transform

import (
	"encoding/json"
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"strings"

	"github.com/ohler55/ojg/jp"
)

// Engine transforms a JSON document
type Engine struct {
	Config  *model.TransformConfig
	Context *model.TransformContext
}

// NewEngine builds a new transform engine
func NewEngine(config *model.TransformConfig, context *model.TransformContext) *Engine {
	return &Engine{
		Config:  config,
		Context: context,
	}
}

// Apply runs the transform over a JSON body
func (e *Engine) Apply(body []byte) (*model.TransformResult, error) {
	if len(body) == 0 {
		return &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}, nil
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}

	vars := e.Context.TemplateVars()
	headers := map[string]string{}

	// counts the rules that actually changed something
	appliedRules := 0

	for _, rule := range e.Config.Rules {
		// JSON rules only
		if rule.GetRuleType() != "json" {
			continue
		}

		if !e.shouldApplyRule(rule, data) {
			continue
		}

		switch {
		case rule.Target == "body":
			if err := e.applyBodyRule(rule, &data, vars); err != nil {
				return nil, fmt.Errorf("apply body rule failed: %w", err)
			}
			appliedRules++

		case rule.Target == "headers":
			if err := e.applyHeaderRule(rule, headers, vars); err != nil {
				return nil, fmt.Errorf("apply header rule failed: %w", err)
			}
			appliedRules++
		}
	}

	newBody, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal result failed: %w", err)
	}

	// Safety check
	if len(newBody) == 0 && len(body) > 0 {
		newBody = body
	}

	return &model.TransformResult{
		Body:    newBody,
		Headers: headers,
		Found:   appliedRules > 0,
	}, nil
}

// shouldApplyRule reports whether this rule runs on this element
func (e *Engine) shouldApplyRule(rule model.TransformRule, data interface{}) bool {
	if len(rule.Conditions) == 0 {
		return true
	}

	expr, err := jp.ParseString(rule.Path)
	if err != nil {
		return false
	}

	matches := expr.Get(data)
	if len(matches) == 0 {
		return false
	}

	// every condition has to pass
	for _, match := range matches {
		if obj, ok := match.(map[string]interface{}); ok {
			for _, condition := range rule.Conditions {
				if val, exists := obj[condition.Field]; exists {
					if e.evaluateCondition(condition, val) {
						return true
					}
				}
			}
		}
	}

	return false
}

// evaluateCondition checks one condition
func (e *Engine) evaluateCondition(cond model.TransformCondition, value interface{}) bool {
	strVal := fmt.Sprintf("%v", value)

	// Equals
	if len(cond.Equals) > 0 {
		match := false
		for _, eq := range cond.Equals {
			if strVal == fmt.Sprintf("%v", eq) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// NotEquals
	if len(cond.NotEquals) > 0 {
		for _, neq := range cond.NotEquals {
			if strVal == fmt.Sprintf("%v", neq) {
				return false
			}
		}
	}

	return true
}

// applyBodyRule rewrites the JSON body
func (e *Engine) applyBodyRule(rule model.TransformRule, data *interface{}, vars map[string]string) error {
	if rule.Path == "" {
		return fmt.Errorf("path is required for body target")
	}

	expr, err := jp.ParseString(rule.Path)
	if err != nil {
		return fmt.Errorf("invalid JSONPath: %w", err)
	}

	switch rule.Action {
	case "replace":
		return e.replaceValues(expr, data, rule, vars)
	case "add":
		val := e.resolveValue(rule.Params, vars)
		expr.Set(*data, val)
	case "append":
		return e.appendValues(expr, data, rule, vars)
	case "remove":
		expr.Del(*data)
	case "rename":
		return e.renameField(expr, data, rule)
	default:
		return fmt.Errorf("unsupported action: %s", rule.Action)
	}

	return nil
}

// replaceValues replaces values in the JSON, matching on shape
func (e *Engine) replaceValues(expr jp.Expr, data *interface{}, rule model.TransformRule, vars map[string]string) error {
	matches := expr.Get(*data)

	for _, match := range matches {
		switch v := match.(type) {
		case string:
			// a string is replaced directly
			newVal := e.performStringReplace(v, rule, vars)
			e.updateStringValue(expr, data, v, newVal)

		case map[string]interface{}:
			// an object takes the named field, or every string field when none is named
			if rule.Params.Field != "" {
				// one named field
				if value, exists := v[rule.Params.Field]; exists {
					if strValue, ok := value.(string); ok {
						newValue := e.performStringReplace(strValue, rule, vars)
						v[rule.Params.Field] = newValue
					}
				}
			} else {
				// no field named. Every string field
				for key, value := range v {
					if strValue, ok := value.(string); ok {
						newValue := e.performStringReplace(strValue, rule, vars)
						v[key] = newValue
					}
				}
			}

		default:
			// anything else is replaced whole
			val := e.resolveValue(rule.Params, vars)
			expr.Set(*data, val)
		}
	}

	return nil
}

// performStringReplace applies the rule's find/replace or regex
func (e *Engine) performStringReplace(original string, rule model.TransformRule, vars map[string]string) string {
	result := original

	// regex wins when set. The compiled regex is cached, as the same rule runs on every request
	if rule.Params.Regex != "" {
		replaceValue := e.replaceTemplateVars(rule.Params.Replace, vars)
		re, err := cachedRegexp(rule.Params.Regex)
		if err == nil {
			result = re.ReplaceAllString(result, replaceValue)
		}
	}

	// otherwise plain find/replace
	if rule.Params.Find != "" {
		findValue := e.replaceTemplateVars(rule.Params.Find, vars)
		replaceValue := e.replaceTemplateVars(rule.Params.Replace, vars)
		result = strings.ReplaceAll(result, findValue, replaceValue)
	}

	return result
}

// updateStringValue writes a string back into the JSON
func (e *Engine) updateStringValue(expr jp.Expr, data *interface{}, oldVal, newVal string) {
	jsonBytes, _ := json.Marshal(*data)
	jsonStr := string(jsonBytes)

	// escape JSON characters
	oldValJSON, _ := json.Marshal(oldVal)
	newValJSON, _ := json.Marshal(newVal)

	// replace inside the encoded JSON
	newJsonStr := strings.ReplaceAll(jsonStr, string(oldValJSON), string(newValJSON))

	// parse it back
	json.Unmarshal([]byte(newJsonStr), data)
}

// appendValues appends to a value
func (e *Engine) appendValues(expr jp.Expr, data *interface{}, rule model.TransformRule, vars map[string]string) error {
	matches := expr.Get(*data)

	for _, match := range matches {
		if strVal, ok := match.(string); ok {
			appendVal := e.resolveValue(rule.Params, vars)
			separator := rule.Params.Separator
			if separator == "" {
				separator = ""
			}
			newVal := strVal + separator + fmt.Sprintf("%v", appendVal)
			e.updateStringValue(expr, data, strVal, newVal)
		} else if arr, ok := match.([]interface{}); ok {
			appendVal := e.resolveValue(rule.Params, vars)
			newArr := append(arr, appendVal)
			expr.Set(*data, newArr)
		}
	}

	return nil
}

// renameField renames a field on an object
func (e *Engine) renameField(expr jp.Expr, data *interface{}, rule model.TransformRule) error {
	from := rule.Params.From
	to := rule.Params.To

	if from == "" || to == "" {
		return fmt.Errorf("rename requires 'from' and 'to' parameters")
	}

	matches := expr.Get(*data)

	for _, match := range matches {
		if obj, ok := match.(map[string]interface{}); ok {
			// nothing to do when the field is absent
			if value, exists := obj[from]; exists {
				// add the new name
				obj[to] = value
				// drop the old one
				delete(obj, from)
			}
		}
	}

	return nil
}

// applyHeaderRule rewrites headers
func (e *Engine) applyHeaderRule(rule model.TransformRule, headers map[string]string, vars map[string]string) error {
	if rule.HeaderName == "" {
		return fmt.Errorf("header_name is required for headers target")
	}

	val := e.resolveValue(rule.Params, vars)

	switch rule.Action {
	case "replace", "add":
		headers[rule.HeaderName] = fmt.Sprintf("%v", val)
	case "append":
		separator := rule.Params.Separator
		if separator == "" {
			separator = ""
		}
		headers[rule.HeaderName] = headers[rule.HeaderName] + separator + fmt.Sprintf("%v", val)
	case "remove":
		delete(headers, rule.HeaderName)
	default:
		return fmt.Errorf("unsupported action: %s", rule.Action)
	}

	return nil
}

// resolveValue returns the concrete value from Params
func (e *Engine) resolveValue(params model.TransformParams, vars map[string]string) interface{} {
	var val string

	switch {
	case params.Value != nil:
		val = fmt.Sprintf("%v", params.Value)
	case params.Replace != "":
		val = params.Replace
	case params.Find != "":
		val = params.Find
	default:
		val = ""
	}

	return e.replaceTemplateVars(val, vars)
}

// replaceTemplateVars expands the {{...}} placeholders
func (e *Engine) replaceTemplateVars(text string, vars map[string]string) string {
	result := text

	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, v)
	}

	return result
}

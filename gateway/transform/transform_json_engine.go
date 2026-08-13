package transform

import (
	"encoding/json"
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"strings"

	"github.com/ohler55/ojg/jp"
)

// Engine - transform engine สำหรับการแปลงข้อมูล
type Engine struct {
	Config  *model.TransformConfig
	Context *model.TransformContext
}

// NewEngine - สร้าง transform engine ใหม่
func NewEngine(config *model.TransformConfig, context *model.TransformContext) *Engine {
	return &Engine{
		Config:  config,
		Context: context,
	}
}

// Apply - ประมวลผลการ transform กับ body JSON
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

	//เพิ่ม counter เพื่อดูว่ามี rule ที่ใช้หรือไม่
	appliedRules := 0

	for _, rule := range e.Config.Rules {
		// เฉพาะ JSON rules
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

// shouldApplyRule - ตรวจสอบว่าควรใช้กฎนี้หรือไม่
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

	// ตรวจสอบเงื่อนไข
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

// evaluateCondition - ประเมินเงื่อนไข
func (e *Engine) evaluateCondition(cond model.TransformCondition, value interface{}) bool {
	strVal := fmt.Sprintf("%v", value)

	// ตรวจสอบ Equals
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

	// ตรวจสอบ NotEquals
	if len(cond.NotEquals) > 0 {
		for _, neq := range cond.NotEquals {
			if strVal == fmt.Sprintf("%v", neq) {
				return false
			}
		}
	}

	return true
}

// applyBodyRule - แปลง JSON body
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

// replaceValues - แทนที่ค่าใน JSON แบบ smart replace
func (e *Engine) replaceValues(expr jp.Expr, data *interface{}, rule model.TransformRule, vars map[string]string) error {
	matches := expr.Get(*data)

	for _, match := range matches {
		switch v := match.(type) {
		case string:
			// ถ้าเป็น string ให้ทำ string replacement
			newVal := e.performStringReplace(v, rule, vars)
			e.updateStringValue(expr, data, v, newVal)

		case map[string]interface{}:
			// ถ้าเป็น object ให้ replace ตาม field ที่ระบุ หรือทุก string fields
			if rule.Params.Field != "" {
				// ระบุ field เฉพาะ
				if value, exists := v[rule.Params.Field]; exists {
					if strValue, ok := value.(string); ok {
						newValue := e.performStringReplace(strValue, rule, vars)
						v[rule.Params.Field] = newValue
					}
				}
			} else {
				// ไม่ระบุ field ให้ replace ทุก string fields
				for key, value := range v {
					if strValue, ok := value.(string); ok {
						newValue := e.performStringReplace(strValue, rule, vars)
						v[key] = newValue
					}
				}
			}

		default:
			// ถ้าไม่ใช่ string หรือ object ให้แทนที่ทั้งค่า
			val := e.resolveValue(rule.Params, vars)
			expr.Set(*data, val)
		}
	}

	return nil
}

// performStringReplace - ทำ string replacement ตาม params
func (e *Engine) performStringReplace(original string, rule model.TransformRule, vars map[string]string) string {
	result := original

	// ถ้ามี regex ใช้ regex replace — cache compiled regex เพราะ rule เดิมถูกใช้ซ้ำทุก request
	if rule.Params.Regex != "" {
		replaceValue := e.replaceTemplateVars(rule.Params.Replace, vars)
		re, err := cachedRegexp(rule.Params.Regex)
		if err == nil {
			result = re.ReplaceAllString(result, replaceValue)
		}
	}

	// ถ้ามี find/replace ใช้ string replace
	if rule.Params.Find != "" {
		findValue := e.replaceTemplateVars(rule.Params.Find, vars)
		replaceValue := e.replaceTemplateVars(rule.Params.Replace, vars)
		result = strings.ReplaceAll(result, findValue, replaceValue)
	}

	return result
}

// updateStringValue - อัปเดตค่า string ใน JSON (workaround)
func (e *Engine) updateStringValue(expr jp.Expr, data *interface{}, oldVal, newVal string) {
	jsonBytes, _ := json.Marshal(*data)
	jsonStr := string(jsonBytes)

	// escape JSON characters
	oldValJSON, _ := json.Marshal(oldVal)
	newValJSON, _ := json.Marshal(newVal)

	// แทนที่ใน JSON string
	newJsonStr := strings.ReplaceAll(jsonStr, string(oldValJSON), string(newValJSON))

	// Parse กลับ
	json.Unmarshal([]byte(newJsonStr), data)
}

// appendValues - ต่อท้ายค่า
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

// renameField - เปลี่ยนชื่อ field ใน object
func (e *Engine) renameField(expr jp.Expr, data *interface{}, rule model.TransformRule) error {
	from := rule.Params.From
	to := rule.Params.To

	if from == "" || to == "" {
		return fmt.Errorf("rename requires 'from' and 'to' parameters")
	}

	matches := expr.Get(*data)

	for _, match := range matches {
		if obj, ok := match.(map[string]interface{}); ok {
			// ตรวจสอบว่ามี field ที่ต้องการเปลี่ยนชื่อหรือไม่
			if value, exists := obj[from]; exists {
				// เพิ่ม field ใหม่
				obj[to] = value
				// ลบ field เก่า
				delete(obj, from)
			}
		}
	}

	return nil
}

// applyHeaderRule - แปลง headers
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

// resolveValue - คืนค่าจริงจาก Params
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

// replaceTemplateVars - แทนค่า template variables
func (e *Engine) replaceTemplateVars(text string, vars map[string]string) string {
	result := text

	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, v)
	}

	return result
}

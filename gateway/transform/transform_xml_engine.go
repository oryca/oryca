package transform

import (
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"regexp"
	"strings"
)

// reLocalName pattern คงที่ — compile ครั้งเดียวตอน start
var reLocalName = regexp.MustCompile(`local-name\(\)=['"]([^'"]+)['"]`)

// XMLStdEngine - XML engine ที่ใช้ string-based approach แบบ generic
type XMLStdEngine struct {
	Config  *model.TransformConfig
	Context *model.TransformContext
}

// NewXMLStdEngine - สร้าง XML engine ใหม่
func NewXMLStdEngine(config *model.TransformConfig, context *model.TransformContext) *XMLStdEngine {
	return &XMLStdEngine{
		Config:  config,
		Context: context,
	}
}

// Apply - ประมวลผล XML transform แบบ string-based
func (xe *XMLStdEngine) Apply(body []byte) (*model.TransformResult, error) {
	if len(body) == 0 {
		return &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}, nil
	}

	vars := xe.Context.TemplateVars()
	headers := map[string]string{}

	xmlContent := string(body)
	appliedRules := 0 // เพิ่ม counter

	for _, rule := range xe.Config.Rules {
		if rule.GetRuleType() != "xml" {
			continue
		}

		switch rule.Target {
		case "body":
			newContent, err := xe.applyXMLRule(rule, xmlContent, vars)
			if err != nil {
				return nil, fmt.Errorf("apply XML rule failed: %w", err)
			}
			xmlContent = newContent
			appliedRules++

		case "headers":
			if err := xe.applyHeaderRule(rule, headers, vars); err != nil {
				return nil, fmt.Errorf("apply header rule failed: %w", err)
			}
			appliedRules++
		}
	}

	resultBody := []byte(xmlContent)

	// Safety check
	if len(resultBody) == 0 && len(body) > 0 {
		fmt.Println("XML transform resulted in empty body, using original")
		resultBody = body
	}

	return &model.TransformResult{
		Body:    resultBody,
		Headers: headers,
		Found:   appliedRules > 0, // เพิ่ม Found flag
	}, nil
}

// applyXMLRule - ประมวลผล XML rule แบบ generic
func (xe *XMLStdEngine) applyXMLRule(rule model.TransformRule, content string, vars map[string]string) (string, error) {
	xpath := rule.GetPathSelector()
	if xpath == "" {
		return content, fmt.Errorf("xpath is required for XML body target")
	}

	switch rule.Action {
	case "replace":
		return xe.replaceByXPath(content, rule, vars), nil
	case "add":
		return xe.addByXPath(content, rule, vars), nil
	case "remove":
		return xe.removeByXPath(content, rule), nil
	default:
		return content, fmt.Errorf("unsupported action: %s", rule.Action)
	}
}

// replaceByXPath - แทนที่ค่าตาม XPath แบบ generic
// replaceByXPath - แทนที่ค่าตาม XPath แบบ generic (แก้ไขแล้ว)
func (xe *XMLStdEngine) replaceByXPath(content string, rule model.TransformRule, vars map[string]string) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field

	// แยก element name จาก xpath
	elementName := xe.extractElementName(xpath)
	if elementName == "" {
		return content
	}

	// ตรวจสอบว่าเป็น attribute หรือ text content
	if strings.HasPrefix(field, "@") {
		// เป็น attribute
		attrName := strings.TrimPrefix(field, "@")

		// สำหรับ local-name() ให้ใช้วิธีพิเศษ
		if strings.Contains(xpath, "local-name()") {
			return xe.replaceAttributeWithLocalName(content, elementName, attrName, rule, vars)
		} else {
			return xe.replaceAttributeGeneric(content, elementName, attrName, rule, vars)
		}
	} else {
		// เป็น text content
		return xe.replaceTextContent(content, elementName, rule, vars)
	}
}

// replaceAttributeWithLocalName - แทนที่ attribute สำหรับ local-name() pattern
func (xe *XMLStdEngine) replaceAttributeWithLocalName(content, elementName, attrName string, rule model.TransformRule, vars map[string]string) string {
	// สร้าง regex ที่หา element ใดๆ ที่มี local name ตรงกับ elementName
	// pattern จะหา: <anyPrefix:ElementName หรือ <ElementName
	patterns := []string{
		// Pattern 1: <prefix:ElementName attr="value"
		fmt.Sprintf(`(<[^:\s]*:%s[^>]*?\s%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Pattern 2: <ElementName attr="value" (ไม่มี namespace)
		fmt.Sprintf(`(<%s[^>]*?\s%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Pattern 3: namespace attribute เช่น xlink:href
		fmt.Sprintf(`(<[^:\s]*:%s[^>]*?\s[^:]*:%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		fmt.Sprintf(`(<%s[^>]*?\s[^:]*:%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Pattern 4: self-closing tags
		fmt.Sprintf(`(<[^:\s]*:%s[^>]*?\s%s=")([^"]*)(".*?/?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		fmt.Sprintf(`(<%s[^>]*?\s%s=")([^"]*)(".*?/?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
	}

	result := content

	for _, pattern := range patterns {
		re, err := cachedRegexp(pattern)
		if err != nil {
			continue
		}

		newResult := re.ReplaceAllStringFunc(result, func(match string) string {
			parts := re.FindStringSubmatch(match)
			if len(parts) != 4 {
				return match
			}

			prefix := parts[1]   // <element...attr="
			oldValue := parts[2] // ค่าเดิม
			suffix := parts[3]   // "...>

			// ทำการแทนที่ตาม rule
			newValue := xe.performStringReplace(oldValue, rule, vars)

			return prefix + newValue + suffix
		})

		// ถ้ามีการเปลี่ยนแปลง ให้ใช้ผลลัพธ์ใหม่
		if newResult != result {
			result = newResult
		}
	}

	return result
}

// extractElementName - ดึงชื่อ element จาก xpath (แก้ไขแล้ว)
func (xe *XMLStdEngine) extractElementName(xpath string) string {
	// รองรับรูปแบบ xpath ต่างๆ

	// สำหรับ local-name() pattern: //*[local-name()='ElementName']
	if strings.Contains(xpath, "local-name()") {
		// หา pattern local-name()='ElementName' หรือ local-name()="ElementName"
		matches := reLocalName.FindStringSubmatch(xpath)
		if len(matches) > 1 {
			return matches[1] // ดึงชื่อ element จากใน quotes
		}
	}

	// //ElementName -> ElementName
	if strings.HasPrefix(xpath, "//") {
		name := strings.TrimPrefix(xpath, "//")
		// ตัดส่วน condition ออก เช่น ElementName[@attr='value'] -> ElementName
		if idx := strings.Index(name, "["); idx != -1 {
			name = name[:idx]
		}
		return name
	}

	// /root/ElementName -> ElementName (เอาตัวสุดท้าย)
	if strings.HasPrefix(xpath, "/") {
		parts := strings.Split(strings.Trim(xpath, "/"), "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			// ตัดส่วน condition ออก
			if idx := strings.Index(name, "["); idx != -1 {
				name = name[:idx]
			}
			return name
		}
	}

	// ElementName (direct name)
	if idx := strings.Index(xpath, "["); idx != -1 {
		return xpath[:idx]
	}

	return xpath
}

// replaceAttributeGeneric - แทนที่ attribute value แบบ generic
func (xe *XMLStdEngine) replaceAttributeGeneric(content, elementName, attrName string, rule model.TransformRule, vars map[string]string) string {
	// สร้าง regex pattern สำหรับหา attribute ในรูปแบบต่างๆ
	patterns := []string{
		// Standard attribute: attrName="value"
		fmt.Sprintf(`(<%s[^>]*?\s%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Namespace attribute: namespace:attrName="value"
		fmt.Sprintf(`(<%s[^>]*?\s[^:]*:%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Self-closing tag: <element attr="value"/>
		fmt.Sprintf(`(<%s[^>]*?\s%s=")([^"]*)(".*?/?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
	}

	result := content

	for _, pattern := range patterns {
		re, err := cachedRegexp(pattern)
		if err != nil {
			continue
		}
		result = re.ReplaceAllStringFunc(result, func(match string) string {
			parts := re.FindStringSubmatch(match)
			if len(parts) != 4 {
				return match
			}

			prefix := parts[1]   // <element...attr="
			oldValue := parts[2] // ค่าเดิม
			suffix := parts[3]   // "...>

			// ทำการแทนที่ตาม rule
			newValue := xe.performStringReplace(oldValue, rule, vars)

			return prefix + newValue + suffix
		})
	}

	return result
}

// replaceTextContent - แทนที่ text content ของ element
func (xe *XMLStdEngine) replaceTextContent(content, elementName string, rule model.TransformRule, vars map[string]string) string {
	// Pattern สำหรับ text content: <element>content</element>
	pattern := fmt.Sprintf(`(<%s[^>]*>)([^<]*)(<%s>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(elementName))
	re, err := cachedRegexp(pattern)
	if err != nil {
		return content
	}

	return re.ReplaceAllStringFunc(content, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}

		openTag := parts[1]  // <element>
		oldText := parts[2]  // text content
		closeTag := parts[3] // </element>

		// ทำการแทนที่
		newText := xe.performStringReplace(oldText, rule, vars)

		return openTag + newText + closeTag
	})
}

// addByXPath - เพิ่มค่าตาม xpath
func (xe *XMLStdEngine) addByXPath(content string, rule model.TransformRule, vars map[string]string) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field
	elementName := xe.extractElementName(xpath)

	if strings.HasPrefix(field, "@") {
		// เพิ่ม attribute
		attrName := strings.TrimPrefix(field, "@")
		value := xe.resolveValue(rule.Params, vars)
		return xe.addAttribute(content, elementName, attrName, fmt.Sprintf("%v", value))
	}

	// เพิ่ม child element (ซับซ้อนกว่า จะทำภายหลัง)
	return content
}

// removeByXPath - ลบตาม xpath
func (xe *XMLStdEngine) removeByXPath(content string, rule model.TransformRule) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field
	elementName := xe.extractElementName(xpath)

	if strings.HasPrefix(field, "@") {
		// ลบ attribute
		attrName := strings.TrimPrefix(field, "@")
		return xe.removeAttribute(content, elementName, attrName)
	}

	// ลบ element (ทำภายหลัง)
	return content
}

// addAttribute - เพิ่ม attribute ใน element
func (xe *XMLStdEngine) addAttribute(content, elementName, attrName, value string) string {
	// หา opening tag และเพิ่ม attribute
	pattern := fmt.Sprintf(`(<%s)([^>]*>)`, regexp.QuoteMeta(elementName))
	re, err := cachedRegexp(pattern)
	if err != nil {
		return content
	}

	return re.ReplaceAllString(content, fmt.Sprintf(`$1 %s="%s"$2`, attrName, value))
}

// removeAttribute - ลบ attribute จาก element
func (xe *XMLStdEngine) removeAttribute(content, elementName, attrName string) string {
	// ลบ attribute ในรูปแบบต่างๆ
	patterns := []string{
		fmt.Sprintf(`\s%s="[^"]*"`, regexp.QuoteMeta(attrName)),
		fmt.Sprintf(`\s[^:]*:%s="[^"]*"`, regexp.QuoteMeta(attrName)),
	}

	result := content
	for _, pattern := range patterns {
		re, err := cachedRegexp(pattern)
		if err != nil {
			continue
		}
		result = re.ReplaceAllString(result, "")
	}

	return result
}

// performStringReplace - ทำ string replacement ตาม rule params
func (xe *XMLStdEngine) performStringReplace(original string, rule model.TransformRule, vars map[string]string) string {
	result := original

	// Regex replacement — cache compiled regex เพราะ rule เดิมถูกใช้ซ้ำทุก request
	if rule.Params.Regex != "" {
		replaceValue := xe.replaceTemplateVars(rule.Params.Replace, vars)
		re, err := cachedRegexp(rule.Params.Regex)
		if err == nil {
			result = re.ReplaceAllString(result, replaceValue)
		}
	}

	// String replacement
	if rule.Params.Find != "" {
		findValue := xe.replaceTemplateVars(rule.Params.Find, vars)
		replaceValue := xe.replaceTemplateVars(rule.Params.Replace, vars)
		result = strings.ReplaceAll(result, findValue, replaceValue)
	}

	return result
}

// Helper functions (เหมือนเดิม)
func (xe *XMLStdEngine) applyHeaderRule(rule model.TransformRule, headers map[string]string, vars map[string]string) error {
	if rule.HeaderName == "" {
		return fmt.Errorf("header_name is required for headers target")
	}

	val := xe.resolveValue(rule.Params, vars)

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

func (xe *XMLStdEngine) resolveValue(params model.TransformParams, vars map[string]string) interface{} {
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

	return xe.replaceTemplateVars(val, vars)
}

func (xe *XMLStdEngine) replaceTemplateVars(text string, vars map[string]string) string {
	result := text

	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, v)
	}

	return result
}

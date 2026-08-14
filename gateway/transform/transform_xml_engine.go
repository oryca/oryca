package transform

import (
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"regexp"
	"strings"
)

// reLocalName is a fixed pattern. Compiled once at start
var reLocalName = regexp.MustCompile(`local-name\(\)=['"]([^'"]+)['"]`)

// XMLStdEngine is a generic, string-based XML engine
type XMLStdEngine struct {
	Config  *model.TransformConfig
	Context *model.TransformContext
}

// NewXMLStdEngine builds a new XML engine
func NewXMLStdEngine(config *model.TransformConfig, context *model.TransformContext) *XMLStdEngine {
	return &XMLStdEngine{
		Config:  config,
		Context: context,
	}
}

// Apply runs the XML transform over the body, as strings
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
	appliedRules := 0 // how many rules actually changed something

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
		Found:   appliedRules > 0, // Found tells the caller whether anything changed
	}, nil
}

// applyXMLRule runs one rule against the document
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

// replaceByXPath replaces whatever the XPath selects
func (xe *XMLStdEngine) replaceByXPath(content string, rule model.TransformRule, vars map[string]string) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field

	// pull the element name out of the xpath
	elementName := xe.extractElementName(xpath)
	if elementName == "" {
		return content
	}

	// an @ prefix means an attribute, otherwise it is text content
	if strings.HasPrefix(field, "@") {
		// attribute
		attrName := strings.TrimPrefix(field, "@")

		// local-name() needs its own path, namespaces are not matched literally
		if strings.Contains(xpath, "local-name()") {
			return xe.replaceAttributeWithLocalName(content, elementName, attrName, rule, vars)
		} else {
			return xe.replaceAttributeGeneric(content, elementName, attrName, rule, vars)
		}
	} else {
		// text content
		return xe.replaceTextContent(content, elementName, rule, vars)
	}
}

// replaceAttributeWithLocalName handles the local-name() form of the selector
func (xe *XMLStdEngine) replaceAttributeWithLocalName(content, elementName, attrName string, rule model.TransformRule, vars map[string]string) string {
	// match any element whose local name is elementName:
	// <anyPrefix:ElementName, or <ElementName with no prefix at all
	patterns := []string{
		// Pattern 1: <prefix:ElementName attr="value"
		fmt.Sprintf(`(<[^:\s]*:%s[^>]*?\s%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Pattern 2: <ElementName attr="value" (no namespace)
		fmt.Sprintf(`(<%s[^>]*?\s%s=")([^"]*)(".*?>)`, regexp.QuoteMeta(elementName), regexp.QuoteMeta(attrName)),
		// Pattern 3: a namespaced attribute, xlink:href for example
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
			oldValue := parts[2] // the value before the rule ran
			suffix := parts[3]   // "...>

			// apply the rule
			newValue := xe.performStringReplace(oldValue, rule, vars)

			return prefix + newValue + suffix
		})

		// only take the rewritten value if it actually changed
		if newResult != result {
			result = newResult
		}
	}

	return result
}

// extractElementName pulls the element name out of an xpath
func (xe *XMLStdEngine) extractElementName(xpath string) string {
	// several xpath shapes are accepted

	// local-name() form: //*[local-name()='ElementName']
	if strings.Contains(xpath, "local-name()") {
		// either quote style is allowed
		matches := reLocalName.FindStringSubmatch(xpath)
		if len(matches) > 1 {
			return matches[1] // the name inside the quotes
		}
	}

	// //ElementName -> ElementName
	if strings.HasPrefix(xpath, "//") {
		name := strings.TrimPrefix(xpath, "//")
		// drop the condition: ElementName[@attr='value'] -> ElementName
		if idx := strings.Index(name, "["); idx != -1 {
			name = name[:idx]
		}
		return name
	}

	// /root/ElementName -> ElementName (the last segment wins)
	if strings.HasPrefix(xpath, "/") {
		parts := strings.Split(strings.Trim(xpath, "/"), "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			// drop the condition
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

// replaceAttributeGeneric replaces an attribute value
func (xe *XMLStdEngine) replaceAttributeGeneric(content, elementName, attrName string, rule model.TransformRule, vars map[string]string) string {
	// one pattern per way the attribute can be written
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
			oldValue := parts[2] // the value before the rule ran
			suffix := parts[3]   // "...>

			// apply the rule
			newValue := xe.performStringReplace(oldValue, rule, vars)

			return prefix + newValue + suffix
		})
	}

	return result
}

// replaceTextContent replaces the text inside an element
func (xe *XMLStdEngine) replaceTextContent(content, elementName string, rule model.TransformRule, vars map[string]string) string {
	// text content pattern: <element>content</element>
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

		// apply the rule
		newText := xe.performStringReplace(oldText, rule, vars)

		return openTag + newText + closeTag
	})
}

// addByXPath adds whatever the xpath points at
func (xe *XMLStdEngine) addByXPath(content string, rule model.TransformRule, vars map[string]string) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field
	elementName := xe.extractElementName(xpath)

	if strings.HasPrefix(field, "@") {
		// add an attribute
		attrName := strings.TrimPrefix(field, "@")
		value := xe.resolveValue(rule.Params, vars)
		return xe.addAttribute(content, elementName, attrName, fmt.Sprintf("%v", value))
	}

	// adding a child element is harder, and not supported yet
	return content
}

// removeByXPath removes whatever the xpath points at
func (xe *XMLStdEngine) removeByXPath(content string, rule model.TransformRule) string {
	xpath := rule.GetPathSelector()
	field := rule.Params.Field
	elementName := xe.extractElementName(xpath)

	if strings.HasPrefix(field, "@") {
		// remove an attribute
		attrName := strings.TrimPrefix(field, "@")
		return xe.removeAttribute(content, elementName, attrName)
	}

	// removing an element is not supported yet
	return content
}

// addAttribute adds an attribute to an element
func (xe *XMLStdEngine) addAttribute(content, elementName, attrName, value string) string {
	// find the opening tag and add it there
	pattern := fmt.Sprintf(`(<%s)([^>]*>)`, regexp.QuoteMeta(elementName))
	re, err := cachedRegexp(pattern)
	if err != nil {
		return content
	}

	return re.ReplaceAllString(content, fmt.Sprintf(`$1 %s="%s"$2`, attrName, value))
}

// removeAttribute removes an attribute from an element
func (xe *XMLStdEngine) removeAttribute(content, elementName, attrName string) string {
	// covers every way the attribute can be written
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

// performStringReplace applies the rule's find/replace or regex
func (xe *XMLStdEngine) performStringReplace(original string, rule model.TransformRule, vars map[string]string) string {
	result := original

	// Regex replacement. The compiled regex is cached, as the same rule runs on every request
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

// Helper functions
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

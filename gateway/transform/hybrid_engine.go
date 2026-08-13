package transform

import (
	"fmt"
	"github.com/oryca/oryca/gateway/transform/model"
	"strings"
)

// HybridEngine - engine ที่รองรับทั้ง JSON และ XML
type HybridEngine struct {
	Config     *model.TransformConfig
	Context    *model.TransformContext
	jsonEngine *Engine
	xmlEngine  *XMLStdEngine
}

// NewHybridEngine - สร้าง hybrid engine ใหม่
func NewHybridEngine(config *model.TransformConfig, context *model.TransformContext) *HybridEngine {
	return &HybridEngine{
		Config:     config,
		Context:    context,
		jsonEngine: NewEngine(config, context),
		xmlEngine:  NewXMLStdEngine(config, context),
	}
}

// Apply - ประมวลผลการ transform โดยตรวจสอบ content type
func (he *HybridEngine) Apply(body []byte) (*model.TransformResult, error) {

	if len(body) == 0 {
		fmt.Println("HybridEngine: input body is empty")
		return &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}, nil
	}

	// ตรวจสอบว่ามี rules สำหรับ XML หรือไม่
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

	// ถ้าไม่มี rules เฉพาะเลย ให้ตรวจสอบจาก content
	if !hasXMLRules && !hasJSONRules {
		// ใช้ heuristic ตรวจสอบ content type
		contentType := he.detectContentType(body)
		if contentType == "xml" {
			hasXMLRules = true
		} else {
			hasJSONRules = true
		}
	}

	var result *model.TransformResult
	var err error

	// ประมวลผล XML ก่อน (ถ้ามี)
	if hasXMLRules {
		result, err = he.xmlEngine.Apply(body)
		if err != nil {
			return nil, fmt.Errorf("XML transform failed: %w", err)
		}

		if len(result.Body) == 0 {
			fmt.Println("XML transform resulted in empty body")
			result.Body = body
		}

		body = result.Body // ใช้ผลลัพธ์จาก XML สำหรับ JSON ต่อ
		result.Found = true
	}

	// ประมวลผล JSON (ถ้ามี)
	if hasJSONRules {
		jsonResult, err := he.jsonEngine.Apply(body)
		if err != nil {
			return nil, fmt.Errorf("JSON transform failed: %w", err)
		}

		if len(jsonResult.Body) == 0 {
			fmt.Println("JSON transform resulted in empty body")
			if len(body) > 0 {
				jsonResult.Body = body // ใช้ body เดิม
			}
		}

		// รวม headers จาก XML และ JSON
		if result != nil {
			for k, v := range result.Headers {
				jsonResult.Headers[k] = v
			}
		}

		result = jsonResult
		result.Found = true
	}

	// ถ้าไม่มี rules เลย
	if result == nil {
		result = &model.TransformResult{
			Found:   false,
			Body:    body,
			Headers: make(map[string]string),
		}
	}

	return result, nil
}

// detectContentType - ตรวจสอบ content type จาก body
func (he *HybridEngine) detectContentType(body []byte) string {
	content := strings.TrimSpace(string(body))

	// ตรวจสอบ XML
	if strings.HasPrefix(content, "<") && strings.HasSuffix(content, ">") {
		return "xml"
	}

	// ตรวจสอบ XML with declaration
	if strings.HasPrefix(content, "<?xml") {
		return "xml"
	}

	// ตรวจสอบ JSON
	if (strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}")) ||
		(strings.HasPrefix(content, "[") && strings.HasSuffix(content, "]")) {
		return "json"
	}

	// default เป็น json
	return "json"
}

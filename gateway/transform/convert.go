package transform

import (
	gwsync "github.com/oryca/oryca/gateway/sync"
	"github.com/oryca/oryca/gateway/transform/model"
)

// FromPayload converts a TransformConfigPayload from sync into the engine's model.
func FromPayload(p *gwsync.TransformConfigPayload) *model.TransformConfig {
	cfg := &model.TransformConfig{
		Rules: make([]model.TransformRule, 0, len(p.Rules)),
	}
	for _, r := range p.Rules {
		rule := model.TransformRule{
			Type:       r.Type,
			Target:     r.Target,
			Action:     r.Action,
			Path:       r.Path,
			XPath:      r.XPath,
			HeaderName: r.HeaderName,
			Params: model.TransformParams{
				Find:      r.Params.Find,
				Replace:   r.Params.Replace,
				Regex:     r.Params.Regex,
				Value:     r.Params.Value,
				Separator: r.Params.Separator,
				From:      r.Params.From,
				To:        r.Params.To,
				Field:     r.Params.Field,
			},
		}
		for _, cond := range r.Conditions {
			rule.Conditions = append(rule.Conditions, model.TransformCondition{
				Field:     cond.Field,
				Equals:    cond.Equals,
				NotEquals: cond.NotEquals,
			})
		}
		cfg.Rules = append(cfg.Rules, rule)
	}
	return cfg
}

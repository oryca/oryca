package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func cfgWith(replace string) *TransformConfigPayload {
	return &TransformConfigPayload{
		ID:        "cfg1",
		ServiceID: "svc1",
		Enabled:   true,
		Rules: []TransformRulePayload{
			{
				Type:   "json",
				Target: "body",
				Action: "replace",
				Path:   "$.links[*]",
				Params: TransformParamsPayload{
					Field:   "href",
					Find:    "https://upstream.example",
					Replace: replace,
				},
			},
		},
	}
}

func TestFingerprint(t *testing.T) {
	t.Run("same rules give the same value", func(t *testing.T) {
		a, b := cfgWith("{{oryca_gateway_url}}"), cfgWith("{{oryca_gateway_url}}")
		a.ComputeFingerprint()
		b.ComputeFingerprint()
		assert.Equal(t, a.Fingerprint(), b.Fingerprint())
		assert.NotEmpty(t, a.Fingerprint())
	})

	// ตัวที่ทำให้ ETag เปลี่ยนตอน admin แก้ rule ถ้าเท่าเดิม client จะติด 304 กับของเก่า
	t.Run("editing a rule changes the value", func(t *testing.T) {
		before, after := cfgWith("https://old.example"), cfgWith("https://new.example")
		before.ComputeFingerprint()
		after.ComputeFingerprint()
		assert.NotEqual(t, before.Fingerprint(), after.Fingerprint())
	})

	t.Run("a disabled config has none", func(t *testing.T) {
		c := cfgWith("x")
		c.Enabled = false
		c.ComputeFingerprint()
		assert.Empty(t, c.Fingerprint())
	})

	t.Run("no rules means none", func(t *testing.T) {
		c := &TransformConfigPayload{Enabled: true}
		c.ComputeFingerprint()
		assert.Empty(t, c.Fingerprint())
	})

	// FindTransformConfig คืน nil ได้เมื่อ service ไม่มี transform ผูกอยู่
	t.Run("a nil config is safe to ask", func(t *testing.T) {
		var c *TransformConfigPayload
		assert.Empty(t, c.Fingerprint())
	})
}

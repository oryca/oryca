package cache

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientETag(t *testing.T) {
	raw := SyntheticETag([]byte(`{"links":[{"href":"https://upstream.example/x"}]}`))

	t.Run("no transform leaves the validator alone", func(t *testing.T) {
		assert.Equal(t, raw, ClientETag(raw, ""))
	})

	t.Run("no upstream validator stays empty", func(t *testing.T) {
		assert.Empty(t, ClientETag("", "abc123"))
	})

	// อาการที่ user รายงาน แก้ transform แล้วผลลัพธ์ไม่เปลี่ยนจนกว่า cache หมดอายุ
	// เพราะ body ดิบเหมือนเดิม ETag เลยเหมือนเดิม client ได้ 304 แล้วใช้ของเก่าต่อ
	t.Run("changing the rules changes the validator", func(t *testing.T) {
		before := ClientETag(raw, "aaaaaaaaaaaaaaaa")
		after := ClientETag(raw, "bbbbbbbbbbbbbbbb")

		assert.NotEqual(t, before, after, "a client holding the old body must not get a 304")
		assert.NotEqual(t, raw, before, "the raw validator must not be handed out as is")
	})

	t.Run("same rules keep the validator stable", func(t *testing.T) {
		assert.Equal(t, ClientETag(raw, "abc"), ClientETag(raw, "abc"))
	})

	t.Run("stays one quoted token", func(t *testing.T) {
		got := ClientETag(`W/"gw-deadbeef"`, "abc123")
		assert.Equal(t, `W/"gw-deadbeef-tabc123"`, got)
	})

	t.Run("works with an upstream supplied validator", func(t *testing.T) {
		got := ClientETag(`"abc-123"`, "ff00")
		assert.Equal(t, `"abc-123-tff00"`, got)
	})

	// upstream ที่ส่ง validator ไม่ตรง spec ต้องไม่ทำให้ ETag ที่ออกไปเสียรูป
	// ถ้าเสียรูป client จะเทียบไม่ตรงตลอด กลายเป็นไม่เคยได้ 304 อีกเลย
	t.Run("repairs a malformed upstream validator", func(t *testing.T) {
		for name, tc := range map[string]struct{ in, want string }{
			"unquoted":        {`abc123`, `"abc123-tff00"`},
			"padded":          {`  "abc"  `, `"abc-tff00"`},
			"weak unquoted":   {`W/abc`, `W/"abc-tff00"`},
			"weak with space": {`W/ "abc"`, `W/"abc-tff00"`},
		} {
			t.Run(name, func(t *testing.T) {
				assert.Equal(t, tc.want, ClientETag(tc.in, "ff00"))
			})
		}
	})

	// strong กับ weak มีความหมายต่างกัน เราไม่ควรเปลี่ยนให้เอง
	t.Run("keeps the strength the upstream chose", func(t *testing.T) {
		assert.True(t, strings.HasPrefix(ClientETag(`W/"a"`, "f0"), "W/"), "weak stays weak")
		assert.False(t, strings.HasPrefix(ClientETag(`"a"`, "f0"), "W/"), "strong stays strong")
	})

	// ผลลัพธ์ต้องมี quote ครบคู่เสมอ ไม่งั้น header เสียรูป
	t.Run("always balances the quotes", func(t *testing.T) {
		for _, in := range []string{`abc`, `"abc"`, `W/"abc"`, `W/abc`, `  "a b"  `} {
			got := ClientETag(in, "ff00")
			assert.Equal(t, 2, strings.Count(got, `"`), "unbalanced for input %q got %q", in, got)
		}
	})
}

package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	t.Run("length=32 ได้ 64 ตัวอักษร hex", func(t *testing.T) {
		token, err := GenerateToken(32)
		require.NoError(t, err)
		assert.Len(t, token, 64)
	})

	t.Run("length=16 ได้ 32 ตัวอักษร hex", func(t *testing.T) {
		token, err := GenerateToken(16)
		require.NoError(t, err)
		assert.Len(t, token, 32)
	})

	t.Run("สร้าง token ไม่ซ้ำกัน", func(t *testing.T) {
		t1, err := GenerateToken(32)
		require.NoError(t, err)
		t2, err := GenerateToken(32)
		require.NoError(t, err)
		assert.NotEqual(t, t1, t2)
	})

	t.Run("token เป็น hex string ที่ถูกต้อง", func(t *testing.T) {
		token, err := GenerateToken(32)
		require.NoError(t, err)
		for _, c := range token {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"character %c is not valid hex", c)
		}
	})
}

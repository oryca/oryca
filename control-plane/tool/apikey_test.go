package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateApiKey(t *testing.T) {
	t.Run("สร้าง key ได้ 64 ตัวอักษร hex", func(t *testing.T) {
		key, err := GenerateApiKey()
		require.NoError(t, err)
		assert.Len(t, key, 64)
	})

	t.Run("สร้าง key ไม่ซ้ำกัน", func(t *testing.T) {
		key1, err := GenerateApiKey()
		require.NoError(t, err)
		key2, err := GenerateApiKey()
		require.NoError(t, err)
		assert.NotEqual(t, key1, key2)
	})

	t.Run("key เป็น hex string ที่ถูกต้อง", func(t *testing.T) {
		key, err := GenerateApiKey()
		require.NoError(t, err)
		for _, c := range key {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"character %c is not valid hex", c)
		}
	})
}

func TestMaskApiKey(t *testing.T) {
	t.Run("key ยาวกว่า 8 ตัว mask ตรงกลาง", func(t *testing.T) {
		result := MaskApiKey("abcd1234efgh5678")
		assert.Equal(t, "abcd********5678", result)
	})

	t.Run("key ยาว 64 ตัว แสดง 4 ตัวแรกและ 4 ตัวท้าย", func(t *testing.T) {
		key := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
		result := MaskApiKey(key)
		assert.Equal(t, "abcd", result[:4])
		assert.Equal(t, "1234", result[len(result)-4:])
		assert.Len(t, result, 64)
	})

	t.Run("key สั้น 8 ตัวหรือน้อยกว่า mask ทั้งหมด", func(t *testing.T) {
		assert.Equal(t, "********", MaskApiKey("12345678"))
		assert.Equal(t, "***", MaskApiKey("abc"))
		assert.Equal(t, "", MaskApiKey(""))
	})

	t.Run("key ยาว 9 ตัว mask 1 ตัวกลาง", func(t *testing.T) {
		result := MaskApiKey("123456789")
		assert.Equal(t, "1234*6789", result)
	})
}

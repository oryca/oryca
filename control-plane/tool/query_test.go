package tool

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseLimit(t *testing.T) {
	t.Run("ค่าปกติ คืนค่าถูกต้อง", func(t *testing.T) {
		n, err := ParseLimit("20")
		require.NoError(t, err)
		assert.Equal(t, 20, n)
	})

	t.Run("ค่า 1 (ต่ำสุด) ผ่าน", func(t *testing.T) {
		n, err := ParseLimit("1")
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})

	t.Run("ค่า MaxLimit ผ่าน", func(t *testing.T) {
		n, err := ParseLimit("10000")
		require.NoError(t, err)
		assert.Equal(t, 10000, n)
	})

	t.Run("0 คืน error", func(t *testing.T) {
		_, err := ParseLimit("0")
		assert.Error(t, err)
	})

	t.Run("ติดลบ คืน error", func(t *testing.T) {
		_, err := ParseLimit("-1")
		assert.Error(t, err)
	})

	t.Run("เกิน MaxLimit คืน error", func(t *testing.T) {
		_, err := ParseLimit("10001")
		assert.Error(t, err)
	})

	t.Run("ไม่ใช่ตัวเลข คืน error", func(t *testing.T) {
		_, err := ParseLimit("abc")
		assert.Error(t, err)
	})
}

func TestParseOffset(t *testing.T) {
	t.Run("ค่าปกติ คืนค่าถูกต้อง", func(t *testing.T) {
		n, err := ParseOffset("10")
		require.NoError(t, err)
		assert.Equal(t, 10, n)
	})

	t.Run("0 ผ่าน (เริ่มต้น)", func(t *testing.T) {
		n, err := ParseOffset("0")
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("ติดลบ คืน error", func(t *testing.T) {
		_, err := ParseOffset("-1")
		assert.Error(t, err)
	})

	t.Run("ไม่ใช่ตัวเลข คืน error", func(t *testing.T) {
		_, err := ParseOffset("abc")
		assert.Error(t, err)
	})
}

// --- SearchWildcard ---

func TestSearchWildcard(t *testing.T) {
	t.Run("สร้าง regex filter ถูกต้อง", func(t *testing.T) {
		result := SearchWildcard("hello", "name")
		assert.Equal(t, bson.M{"name": bson.M{"$regex": "hello", "$options": "i"}}, result)
	})
}

// --- IsStringInSlice ---

func TestIsStringInSlice(t *testing.T) {
	slice := []string{"asc", "desc"}

	t.Run("พบค่าใน slice", func(t *testing.T) {
		assert.True(t, IsStringInSlice("asc", slice))
		assert.True(t, IsStringInSlice("desc", slice))
	})

	t.Run("ไม่พบค่าใน slice", func(t *testing.T) {
		assert.False(t, IsStringInSlice("up", slice))
	})

	t.Run("slice ว่าง คืน false", func(t *testing.T) {
		assert.False(t, IsStringInSlice("a", []string{}))
	})
}

// --- EscapeRegexPattern ---

func TestEscapeRegexPattern(t *testing.T) {
	t.Run("escape อักขระพิเศษ", func(t *testing.T) {
		assert.Equal(t, `hello\.world`, EscapeRegexPattern("hello.world"))
		assert.Equal(t, `a\+b\*c`, EscapeRegexPattern("a+b*c"))
		assert.Equal(t, `\(test\)`, EscapeRegexPattern("(test)"))
		assert.Equal(t, `\[0\]`, EscapeRegexPattern("[0]"))
		assert.Equal(t, `\{key\}`, EscapeRegexPattern("{key}"))
		assert.Equal(t, `\^start\$end`, EscapeRegexPattern("^start$end"))
		assert.Equal(t, `a\|b`, EscapeRegexPattern("a|b"))
		assert.Equal(t, `path\\to`, EscapeRegexPattern(`path\to`))
		assert.Equal(t, `test\?`, EscapeRegexPattern("test?"))
	})

	t.Run("ไม่มี special chars ไม่เปลี่ยน", func(t *testing.T) {
		assert.Equal(t, "hello", EscapeRegexPattern("hello"))
	})
}

// --- OptionSortBson ---

func TestOptionSortBson(t *testing.T) {
	t.Run("single field asc", func(t *testing.T) {
		result, err := OptionSortBson("name:asc")
		require.NoError(t, err)
		assert.Equal(t, bson.D{{Key: "name", Value: 1}}, result)
	})

	t.Run("single field desc", func(t *testing.T) {
		result, err := OptionSortBson("createdAt:desc")
		require.NoError(t, err)
		assert.Equal(t, bson.D{{Key: "createdAt", Value: -1}}, result)
	})

	t.Run("ไม่ระบุ direction default เป็น asc", func(t *testing.T) {
		result, err := OptionSortBson("name")
		require.NoError(t, err)
		assert.Equal(t, bson.D{{Key: "name", Value: 1}}, result)
	})

	t.Run("multiple fields", func(t *testing.T) {
		result, err := OptionSortBson("name:asc,createdAt:desc")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "name", result[0].Key)
		assert.Equal(t, 1, result[0].Value)
		assert.Equal(t, "createdAt", result[1].Key)
		assert.Equal(t, -1, result[1].Value)
	})

	t.Run("id แปลงเป็น _id", func(t *testing.T) {
		result, err := OptionSortBson("id:desc")
		require.NoError(t, err)
		assert.Equal(t, "_id", result[0].Key)
	})

	t.Run("ค่าว่าง คืน error", func(t *testing.T) {
		_, err := OptionSortBson("")
		assert.Error(t, err)
	})

	t.Run("direction ไม่ถูกต้อง คืน error", func(t *testing.T) {
		_, err := OptionSortBson("name:up")
		assert.Error(t, err)
	})

	t.Run("format ผิด (มากกว่า 2 parts) คืน error", func(t *testing.T) {
		_, err := OptionSortBson("a:b:c")
		assert.Error(t, err)
	})
}

// --- GenerateFilterBson ---

func TestGenerateFilterBson(t *testing.T) {
	ignored := []string{"sort", "offset", "limit"}

	t.Run("ข้าม ignored params", func(t *testing.T) {
		params := url.Values{"sort": {"name:asc"}, "limit": {"10"}, "name": {"test"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("ไม่มี params คืน empty", func(t *testing.T) {
		result, err := GenerateFilterBson(url.Values{}, ignored)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("ค่า null สร้าง $exists:false", func(t *testing.T) {
		params := url.Values{"deletedAt": {"null"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("ค่า !null สร้าง $exists:true,$ne:nil", func(t *testing.T) {
		params := url.Values{"deletedAt": {"!null"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("wildcard *value* สร้าง contains regex", func(t *testing.T) {
		params := url.Values{"name": {"*test*"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("wildcard *value สร้าง ends-with regex", func(t *testing.T) {
		params := url.Values{"name": {"*test"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("wildcard value* สร้าง starts-with regex", func(t *testing.T) {
		params := url.Values{"name": {"test*"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude !value สร้าง $not regex", func(t *testing.T) {
		params := url.Values{"status": {"!active"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude !*value* สร้าง $not contains", func(t *testing.T) {
		params := url.Values{"name": {"!*test*"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude !*value สร้าง $not ends-with", func(t *testing.T) {
		params := url.Values{"name": {"!*test"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude !value* สร้าง $not starts-with", func(t *testing.T) {
		params := url.Values{"name": {"!test*"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("ค่าตัวเลข สร้าง numeric filter ด้วย", func(t *testing.T) {
		params := url.Values{"age": {"25"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("ค่า boolean สร้าง bool filter ด้วย", func(t *testing.T) {
		params := url.Values{"enabled": {"true"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("ค่า ObjectID hex สร้าง ObjectID filter ด้วย", func(t *testing.T) {
		id := bson.NewObjectID()
		params := url.Values{"userId": {id.Hex()}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("ค่าว่าง คืน error", func(t *testing.T) {
		params := url.Values{"name": {""}}
		_, err := GenerateFilterBson(params, ignored)
		assert.Error(t, err)
	})

	t.Run(".id แปลงเป็น ._id", func(t *testing.T) {
		id := bson.NewObjectID()
		params := url.Values{"user.id": {id.Hex()}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("comma-separated values สร้าง OR", func(t *testing.T) {
		params := url.Values{"status": {"active,inactive"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude numeric !25 สร้าง $ne", func(t *testing.T) {
		params := url.Values{"age": {"!25"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude boolean !true สร้าง $ne", func(t *testing.T) {
		params := url.Values{"enabled": {"!true"}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("exclude ObjectID สร้าง $ne", func(t *testing.T) {
		id := bson.NewObjectID()
		params := url.Values{"userId": {"!" + id.Hex()}}
		result, err := GenerateFilterBson(params, ignored)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})
}

// --- ParseDatetimeInterval ---

func TestParseDatetimeInterval(t *testing.T) {
	t.Run("instant เดียว Start และ End เท่ากัน", func(t *testing.T) {
		iv, err := ParseDatetimeInterval("2024-01-15T00:00:00Z")
		require.NoError(t, err)
		require.NotNil(t, iv.Start)
		require.NotNil(t, iv.End)
		assert.Equal(t, *iv.Start, *iv.End)
	})

	t.Run("interval start/end", func(t *testing.T) {
		iv, err := ParseDatetimeInterval("2024-01-01T00:00:00Z/2024-12-31T23:59:59Z")
		require.NoError(t, err)
		require.NotNil(t, iv.Start)
		require.NotNil(t, iv.End)
		assert.True(t, iv.End.After(*iv.Start))
	})

	t.Run("open start ../end", func(t *testing.T) {
		iv, err := ParseDatetimeInterval("../2024-12-31T23:59:59Z")
		require.NoError(t, err)
		assert.Nil(t, iv.Start)
		require.NotNil(t, iv.End)
	})

	t.Run("open end start/..", func(t *testing.T) {
		iv, err := ParseDatetimeInterval("2024-01-01T00:00:00Z/..")
		require.NoError(t, err)
		require.NotNil(t, iv.Start)
		assert.Nil(t, iv.End)
	})

	t.Run("format ผิด คืน error", func(t *testing.T) {
		_, err := ParseDatetimeInterval("not-a-date")
		assert.Error(t, err)
	})

	t.Run("start format ผิด คืน error", func(t *testing.T) {
		_, err := ParseDatetimeInterval("bad/2024-12-31T23:59:59Z")
		assert.Error(t, err)
	})

	t.Run("end format ผิด คืน error", func(t *testing.T) {
		_, err := ParseDatetimeInterval("2024-01-01T00:00:00Z/bad")
		assert.Error(t, err)
	})
}

// --- ParseDatetimeParams ---

func TestParseDatetimeParams(t *testing.T) {
	allowed := []string{"createdAt", "expiredAt"}

	t.Run("ไม่มี datetime param คืน empty", func(t *testing.T) {
		params := url.Values{"limit": {"10"}}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("interval สร้าง $gte และ $lte", func(t *testing.T) {
		params := url.Values{"createdAt": {"2024-01-01T00:00:00Z/2024-12-31T23:59:59Z"}}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		require.Len(t, result, 1)
		cond := result[0]["createdAt"].(bson.M)
		assert.NotNil(t, cond["$gte"])
		assert.NotNil(t, cond["$lte"])
	})

	t.Run("open start สร้างเฉพาะ $lte", func(t *testing.T) {
		params := url.Values{"createdAt": {"../2024-12-31T23:59:59Z"}}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		require.Len(t, result, 1)
		cond := result[0]["createdAt"].(bson.M)
		assert.Nil(t, cond["$gte"])
		assert.NotNil(t, cond["$lte"])
	})

	t.Run("open end สร้างเฉพาะ $gte", func(t *testing.T) {
		params := url.Values{"createdAt": {"2024-01-01T00:00:00Z/.."}}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		require.Len(t, result, 1)
		cond := result[0]["createdAt"].(bson.M)
		assert.NotNil(t, cond["$gte"])
		assert.Nil(t, cond["$lte"])
	})

	t.Run("field ที่ไม่อยู่ใน whitelist ถูกข้าม", func(t *testing.T) {
		params := url.Values{"updatedAt": {"2024-01-01T00:00:00Z/.."}}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("หลาย fields คืน filter หลายตัว", func(t *testing.T) {
		params := url.Values{
			"createdAt": {"2024-01-01T00:00:00Z/.."},
			"expiredAt": {"../2024-12-31T23:59:59Z"},
		}
		result, err := ParseDatetimeParams(params, allowed)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("format ผิด คืน error", func(t *testing.T) {
		params := url.Values{"createdAt": {"bad-value"}}
		_, err := ParseDatetimeParams(params, allowed)
		assert.Error(t, err)
	})
}

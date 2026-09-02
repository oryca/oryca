package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVariablesCtx(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/response-transforms/variables?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func decodeVariablesResp(t *testing.T, rec *httptest.ResponseRecorder) model.ListResponse[transformVariable] {
	t.Helper()
	var body model.ListResponse[transformVariable]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return body
}

func TestTransformConfigVariables(t *testing.T) {
	e := echo.New()
	h := &TransformConfigHandler{}

	t.Run("ต้องได้ status 200", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "")
		require.NoError(t, h.Variables(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ต้องได้ Content-Type เป็น JSON", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "")
		h.Variables(c)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	})

	t.Run("ต้องได้ response ตาม ListResponse format", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		assert.Equal(t, body.NumberMatched, body.NumberReturned)
		assert.Equal(t, len(body.Items), body.NumberReturned)
	})

	t.Run("ทุก variable ต้องมี name, description, example", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		require.NotEmpty(t, body.Items)
		for _, v := range body.Items {
			assert.NotEmpty(t, v.Name)
			assert.NotEmpty(t, v.Description)
			assert.NotEmpty(t, v.Example)
		}
	})

	t.Run("ต้องมี {{oryca_gateway_url}} และ {{oryca_auth}}", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		names := make(map[string]bool)
		for _, v := range body.Items {
			names[v.Name] = true
		}
		assert.True(t, names["{{oryca_gateway_url}}"])
		assert.True(t, names["{{oryca_auth}}"])
	})

	t.Run("search ค้นหา name และ description", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "search=gateway")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		require.NotEmpty(t, body.Items)
		for _, v := range body.Items {
			matched := matchWildcard("gateway", v.Name) || matchWildcard("gateway", v.Description)
			assert.True(t, matched, "variable %q should match search 'gateway'", v.Name)
		}
	})

	t.Run("search ไม่ match ต้องได้ items ว่าง", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "search=xxxxnotfound")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		assert.Empty(t, body.Items)
		assert.Equal(t, 0, body.NumberMatched)
	})

	t.Run("search case-insensitive", func(t *testing.T) {
		c1, rec1 := newVariablesCtx(e, "search=GATEWAY")
		h.Variables(c1)
		c2, rec2 := newVariablesCtx(e, "search=gateway")
		h.Variables(c2)

		assert.Equal(t, decodeVariablesResp(t, rec1).NumberMatched, decodeVariablesResp(t, rec2).NumberMatched)
	})

	t.Run("search wildcard *auth* ต้องได้ตรง pattern", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "search=*auth*")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		require.NotEmpty(t, body.Items)
		for _, v := range body.Items {
			matched := matchWildcard("*auth*", v.Name) || matchWildcard("*auth*", v.Description)
			assert.True(t, matched, "variable %q should match '*auth*'", v.Name)
		}
	})

	t.Run("name filter ค้นหาเฉพาะ name field", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "name=*gateway*")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		require.NotEmpty(t, body.Items)
		for _, v := range body.Items {
			assert.True(t, matchWildcard("*gateway*", v.Name), "name %q should match '*gateway*'", v.Name)
		}
	})

	t.Run("name filter wildcard oryca_* ต้องได้ทุก variable ที่ชื่อขึ้นต้น oryca_", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "name=*oryca_*")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		require.NotEmpty(t, body.Items)
		for _, v := range body.Items {
			assert.True(t, matchWildcard("*oryca_*", v.Name), "name %q should match '*oryca_*'", v.Name)
		}
	})

	t.Run("search และ name filter ใช้พร้อมกัน ต้อง AND กัน", func(t *testing.T) {
		c, rec := newVariablesCtx(e, "search=url&name=*gateway*")
		h.Variables(c)

		body := decodeVariablesResp(t, rec)
		for _, v := range body.Items {
			matchedSearch := matchWildcard("url", v.Name) || matchWildcard("url", v.Description)
			matchedName := matchWildcard("*gateway*", v.Name)
			assert.True(t, matchedSearch && matchedName, "variable %q should match both filters", v.Name)
		}
	})
}

func TestMatchWildcard(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"gateway", "oryca_gateway_url", true},
		{"gateway", "oryca_auth", false},
		{"*auth*", "{{oryca_auth}}", true},
		{"*auth*", "{{oryca_gateway_url}}", false},
		{"oryca_*", "oryca_gateway_url", true},
		{"oryca_*", "other_var", false},
		{"*_url", "oryca_gateway_url", true},
		{"*_url", "oryca_auth", false},
		{"", "anything", true},
		{"*", "anything", true},
	}

	for _, tc := range cases {
		t.Run(tc.pattern+"/"+tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, matchWildcard(tc.pattern, tc.input))
		})
	}
}

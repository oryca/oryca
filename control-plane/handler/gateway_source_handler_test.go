package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testSrcAlias = "my-src"
	testSrcName  = "My Source"
)

// --- mocks ---

type mockGatewaySourceSvc struct{ mock.Mock }

func (m *mockGatewaySourceSvc) List(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewaySource), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockGatewaySourceSvc) GetByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error) {
	args := m.Called(ctx, idOrAlias)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceSvc) Create(ctx context.Context, body *model.GatewaySourceCreate, ctxUser *model.User) (*model.GatewaySource, error) {
	args := m.Called(ctx, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceSvc) Update(ctx context.Context, idOrAlias string, body *model.GatewaySourceUpdate, ctxUser *model.User) (*model.GatewaySource, error) {
	args := m.Called(ctx, idOrAlias, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceSvc) Delete(ctx context.Context, idOrAlias string, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, idOrAlias, forever, ctxUser).Error(0)
}

const srcBasePath = "/control-plane/api/v1/sources"

func newGatewaySourceHandler(svc *mockGatewaySourceSvc) *GatewaySourceHandler {
	return NewGatewaySourceHandler(svc)
}

func newSrcListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, srcBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newSrcGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, srcBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sourceId")
	c.SetParamValues(id)
	return c, rec
}

func newSrcPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, srcBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newSrcPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, srcBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sourceId")
	c.SetParamValues(id)
	return c, rec
}

func newSrcDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := srcBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sourceId")
	c.SetParamValues(id)
	return c, rec
}

// --- GetSources ---

func TestGetSources(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{Role: "admin"}

	t.Run("ไม่มี user ใน context ต้องได้ 403", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcListCtx(e)
		require.NoError(t, newGatewaySourceHandler(svc).GetSources(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("user role ต้องได้ 200 และ public fields เท่านั้น", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		list := []*model.GatewaySource{{Alias: "src-1", Name: "Source 1", Type: "upstream"}}
		svc.On("List", mock.Anything, mock.Anything).Return(list, int64(1), nil)
		c, rec := newSrcListCtx(e)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewaySourceHandler(svc).GetSources(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.GatewaySourcePublic]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 1, resp.NumberMatched)
	})

	t.Run("มีข้อมูล ต้องได้ 200 และ list response", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		list := []*model.GatewaySource{{Alias: "src-1"}, {Alias: "src-2"}}
		svc.On("List", mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newSrcListCtx(e)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).GetSources(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.GatewaySource]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newSrcListCtx(e)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).GetSources(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.GatewaySource]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotNil(t, resp.Items)
	})
}

// --- GetSource ---

func TestGetSource(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{Role: "admin"}
	srcID := bson.NewObjectID().Hex()

	t.Run("user role ต้องได้ 200 และ public fields เท่านั้น", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		src := &model.GatewaySource{Alias: testSrcAlias, Name: testSrcName, Type: "upstream"}
		svc.On("GetByIDOrAlias", mock.Anything, srcID).Return(src, nil)
		c, rec := newSrcGetCtx(e, srcID)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewaySourceHandler(svc).GetSource(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.GatewaySourcePublic
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, testSrcAlias, resp.Alias)
	})

	t.Run("พบ source ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		src := &model.GatewaySource{Alias: testSrcAlias}
		svc.On("GetByIDOrAlias", mock.Anything, srcID).Return(src, nil)

		c, rec := newSrcGetCtx(e, srcID)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).GetSource(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ไม่พบ source ต้องได้ 404", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("GetByIDOrAlias", mock.Anything, "unknown").Return(nil, errors.New("not found"))

		c, rec := newSrcGetCtx(e, "unknown")
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).GetSource(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewaySourceNotFound, ex.Code)
	})
}

// --- CreateSource ---

func TestCreateSource(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	validBody := map[string]any{"alias": "my-alias", "name": testSrcName, "type": "upstream"}

	t.Run("ไม่มี user ใน context ต้องได้ 403", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPostCtx(e, validBody)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("user role ต้องได้ 403", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPostCtx(e, validBody)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่มี alias ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPostCtx(e, map[string]any{"name": "x", "type": "upstream"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี name ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPostCtx(e, map[string]any{"alias": "x", "type": "upstream"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี type ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPostCtx(e, map[string]any{"alias": "x", "name": "x"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ 201", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		src := &model.GatewaySource{Alias: "my-alias"}
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(src, nil)

		c, rec := newSrcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("alias ซ้ำ ต้องได้ 409 และ error code", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, service.ErrGatewaySourceAliasDuplicate)

		c, rec := newSrcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).CreateSource(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewaySourceAliasDuplicate, ex.Code)
	})
}

// --- UpdateSource ---

func TestUpdateSource(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	srcID := bson.NewObjectID().Hex()
	validBody := map[string]any{"alias": "my-alias", "name": testSrcName, "type": "upstream"}

	t.Run("ไม่มี user ใน context ต้องได้ 403", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPutCtx(e, srcID, validBody)
		require.NoError(t, newGatewaySourceHandler(svc).UpdateSource(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่มี required fields ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcPutCtx(e, srcID, map[string]any{})
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).UpdateSource(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		src := &model.GatewaySource{Alias: "my-alias"}
		svc.On("Update", mock.Anything, srcID, mock.Anything, adminUser).Return(src, nil)

		c, rec := newSrcPutCtx(e, srcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).UpdateSource(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ไม่พบ source ต้องได้ 404", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Update", mock.Anything, srcID, mock.Anything, adminUser).Return(nil, service.ErrGatewaySourceNotFound)

		c, rec := newSrcPutCtx(e, srcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).UpdateSource(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("alias ซ้ำ ต้องได้ 409", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Update", mock.Anything, srcID, mock.Anything, adminUser).Return(nil, service.ErrGatewaySourceAliasDuplicate)

		c, rec := newSrcPutCtx(e, srcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).UpdateSource(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

// --- DeleteSource ---

func TestDeleteSource(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	srcID := bson.NewObjectID().Hex()

	t.Run("ไม่มี user ใน context ต้องได้ 403", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		c, rec := newSrcDeleteCtx(e, srcID, false)
		require.NoError(t, newGatewaySourceHandler(svc).DeleteSource(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ลบสำเร็จ ต้องได้ 204", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Delete", mock.Anything, srcID, false, adminUser).Return(nil)

		c, rec := newSrcDeleteCtx(e, srcID, false)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).DeleteSource(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ไม่พบ source ต้องได้ 404", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Delete", mock.Anything, srcID, false, adminUser).Return(service.ErrGatewaySourceNotFound)

		c, rec := newSrcDeleteCtx(e, srcID, false)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).DeleteSource(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewaySourceNotFound, ex.Code)
	})

	t.Run("alias ถูกใช้งานอยู่ ต้องได้ 422", func(t *testing.T) {
		svc := new(mockGatewaySourceSvc)
		svc.On("Delete", mock.Anything, srcID, false, adminUser).Return(service.ErrGatewaySourceAliasInUse)

		c, rec := newSrcDeleteCtx(e, srcID, false)
		c.Set("user", adminUser)
		require.NoError(t, newGatewaySourceHandler(svc).DeleteSource(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

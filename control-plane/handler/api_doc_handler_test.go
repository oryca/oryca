package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type mockApiDocSvc struct{ mock.Mock }

func (m *mockApiDocSvc) List(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.ApiDoc), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockApiDocSvc) GetByID(ctx context.Context, id bson.ObjectID, params url.Values) (*model.ApiDoc, error) {
	args := m.Called(ctx, id, params)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiDoc), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockApiDocSvc) Create(ctx context.Context, body *model.ApiDocCreate, ctxUser *model.User) (*model.ApiDoc, error) {
	args := m.Called(ctx, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiDoc), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockApiDocSvc) Update(ctx context.Context, id bson.ObjectID, body *model.ApiDocUpdate, ctxUser *model.User) (*model.ApiDoc, error) {
	args := m.Called(ctx, id, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiDoc), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockApiDocSvc) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, id, forever, ctxUser).Error(0)
}
func (m *mockApiDocSvc) GetPathsByID(ctx context.Context, id bson.ObjectID, params url.Values) ([]*model.ApiDocPath, int64, error) {
	args := m.Called(ctx, id, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.ApiDocPath), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

const apiDocBasePath = "/control-plane/api/v1/api-docs"

func newApiDocHandler(svc *mockApiDocSvc) *ApiDocHandler {
	return NewApiDocHandler(svc, "https://api.example.com")
}

func newApiDocListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, apiDocBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newApiDocGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, apiDocBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiDocId")
	c.SetParamValues(id)
	return c, rec
}

func newApiDocPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, apiDocBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newApiDocPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, apiDocBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiDocId")
	c.SetParamValues(id)
	return c, rec
}

func newApiDocDeleteCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodDelete, apiDocBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiDocId")
	c.SetParamValues(id)
	return c, rec
}

// --- GetApiDocs ---

func TestGetApiDocs(t *testing.T) {
	e := echo.New()

	t.Run("ไม่มี user ต้องบังคับ authenRequired=none และได้ 200", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("List", mock.Anything, mock.MatchedBy(func(p url.Values) bool {
			return p.Get("authenRequired") == "none" && p.Get("publishStatus") == "published"
		})).Return([]*model.ApiDoc{}, int64(0), nil)

		c, rec := newApiDocListCtx(e)
		require.NoError(t, newApiDocHandler(svc).GetApiDocs(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("admin ต้องเห็นทุก publishStatus และได้ 200", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("List", mock.Anything, mock.MatchedBy(func(p url.Values) bool {
			return p.Get("publishStatus") == ""
		})).Return([]*model.ApiDoc{{Info: model.ApiDocInfo{Title: "Doc"}}}, int64(1), nil)

		c, rec := newApiDocListCtx(e)
		c.Set("user", &model.User{Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).GetApiDocs(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.ApiDoc]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 1, resp.NumberMatched)
	})
}

// --- GetApiDoc ---

func TestGetApiDoc(t *testing.T) {
	e := echo.New()
	id := bson.NewObjectID()

	t.Run("id format ผิด ต้องได้ 404", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		c, rec := newApiDocGetCtx(e, "not-an-id")
		require.NoError(t, newApiDocHandler(svc).GetApiDoc(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("ไม่พบเอกสาร ต้องได้ 404", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("GetByID", mock.Anything, id, mock.Anything).Return(nil, service.ErrApiDocNotFound)

		c, rec := newApiDocGetCtx(e, id.Hex())
		require.NoError(t, newApiDocHandler(svc).GetApiDoc(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("พบเอกสาร ต้องได้ 200", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		doc := &model.ApiDoc{ID: id, Info: model.ApiDocInfo{Title: "Doc"}}
		svc.On("GetByID", mock.Anything, id, mock.Anything).Return(doc, nil)

		c, rec := newApiDocGetCtx(e, id.Hex())
		require.NoError(t, newApiDocHandler(svc).GetApiDoc(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// --- CreateApiDoc ---

func TestCreateApiDoc(t *testing.T) {
	e := echo.New()
	validBody := map[string]any{
		"authenRequired": model.ApiDocAuthenRequiredNone,
		"publishStatus":  model.ApiDocPublishStatusDraft,
		"info":           map[string]any{"title": "Doc", "version": "1.0.0"},
	}

	t.Run("ไม่มี user ต้องได้ 403", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		c, rec := newApiDocPostCtx(e, validBody)
		require.NoError(t, newApiDocHandler(svc).CreateApiDoc(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("authenRequired ไม่ถูกต้อง ต้องได้ 400", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		body := map[string]any{
			"authenRequired": "invalid",
			"publishStatus":  model.ApiDocPublishStatusDraft,
			"info":           map[string]any{"title": "Doc", "version": "1.0.0"},
		}
		c, rec := newApiDocPostCtx(e, body)
		c.Set("user", &model.User{Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).CreateApiDoc(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("admin สร้างเอกสารถูกต้อง ต้องได้ 201", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		created := &model.ApiDoc{ID: bson.NewObjectID(), Info: model.ApiDocInfo{Title: "Doc", Version: "1.0.0"}}
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(created, nil)

		c, rec := newApiDocPostCtx(e, validBody)
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).CreateApiDoc(c))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}

// --- UpdateApiDoc ---

func TestUpdateApiDoc(t *testing.T) {
	e := echo.New()
	id := bson.NewObjectID()
	validBody := map[string]any{
		"authenRequired": model.ApiDocAuthenRequiredNone,
		"publishStatus":  model.ApiDocPublishStatusPublished,
		"info":           map[string]any{"title": "Doc", "version": "1.0.0"},
	}

	t.Run("ไม่มี user ต้องได้ 403", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		c, rec := newApiDocPutCtx(e, id.Hex(), validBody)
		require.NoError(t, newApiDocHandler(svc).UpdateApiDoc(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่พบเอกสาร ต้องได้ 404", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("Update", mock.Anything, id, mock.Anything, mock.Anything).Return(nil, service.ErrApiDocNotFound)

		c, rec := newApiDocPutCtx(e, id.Hex(), validBody)
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).UpdateApiDoc(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("admin อัปเดตสำเร็จ ต้องได้ 200", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		updated := &model.ApiDoc{ID: id, PublishStatus: model.ApiDocPublishStatusPublished}
		svc.On("Update", mock.Anything, id, mock.Anything, mock.Anything).Return(updated, nil)

		c, rec := newApiDocPutCtx(e, id.Hex(), validBody)
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).UpdateApiDoc(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// --- DeleteApiDoc ---

func TestDeleteApiDoc(t *testing.T) {
	e := echo.New()
	id := bson.NewObjectID()

	t.Run("ไม่มี user ต้องได้ 403", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		c, rec := newApiDocDeleteCtx(e, id.Hex())
		require.NoError(t, newApiDocHandler(svc).DeleteApiDoc(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่พบเอกสาร ต้องได้ 404", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("Delete", mock.Anything, id, false, mock.Anything).Return(service.ErrApiDocNotFound)

		c, rec := newApiDocDeleteCtx(e, id.Hex())
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).DeleteApiDoc(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("admin ลบสำเร็จ ต้องได้ 204", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("Delete", mock.Anything, id, false, mock.Anything).Return(nil)

		c, rec := newApiDocDeleteCtx(e, id.Hex())
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "admin"})
		require.NoError(t, newApiDocHandler(svc).DeleteApiDoc(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

// --- GetApiDocPaths ---

func TestGetApiDocPaths(t *testing.T) {
	e := echo.New()
	id := bson.NewObjectID()

	t.Run("ไม่พบเอกสาร ต้องได้ 404", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		svc.On("GetPathsByID", mock.Anything, id, mock.Anything).Return(nil, int64(0), service.ErrApiDocNotFound)

		c, rec := newApiDocGetCtx(e, id.Hex())
		c.SetParamNames("apiDocId")
		c.SetParamValues(id.Hex())
		require.NoError(t, newApiDocHandler(svc).GetApiDocPaths(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("พบ paths ต้องได้ 200 พร้อม list response", func(t *testing.T) {
		svc := new(mockApiDocSvc)
		paths := []*model.ApiDocPath{{Path: "/foo", Method: "GET"}}
		svc.On("GetPathsByID", mock.Anything, id, mock.Anything).Return(paths, int64(1), nil)

		c, rec := newApiDocGetCtx(e, id.Hex())
		require.NoError(t, newApiDocHandler(svc).GetApiDocPaths(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.ApiDocPath]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 1, resp.NumberMatched)
	})
}

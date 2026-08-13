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

// --- mocks ---

type mockGatewayServiceSvc struct{ mock.Mock }

func (m *mockGatewayServiceSvc) ListWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewayServiceWithSource), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockGatewayServiceSvc) GetByIDWithSource(ctx context.Context, id string) (*model.GatewayServiceWithSource, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayServiceWithSource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceSvc) Create(ctx context.Context, body *model.GatewayServiceCreate, ctxUser *model.User) (*model.GatewayServiceWithSource, error) {
	args := m.Called(ctx, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayServiceWithSource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceSvc) Update(ctx context.Context, id string, body *model.GatewayServiceUpdate, ctxUser *model.User) (*model.GatewayServiceWithSource, error) {
	args := m.Called(ctx, id, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayServiceWithSource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceSvc) Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, id, forever, ctxUser).Error(0)
}
func (m *mockGatewayServiceSvc) CheckPaths(ctx context.Context, basePath string, resourcePaths []string, excludeID *bson.ObjectID) (*model.GatewayServiceCheckPathsResponse, error) {
	args := m.Called(ctx, basePath, resourcePaths, excludeID)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayServiceCheckPathsResponse), args.Error(1)
	}
	return nil, args.Error(1)
}

const (
	svcBasePath      = "/control-plane/api/v1/services"
	msgUserForbidden = "user role ต้องได้ 403"
	msgNoCtxUser     = "ไม่มี user ใน context ต้องได้ 403"
	testSvcBasePath  = "/my-svc"
	testSvcName      = "My Service"
)

func newGatewayServiceHandler(svc *mockGatewayServiceSvc) *GatewayServiceHandler {
	return NewGatewayServiceHandler(svc)
}

func newSvcListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, svcBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newSvcGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, svcBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("serviceId")
	c.SetParamValues(id)
	return c, rec
}

func newSvcPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, svcBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newSvcPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, svcBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("serviceId")
	c.SetParamValues(id)
	return c, rec
}

func newSvcDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := svcBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("serviceId")
	c.SetParamValues(id)
	return c, rec
}

// --- GetServices ---

func TestGetServices(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{Role: "admin"}

	t.Run(msgNoCtxUser, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcListCtx(e)
		require.NoError(t, newGatewayServiceHandler(svc).GetServices(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("admin มีข้อมูล ต้องได้ 200 และ list response", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		list := []*model.GatewayServiceWithSource{{Name: "svc-1"}, {Name: "svc-2"}}
		svc.On("ListWithSource", mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newSvcListCtx(e)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).GetServices(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.GatewayServiceWithSource]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
	})

	t.Run("user role ดูได้เฉพาะ enabled=true ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		enabled := true
		list := []*model.GatewayServiceWithSource{{Name: "svc-1", Enabled: &enabled}}
		svc.On("ListWithSource", mock.Anything, mock.Anything).Return(list, int64(1), nil)

		c, rec := newSvcListCtx(e)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewayServiceHandler(svc).GetServices(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("ListWithSource", mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newSvcListCtx(e)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).GetServices(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.GatewayServiceWithSource]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotNil(t, resp.Items)
	})
}

// --- GetService ---

func TestGetService(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{Role: "admin"}
	svcID := bson.NewObjectID().Hex()
	enabled := true
	disabled := false

	t.Run(msgNoCtxUser, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcGetCtx(e, svcID)
		require.NoError(t, newGatewayServiceHandler(svc).GetService(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("admin พบ service ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		gs := &model.GatewayServiceWithSource{Name: testSvcName}
		svc.On("GetByIDWithSource", mock.Anything, svcID).Return(gs, nil)

		c, rec := newSvcGetCtx(e, svcID)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).GetService(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("user role พบ service enabled=true ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		gs := &model.GatewayServiceWithSource{Name: testSvcName, Enabled: &enabled}
		svc.On("GetByIDWithSource", mock.Anything, svcID).Return(gs, nil)

		c, rec := newSvcGetCtx(e, svcID)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewayServiceHandler(svc).GetService(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("user role พบ service enabled=false ต้องได้ 404", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		gs := &model.GatewayServiceWithSource{Name: testSvcName, Enabled: &disabled}
		svc.On("GetByIDWithSource", mock.Anything, svcID).Return(gs, nil)

		c, rec := newSvcGetCtx(e, svcID)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewayServiceHandler(svc).GetService(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("ไม่พบ service ต้องได้ 404 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("GetByIDWithSource", mock.Anything, svcID).Return(nil, errors.New("not found"))

		c, rec := newSvcGetCtx(e, svcID)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).GetService(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceNotFound, ex.Code)
	})
}

// --- CreateService ---

func TestCreateService(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	validBody := map[string]any{"name": testSvcName, "type": "General", "basePath": testSvcBasePath}

	t.Run(msgNoCtxUser, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPostCtx(e, validBody)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่มี name ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPostCtx(e, map[string]any{"type": "General", "basePath": "/x"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี type ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPostCtx(e, map[string]any{"name": "x", "basePath": "/x"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี basePath ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPostCtx(e, map[string]any{"name": "x", "type": "General"})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ 201", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		gs := &model.GatewayServiceWithSource{Name: testSvcName}
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(gs, nil)

		c, rec := newSvcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("basePath ซ้ำ ต้องได้ 409 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceBasePathDuplicate)

		c, rec := newSvcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceBasePathDuplicate, ex.Code)
	})

	t.Run("resourcePath ชน ต้องได้ 409 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceResourcePathConflict)

		c, rec := newSvcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceResourcePathConflict, ex.Code)
	})

	t.Run("sourceAlias ไม่มีใน DB ต้องได้ 400 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceSourceAliasInvalid)

		c, rec := newSvcPostCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CreateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceSourceAliasInvalid, ex.Code)
	})
}

// --- UpdateService ---

func TestUpdateService(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	svcID := bson.NewObjectID().Hex()
	validBody := map[string]any{"name": testSvcName, "type": "General", "basePath": testSvcBasePath}

	t.Run(msgNoCtxUser, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPutCtx(e, svcID, validBody)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่มี required fields ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcPutCtx(e, svcID, map[string]any{})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ 200", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		gs := &model.GatewayServiceWithSource{Name: testSvcName}
		svc.On("Update", mock.Anything, svcID, mock.Anything, adminUser).Return(gs, nil)

		c, rec := newSvcPutCtx(e, svcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ไม่พบ service ต้องได้ 404", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Update", mock.Anything, svcID, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceNotFound)

		c, rec := newSvcPutCtx(e, svcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("basePath ซ้ำ ต้องได้ 409 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Update", mock.Anything, svcID, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceBasePathDuplicate)

		c, rec := newSvcPutCtx(e, svcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceBasePathDuplicate, ex.Code)
	})

	t.Run("resourcePath ชน ต้องได้ 409", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Update", mock.Anything, svcID, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceResourcePathConflict)

		c, rec := newSvcPutCtx(e, svcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("sourceAlias ไม่มีใน DB ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Update", mock.Anything, svcID, mock.Anything, adminUser).Return(nil, service.ErrGatewayServiceSourceAliasInvalid)

		c, rec := newSvcPutCtx(e, svcID, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).UpdateService(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- CheckResourcePaths ---

func newSvcCheckPathsCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, svcBasePath+"/check-paths", bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestCheckResourcePaths(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{Role: "admin"}
	validBody := map[string]any{
		"basePath":      testSvcBasePath,
		"resourcePaths": []string{"/a", "/b"},
	}

	t.Run(msgUserForbidden, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcCheckPathsCtx(e, validBody)
		c.Set("user", &model.User{Role: "user"})
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ไม่มี basePath ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcCheckPathsCtx(e, map[string]any{"resourcePaths": []string{"/a"}})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี resourcePaths ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcCheckPathsCtx(e, map[string]any{"basePath": testSvcBasePath})
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("wildcard ไม่อยู่ท้าย path ต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		body := map[string]any{"basePath": testSvcBasePath, "resourcePaths": []string{"/a/*/b"}}
		c, rec := newSvcCheckPathsCtx(e, body)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("excludeServiceId format ผิดต้องได้ 400", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		body := map[string]any{"basePath": testSvcBasePath, "resourcePaths": []string{"/a"}, "excludeServiceId": "invalid"}
		c, rec := newSvcCheckPathsCtx(e, body)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ไม่มี conflict ต้องได้ 200 hasConflict=false", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("CheckPaths", mock.Anything, testSvcBasePath, []string{"/a", "/b"}, (*bson.ObjectID)(nil)).
			Return(&model.GatewayServiceCheckPathsResponse{HasConflict: false}, nil)

		c, rec := newSvcCheckPathsCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.GatewayServiceCheckPathsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.False(t, resp.HasConflict)
	})

	t.Run("basePath ชน ต้องได้ 200 hasConflict=true พร้อม basePathConflict", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		resp := &model.GatewayServiceCheckPathsResponse{
			HasConflict: true,
			BasePathConflict: &model.BasePathConflictInfo{
				ConflictingServiceID:   bson.NewObjectID().Hex(),
				ConflictingServiceName: "Other Service",
				ConflictingBasePath:    testSvcBasePath,
			},
		}
		svc.On("CheckPaths", mock.Anything, testSvcBasePath, []string{"/a", "/b"}, (*bson.ObjectID)(nil)).Return(resp, nil)

		c, rec := newSvcCheckPathsCtx(e, validBody)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.GatewayServiceCheckPathsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.True(t, got.HasConflict)
		assert.NotNil(t, got.BasePathConflict)
	})

	t.Run("resourcePath ชนกันภายใน ต้องได้ 200 hasConflict=true พร้อม resourcePathConflicts", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		body := map[string]any{"basePath": testSvcBasePath, "resourcePaths": []string{"/a", "/a"}}
		resp := &model.GatewayServiceCheckPathsResponse{
			HasConflict: true,
			ResourcePathConflicts: []*model.InternalPathConflict{
				{ConflictingPaths: []string{"/a", "/a"}},
			},
		}
		svc.On("CheckPaths", mock.Anything, testSvcBasePath, []string{"/a", "/a"}, (*bson.ObjectID)(nil)).Return(resp, nil)

		c, rec := newSvcCheckPathsCtx(e, body)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).CheckResourcePaths(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.GatewayServiceCheckPathsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.True(t, got.HasConflict)
		assert.Len(t, got.ResourcePathConflicts, 1)
	})
}

// --- DeleteService ---

func TestDeleteService(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	svcID := bson.NewObjectID().Hex()

	t.Run(msgNoCtxUser, func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		c, rec := newSvcDeleteCtx(e, svcID, false)
		require.NoError(t, newGatewayServiceHandler(svc).DeleteService(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ลบสำเร็จ ต้องได้ 204", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Delete", mock.Anything, svcID, false, adminUser).Return(nil)

		c, rec := newSvcDeleteCtx(e, svcID, false)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).DeleteService(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ไม่พบ service ต้องได้ 404 และ error code", func(t *testing.T) {
		svc := new(mockGatewayServiceSvc)
		svc.On("Delete", mock.Anything, svcID, false, adminUser).Return(service.ErrGatewayServiceNotFound)

		c, rec := newSvcDeleteCtx(e, svcID, false)
		c.Set("user", adminUser)
		require.NoError(t, newGatewayServiceHandler(svc).DeleteService(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var ex model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&ex))
		assert.Equal(t, tool.CodeGatewayServiceNotFound, ex.Code)
	})
}

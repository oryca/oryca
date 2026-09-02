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

// --- mock ---

type mockApiKeyService struct {
	mock.Mock
}

func (m *mockApiKeyService) List(ctx context.Context, params url.Values, ctxUser *model.User) ([]*model.ApiKey, int64, error) {
	args := m.Called(ctx, params, ctxUser)
	if v := args.Get(0); v != nil {
		return v.([]*model.ApiKey), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockApiKeyService) GetByID(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error) {
	args := m.Called(ctx, id, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyService) Create(ctx context.Context, body *model.ApiKeyCreate, ctxUser *model.User, targetUser *model.User) (*model.ApiKey, error) {
	args := m.Called(ctx, body, ctxUser, targetUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyService) Update(ctx context.Context, id bson.ObjectID, body *model.ApiKeyUpdate, ctxUser *model.User) (*model.ApiKey, error) {
	args := m.Called(ctx, id, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyService) Regenerate(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error) {
	args := m.Called(ctx, id, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, id, forever, ctxUser).Error(0)
}

func (m *mockApiKeyService) BulkDelete(ctx context.Context, body *model.ApiKeyBulkDelete, ctxUser *model.User) error {
	return m.Called(ctx, body, ctxUser).Error(0)
}

type mockApiKeyUserFinder struct {
	mock.Mock
}

func (m *mockApiKeyUserFinder) GetByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- constants ---

const (
	akBasePath     = "/control-plane/api/v1/api-keys"
	akErrDB        = "db error"
	akSubForbidden = "ไม่มี user ใน context ต้องได้ status 403"
	akSubBadID     = "id ผิดรูปแบบ ต้องได้ status 400 และ error code"
	akSubSvcError  = "service error ต้องได้ status 422 และ error code"
)

// --- helpers ---

func newAKHandler(svc apiKeyService, userSvc apiKeyUserFinder) *ApiKeyHandler {
	return NewApiKeyHandler(svc, userSvc)
}

func newAKListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, akBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newAKListCtxWithQuery(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, akBasePath+"?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newAKGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, akBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKeyId")
	c.SetParamValues(id)
	return c, rec
}

func newAKPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, akBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newAKPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, akBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKeyId")
	c.SetParamValues(id)
	return c, rec
}

func newAKRegenerateCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, akBasePath+"/"+id+"/regenerate", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKeyId")
	c.SetParamValues(id)
	return c, rec
}

func newAKDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := akBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKeyId")
	c.SetParamValues(id)
	return c, rec
}

func newAKBulkDeleteCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, akBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- GetApiKeys ---

func TestGetApiKeys(t *testing.T) {
	e := echo.New()
	ownerID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}
	normalUser := &model.User{ID: ownerID, Role: "user"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtx(e)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run("มีข้อมูล ต้องได้ status 200 และ list response", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		list := []*model.ApiKey{
			{ID: bson.NewObjectID(), Name: "key1", ApiKey: "abc123", OwnerBy: &ownerID, CreatedBy: &ownerID},
			{ID: bson.NewObjectID(), Name: "key2", ApiKey: "def456", OwnerBy: &ownerID, CreatedBy: &ownerID},
		}
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newAKListCtx(e)
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.ApiKey]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
		assert.Equal(t, 2, resp.NumberReturned)
		assert.Len(t, resp.Items, 2)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newAKListCtx(e)
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.ApiKey]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 0, resp.NumberMatched)
		assert.NotNil(t, resp.Items)
	})

	t.Run("service คืน ErrApiKeyForbidden ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), service.ErrApiKeyForbidden)

		c, rec := newAKListCtx(e)
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), errors.New(akErrDB))

		c, rec := newAKListCtx(e)
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})

	t.Run("limit=0 คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "limit=0")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run("limit ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "limit=-1")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit เกิน 10000 คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "limit=10001")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "limit=abc")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("offset ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "offset=-1")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run("offset ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKListCtxWithQuery(e, "offset=abc")
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("root ดู key ของคนอื่น apiKey ต้องถูก mask", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		otherID := bson.NewObjectID()
		list := []*model.ApiKey{
			{ID: bson.NewObjectID(), Name: "other-key", ApiKey: "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234", OwnerBy: &otherID, CreatedBy: &otherID},
		}
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(list, int64(1), nil)

		c, rec := newAKListCtx(e)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.ApiKey]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Len(t, resp.Items, 1)
		assert.Contains(t, resp.Items[0].ApiKey, "****")
	})
}

// --- GetApiKey ---

func TestGetApiKey(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	ownerID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}
	ownerUser := &model.User{ID: ownerID, Role: "user"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKGetCtx(e, validID.Hex())
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("พบข้อมูล (เจ้าของ) ต้องได้ status 200 และ apiKey ไม่ถูก mask", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		apiKeyValue := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
		ak := &model.ApiKey{ID: validID, Name: "my-key", ApiKey: apiKeyValue, OwnerBy: &ownerID, CreatedBy: &ownerID}
		svc.On("GetByID", mock.Anything, validID, mock.Anything).Return(ak, nil)

		c, rec := newAKGetCtx(e, validID.Hex())
		c.Set("user", ownerUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, apiKeyValue, resp.ApiKey)
	})

	t.Run("root ดู key ของคนอื่น apiKey ต้องถูก mask", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		apiKeyValue := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
		ak := &model.ApiKey{ID: validID, Name: "other-key", ApiKey: apiKeyValue, OwnerBy: &ownerID, CreatedBy: &ownerID}
		svc.On("GetByID", mock.Anything, validID, mock.Anything).Return(ak, nil)

		c, rec := newAKGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Contains(t, resp.ApiKey, "****")
		assert.NotEqual(t, apiKeyValue, resp.ApiKey)
	})

	t.Run(akSubBadID, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKGetCtx(e, "invalid-id")
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ไม่พบ key ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("GetByID", mock.Anything, validID, mock.Anything).Return(nil, service.ErrApiKeyNotFound)

		c, rec := newAKGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyNotFound, resp.Code)
	})

	t.Run("ไม่มีสิทธิ์ดู ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("GetByID", mock.Anything, validID, mock.Anything).Return(nil, service.ErrApiKeyForbidden)

		c, rec := newAKGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("GetByID", mock.Anything, validID, mock.Anything).Return(nil, errors.New(akErrDB))

		c, rec := newAKGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).GetApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- CreateApiKey ---

func TestCreateApiKey(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}
	normalUser := &model.User{ID: bson.NewObjectID(), Role: "user"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKPostCtx(e, map[string]any{"name": "new-key"})
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("สร้างสำเร็จ (ไม่ระบุ userId) ต้องได้ status 201", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		created := &model.ApiKey{ID: bson.NewObjectID(), Name: "new-key", Description: "my desc", ApiKey: "generatedkey123"}
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil)

		c, rec := newAKPostCtx(e, map[string]any{"name": "new-key", "description": "my desc"})
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "new-key", resp.Name)
		assert.Equal(t, "my desc", resp.Description)
	})

	t.Run("สร้างสำเร็จ (ระบุ userId) ต้องได้ status 201", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		targetID := bson.NewObjectID()
		targetUser := &model.User{ID: targetID, Role: "user"}
		userSvc.On("GetByID", mock.Anything, mock.Anything).Return(targetUser, nil)
		created := &model.ApiKey{ID: bson.NewObjectID(), Name: "assigned-key", OwnerBy: &targetID}
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil)

		c, rec := newAKPostCtx(e, map[string]any{"name": "assigned-key", "userId": targetID.Hex()})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		req := httptest.NewRequest(http.MethodPost, akBasePath, bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", normalUser)

		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("target user ไม่พบ ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		targetID := bson.NewObjectID()
		userSvc.On("GetByID", mock.Anything, mock.Anything).Return(nil, errors.New("not found"))

		c, rec := newAKPostCtx(e, map[string]any{"name": "key", "userId": targetID.Hex()})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("service คืน ErrApiKeyForbidden ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrApiKeyForbidden)

		c, rec := newAKPostCtx(e, map[string]any{"name": "key"})
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New(akErrDB))

		c, rec := newAKPostCtx(e, map[string]any{"name": "key"})
		c.Set("user", normalUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})

	t.Run("admin สร้าง key ให้คนอื่น apiKey ต้องถูก mask ใน response", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		adminID := bson.NewObjectID()
		adminUser := &model.User{ID: adminID, Role: "admin"}
		targetID := bson.NewObjectID()
		targetUser := &model.User{ID: targetID, Role: "user"}
		apiKeyValue := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
		userSvc.On("GetByID", mock.Anything, mock.Anything).Return(targetUser, nil)
		created := &model.ApiKey{ID: bson.NewObjectID(), Name: "assigned-key", ApiKey: apiKeyValue, OwnerBy: &targetID}
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil)

		c, rec := newAKPostCtx(e, map[string]any{"name": "assigned-key", "userId": targetID.Hex()})
		c.Set("user", adminUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Contains(t, resp.ApiKey, "****")
		assert.NotEqual(t, apiKeyValue, resp.ApiKey)
	})

	t.Run("user สร้าง key ของตัวเอง apiKey ไม่ถูก mask ใน response", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		userID := bson.NewObjectID()
		selfUser := &model.User{ID: userID, Role: "user"}
		apiKeyValue := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
		created := &model.ApiKey{ID: bson.NewObjectID(), Name: "my-key", ApiKey: apiKeyValue, OwnerBy: &userID, CreatedBy: &userID}
		svc.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil)

		c, rec := newAKPostCtx(e, map[string]any{"name": "my-key"})
		c.Set("user", selfUser)
		err := newAKHandler(svc, userSvc).CreateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, apiKeyValue, resp.ApiKey)
		assert.NotContains(t, resp.ApiKey, "****")
	})
}

// --- UpdateApiKey ---

func TestUpdateApiKey(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKPutCtx(e, validID.Hex(), map[string]any{"name": "updated"})
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(akSubBadID, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKPutCtx(e, "bad-id", map[string]any{"name": "updated"})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		req := httptest.NewRequest(http.MethodPut, akBasePath+"/"+validID.Hex(), bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("apiKeyId")
		c.SetParamValues(validID.Hex())
		c.Set("user", rootUser)

		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ status 200", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		updated := &model.ApiKey{ID: validID, Name: "updated-key", Description: "updated desc"}
		svc.On("Update", mock.Anything, validID, mock.Anything, mock.Anything).Return(updated, nil)

		c, rec := newAKPutCtx(e, validID.Hex(), map[string]any{"name": "updated-key", "description": "updated desc"})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "updated-key", resp.Name)
		assert.Equal(t, "updated desc", resp.Description)
	})

	t.Run("ไม่พบ key ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Update", mock.Anything, validID, mock.Anything, mock.Anything).Return(nil, service.ErrApiKeyNotFound)

		c, rec := newAKPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyNotFound, resp.Code)
	})

	t.Run("ไม่มีสิทธิ์แก้ไข ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Update", mock.Anything, validID, mock.Anything, mock.Anything).Return(nil, service.ErrApiKeyForbidden)

		c, rec := newAKPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Update", mock.Anything, validID, mock.Anything, mock.Anything).Return(nil, errors.New(akErrDB))

		c, rec := newAKPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).UpdateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- RegenerateApiKey ---

func TestRegenerateApiKey(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKRegenerateCtx(e, validID.Hex())
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(akSubBadID, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKRegenerateCtx(e, "bad-id")
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("regenerate สำเร็จ ต้องได้ status 200 และ key ใหม่", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		rootID := rootUser.ID
		updated := &model.ApiKey{ID: validID, ApiKey: "newkey123", OwnerBy: &rootID}
		svc.On("Regenerate", mock.Anything, validID, mock.Anything).Return(updated, nil)

		c, rec := newAKRegenerateCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ApiKey
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "newkey123", resp.ApiKey)
	})

	t.Run("ไม่พบ key ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Regenerate", mock.Anything, validID, mock.Anything).Return(nil, service.ErrApiKeyNotFound)

		c, rec := newAKRegenerateCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyNotFound, resp.Code)
	})

	t.Run("ไม่มีสิทธิ์ regenerate ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Regenerate", mock.Anything, validID, mock.Anything).Return(nil, service.ErrApiKeyForbidden)

		c, rec := newAKRegenerateCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Regenerate", mock.Anything, validID, mock.Anything).Return(nil, errors.New(akErrDB))

		c, rec := newAKRegenerateCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).RegenerateApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- DeleteApiKey ---

func TestDeleteApiKey(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKDeleteCtx(e, validID.Hex(), false)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(akSubBadID, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKDeleteCtx(e, "bad-id", false)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ลบสำเร็จ (soft delete) ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Delete", mock.Anything, validID, false, mock.Anything).Return(nil)

		c, rec := newAKDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ลบสำเร็จ (hard delete, forever=true) ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Delete", mock.Anything, validID, true, mock.Anything).Return(nil)

		c, rec := newAKDeleteCtx(e, validID.Hex(), true)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ไม่พบ key ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Delete", mock.Anything, validID, false, mock.Anything).Return(service.ErrApiKeyNotFound)

		c, rec := newAKDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyNotFound, resp.Code)
	})

	t.Run("ไม่มีสิทธิ์ลบ ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Delete", mock.Anything, validID, false, mock.Anything).Return(service.ErrApiKeyForbidden)

		c, rec := newAKDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("Delete", mock.Anything, validID, false, mock.Anything).Return(errors.New(akErrDB))

		c, rec := newAKDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).DeleteApiKey(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- BulkDeleteApiKeys ---

func TestBulkDeleteApiKeys(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}

	t.Run(akSubForbidden, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		c, rec := newAKBulkDeleteCtx(e, map[string]any{
			"apiKeyIds": []string{bson.NewObjectID().Hex()},
			"forever":   false,
		})
		err := newAKHandler(svc, userSvc).BulkDeleteApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)

		req := httptest.NewRequest(http.MethodDelete, akBasePath, bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", rootUser)

		err := newAKHandler(svc, userSvc).BulkDeleteApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("ลบสำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		c, rec := newAKBulkDeleteCtx(e, map[string]any{
			"apiKeyIds": []string{bson.NewObjectID().Hex()},
			"forever":   false,
		})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).BulkDeleteApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("service คืน ErrApiKeyForbidden ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(service.ErrApiKeyForbidden)

		c, rec := newAKBulkDeleteCtx(e, map[string]any{
			"apiKeyIds": []string{bson.NewObjectID().Hex()},
			"forever":   false,
		})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).BulkDeleteApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeApiKeyForbidden, resp.Code)
	})

	t.Run(akSubSvcError, func(t *testing.T) {
		svc := new(mockApiKeyService)
		userSvc := new(mockApiKeyUserFinder)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(errors.New(akErrDB))

		c, rec := newAKBulkDeleteCtx(e, map[string]any{
			"apiKeyIds": []string{bson.NewObjectID().Hex()},
			"forever":   false,
		})
		c.Set("user", rootUser)
		err := newAKHandler(svc, userSvc).BulkDeleteApiKeys(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

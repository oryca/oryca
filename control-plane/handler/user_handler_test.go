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

type mockUserService struct {
	mock.Mock
}

func (m *mockUserService) List(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error) {
	args := m.Called(ctx, params, excludeRoot)
	if v := args.Get(0); v != nil {
		return v.([]*model.User), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockUserService) GetByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) Create(ctx context.Context, body *model.UserCreate) (*model.User, error) {
	args := m.Called(ctx, body)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) Update(ctx context.Context, id bson.ObjectID, body *model.UserUpdate) (*model.User, error) {
	args := m.Called(ctx, id, body)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUserID bson.ObjectID) error {
	return m.Called(ctx, id, forever, ctxUserID).Error(0)
}

func (m *mockUserService) BulkDelete(ctx context.Context, body *model.UserBulkDelete, ctxUserID bson.ObjectID) error {
	return m.Called(ctx, body, ctxUserID).Error(0)
}

// --- constants ---

const (
	userBasePath = "/control-plane/api/v1/users"

	userFirstName    = "John"
	userErrDB        = "db error"
	userSubBadID     = "id ผิดรูปแบบ ต้องได้ status 400 และ error code"
	userSubSvcErr    = "service error ต้องได้ status 422 และ error code"
	userSubNotFound  = "ไม่พบ user ต้องได้ status 404 และ error code"
	userSubForbidden = "ไม่มี user ใน context ต้องได้ status 403"
)

// --- helpers ---

func newUserHandler(svc userService) *UserHandler {
	return NewUserHandler(svc)
}

func newUserListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, userBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newUserListCtxWithQuery(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, userBasePath+"?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newUserGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, userBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(id)
	return c, rec
}

func newUserGetPublicCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, userBasePath+"/"+id+"/public", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(id)
	return c, rec
}

func newUserPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, userBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newUserPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, userBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(id)
	return c, rec
}

func newUserDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := userBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("userId")
	c.SetParamValues(id)
	return c, rec
}

func newUserBulkDeleteCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, userBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- GetUsers ---

func TestGetUsers(t *testing.T) {
	e := echo.New()

	t.Run("มีข้อมูล ต้องได้ status 200 และ list response", func(t *testing.T) {
		svc := new(mockUserService)
		list := []*model.User{{FirstName: userFirstName}, {FirstName: "Jane"}}
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newUserListCtx(e)
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.User]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
		assert.Len(t, resp.Items, 2)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newUserListCtx(e)
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.User]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 0, resp.NumberMatched)
		assert.NotNil(t, resp.Items)
	})

	t.Run(userSubSvcErr, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), errors.New(userErrDB))

		c, rec := newUserListCtx(e)
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})

	t.Run("limit=0 คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "limit=0")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("limit ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "limit=-1")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit เกิน 10000 คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "limit=10001")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "limit=abc")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("offset ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "offset=-1")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("offset ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockUserService)
		c, rec := newUserListCtxWithQuery(e, "offset=abc")
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("user role ต้องได้ UserPublic ไม่ใช่ full user", func(t *testing.T) {
		svc := new(mockUserService)
		list := []*model.User{
			{ID: bson.NewObjectID(), FirstName: userFirstName, Role: "admin"},
			{ID: bson.NewObjectID(), FirstName: "Jane", Role: "user"},
		}
		svc.On("List", mock.Anything, mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newUserListCtx(e)
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "user"})
		err := newUserHandler(svc).GetUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.UserPublic]
		require.NoError(t, json.Unmarshal([]byte(rec.Body.Bytes()), &resp))
		assert.Equal(t, 2, resp.NumberMatched)
		assert.Equal(t, "admin", resp.Items[0].Role)
		assert.Equal(t, "user", resp.Items[1].Role)
	})
}

// --- GetUser ---

func TestGetUser(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()

	t.Run("พบข้อมูล ต้องได้ status 200 และ user", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(&model.User{ID: validID, FirstName: userFirstName}, nil)

		c, rec := newUserGetCtx(e, validID.Hex())
		err := newUserHandler(svc).GetUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.User
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, userFirstName, resp.FirstName)
	})

	t.Run(userSubBadID, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserGetCtx(e, "bad-id")
		err := newUserHandler(svc).GetUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run(userSubNotFound, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, service.ErrUserNotFound)

		c, rec := newUserGetCtx(e, validID.Hex())
		err := newUserHandler(svc).GetUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("user role ดูตัวเองต้องได้ full user", func(t *testing.T) {
		svc := new(mockUserService)
		selfID := bson.NewObjectID()
		svc.On("GetByID", mock.Anything, selfID).Return(&model.User{ID: selfID, FirstName: userFirstName, Role: "user"}, nil)

		c, rec := newUserGetCtx(e, selfID.Hex())
		c.Set("user", &model.User{ID: selfID, Role: "user"})
		err := newUserHandler(svc).GetUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		raw := rec.Body.String()
		assert.Contains(t, raw, `"role"`)
	})

	t.Run("user role ดูคนอื่นต้องได้ UserPublic เท่านั้น", func(t *testing.T) {
		svc := new(mockUserService)
		otherID := bson.NewObjectID()
		svc.On("GetByID", mock.Anything, otherID).Return(&model.User{ID: otherID, FirstName: "Jane", Role: "user"}, nil)

		c, rec := newUserGetCtx(e, otherID.Hex())
		c.Set("user", &model.User{ID: bson.NewObjectID(), Role: "user"})
		err := newUserHandler(svc).GetUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.UserPublic
		require.NoError(t, json.Unmarshal([]byte(rec.Body.Bytes()), &resp))
		assert.Equal(t, otherID, resp.ID)
		assert.Equal(t, "user", resp.Role)
	})
}

// --- GetUserPublic ---

func TestGetUserPublic(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()

	t.Run("พบข้อมูล ต้องได้ status 200 และ public profile เท่านั้น", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(&model.User{
			ID: validID, FirstName: userFirstName, LastName: "Doe",
		}, nil)

		c, rec := newUserGetPublicCtx(e, validID.Hex())
		err := newUserHandler(svc).GetUserPublic(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.UserPublic
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, userFirstName, resp.FirstName)
		assert.Equal(t, "Doe", resp.LastName)
	})

	t.Run(userSubBadID, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserGetPublicCtx(e, "bad-id")
		err := newUserHandler(svc).GetUserPublic(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run(userSubNotFound, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, service.ErrUserNotFound)

		c, rec := newUserGetPublicCtx(e, validID.Hex())
		err := newUserHandler(svc).GetUserPublic(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- CreateUser ---

func TestCreateUser(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run(userSubForbidden, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName})
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ status 201 และ user", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(&model.User{FirstName: userFirstName}, nil)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe", "email": "john@example.com"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.User
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, userFirstName, resp.FirstName)
	})

	t.Run("email ซ้ำ ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, service.ErrEmailDuplicate)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe", "email": "dup@example.com"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailDuplicate, resp.Code)
	})

	t.Run("username ซ้ำ ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, service.ErrUsernameDuplicate)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe", "username": "dupuser"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUsernameDuplicate, resp.Code)
	})

	t.Run("phone ซ้ำ ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, service.ErrPhoneDuplicate)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe", "phone": "0891234567"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodePhoneDuplicate, resp.Code)
	})

	t.Run("role ไม่ถูกต้อง ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, service.ErrInvalidRole)

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe", "role": "superadmin"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserRoleInvalid, resp.Code)
	})

	t.Run(userSubSvcErr, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New(userErrDB))

		c, rec := newUserPostCtx(e, map[string]any{"firstName": userFirstName, "lastName": "Doe"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).CreateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- UpdateUser ---

func TestUpdateUser(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}
	userTarget := &model.User{ID: validID, Role: "user"}

	t.Run(userSubForbidden, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserPutCtx(e, validID.Hex(), map[string]any{})
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ status 200 และ user ที่อัปเดตแล้ว", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(userTarget, nil).Once()
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(&model.User{ID: validID, FirstName: "Updated"}, nil)

		c, rec := newUserPutCtx(e, validID.Hex(), map[string]any{"firstName": "Updated"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.User
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Updated", resp.FirstName)
	})

	t.Run(userSubBadID, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserPutCtx(e, "bad-id", map[string]any{})
		c.Set("user", rootUser)
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run(userSubNotFound, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, service.ErrUserNotFound)

		c, rec := newUserPutCtx(e, validID.Hex(), map[string]any{})
		c.Set("user", rootUser)
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("role ไม่ถูกต้อง ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(userTarget, nil).Once()
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(nil, service.ErrInvalidRole)

		c, rec := newUserPutCtx(e, validID.Hex(), map[string]any{"role": "superadmin"})
		c.Set("user", rootUser)
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserRoleInvalid, resp.Code)
	})

	t.Run(userSubSvcErr, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(userTarget, nil).Once()
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(nil, errors.New(userErrDB))

		c, rec := newUserPutCtx(e, validID.Hex(), map[string]any{})
		c.Set("user", rootUser)
		err := newUserHandler(svc).UpdateUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- DeleteUser ---

func TestDeleteUser(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	ctxUserID := bson.NewObjectID()
	rootUser := &model.User{ID: ctxUserID, Role: "root"}
	userTarget := &model.User{ID: validID, Role: "user"}

	t.Run(userSubForbidden, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserDeleteCtx(e, validID.Hex(), false)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ลบสำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(userTarget, nil)
		svc.On("Delete", mock.Anything, validID, false, ctxUserID).Return(nil)

		c, rec := newUserDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ลบแบบ hard delete (forever=true) ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(userTarget, nil)
		svc.On("Delete", mock.Anything, validID, true, ctxUserID).Return(nil)

		c, rec := newUserDeleteCtx(e, validID.Hex(), true)
		c.Set("user", rootUser)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run(userSubBadID, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserDeleteCtx(e, "bad-id", false)
		c.Set("user", rootUser)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ลบตัวเองไม่ได้ ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		// target ID == ctxUser ID → canDeleteUser returns false
		svc.On("GetByID", mock.Anything, ctxUserID).Return(&model.User{ID: ctxUserID, Role: "admin"}, nil)

		c, rec := newUserDeleteCtx(e, ctxUserID.Hex(), false)
		c.Set("user", rootUser)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeCannotDeleteAccount, resp.Code)
	})

	t.Run(userSubNotFound, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, service.ErrUserNotFound)

		c, rec := newUserDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newUserHandler(svc).DeleteUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})
}

// --- BulkDeleteUsers ---

func TestBulkDeleteUsers(t *testing.T) {
	e := echo.New()
	id1 := bson.NewObjectID()
	id2 := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run(userSubForbidden, func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserBulkDeleteCtx(e, map[string]any{"userIds": []string{id1.Hex()}})
		err := newUserHandler(svc).BulkDeleteUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ลบหลาย user สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		c, rec := newUserBulkDeleteCtx(e, map[string]any{
			"userIds": []string{id1.Hex(), id2.Hex()},
			"forever": false,
		})
		c.Set("user", rootUser)
		err := newUserHandler(svc).BulkDeleteUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("userIds ว่าง ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockUserService)

		c, rec := newUserBulkDeleteCtx(e, map[string]any{"userIds": []string{}})
		c.Set("user", rootUser)
		err := newUserHandler(svc).BulkDeleteUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("มี userId ของตัวเองอยู่ใน list ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(service.ErrCannotDeleteSelf)

		c, rec := newUserBulkDeleteCtx(e, map[string]any{
			"userIds": []string{id1.Hex()},
			"forever": false,
		})
		c.Set("user", rootUser)
		err := newUserHandler(svc).BulkDeleteUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeCannotDeleteAccount, resp.Code)
	})

	t.Run(userSubSvcErr, func(t *testing.T) {
		svc := new(mockUserService)
		svc.On("BulkDelete", mock.Anything, mock.Anything, mock.Anything).Return(errors.New(userErrDB))

		c, rec := newUserBulkDeleteCtx(e, map[string]any{
			"userIds": []string{id1.Hex()},
		})
		c.Set("user", rootUser)
		err := newUserHandler(svc).BulkDeleteUsers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

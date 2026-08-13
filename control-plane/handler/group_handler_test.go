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

type mockGroupService struct {
	mock.Mock
}

func (m *mockGroupService) List(ctx context.Context, params url.Values) ([]*model.Group, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.Group), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockGroupService) GetByID(ctx context.Context, id bson.ObjectID) (*model.Group, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.Group), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupService) Create(ctx context.Context, body *model.GroupCreate, ctxUser *model.User) (*model.Group, error) {
	args := m.Called(ctx, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.Group), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupService) Update(ctx context.Context, id bson.ObjectID, body *model.GroupUpdate, ctxUser *model.User) (*model.Group, error) {
	args := m.Called(ctx, id, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.Group), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, id, forever, ctxUser).Error(0)
}

func (m *mockGroupService) GetGroupUsers(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error) {
	args := m.Called(ctx, groupID, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GroupUserDetail), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockGroupService) AddGroupUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, ctxUser *model.User) (*model.GroupUserAddResult, error) {
	args := m.Called(ctx, groupID, userIDs, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.GroupUserAddResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupService) RemoveGroupUsers(ctx context.Context, groupID bson.ObjectID, body *model.GroupUserRemove, ctxUser *model.User) error {
	return m.Called(ctx, groupID, body, ctxUser).Error(0)
}

// --- constants ---

const (
	grpBasePath  = "/control-plane/api/v1/groups"
	grpErrDB     = "db error"
	grpSubBadID  = "groupId ผิดรูปแบบ ต้องได้ status 400 และ error code"
	grpSubNoUser = "ไม่มี user ใน context ต้องได้ status 403"
	grpSubNoRole = "role ไม่มีสิทธิ์ต้องได้ status 403"
	grpSubSvcErr = "service error ต้องได้ status 422 และ error code"
)

// --- helpers ---

func newGrpHandler(svc groupService) *GroupHandler {
	return NewGroupHandler(svc)
}

func newGrpListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, grpBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newGrpListCtxWithQuery(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, grpBasePath+"?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newGrpGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, grpBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(id)
	return c, rec
}

func newGrpPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, grpBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newGrpPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, grpBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(id)
	return c, rec
}

func newGrpDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := grpBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(id)
	return c, rec
}

func newGrpUsersGetCtx(e *echo.Echo, groupID string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, grpBasePath+"/"+groupID+"/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	return c, rec
}

func newGrpUsersGetCtxWithQuery(e *echo.Echo, groupID, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, grpBasePath+"/"+groupID+"/users?"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	return c, rec
}

func newGrpUsersPostCtx(e *echo.Echo, groupID string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, grpBasePath+"/"+groupID+"/users", bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	return c, rec
}

func newGrpUsersDeleteCtx(e *echo.Echo, groupID string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, grpBasePath+"/"+groupID+"/users", bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	return c, rec
}

// --- GetGroups ---

func TestGetGroups(t *testing.T) {
	e := echo.New()

	t.Run("มีข้อมูล ต้องได้ status 200 และ list response", func(t *testing.T) {
		svc := new(mockGroupService)
		list := []*model.Group{{ID: bson.NewObjectID(), Name: "Engineering"}}
		svc.On("List", mock.Anything, mock.Anything).Return(list, int64(1), nil)

		c, rec := newGrpListCtx(e)
		require.NoError(t, newGrpHandler(svc).GetGroups(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.ListResponse[*model.Group]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 1, resp.NumberMatched)
		assert.Len(t, resp.Items, 1)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newGrpListCtx(e)
		require.NoError(t, newGrpHandler(svc).GetGroups(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.ListResponse[*model.Group]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotNil(t, resp.Items)
		assert.Len(t, resp.Items, 0)
	})

	t.Run("limit ไม่ถูกต้อง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpListCtxWithQuery(e, "limit=abc")
		require.NoError(t, newGrpHandler(svc).GetGroups(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run("offset ไม่ถูกต้อง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpListCtxWithQuery(e, "offset=-1")
		require.NoError(t, newGrpHandler(svc).GetGroups(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), errors.New(grpErrDB))

		c, rec := newGrpListCtx(e)
		require.NoError(t, newGrpHandler(svc).GetGroups(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- GetGroup ---

func TestGetGroup(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()

	t.Run("พบ group ต้องได้ status 200", func(t *testing.T) {
		svc := new(mockGroupService)
		group := &model.Group{ID: validID, Name: "Engineering", UserCount: 5}
		svc.On("GetByID", mock.Anything, validID).Return(group, nil)

		c, rec := newGrpGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroup(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.Group
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Engineering", resp.Name)
		assert.Equal(t, 5, resp.UserCount)
	})

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpGetCtx(e, "bad-id")
		require.NoError(t, newGrpHandler(svc).GetGroup(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, errors.New("not found"))

		c, rec := newGrpGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroup(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})
}

// --- CreateGroup ---

func TestCreateGroup(t *testing.T) {
	e := echo.New()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	nonAdminUser := &model.User{ID: bson.NewObjectID(), Role: "user"}

	t.Run(grpSubNoUser, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPostCtx(e, map[string]any{"name": "Engineering"})
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run(grpSubNoRole, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPostCtx(e, map[string]any{"name": "Engineering"})
		c.Set("user", nonAdminUser)
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ status 201", func(t *testing.T) {
		svc := new(mockGroupService)
		created := &model.Group{ID: bson.NewObjectID(), Name: "Engineering"}
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(created, nil)

		c, rec := newGrpPostCtx(e, map[string]any{"name": "Engineering"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusCreated, rec.Code)
		var resp model.Group
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Engineering", resp.Name)
	})

	t.Run("name ว่าง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPostCtx(e, map[string]any{"description": "no name"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("alias ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, service.ErrGroupAliasDuplicate)

		c, rec := newGrpPostCtx(e, map[string]any{"name": "Engineering", "alias": "eng"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusConflict, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupAliasDuplicate, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Create", mock.Anything, mock.Anything, adminUser).Return(nil, errors.New(grpErrDB))

		c, rec := newGrpPostCtx(e, map[string]any{"name": "Engineering"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).CreateGroup(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- UpdateGroup ---

func TestUpdateGroup(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	nonAdminUser := &model.User{ID: bson.NewObjectID(), Role: "user"}

	t.Run(grpSubNoUser, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "New Name"})
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run(grpSubNoRole, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "New Name"})
		c.Set("user", nonAdminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpPutCtx(e, "bad-id", map[string]any{"name": "New Name"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ status 200", func(t *testing.T) {
		svc := new(mockGroupService)
		updated := &model.Group{ID: validID, Name: "New Name"}
		svc.On("Update", mock.Anything, validID, mock.Anything, adminUser).Return(updated, nil)

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "New Name"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.Group
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "New Name", resp.Name)
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Update", mock.Anything, validID, mock.Anything, adminUser).Return(nil, service.ErrGroupNotFound)

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})

	t.Run("alias ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Update", mock.Anything, validID, mock.Anything, adminUser).Return(nil, service.ErrGroupAliasDuplicate)

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "x", "alias": "dup"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusConflict, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupAliasDuplicate, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Update", mock.Anything, validID, mock.Anything, adminUser).Return(nil, errors.New(grpErrDB))

		c, rec := newGrpPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).UpdateGroup(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- DeleteGroup ---

func TestDeleteGroup(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	nonAdminUser := &model.User{ID: bson.NewObjectID(), Role: "user"}

	t.Run(grpSubNoUser, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpDeleteCtx(e, validID.Hex(), false)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run(grpSubNoRole, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpDeleteCtx(e, validID.Hex(), false)
		c.Set("user", nonAdminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpDeleteCtx(e, "bad-id", false)
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("soft delete สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Delete", mock.Anything, validID, false, adminUser).Return(nil)

		c, rec := newGrpDeleteCtx(e, validID.Hex(), false)
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("hard delete (?forever=true) สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Delete", mock.Anything, validID, true, adminUser).Return(nil)

		c, rec := newGrpDeleteCtx(e, validID.Hex(), true)
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Delete", mock.Anything, validID, false, adminUser).Return(service.ErrGroupNotFound)

		c, rec := newGrpDeleteCtx(e, validID.Hex(), false)
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("Delete", mock.Anything, validID, false, adminUser).Return(errors.New(grpErrDB))

		c, rec := newGrpDeleteCtx(e, validID.Hex(), false)
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).DeleteGroup(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- GetGroupUsers ---

func TestGetGroupUsers(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersGetCtx(e, "bad-id")
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("คืน list users พร้อม joined data ต้องได้ status 200", func(t *testing.T) {
		svc := new(mockGroupService)
		uid := bson.NewObjectID()
		list := []*model.GroupUserDetail{{
			ID:   bson.NewObjectID(),
			User: model.GroupUserUserRef{ID: uid, Email: "user@example.com"},
		}}
		svc.On("GetGroupUsers", mock.Anything, validID, mock.Anything).Return(list, int64(1), nil)

		c, rec := newGrpUsersGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.ListResponse[*model.GroupUserDetail]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 1, resp.NumberMatched)
		assert.Len(t, resp.Items, 1)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("GetGroupUsers", mock.Anything, validID, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newGrpUsersGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp model.ListResponse[*model.GroupUserDetail]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotNil(t, resp.Items)
	})

	t.Run("limit ไม่ถูกต้อง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersGetCtxWithQuery(e, validID.Hex(), "limit=abc")
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run("offset ไม่ถูกต้อง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersGetCtxWithQuery(e, validID.Hex(), "offset=-1")
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, resp.Code)
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("GetGroupUsers", mock.Anything, validID, mock.Anything).Return(nil, int64(0), service.ErrGroupNotFound)

		c, rec := newGrpUsersGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("GetGroupUsers", mock.Anything, validID, mock.Anything).Return(nil, int64(0), errors.New(grpErrDB))

		c, rec := newGrpUsersGetCtx(e, validID.Hex())
		require.NoError(t, newGrpHandler(svc).GetGroupUsers(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- AddGroupUsers ---

func TestAddGroupUsers(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	nonAdminUser := &model.User{ID: bson.NewObjectID(), Role: "user"}
	uid1 := bson.NewObjectID()
	uid2 := bson.NewObjectID()

	t.Run(grpSubNoUser, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run(grpSubNoRole, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", nonAdminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersPostCtx(e, "bad-id", map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("userIds ว่าง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{"userIds": []string{}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("เพิ่มสำเร็จ ต้องได้ status 201 พร้อม added/skipped/groupId", func(t *testing.T) {
		svc := new(mockGroupService)
		result := &model.GroupUserAddResult{Added: 2, Skipped: 0}
		svc.On("AddGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(result, nil)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{
			"userIds": []string{uid1.Hex(), uid2.Hex()},
		})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusCreated, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupUserAdded, resp.Code)
		require.NotNil(t, resp.Properties)
		assert.Equal(t, validID.Hex(), resp.Properties["groupId"])
		assert.EqualValues(t, 2, resp.Properties["added"])
		assert.EqualValues(t, 0, resp.Properties["skipped"])
	})

	t.Run("บาง user ซ้ำ ต้องได้ added/skipped ถูกต้อง", func(t *testing.T) {
		svc := new(mockGroupService)
		result := &model.GroupUserAddResult{Added: 1, Skipped: 1}
		svc.On("AddGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(result, nil)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{
			"userIds": []string{uid1.Hex(), uid2.Hex()},
		})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusCreated, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.EqualValues(t, 1, resp.Properties["added"])
		assert.EqualValues(t, 1, resp.Properties["skipped"])
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("AddGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(nil, service.ErrGroupNotFound)

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("AddGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(nil, errors.New(grpErrDB))

		c, rec := newGrpUsersPostCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).AddGroupUsers(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- RemoveGroupUsers ---

func TestRemoveGroupUsers(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	nonAdminUser := &model.User{ID: bson.NewObjectID(), Role: "user"}
	uid1 := bson.NewObjectID()

	t.Run(grpSubNoUser, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run(grpSubNoRole, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", nonAdminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run(grpSubBadID, func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersDeleteCtx(e, "bad-id", map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("userIds ว่าง ต้องได้ status 400", func(t *testing.T) {
		svc := new(mockGroupService)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{"userIds": []string{}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("soft remove สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("RemoveGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(nil)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{
			"userIds": []string{uid1.Hex()},
			"forever": false,
		})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("hard remove (forever=true) สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("RemoveGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(nil)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{
			"userIds": []string{uid1.Hex()},
			"forever": true,
		})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ไม่พบ group ต้องได้ status 404", func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("RemoveGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(service.ErrGroupNotFound)

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeGroupNotFound, resp.Code)
	})

	t.Run(grpSubSvcErr, func(t *testing.T) {
		svc := new(mockGroupService)
		svc.On("RemoveGroupUsers", mock.Anything, validID, mock.Anything, adminUser).Return(errors.New(grpErrDB))

		c, rec := newGrpUsersDeleteCtx(e, validID.Hex(), map[string]any{"userIds": []string{uid1.Hex()}})
		c.Set("user", adminUser)
		require.NoError(t, newGrpHandler(svc).RemoveGroupUsers(c))

		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

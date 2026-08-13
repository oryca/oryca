package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mock ---

type mockPackageSvcForAssign struct {
	mock.Mock
}

func (m *mockPackageSvcForAssign) List(ctx context.Context, params url.Values) ([]*model.Package, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.Package), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockPackageSvcForAssign) GetByID(ctx context.Context, id string) (*model.Package, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.Package), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPackageSvcForAssign) Create(ctx context.Context, body *model.PackageCreate, ctxUser *model.User) (*model.Package, error) {
	args := m.Called(ctx, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.Package), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPackageSvcForAssign) Update(ctx context.Context, id string, body *model.PackageUpdate, ctxUser *model.User) (*model.Package, error) {
	args := m.Called(ctx, id, body, ctxUser)
	if v := args.Get(0); v != nil {
		return v.(*model.Package), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockPackageSvcForAssign) Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error {
	return m.Called(ctx, id, forever, ctxUser).Error(0)
}

type mockPackageUserSvcForAssign struct {
	mock.Mock
}

func (m *mockPackageUserSvcForAssign) GetUsers(ctx context.Context, packageID string, params url.Values) ([]*model.User, int64, error) {
	args := m.Called(ctx, packageID, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.User), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockPackageUserSvcForAssign) AssignUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (int64, int, error) {
	args := m.Called(ctx, packageID, rawIDs, groupID, ctxUser)
	return args.Get(0).(int64), args.Int(1), args.Error(2)
}

func (m *mockPackageUserSvcForAssign) RemoveUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (int64, int, error) {
	args := m.Called(ctx, packageID, rawIDs, groupID, ctxUser)
	return args.Get(0).(int64), args.Int(1), args.Error(2)
}

// --- helper ---

func newAssignCtx(t *testing.T, packageID, body string, ctxUser *model.User) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("packageId")
	c.SetParamValues(packageID)
	c.Set("user", ctxUser)
	return c, rec
}

func enabledPackage(id bson.ObjectID, enabled bool) *model.Package {
	return &model.Package{ID: id, Name: "Basic", Enabled: &enabled}
}

// --- tests ---

func TestAssignPackageUsers_SelfServiceAllowed(t *testing.T) {
	userID := bson.NewObjectID()
	pkgID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID, Role: "user"}

	pkgSvc := new(mockPackageSvcForAssign)
	pkgSvc.On("GetByID", mock.Anything, pkgID.Hex()).Return(enabledPackage(pkgID, true), nil)
	puSvc := new(mockPackageUserSvcForAssign)
	puSvc.On("AssignUsers", mock.Anything, pkgID.Hex(), []string{userID.Hex()}, (*string)(nil), ctxUser).Return(int64(1), 0, nil)

	h := NewPackageHandler(pkgSvc, nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+userID.Hex()+`"]}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	puSvc.AssertExpectations(t)
}

func TestAssignPackageUsers_SelfServiceRejectsOtherUser(t *testing.T) {
	ctxUser := &model.User{ID: bson.NewObjectID(), Role: "user"}
	pkgID := bson.NewObjectID()

	puSvc := new(mockPackageUserSvcForAssign)
	h := NewPackageHandler(new(mockPackageSvcForAssign), nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+bson.NewObjectID().Hex()+`"]}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	puSvc.AssertNotCalled(t, "AssignUsers", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAssignPackageUsers_SelfServiceRejectsMultipleIDs(t *testing.T) {
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID, Role: "user"}
	pkgID := bson.NewObjectID()

	puSvc := new(mockPackageUserSvcForAssign)
	h := NewPackageHandler(new(mockPackageSvcForAssign), nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+userID.Hex()+`","`+bson.NewObjectID().Hex()+`"]}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	puSvc.AssertNotCalled(t, "AssignUsers", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAssignPackageUsers_SelfServiceRejectsGroupID(t *testing.T) {
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID, Role: "user"}
	pkgID := bson.NewObjectID()
	gid := bson.NewObjectID().Hex()

	puSvc := new(mockPackageUserSvcForAssign)
	h := NewPackageHandler(new(mockPackageSvcForAssign), nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+userID.Hex()+`"],"groupId":"`+gid+`"}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	puSvc.AssertNotCalled(t, "AssignUsers", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAssignPackageUsers_SelfServiceRejectsDisabledPackage(t *testing.T) {
	userID := bson.NewObjectID()
	pkgID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID, Role: "user"}

	pkgSvc := new(mockPackageSvcForAssign)
	pkgSvc.On("GetByID", mock.Anything, pkgID.Hex()).Return(enabledPackage(pkgID, false), nil)
	puSvc := new(mockPackageUserSvcForAssign)

	h := NewPackageHandler(pkgSvc, nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+userID.Hex()+`"]}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	puSvc.AssertNotCalled(t, "AssignUsers", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAssignPackageUsers_AdminCanAssignOthersAndDisabledPackage(t *testing.T) {
	pkgID := bson.NewObjectID()
	ctxUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	targetID := bson.NewObjectID().Hex()
	gid := bson.NewObjectID().Hex()

	pkgSvc := new(mockPackageSvcForAssign)
	puSvc := new(mockPackageUserSvcForAssign)
	puSvc.On("AssignUsers", mock.Anything, pkgID.Hex(), []string{targetID}, &gid, ctxUser).Return(int64(3), 0, nil)

	h := NewPackageHandler(pkgSvc, nil, puSvc)
	c, rec := newAssignCtx(t, pkgID.Hex(), `{"userIds":["`+targetID+`"],"groupId":"`+gid+`"}`, ctxUser)

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	// admin ไม่ต้องผ่านการเช็ค package enabled
	pkgSvc.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
	puSvc.AssertExpectations(t)
}

func TestAssignPackageUsers_NoCtxUserForbidden(t *testing.T) {
	pkgID := bson.NewObjectID()
	puSvc := new(mockPackageUserSvcForAssign)
	h := NewPackageHandler(new(mockPackageSvcForAssign), nil, puSvc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"userIds":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("packageId")
	c.SetParamValues(pkgID.Hex())

	assert.NoError(t, h.AssignPackageUsers(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

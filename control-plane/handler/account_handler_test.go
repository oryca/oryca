package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

type mockAccountSvc struct{ mock.Mock }

func (m *mockAccountSvc) UpdateProfile(ctx context.Context, userID bson.ObjectID, body *model.AccountProfileUpdate) (*model.User, error) {
	args := m.Called(ctx, userID, body)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountSvc) ChangePassword(ctx context.Context, userID bson.ObjectID, oldPassword, newPassword string) (*model.User, error) {
	args := m.Called(ctx, userID, oldPassword, newPassword)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountSvc) DeleteProfile(ctx context.Context, userID bson.ObjectID) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *mockAccountSvc) GetSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) ([]*model.Session, error) {
	args := m.Called(ctx, userID, currentSessionID)
	if v := args.Get(0); v != nil {
		return v.([]*model.Session), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountSvc) DeleteSession(ctx context.Context, sessionID, userID bson.ObjectID) error {
	return m.Called(ctx, sessionID, userID).Error(0)
}

func (m *mockAccountSvc) GetSessionHistory(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error) {
	args := m.Called(ctx, userID)
	if v := args.Get(0); v != nil {
		return v.([]*model.UserSession), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountSvc) DeleteAllSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) error {
	return m.Called(ctx, userID, currentSessionID).Error(0)
}

// --- helpers ---

const (
	acctErrDB          = "db error"
	pathChangePassword = "/account/change-password"
	pathProfile        = "/account/profile"
	pathSessions       = "/account/sessions"
	pathSessionsSlash  = "/account/sessions/"
	pathSessionHistory = "/account/sessions/history"
	msgNo401           = "ไม่มี ctxUser คืน 401"
	msgSvcErr422       = "svc error คืน 422"
)

func newAccountEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	return e
}

func setupAccountCtx(e *echo.Echo, method, path string, body []byte, ctxUser *model.User, sessionID *bson.ObjectID) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewBuffer(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if ctxUser != nil {
		c.Set("user", ctxUser)
	}
	if sessionID != nil {
		c.Set("session", *sessionID)
	}
	return c, rec
}

func newHandler(svc *mockAccountSvc) *AccountHandler {
	return NewAccountHandler(svc)
}

// --- GetProfile ---

func TestGetProfile(t *testing.T) {
	e := newAccountEcho()

	t.Run("คืน 200 พร้อม user", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		user := &model.User{ID: bson.NewObjectID(), FirstName: "John"}

		c, rec := setupAccountCtx(e, http.MethodGet, pathProfile, nil, user, nil)
		require.NoError(t, h.GetProfile(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "John", got.FirstName)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodGet, pathProfile, nil, nil, nil)
		require.NoError(t, h.GetProfile(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeUnauthorizedAccess, ex.Code)
	})

}

// --- UpdateProfile ---

func TestUpdateProfile(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}

	t.Run("อัปเดตสำเร็จคืน 200", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		updated := &model.User{ID: userID, FirstName: "New"}
		svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(updated, nil)

		body, _ := json.Marshal(model.AccountProfileUpdate{FirstName: "New"})
		c, rec := setupAccountCtx(e, http.MethodPut, pathProfile, body, ctxUser, nil)
		require.NoError(t, h.UpdateProfile(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "New", got.FirstName)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("UpdateProfile", mock.Anything, userID, mock.Anything).Return(nil, errors.New(acctErrDB))

		body, _ := json.Marshal(model.AccountProfileUpdate{})
		c, rec := setupAccountCtx(e, http.MethodPut, pathProfile, body, ctxUser, nil)
		require.NoError(t, h.UpdateProfile(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		body, _ := json.Marshal(model.AccountProfileUpdate{})
		c, rec := setupAccountCtx(e, http.MethodPut, pathProfile, body, nil, nil)
		require.NoError(t, h.UpdateProfile(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- ChangePassword ---

func TestChangePassword(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}

	t.Run("สำเร็จคืน 200 พร้อม user", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		out := &model.User{ID: userID, FirstName: "Jane"}
		svc.On("ChangePassword", mock.Anything, userID, "old", "new").Return(out, nil)

		body, _ := json.Marshal(model.AccountChangePasswordRequest{OldPassword: "old", NewPassword: "new"})
		c, rec := setupAccountCtx(e, http.MethodPost, pathChangePassword, body, ctxUser, nil)
		require.NoError(t, h.ChangePassword(c))
		assert.Equal(t, http.StatusOK, rec.Code)
		var got model.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "Jane", got.FirstName)
	})

	t.Run("รหัสเดิมผิด 400 INCORRECT_PASSWORD", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("ChangePassword", mock.Anything, userID, "old", "new").Return(nil, service.ErrChangePasswordWrongCurrent)

		body, _ := json.Marshal(model.AccountChangePasswordRequest{OldPassword: "old", NewPassword: "new"})
		c, rec := setupAccountCtx(e, http.MethodPost, pathChangePassword, body, ctxUser, nil)
		require.NoError(t, h.ChangePassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeIncorrectPassword, ex.Code)
	})

	t.Run("ไม่มี local password 403 CANNOT_CHANGE_PASSWORD", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("ChangePassword", mock.Anything, userID, "old", "new").Return(nil, service.ErrChangePasswordNoLocal)

		body, _ := json.Marshal(model.AccountChangePasswordRequest{OldPassword: "old", NewPassword: "new"})
		c, rec := setupAccountCtx(e, http.MethodPost, pathChangePassword, body, ctxUser, nil)
		require.NoError(t, h.ChangePassword(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeCannotChangePassword, ex.Code)
	})

	t.Run("ขาดฟิลด์ 400", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		body, _ := json.Marshal(model.AccountChangePasswordRequest{OldPassword: "", NewPassword: "x"})
		c, rec := setupAccountCtx(e, http.MethodPost, pathChangePassword, body, ctxUser, nil)
		require.NoError(t, h.ChangePassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		body, _ := json.Marshal(model.AccountChangePasswordRequest{OldPassword: "a", NewPassword: "b"})
		c, rec := setupAccountCtx(e, http.MethodPost, pathChangePassword, body, nil, nil)
		require.NoError(t, h.ChangePassword(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- DeleteProfile ---

func TestDeleteProfile(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}

	t.Run("ลบสำเร็จคืน 204", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteProfile", mock.Anything, userID).Return(nil)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathProfile, nil, ctxUser, nil)
		require.NoError(t, h.DeleteProfile(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("root ลบตัวเองไม่ได้ คืน 403", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		rootUser := &model.User{ID: userID, Role: "root"}

		c, rec := setupAccountCtx(e, http.MethodDelete, pathProfile, nil, rootUser, nil)
		require.NoError(t, h.DeleteProfile(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeCannotDeleteAccount, ex.Code)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteProfile", mock.Anything, userID).Return(errors.New(acctErrDB))

		c, rec := setupAccountCtx(e, http.MethodDelete, pathProfile, nil, ctxUser, nil)
		require.NoError(t, h.DeleteProfile(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathProfile, nil, nil, nil)
		require.NoError(t, h.DeleteProfile(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- GetSessions ---

func TestGetSessions(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}
	sessionID := bson.NewObjectID()

	t.Run("คืน 200 พร้อม list", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		sessions := []*model.Session{{ID: sessionID, Current: true}}
		svc.On("GetSessions", mock.Anything, userID, sessionID).Return(sessions, nil)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessions, nil, ctxUser, &sessionID)
		require.NoError(t, h.GetSessions(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.ListResponse[*model.Session]
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 1, got.NumberReturned)
	})

	t.Run("nil sessions คืน empty array", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("GetSessions", mock.Anything, userID, mock.Anything).Return(nil, nil)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessions, nil, ctxUser, nil)
		require.NoError(t, h.GetSessions(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.ListResponse[*model.Session]
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 0, got.NumberReturned)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("GetSessions", mock.Anything, userID, mock.Anything).Return(nil, errors.New(acctErrDB))

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessions, nil, ctxUser, nil)
		require.NoError(t, h.GetSessions(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessions, nil, nil, nil)
		require.NoError(t, h.GetSessions(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- DeleteSession ---

func TestDeleteSession(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}
	sessionID := bson.NewObjectID()

	t.Run("ลบสำเร็จคืน 204", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteSession", mock.Anything, sessionID, userID).Return(nil)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessionsSlash+sessionID.Hex(), nil, ctxUser, nil)
		c.SetParamNames("sessionId")
		c.SetParamValues(sessionID.Hex())
		require.NoError(t, h.DeleteSession(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("sessionId invalid คืน 400", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessionsSlash+"bad-id", nil, ctxUser, nil)
		c.SetParamNames("sessionId")
		c.SetParamValues("bad-id")
		require.NoError(t, h.DeleteSession(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeParamInvalidFormat, ex.Code)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteSession", mock.Anything, sessionID, userID).Return(errors.New(acctErrDB))

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessionsSlash+sessionID.Hex(), nil, ctxUser, nil)
		c.SetParamNames("sessionId")
		c.SetParamValues(sessionID.Hex())
		require.NoError(t, h.DeleteSession(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessionsSlash+sessionID.Hex(), nil, nil, nil)
		c.SetParamNames("sessionId")
		c.SetParamValues(sessionID.Hex())
		require.NoError(t, h.DeleteSession(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- DeleteAllSessions ---

func TestDeleteAllSessions(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}

	t.Run("ลบทุก session ยกเว้น current สำเร็จคืน 204", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteAllSessions", mock.Anything, userID, mock.Anything).Return(nil)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessions, nil, ctxUser, nil)
		require.NoError(t, h.DeleteAllSessions(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("DeleteAllSessions", mock.Anything, userID, mock.Anything).Return(errors.New(acctErrDB))

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessions, nil, ctxUser, nil)
		require.NoError(t, h.DeleteAllSessions(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodDelete, pathSessions, nil, nil, nil)
		require.NoError(t, h.DeleteAllSessions(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- GetSessionHistory ---

func TestGetSessionHistory(t *testing.T) {
	e := newAccountEcho()
	userID := bson.NewObjectID()
	ctxUser := &model.User{ID: userID}
	loginAt := time.Now().UTC().Truncate(time.Second)

	t.Run("คืน 200 พร้อม history list", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		sessions := []*model.UserSession{
			{ID: bson.NewObjectID(), UserID: userID, Browser: "Chrome", LoginAt: loginAt},
		}
		svc.On("GetSessionHistory", mock.Anything, userID).Return(sessions, nil)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessionHistory, nil, ctxUser, nil)
		require.NoError(t, h.GetSessionHistory(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.ListResponse[*model.UserSession]
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 1, got.NumberReturned)
	})

	t.Run("nil sessions คืน empty array", func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("GetSessionHistory", mock.Anything, userID).Return(nil, nil)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessionHistory, nil, ctxUser, nil)
		require.NoError(t, h.GetSessionHistory(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.ListResponse[*model.UserSession]
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 0, got.NumberReturned)
	})

	t.Run(msgSvcErr422, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)
		svc.On("GetSessionHistory", mock.Anything, userID).Return(nil, errors.New(acctErrDB))

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessionHistory, nil, ctxUser, nil)
		require.NoError(t, h.GetSessionHistory(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run(msgNo401, func(t *testing.T) {
		svc := new(mockAccountSvc)
		h := newHandler(svc)

		c, rec := setupAccountCtx(e, http.MethodGet, pathSessionHistory, nil, nil, nil)
		require.NoError(t, h.GetSessionHistory(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- GetAuthMethods ---

// --- UnlinkAuthMethod ---

// --- LinkAuthMethod ---

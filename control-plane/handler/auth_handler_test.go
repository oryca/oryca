package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

type mockAuthSvc struct{ mock.Mock }

func (m *mockAuthSvc) Login(ctx context.Context, username, password, deviceInfo, ipAddress string) (*model.Auth, error) {
	args := m.Called(ctx, username, password, deviceInfo, ipAddress)
	if v := args.Get(0); v != nil {
		return v.(*model.Auth), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAuthSvc) Register(ctx context.Context, body *model.RegisterRequest) (*model.User, error) {
	args := m.Called(ctx, body)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAuthSvc) RefreshToken(ctx context.Context, refreshToken string) (*model.Auth, error) {
	args := m.Called(ctx, refreshToken)
	if v := args.Get(0); v != nil {
		return v.(*model.Auth), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAuthSvc) Logout(ctx context.Context, sessionID, userID bson.ObjectID) error {
	return m.Called(ctx, sessionID, userID).Error(0)
}

// --- constants ---

const (
	authEmail     = "user@example.com"
	authPassword  = "secret123"
	authToken     = "valid-token"
	authErrDB     = "db error"
	authSubSvcErr = "service error ต้องได้ status 422"

	pathLogin    = "/auth/login"
	pathRegister = "/auth/register"
	pathToken    = "/auth/token"
	pathLogout   = "/auth/logout"
)

// --- helpers ---

func newAuthHandler() (*AuthHandler, *mockAuthSvc) {
	a := new(mockAuthSvc)
	h := NewAuthHandler(AuthHandlerDeps{AuthSvc: a})
	return h, a
}

func newAuthPostCtx(e *echo.Echo, path string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newAuthGetCtx(e *echo.Echo, path string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- TestLogin ---

func TestLogin(t *testing.T) {
	e := echo.New()

	t.Run("body ผิดรูปแบบ ต้องได้ status 400", func(t *testing.T) {
		h, _ := newAuthHandler()
		req := httptest.NewRequest(http.MethodPost, pathLogin, bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("username หรือ password ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": "", "password": ""})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("รหัสผ่านผิด ต้องได้ status 400 CodeIncorrectPassword", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrIncorrectPassword)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeIncorrectPassword, resp.Code)
	})

	t.Run("user ยังไม่ verify ต้องได้ status 403 CodeUserNotVerified", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrUserNotVerifiedAuth)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotVerified, resp.Code)
	})

	t.Run("user ถูก disable ต้องได้ status 403 CodeUserNotEnabled", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrUserNotEnabledAuth)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotEnabled, resp.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, errors.New(authErrDB))
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("login สำเร็จ ต้องได้ status 200 และ auth token", func(t *testing.T) {
		h, a := newAuthHandler()
		auth := &model.Auth{AccessToken: "access-token", RefreshToken: "refresh-token"}
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(auth, nil)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.Auth
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "access-token", resp.AccessToken)
	})
}

// --- TestRegister ---

func TestRegister(t *testing.T) {
	e := echo.New()

	t.Run("email ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": ""})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("register ปิดอยู่ ต้องได้ status 403", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrRegisterNotEnabled)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeRegisterNotEnabled, resp.Code)
	})

	t.Run("email ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrEmailDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailDuplicate, resp.Code)
	})

	t.Run("username ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrUsernameDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe", "username": "dupuser"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUsernameDuplicate, resp.Code)
	})

	t.Run("phone ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrPhoneDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe", "phone": "0891234567"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodePhoneDuplicate, resp.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, errors.New(authErrDB))
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("register สำเร็จ ต้องได้ status 201", func(t *testing.T) {
		h, a := newAuthHandler()
		user := &model.User{ID: bson.NewObjectID()}
		a.On("Register", mock.Anything, mock.Anything).Return(user, nil)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}

// --- TestRefreshToken ---

func TestRefreshToken(t *testing.T) {
	e := echo.New()

	t.Run("refreshToken ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathToken, map[string]any{"refreshToken": ""})

		require.NoError(t, h.RefreshToken(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("token ไม่ถูกต้อง ต้องได้ status 401", func(t *testing.T) {
		h, a := newAuthHandler()
		a.On("RefreshToken", mock.Anything, authToken).Return(nil, service.ErrInvalidRefreshToken)
		c, rec := newAuthPostCtx(e, pathToken, map[string]any{"refreshToken": authToken})

		require.NoError(t, h.RefreshToken(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeInvalidRefreshToken, resp.Code)
	})

	t.Run("refresh สำเร็จ ต้องได้ status 200 และ auth token ใหม่", func(t *testing.T) {
		h, a := newAuthHandler()
		auth := &model.Auth{AccessToken: "new-access", RefreshToken: "new-refresh"}
		a.On("RefreshToken", mock.Anything, authToken).Return(auth, nil)
		c, rec := newAuthPostCtx(e, pathToken, map[string]any{"refreshToken": authToken})

		require.NoError(t, h.RefreshToken(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.Auth
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "new-access", resp.AccessToken)
	})
}

// --- TestLogout ---

func TestLogout(t *testing.T) {
	e := echo.New()

	t.Run("ไม่มี user ใน context ต้องได้ status 401", func(t *testing.T) {
		h, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogout, nil)

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("ไม่มี session ใน context ต้องได้ status 401", func(t *testing.T) {
		h, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogout, nil)
		c.Set("user", &model.User{ID: bson.NewObjectID()})

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a := newAuthHandler()
		userID := bson.NewObjectID()
		sessionID := bson.NewObjectID()
		a.On("Logout", mock.Anything, sessionID, userID).Return(errors.New("redis error"))

		c, rec := newAuthPostCtx(e, pathLogout, nil)
		c.Set("user", &model.User{ID: userID})
		c.Set("session", sessionID)

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("logout สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		h, a := newAuthHandler()
		userID := bson.NewObjectID()
		sessionID := bson.NewObjectID()
		a.On("Logout", mock.Anything, sessionID, userID).Return(nil)

		c, rec := newAuthPostCtx(e, pathLogout, nil)
		c.Set("user", &model.User{ID: userID})
		c.Set("session", sessionID)

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

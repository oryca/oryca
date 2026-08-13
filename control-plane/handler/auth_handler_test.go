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

type mockSetPasswordSvc struct{ mock.Mock }

func (m *mockSetPasswordSvc) ForgotPassword(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	args := m.Called(ctx, email, callbackUrl)
	if v := args.Get(0); v != nil {
		return v.(*model.SendEmailResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSetPasswordSvc) Verify(ctx context.Context, token string) (*model.SetPasswordTokenInfo, error) {
	args := m.Called(ctx, token)
	if v := args.Get(0); v != nil {
		return v.(*model.SetPasswordTokenInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSetPasswordSvc) SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	args := m.Called(ctx, email, callbackUrl)
	if v := args.Get(0); v != nil {
		return v.(*model.SendEmailResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSetPasswordSvc) SetPassword(ctx context.Context, token, password string) error {
	return m.Called(ctx, token, password).Error(0)
}

type mockVerifyEmailSvc struct{ mock.Mock }

func (m *mockVerifyEmailSvc) Verify(ctx context.Context, token string) error {
	return m.Called(ctx, token).Error(0)
}

func (m *mockVerifyEmailSvc) SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	args := m.Called(ctx, email, callbackUrl)
	if v := args.Get(0); v != nil {
		return v.(*model.SendEmailResult), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- constants ---

const (
	authEmail       = "user@example.com"
	authPassword    = "secret123"
	authToken       = "valid-token"
	authCallbackUrl = "https://app.example.com/reset"
	authErrDB       = "db error"
	authSubSvcErr   = "service error ต้องได้ status 422"

	pathLogin          = "/auth/login"
	pathRegister       = "/auth/register"
	pathToken          = "/auth/token"
	pathLogout         = "/auth/logout"
	pathSetPassword    = "/auth/set-password"
	pathSetPasswordQ   = "/auth/set-password?token="
	pathForgotPassword = "/auth/forgot-password"
)

// --- helpers ---

func newAuthHandler() (*AuthHandler, *mockAuthSvc, *mockSetPasswordSvc, *mockVerifyEmailSvc) {
	a := new(mockAuthSvc)
	sp := new(mockSetPasswordSvc)
	ve := new(mockVerifyEmailSvc)
	h := NewAuthHandler(AuthHandlerDeps{
		AuthSvc:        a,
		SetPasswordSvc: sp,
		VerifyEmailSvc: ve,
	})
	return h, a, sp, ve
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
		h, _, _, _ := newAuthHandler()
		req := httptest.NewRequest(http.MethodPost, pathLogin, bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("username หรือ password ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": "", "password": ""})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("รหัสผ่านผิด ต้องได้ status 400 CodeIncorrectPassword", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrIncorrectPassword)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeIncorrectPassword, resp.Code)
	})

	t.Run("user ยังไม่ verify ต้องได้ status 403 CodeUserNotVerified", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrUserNotVerifiedAuth)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotVerified, resp.Code)
	})

	t.Run("user ถูก disable ต้องได้ status 403 CodeUserNotEnabled", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, service.ErrUserNotEnabledAuth)
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotEnabled, resp.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Login", mock.Anything, authEmail, authPassword, mock.Anything, mock.Anything).Return(nil, errors.New(authErrDB))
		c, rec := newAuthPostCtx(e, pathLogin, map[string]any{"username": authEmail, "password": authPassword})

		require.NoError(t, h.Login(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("login สำเร็จ ต้องได้ status 200 และ auth token", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
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
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": ""})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("register ปิดอยู่ ต้องได้ status 403", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrRegisterNotEnabled)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeRegisterNotEnabled, resp.Code)
	})

	t.Run("email ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrEmailDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailDuplicate, resp.Code)
	})

	t.Run("username ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrUsernameDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe", "username": "dupuser"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUsernameDuplicate, resp.Code)
	})

	t.Run("phone ซ้ำ ต้องได้ status 409", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, service.ErrPhoneDuplicate)
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe", "phone": "0891234567"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodePhoneDuplicate, resp.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("Register", mock.Anything, mock.Anything).Return(nil, errors.New(authErrDB))
		c, rec := newAuthPostCtx(e, pathRegister, map[string]any{"email": authEmail, "firstName": "John", "lastName": "Doe"})

		require.NoError(t, h.Register(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("register สำเร็จ ต้องได้ status 201", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
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
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathToken, map[string]any{"refreshToken": ""})

		require.NoError(t, h.RefreshToken(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("token ไม่ถูกต้อง ต้องได้ status 401", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
		a.On("RefreshToken", mock.Anything, authToken).Return(nil, service.ErrInvalidRefreshToken)
		c, rec := newAuthPostCtx(e, pathToken, map[string]any{"refreshToken": authToken})

		require.NoError(t, h.RefreshToken(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeInvalidRefreshToken, resp.Code)
	})

	t.Run("refresh สำเร็จ ต้องได้ status 200 และ auth token ใหม่", func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
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
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogout, nil)

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("ไม่มี session ใน context ต้องได้ status 401", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathLogout, nil)
		c.Set("user", &model.User{ID: bson.NewObjectID()})

		require.NoError(t, h.Logout(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, a, _, _ := newAuthHandler()
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
		h, a, _, _ := newAuthHandler()
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

// --- TestVerifyEmail ---

func TestVerifyEmail(t *testing.T) {
	e := echo.New()

	t.Run("token ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		req := httptest.NewRequest(http.MethodGet, "/auth/verify-email", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifyEmail(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeQueryParamRequired, resp.Code)
	})

	t.Run("token ไม่ถูกต้องหรือหมดอายุ ต้องได้ status 400", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		ve.On("Verify", mock.Anything, authToken).Return(service.ErrTokenInvalidOrExpired)

		req := httptest.NewRequest(http.MethodGet, "/auth/verify-email?token="+authToken, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifyEmail(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeTokenInvalidOrExpired, resp.Code)
	})

	t.Run("verify สำเร็จ ต้องได้ status 204", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		ve.On("Verify", mock.Anything, authToken).Return(nil)

		req := httptest.NewRequest(http.MethodGet, "/auth/verify-email?token="+authToken, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifyEmail(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

// --- TestVerifySetPasswordToken ---

func TestVerifySetPasswordToken(t *testing.T) {
	e := echo.New()

	t.Run("token ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		req := httptest.NewRequest(http.MethodGet, pathSetPassword, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifySetPasswordToken(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("token หมดอายุ ต้องได้ status 400", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("Verify", mock.Anything, authToken).Return(nil, service.ErrTokenInvalidOrExpired)

		req := httptest.NewRequest(http.MethodGet, pathSetPasswordQ+authToken, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifySetPasswordToken(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeTokenInvalidOrExpired, resp.Code)
	})

	t.Run("ไม่พบ user ต้องได้ status 404", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("Verify", mock.Anything, authToken).Return(nil, errors.New("not found"))

		req := httptest.NewRequest(http.MethodGet, pathSetPasswordQ+authToken, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifySetPasswordToken(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("token ถูกต้อง ต้องได้ status 200 และ token info", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		info := &model.SetPasswordTokenInfo{Email: authEmail}
		sp.On("Verify", mock.Anything, authToken).Return(info, nil)

		req := httptest.NewRequest(http.MethodGet, pathSetPasswordQ+authToken, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		require.NoError(t, h.VerifySetPasswordToken(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.SetPasswordTokenInfo
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, authEmail, resp.Email)
	})
}

// --- TestSetPassword ---

func TestSetPassword(t *testing.T) {
	e := echo.New()

	t.Run("token หรือ password ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathSetPassword, map[string]any{"token": "", "password": ""})

		require.NoError(t, h.SetPassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("token หมดอายุ ต้องได้ status 400", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("SetPassword", mock.Anything, authToken, authPassword).Return(service.ErrTokenInvalidOrExpired)
		c, rec := newAuthPostCtx(e, pathSetPassword, map[string]any{"token": authToken, "password": authPassword})

		require.NoError(t, h.SetPassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeTokenInvalidOrExpired, resp.Code)
	})

	t.Run(authSubSvcErr, func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("SetPassword", mock.Anything, authToken, authPassword).Return(errors.New(authErrDB))
		c, rec := newAuthPostCtx(e, pathSetPassword, map[string]any{"token": authToken, "password": authPassword})

		require.NoError(t, h.SetPassword(c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("ตั้งรหัสผ่านสำเร็จ ต้องได้ status 204", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("SetPassword", mock.Anything, authToken, authPassword).Return(nil)
		c, rec := newAuthPostCtx(e, pathSetPassword, map[string]any{"token": authToken, "password": authPassword})

		require.NoError(t, h.SetPassword(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

// --- TestForgotPassword ---

func TestForgotPassword(t *testing.T) {
	e := echo.New()

	t.Run("email หรือ callbackUrl ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathForgotPassword, map[string]any{"email": "", "callbackUrl": ""})

		require.NoError(t, h.ForgotPassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("email ไม่มีในระบบ ต้องได้ status 404", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("ForgotPassword", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotFound)
		c, rec := newAuthPostCtx(e, pathForgotPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ForgotPassword(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("user ยังไม่ verify email ต้องได้ status 403", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("ForgotPassword", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotVerifiedAuth)
		c, rec := newAuthPostCtx(e, pathForgotPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ForgotPassword(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotVerified, resp.Code)
	})

	t.Run("user ถูก disable ต้องได้ status 403", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("ForgotPassword", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotEnabledAuth)
		c, rec := newAuthPostCtx(e, pathForgotPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ForgotPassword(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotEnabled, resp.Code)
	})

	t.Run("ส่ง email สำเร็จ ต้องได้ status 200 พร้อม email และ expiredAt", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		expiredAt := time.Now().Add(3600 * time.Second)
		result := &model.SendEmailResult{Email: authEmail, ExpiredAt: expiredAt}
		sp.On("ForgotPassword", mock.Anything, authEmail, authCallbackUrl).Return(result, nil)
		c, rec := newAuthPostCtx(e, pathForgotPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ForgotPassword(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.SendEmailResult
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, authEmail, resp.Email)
		assert.WithinDuration(t, expiredAt, resp.ExpiredAt, time.Second)
	})
}

// --- TestResendVerifyEmail ---

func TestResendVerifyEmail(t *testing.T) {
	e := echo.New()
	pathResendVerifyEmail := "/auth/verify-email"

	t.Run("email หรือ callbackUrl ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathResendVerifyEmail, map[string]any{"email": "", "callbackUrl": ""})

		require.NoError(t, h.ResendVerifyEmail(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("email ไม่มีในระบบ ต้องได้ status 404", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		ve.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotFound)
		c, rec := newAuthPostCtx(e, pathResendVerifyEmail, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ResendVerifyEmail(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("user verified แล้ว ต้องได้ status 409", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		ve.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserAlreadyVerified)
		c, rec := newAuthPostCtx(e, pathResendVerifyEmail, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ResendVerifyEmail(c))
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserAlreadyVerified, resp.Code)
	})

	t.Run("user ถูก disable ต้องได้ status 403", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		ve.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotEnabledAuth)
		c, rec := newAuthPostCtx(e, pathResendVerifyEmail, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ResendVerifyEmail(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotEnabled, resp.Code)
	})

	t.Run("ส่ง email สำเร็จ ต้องได้ status 200 พร้อม email และ expiredAt", func(t *testing.T) {
		h, _, _, ve := newAuthHandler()
		expiredAt := time.Now().Add(3600 * time.Second)
		result := &model.SendEmailResult{Email: authEmail, ExpiredAt: expiredAt}
		ve.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(result, nil)
		c, rec := newAuthPostCtx(e, pathResendVerifyEmail, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.ResendVerifyEmail(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.SendEmailResult
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, authEmail, resp.Email)
		assert.WithinDuration(t, expiredAt, resp.ExpiredAt, time.Second)
	})
}

// --- TestSettingPassword ---

func TestSettingPassword(t *testing.T) {
	e := echo.New()
	pathSettingPassword := "/auth/setting-password"

	t.Run("email หรือ callbackUrl ว่าง ต้องได้ status 400", func(t *testing.T) {
		h, _, _, _ := newAuthHandler()
		c, rec := newAuthPostCtx(e, pathSettingPassword, map[string]any{"email": "", "callbackUrl": ""})

		require.NoError(t, h.SettingPassword(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyIsRequired, resp.Code)
	})

	t.Run("email ไม่มีในระบบ ต้องได้ status 404", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotFound)
		c, rec := newAuthPostCtx(e, pathSettingPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.SettingPassword(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotFound, resp.Code)
	})

	t.Run("user ถูก disable ต้องได้ status 403", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		sp.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(nil, service.ErrUserNotEnabledAuth)
		c, rec := newAuthPostCtx(e, pathSettingPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.SettingPassword(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUserNotEnabled, resp.Code)
	})

	t.Run("ส่ง email สำเร็จ ต้องได้ status 200 พร้อม email และ expiredAt", func(t *testing.T) {
		h, _, sp, _ := newAuthHandler()
		expiredAt := time.Now().Add(3600 * time.Second)
		result := &model.SendEmailResult{Email: authEmail, ExpiredAt: expiredAt}
		sp.On("SendByEmail", mock.Anything, authEmail, authCallbackUrl).Return(result, nil)
		c, rec := newAuthPostCtx(e, pathSettingPassword, map[string]any{"email": authEmail, "callbackUrl": authCallbackUrl})

		require.NoError(t, h.SettingPassword(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.SendEmailResult
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, authEmail, resp.Email)
		assert.WithinDuration(t, expiredAt, resp.ExpiredAt, time.Second)
	})
}

// --- TestIdpAuthorize ---

// --- TestIdpCallback ---

// --- TestIdpLogin ---

// --- TestIdpRedirect ---

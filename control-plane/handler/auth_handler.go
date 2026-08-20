package handler

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgTokenInvalidOrExpired = "Token is invalid or expired"
	msgCallbackUrlInvalid    = "callbackUrl is invalid or contains reserved query param 'token'"
	msgNoPermission          = "No permission"
	msgNotFound              = "Not found"
	msgAuthRequired          = "Authentication required"
)

type authSetPasswordSvc interface {
	ForgotPassword(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error)
	SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error)
	Verify(ctx context.Context, token string) (*model.SetPasswordTokenInfo, error)
	SetPassword(ctx context.Context, token, password string) error
}

type authSvc interface {
	Login(ctx context.Context, username, password, deviceInfo, ipAddress string) (*model.Auth, error)
	Register(ctx context.Context, body *model.RegisterRequest) (*model.User, error)
	RefreshToken(ctx context.Context, refreshToken string) (*model.Auth, error)
	Logout(ctx context.Context, sessionID, userID bson.ObjectID) error
}

type verifyEmailSvc interface {
	Verify(ctx context.Context, token string) error
	SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error)
}

type AuthHandler struct {
	authSvc        authSvc
	setPasswordSvc authSetPasswordSvc
	verifyEmailSvc verifyEmailSvc
	allowedOrigins []string // origin ที่เชื่อได้ ใช้เลือก postMessage targetOrigin ของ IDP popup flow
}

// AuthHandlerDeps รวม dependencies ของ AuthHandler (ลดจำนวน parameter ของ constructor)
type AuthHandlerDeps struct {
	AuthSvc        authSvc
	SetPasswordSvc authSetPasswordSvc
	VerifyEmailSvc verifyEmailSvc
	AllowedOrigins []string // origin ที่เชื่อได้ ใช้เลือก postMessage targetOrigin ของ IDP popup flow
}

func NewAuthHandler(deps AuthHandlerDeps) *AuthHandler {
	return &AuthHandler{
		authSvc:        deps.AuthSvc,
		setPasswordSvc: deps.SetPasswordSvc,
		verifyEmailSvc: deps.VerifyEmailSvc,
		allowedOrigins: deps.AllowedOrigins,
	}
}

// resolveTrustedOrigin คืน origin ที่ตรงกับ allowlist ที่ config ไว้ (เช่น ORYCA_API_ALLOW_ORIGIN)
// ถ้า allowlist เป็น "*" (ไม่จำกัด) คืน "*" กลับไปตรงๆ. ไม่งั้นคืน "" เมื่อไม่ match (ห้าม reflect ค่าที่ validate ไม่ผ่าน)
func (h *AuthHandler) resolveTrustedOrigin(candidate string) string {
	for _, o := range h.allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			return "*"
		}
		if o != "" && o == candidate {
			return candidate
		}
	}
	return ""
}

func (h *AuthHandler) Login(c echo.Context) error {
	var body model.LoginRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Username == "" || body.Password == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'username' and 'password' are required",
		})
	}

	deviceInfo := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()
	auth, err := h.authSvc.Login(c.Request().Context(), body.Username, body.Password, deviceInfo, ipAddress)
	if err != nil {
		if errors.Is(err, service.ErrIncorrectPassword) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeIncorrectPassword,
				Status: http.StatusBadRequest,
				Detail: "Incorrect email or password",
			})
		}
		if errors.Is(err, service.ErrUserNotVerifiedAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotVerified,
				Status: http.StatusForbidden,
				Detail: "User not verified",
			})
		}
		if errors.Is(err, service.ErrUserNotEnabledAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotEnabled,
				Status: http.StatusForbidden,
				Detail: "User not enabled",
			})
		}
		logger.Error("login failed: " + err.Error())
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not login",
		})
	}

	return c.JSON(http.StatusOK, auth)
}

func (h *AuthHandler) Register(c echo.Context) error {
	var body model.RegisterRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Email == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'email' is required",
		})
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if _, err := mail.ParseAddress(body.Email); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'email' is invalid",
		})
	}
	if body.FirstName == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'firstName' is required",
		})
	}
	if body.LastName == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'lastName' is required",
		})
	}

	user, err := h.authSvc.Register(c.Request().Context(), &body)
	if err != nil {
		if errors.Is(err, service.ErrRegisterNotEnabled) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeRegisterNotEnabled,
				Status: http.StatusForbidden,
				Detail: "Registration is not enabled",
			})
		}
		if errors.Is(err, service.ErrEmailDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeEmailDuplicate,
				Status: http.StatusConflict,
				Detail: "Email already exists",
			})
		}
		if errors.Is(err, service.ErrUsernameDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeUsernameDuplicate,
				Status: http.StatusConflict,
				Detail: "Username already exists",
			})
		}
		if errors.Is(err, service.ErrPhoneDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodePhoneDuplicate,
				Status: http.StatusConflict,
				Detail: "Phone already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not register",
		})
	}

	return c.JSON(http.StatusCreated, user)
}

func (h *AuthHandler) RefreshToken(c echo.Context) error {
	var body model.RefreshTokenRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'refreshToken' is required",
		})
	}

	auth, err := h.authSvc.RefreshToken(c.Request().Context(), body.RefreshToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeInvalidRefreshToken,
			Status: http.StatusUnauthorized,
			Detail: "Refresh token is invalid or expired",
		})
	}

	return c.JSON(http.StatusOK, auth)
}

func (h *AuthHandler) Logout(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	sessionID, ok := c.Get("session").(bson.ObjectID)
	if !ok {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	if err := h.authSvc.Logout(c.Request().Context(), sessionID, ctxUser.ID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not logout",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *AuthHandler) ForgotPassword(c echo.Context) error {
	var body model.ForgotPasswordRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Email == "" || body.CallbackUrl == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'email' and 'callbackUrl' are required",
		})
	}

	result, err := h.setPasswordSvc.ForgotPassword(c.Request().Context(), body.Email, body.CallbackUrl)
	if err != nil {
		if errors.Is(err, service.ErrCallbackUrlInvalid) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeCallbackUrlInvalid,
				Status: http.StatusBadRequest,
				Detail: msgCallbackUrlInvalid,
			})
		}
		if errors.Is(err, service.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeUserNotFound,
				Status: http.StatusNotFound,
				Detail: "User not found",
			})
		}
		if errors.Is(err, service.ErrUserNotVerifiedAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotVerified,
				Status: http.StatusForbidden,
				Detail: "User email is not verified",
			})
		}
		if errors.Is(err, service.ErrUserNotEnabledAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotEnabled,
				Status: http.StatusForbidden,
				Detail: "User is not enabled",
			})
		}
		logger.Error("forgot password failed: " + err.Error())
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not send email",
		})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *AuthHandler) ResendVerifyEmail(c echo.Context) error {
	var body model.SendEmailRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Email == "" || body.CallbackUrl == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'email' and 'callbackUrl' are required",
		})
	}

	result, err := h.verifyEmailSvc.SendByEmail(c.Request().Context(), body.Email, body.CallbackUrl)
	if err != nil {
		if errors.Is(err, service.ErrCallbackUrlInvalid) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeCallbackUrlInvalid,
				Status: http.StatusBadRequest,
				Detail: msgCallbackUrlInvalid,
			})
		}
		if errors.Is(err, service.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeUserNotFound,
				Status: http.StatusNotFound,
				Detail: "User not found",
			})
		}
		if errors.Is(err, service.ErrUserAlreadyVerified) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeUserAlreadyVerified,
				Status: http.StatusConflict,
				Detail: "User is already verified",
			})
		}
		if errors.Is(err, service.ErrUserNotEnabledAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotEnabled,
				Status: http.StatusForbidden,
				Detail: "User is not enabled",
			})
		}
		logger.Error("resend verify email failed: " + err.Error())
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not send email",
		})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *AuthHandler) SettingPassword(c echo.Context) error {
	var body model.SendEmailRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Email == "" || body.CallbackUrl == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'email' and 'callbackUrl' are required",
		})
	}

	result, err := h.setPasswordSvc.SendByEmail(c.Request().Context(), body.Email, body.CallbackUrl)
	if err != nil {
		if errors.Is(err, service.ErrCallbackUrlInvalid) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeCallbackUrlInvalid,
				Status: http.StatusBadRequest,
				Detail: msgCallbackUrlInvalid,
			})
		}
		if errors.Is(err, service.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeUserNotFound,
				Status: http.StatusNotFound,
				Detail: "User not found",
			})
		}
		if errors.Is(err, service.ErrUserNotEnabledAuth) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeUserNotEnabled,
				Status: http.StatusForbidden,
				Detail: "User is not enabled",
			})
		}
		logger.Error("setting password failed: " + err.Error())
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not send email",
		})
	}

	return c.JSON(http.StatusOK, result)
}

// originFromRequest ดึง origin ของหน้าที่ยิง request มา. ใช้ header Origin ก่อน
// ตกมาที่ Referer ถ้า Origin ไม่มี (บาง browser ไม่ส่ง Origin บน GET navigation ปกติ)
func originFromRequest(c echo.Context) string {
	if o := c.Request().Header.Get(echo.HeaderOrigin); o != "" {
		return o
	}
	if ref := c.Request().Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

func (h *AuthHandler) VerifyEmail(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeQueryParamRequired,
			Status: http.StatusBadRequest,
			Detail: "Query param 'token' is required",
		})
	}

	if err := h.verifyEmailSvc.Verify(c.Request().Context(), token); err != nil {
		if errors.Is(err, service.ErrTokenInvalidOrExpired) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeTokenInvalidOrExpired,
				Status: http.StatusBadRequest,
				Detail: msgTokenInvalidOrExpired,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not verify email",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *AuthHandler) VerifySetPasswordToken(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeQueryParamRequired,
			Status: http.StatusBadRequest,
			Detail: "Query param 'token' is required",
		})
	}

	info, err := h.setPasswordSvc.Verify(c.Request().Context(), token)
	if err != nil {
		if errors.Is(err, service.ErrTokenInvalidOrExpired) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeTokenInvalidOrExpired,
				Status: http.StatusBadRequest,
				Detail: msgTokenInvalidOrExpired,
			})
		}
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}

	return c.JSON(http.StatusOK, info)
}

func (h *AuthHandler) SetPassword(c echo.Context) error {
	var body model.SetPasswordRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Token == "" || body.Password == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'token' and 'password' are required",
		})
	}

	if err := h.setPasswordSvc.SetPassword(c.Request().Context(), body.Token, body.Password); err != nil {
		if errors.Is(err, service.ErrTokenInvalidOrExpired) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeTokenInvalidOrExpired,
				Status: http.StatusBadRequest,
				Detail: msgTokenInvalidOrExpired,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not set password",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

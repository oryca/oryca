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
	msgNoPermission = "No permission"
	msgNotFound     = "Not found"
	msgAuthRequired = "Authentication required"
)

type authSvc interface {
	Login(ctx context.Context, username, password, deviceInfo, ipAddress string) (*model.Auth, error)
	Register(ctx context.Context, body *model.RegisterRequest) (*model.User, error)
	RefreshToken(ctx context.Context, refreshToken string) (*model.Auth, error)
	Logout(ctx context.Context, sessionID, userID bson.ObjectID) error
}

type AuthHandler struct {
	authSvc        authSvc
	allowedOrigins []string // origin ที่เชื่อได้ ใช้เลือก postMessage targetOrigin ของ IDP popup flow
}

// AuthHandlerDeps รวม dependencies ของ AuthHandler (ลดจำนวน parameter ของ constructor)
type AuthHandlerDeps struct {
	AuthSvc        authSvc
	AllowedOrigins []string // origin ที่เชื่อได้ ใช้เลือก postMessage targetOrigin ของ IDP popup flow
}

func NewAuthHandler(deps AuthHandlerDeps) *AuthHandler {
	return &AuthHandler{
		authSvc:        deps.AuthSvc,
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

package handler

import (
	"errors"
	"github.com/oryca/oryca/gateway/model"
	"github.com/oryca/oryca/gateway/tool"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
)

// errAuthWritten is a sentinel returned after an auth error response has already been written to the client.
// Callers must not write any further response when they receive this error.
var errAuthWritten = errors.New("auth response already written")

// authenticate resolves caller identity and validates user status + package access.
func (h *Handler) authenticate(c echo.Context, svc *model.GatewayService) (*model.User, error) {
	user, err := h.resolveIdentity(c)
	if err != nil {
		return nil, err
	}
	if err := h.checkUserStatus(c, user, svc); err != nil {
		return nil, err
	}
	return user, nil
}

// resolveIdentity authenticates via Bearer token or API key.
// On auth failure it writes the error response itself and returns (nil, errAuthWritten)
// so callers must not write any further response.
func (h *Handler) resolveIdentity(c echo.Context) (*model.User, error) {
	req := c.Request()
	if bearer := extractBearer(req); bearer != "" {
		return h.resolveBearer(c, bearer)
	}
	if apiKeyValue := extractApiKey(req); apiKeyValue != "" {
		return h.resolveApiKey(c, apiKeyValue)
	}
	_ = c.JSON(http.StatusUnauthorized, model.Exception{
		Code:   tool.CodeInvalidToken,
		Status: http.StatusUnauthorized,
		Detail: "Authentication required",
	})
	return nil, errAuthWritten
}

func (h *Handler) resolveBearer(c echo.Context, token string) (*model.User, error) {
	user, err := h.authSvc.ValidateBearer(c.Request().Context(), token)
	if err != nil {
		_ = c.JSON(http.StatusUnauthorized, model.Exception{
			Code:   tool.CodeInvalidToken,
			Status: http.StatusUnauthorized,
			Detail: "Invalid or expired token",
		})
		return nil, errAuthWritten
	}
	return user, nil
}

func (h *Handler) resolveApiKey(c echo.Context, keyValue string) (*model.User, error) {
	req := c.Request()
	user, ak, err := h.authSvc.ValidateApiKey(req.Context(), keyValue)
	if err != nil {
		code, detail := apiKeyErrorResponse(err.Error())
		_ = c.JSON(http.StatusUnauthorized, model.Exception{
			Code:   code,
			Status: http.StatusUnauthorized,
			Detail: detail,
		})
		return nil, errAuthWritten
	}
	if restrictErr := checkApiKeyRestriction(ak, req, c.RealIP()); restrictErr != nil {
		errCode := tool.CodeIPNotAllowed
		if restrictErr.Error() == "referer not allowed" {
			errCode = tool.CodeRefererNotAllowed
		}
		_ = c.JSON(http.StatusForbidden, model.Exception{
			Code:   errCode,
			Status: http.StatusForbidden,
			Detail: restrictErr.Error(),
		})
		return nil, errAuthWritten
	}
	c.Set("apiKeyId", ak.ID)
	return user, nil
}

// checkUserStatus validates enabled/verified/expired state and package access.
func (h *Handler) checkUserStatus(c echo.Context, user *model.User, svc *model.GatewayService) error {
	writeAndReturn := func(status int, ex model.Exception) error {
		_ = c.JSON(status, ex)
		return errAuthWritten
	}
	if user.Enabled != nil && !*user.Enabled {
		return writeAndReturn(http.StatusForbidden, model.Exception{
			Code: tool.CodeUserNotEnabled, Status: http.StatusForbidden, Detail: "User account is disabled",
		})
	}
	if user.Verified != nil && !*user.Verified {
		return writeAndReturn(http.StatusForbidden, model.Exception{
			Code: tool.CodeUserNotVerified, Status: http.StatusForbidden, Detail: "User account is not verified",
		})
	}
	if user.ExpiredAt != nil && user.ExpiredAt.Before(timeNow()) {
		return writeAndReturn(http.StatusForbidden, model.Exception{
			Code: tool.CodeAccountExpired, Status: http.StatusForbidden, Detail: "Account has expired",
		})
	}
	// Policy: a private (non-IsPublic) service must be linked to at least one package to be
	// callable at all — an empty svc.PackageIDs means no package has been granted access yet,
	// not "open to everyone." Fail closed, not open.
	if !slices.Contains(svc.PackageIDs, user.PackageID) {
		return writeAndReturn(http.StatusForbidden, model.Exception{
			Code: tool.CodePackageAccessDenied, Status: http.StatusForbidden, Detail: "Package does not have access to this service",
		})
	}
	return nil
}

func apiKeyErrorResponse(errMsg string) (string, string) {
	switch errMsg {
	case "api key not found":
		return tool.CodeApiKeyNotFound, "API key not found"
	case "api key disabled":
		return tool.CodeApiKeyDisabled, "API key is disabled"
	case "api key expired":
		return tool.CodeApiKeyExpired, "API key has expired"
	default:
		return tool.CodeInvalidToken, "Invalid API key"
	}
}

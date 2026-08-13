package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgInvalidSessionID    = "Param 'sessionId' is invalid"
	msgInvalidAuthMethodID = "Param 'authMethodId' is invalid"
)

type accountService interface {
	UpdateProfile(ctx context.Context, userID bson.ObjectID, body *model.AccountProfileUpdate) (*model.User, error)
	ChangePassword(ctx context.Context, userID bson.ObjectID, oldPassword, newPassword string) (*model.User, error)
	DeleteProfile(ctx context.Context, userID bson.ObjectID) error
	GetSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) ([]*model.Session, error)
	GetSessionHistory(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error)
	DeleteSession(ctx context.Context, sessionID, userID bson.ObjectID) error
	DeleteAllSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) error
}

type AccountHandler struct {
	svc accountService
}

func NewAccountHandler(svc accountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

func (h *AccountHandler) GetProfile(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	profile := model.ToUserProfile(ctxUser)
	return c.JSON(http.StatusOK, profile)
}

func (h *AccountHandler) UpdateProfile(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	var body model.AccountProfileUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	u, err := h.svc.UpdateProfile(c.Request().Context(), ctxUser.ID, &body)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update profile",
		})
	}
	return c.JSON(http.StatusOK, u)
}

func (h *AccountHandler) ChangePassword(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	var body model.AccountChangePasswordRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.OldPassword == "" || body.NewPassword == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: "Body 'oldPassword' and 'newPassword' are required",
		})
	}

	u, err := h.svc.ChangePassword(c.Request().Context(), ctxUser.ID, body.OldPassword, body.NewPassword)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeUserNotFound,
				Status: http.StatusNotFound,
				Detail: "User not found",
			})
		}
		if errors.Is(err, service.ErrChangePasswordNoLocal) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeCannotChangePassword,
				Status: http.StatusForbidden,
				Detail: "This account has no local password; use identity provider or set-password link",
			})
		}
		if errors.Is(err, service.ErrChangePasswordWrongCurrent) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeIncorrectPassword,
				Status: http.StatusBadRequest,
				Detail: "Current password is incorrect",
			})
		}
		if errors.Is(err, service.ErrChangePasswordUnchanged) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeBodyInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: "New password must differ from current password",
			})
		}
		if errors.Is(err, service.ErrChangePasswordHashFailed) {
			return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
				Code:   tool.CodeHashPasswordError,
				Status: http.StatusUnprocessableEntity,
				Detail: "Could not hash new password",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not change password",
		})
	}

	return c.JSON(http.StatusOK, u)
}

func (h *AccountHandler) DeleteProfile(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	if ctxUser.Role == "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeCannotDeleteAccount,
			Status: http.StatusForbidden,
			Detail: "Root account cannot be deleted",
		})
	}

	if err := h.svc.DeleteProfile(c.Request().Context(), ctxUser.ID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete profile",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AccountHandler) GetSessions(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	currentSessionID, _ := c.Get("session").(bson.ObjectID)

	sessions, err := h.svc.GetSessions(c.Request().Context(), ctxUser.ID, currentSessionID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get sessions",
		})
	}

	if sessions == nil {
		sessions = []*model.Session{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.Session]{
		NumberMatched:  len(sessions),
		NumberReturned: len(sessions),
		Items:          sessions,
	})
}

func (h *AccountHandler) DeleteSession(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	sessionID, err := bson.ObjectIDFromHex(c.Param("sessionId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidSessionID,
		})
	}

	if err := h.svc.DeleteSession(c.Request().Context(), sessionID, ctxUser.ID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete session",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AccountHandler) GetSessionHistory(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	sessions, err := h.svc.GetSessionHistory(c.Request().Context(), ctxUser.ID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get session history",
		})
	}

	if sessions == nil {
		sessions = []*model.UserSession{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.UserSession]{
		NumberMatched:  len(sessions),
		NumberReturned: len(sessions),
		Items:          sessions,
	})
}

func (h *AccountHandler) DeleteAllSessions(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}

	currentSessionID, _ := c.Get("session").(bson.ObjectID)
	if err := h.svc.DeleteAllSessions(c.Request().Context(), ctxUser.ID, currentSessionID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete sessions",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

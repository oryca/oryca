package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgInvalidSystemThemeID    = "Param 'systemThemeId' is invalid"
	msgSystemThemeNoPermission = "No permission"
)

type systemThemeService interface {
	List(ctx context.Context, params url.Values) ([]*model.SystemTheme, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.SystemTheme, error)
	GetDefault(ctx context.Context) (*model.SystemTheme, error)
	Create(ctx context.Context, body *model.SystemThemeCreate, createdBy bson.ObjectID) (*model.SystemTheme, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.SystemThemeUpdate, updatedBy bson.ObjectID) (*model.SystemTheme, error)
	Delete(ctx context.Context, id bson.ObjectID, deletedBy bson.ObjectID) error
}

type SystemThemeHandler struct {
	svc systemThemeService
}

func NewSystemThemeHandler(svc systemThemeService) *SystemThemeHandler {
	return &SystemThemeHandler{svc: svc}
}

// GetDefaultSystemTheme — public, no auth required
func (h *SystemThemeHandler) GetDefaultSystemTheme(c echo.Context) error {
	theme, err := h.svc.GetDefault(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Default theme not found",
		})
	}
	return c.JSON(http.StatusOK, theme)
}

func (h *SystemThemeHandler) GetSystemThemes(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgSystemThemeNoPermission,
		})
	}

	params := c.Request().URL.Query()

	if v := params.Get("limit"); v != "" {
		if _, err := tool.ParseLimit(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	if v := params.Get("offset"); v != "" {
		if _, err := tool.ParseOffset(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}

	list, total, err := h.svc.List(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get system themes",
		})
	}

	if list == nil {
		list = []*model.SystemTheme{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.SystemTheme]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *SystemThemeHandler) GetSystemTheme(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgSystemThemeNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("systemThemeId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidSystemThemeID,
		})
	}

	theme, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}
	return c.JSON(http.StatusOK, theme)
}

func (h *SystemThemeHandler) CreateSystemTheme(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgSystemThemeNoPermission,
		})
	}

	var body model.SystemThemeCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	theme, err := h.svc.Create(c.Request().Context(), &body, ctxUser.ID)
	if err != nil {
		if errors.Is(err, service.ErrDefaultSystemThemeExists) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeSystemThemeDefaultExists,
				Status: http.StatusConflict,
				Detail: "A default system theme already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create system theme",
		})
	}
	return c.JSON(http.StatusCreated, theme)
}

func (h *SystemThemeHandler) UpdateSystemTheme(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgSystemThemeNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("systemThemeId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidSystemThemeID,
		})
	}

	var body model.SystemThemeUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	theme, err := h.svc.Update(c.Request().Context(), id, &body, ctxUser.ID)
	if err != nil {
		if errors.Is(err, service.ErrDefaultSystemThemeExists) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeSystemThemeDefaultExists,
				Status: http.StatusConflict,
				Detail: "A default system theme already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update system theme",
		})
	}
	return c.JSON(http.StatusOK, theme)
}

func (h *SystemThemeHandler) DeleteSystemTheme(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgSystemThemeNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("systemThemeId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidSystemThemeID,
		})
	}

	if err := h.svc.Delete(c.Request().Context(), id, ctxUser.ID); err != nil {
		if errors.Is(err, service.ErrSystemThemeNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: "Not found",
			})
		}
		if errors.Is(err, service.ErrDeleteDefaultSystemTheme) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeSystemThemeCannotDelete,
				Status: http.StatusConflict,
				Detail: "Cannot delete the default system theme",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete system theme",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

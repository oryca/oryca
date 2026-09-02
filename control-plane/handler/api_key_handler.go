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
	msgInvalidApiKeyID = "Param 'apiKeyId' is invalid"
	msgApiKeyNotFound  = "Not found"
)

type apiKeyService interface {
	List(ctx context.Context, params url.Values, ctxUser *model.User) ([]*model.ApiKey, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error)
	Create(ctx context.Context, body *model.ApiKeyCreate, ctxUser *model.User, targetUser *model.User) (*model.ApiKey, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.ApiKeyUpdate, ctxUser *model.User) (*model.ApiKey, error)
	Regenerate(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error
	BulkDelete(ctx context.Context, body *model.ApiKeyBulkDelete, ctxUser *model.User) error
}

type apiKeyUserFinder interface {
	GetByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
}

type ApiKeyHandler struct {
	svc     apiKeyService
	userSvc apiKeyUserFinder
}

func NewApiKeyHandler(svc apiKeyService, userSvc apiKeyUserFinder) *ApiKeyHandler {
	return &ApiKeyHandler{svc: svc, userSvc: userSvc}
}

func (h *ApiKeyHandler) GetApiKeys(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	params := c.QueryParams()

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

	list, total, err := h.svc.List(c.Request().Context(), params, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get API Key list",
		})
	}

	items := make([]*model.ApiKey, len(list))
	for i, ak := range list {
		items[i] = maskForViewer(ak, ctxUser)
	}

	return c.JSON(http.StatusOK, model.ListResponse[*model.ApiKey]{
		NumberMatched:  int(total),
		NumberReturned: len(items),
		Items:          items,
	})
}

func (h *ApiKeyHandler) GetApiKey(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiKeyId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidApiKeyID,
		})
	}

	ak, err := h.svc.GetByID(c.Request().Context(), id, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeApiKeyNotFound,
				Status: http.StatusNotFound,
				Detail: msgApiKeyNotFound,
			})
		}
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get API Key",
		})
	}

	return c.JSON(http.StatusOK, maskForViewer(ak, ctxUser))
}

func (h *ApiKeyHandler) CreateApiKey(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	var body model.ApiKeyCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	// หา targetUser ถ้าระบุ userId
	var targetUser *model.User
	if body.UserID != nil {
		u, err := h.userSvc.GetByID(c.Request().Context(), *body.UserID)
		if err != nil {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeUserNotFound,
				Status: http.StatusNotFound,
				Detail: "Target user not found",
			})
		}
		targetUser = u
	}

	created, err := h.svc.Create(c.Request().Context(), &body, ctxUser, targetUser)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create API Key",
		})
	}

	return c.JSON(http.StatusCreated, maskForViewer(created, ctxUser))
}

func (h *ApiKeyHandler) UpdateApiKey(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiKeyId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidApiKeyID,
		})
	}

	var body model.ApiKeyUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	updated, err := h.svc.Update(c.Request().Context(), id, &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeApiKeyNotFound,
				Status: http.StatusNotFound,
				Detail: msgApiKeyNotFound,
			})
		}
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update API Key",
		})
	}

	return c.JSON(http.StatusOK, maskForViewer(updated, ctxUser))
}

func (h *ApiKeyHandler) RegenerateApiKey(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiKeyId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidApiKeyID,
		})
	}

	updated, err := h.svc.Regenerate(c.Request().Context(), id, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiKeyNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeApiKeyNotFound,
				Status: http.StatusNotFound,
				Detail: msgApiKeyNotFound,
			})
		}
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not regenerate API Key",
		})
	}

	return c.JSON(http.StatusOK, maskForViewer(updated, ctxUser))
}

func (h *ApiKeyHandler) DeleteApiKey(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiKeyId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidApiKeyID,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), id, forever, ctxUser); err != nil {
		if errors.Is(err, service.ErrApiKeyNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeApiKeyNotFound,
				Status: http.StatusNotFound,
				Detail: msgApiKeyNotFound,
			})
		}
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete API Key",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *ApiKeyHandler) BulkDeleteApiKeys(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgAuthRequired,
		})
	}

	var body model.ApiKeyBulkDelete
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if err := h.svc.BulkDelete(c.Request().Context(), &body, ctxUser); err != nil {
		if errors.Is(err, service.ErrApiKeyForbidden) {
			return c.JSON(http.StatusForbidden, &model.Exception{
				Code:   tool.CodeApiKeyForbidden,
				Status: http.StatusForbidden,
				Detail: "No permission",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete API Keys",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// maskForViewer ซ่อนค่า apiKey ถ้า viewer ไม่ใช่เจ้าของ
func maskForViewer(ak *model.ApiKey, viewer *model.User) *model.ApiKey {
	if ak.OwnerBy != nil && *ak.OwnerBy == viewer.ID {
		return ak
	}
	masked := *ak
	masked.ApiKey = tool.MaskApiKey(ak.ApiKey)
	return &masked
}

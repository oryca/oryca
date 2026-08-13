package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
)

const (
	msgInvalidSourceID = "Param 'sourceId' is invalid"
	msgSourceNotFound  = "Not found"
)

type gatewaySourceService interface {
	List(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error)
	GetByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error)
	Create(ctx context.Context, body *model.GatewaySourceCreate, ctxUser *model.User) (*model.GatewaySource, error)
	Update(ctx context.Context, idOrAlias string, body *model.GatewaySourceUpdate, ctxUser *model.User) (*model.GatewaySource, error)
	Delete(ctx context.Context, idOrAlias string, forever bool, ctxUser *model.User) error
}

type GatewaySourceHandler struct {
	svc gatewaySourceService
}

func NewGatewaySourceHandler(svc gatewaySourceService) *GatewaySourceHandler {
	return &GatewaySourceHandler{svc: svc}
}

func (h *GatewaySourceHandler) hasWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *GatewaySourceHandler) GetSources(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
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
			Detail: "Could not get sources",
		})
	}

	if ctxUser.Role == "user" {
		public := make([]*model.GatewaySourcePublic, len(list))
		for i, src := range list {
			public[i] = toSourcePublic(src)
		}
		return c.JSON(http.StatusOK, &model.ListResponse[*model.GatewaySourcePublic]{
			NumberMatched:  int(total),
			NumberReturned: len(public),
			Items:          public,
		})
	}

	if list == nil {
		list = []*model.GatewaySource{}
	}
	return c.JSON(http.StatusOK, &model.ListResponse[*model.GatewaySource]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *GatewaySourceHandler) GetSource(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	src, err := h.svc.GetByIDOrAlias(c.Request().Context(), c.Param("sourceId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewaySourceNotFound,
			Status: http.StatusNotFound,
			Detail: msgSourceNotFound,
		})
	}

	if ctxUser.Role == "user" {
		return c.JSON(http.StatusOK, toSourcePublic(src))
	}
	return c.JSON(http.StatusOK, src)
}

func toSourcePublic(src *model.GatewaySource) *model.GatewaySourcePublic {
	return &model.GatewaySourcePublic{
		ID:          src.ID,
		Alias:       src.Alias,
		Name:        src.Name,
		Description: src.Description,
		Type:        src.Type,
		Protocol:    src.Protocol,
	}
}

func (h *GatewaySourceHandler) CreateSource(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewaySourceCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Alias == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'alias' is required",
		})
	}
	if body.Name == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'name' is required",
		})
	}
	if body.Type == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'type' is required",
		})
	}

	src, err := h.svc.Create(c.Request().Context(), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrGatewaySourceAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeGatewaySourceAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Body 'alias' already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create source",
		})
	}

	return c.JSON(http.StatusCreated, src)
}

func (h *GatewaySourceHandler) UpdateSource(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewaySourceUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Alias == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'alias' is required",
		})
	}
	if body.Name == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'name' is required",
		})
	}
	if body.Type == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'type' is required",
		})
	}

	src, err := h.svc.Update(c.Request().Context(), c.Param("sourceId"), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrGatewaySourceNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGatewaySourceNotFound,
				Status: http.StatusNotFound,
				Detail: msgSourceNotFound,
			})
		}
		if errors.Is(err, service.ErrGatewaySourceAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeGatewaySourceAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Body 'alias' already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update source",
		})
	}

	return c.JSON(http.StatusOK, src)
}

func (h *GatewaySourceHandler) DeleteSource(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), c.Param("sourceId"), forever, ctxUser); err != nil {
		if errors.Is(err, service.ErrGatewaySourceNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGatewaySourceNotFound,
				Status: http.StatusNotFound,
				Detail: msgSourceNotFound,
			})
		}

		log.Default().Printf("Failed to delete source: %v", err)
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete source",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

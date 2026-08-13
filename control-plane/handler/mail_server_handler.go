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
	msgInvalidMailServerID = "Param 'mailServerId' is invalid"
	msgNoPermission        = "No permission"
	msgNotFound            = "Not found"
	msgAuthRequired        = "Authentication required"
)

type mailServerService interface {
	List(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error)
	Create(ctx context.Context, body *model.MailServerCreate) (*model.MailServer, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.MailServerUpdate) (*model.MailServer, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error
}

type MailServerHandler struct {
	svc mailServerService
}

func NewMailServerHandler(svc mailServerService) *MailServerHandler {
	return &MailServerHandler{svc: svc}
}

func (h *MailServerHandler) GetMailServers(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
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
			Detail: "Could not get mail servers",
		})
	}

	if list == nil {
		list = []*model.MailServer{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.MailServer]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *MailServerHandler) GetMailServer(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("mailServerId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidMailServerID,
		})
	}

	ms, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeMailServerNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}
	return c.JSON(http.StatusOK, ms)
}

func (h *MailServerHandler) CreateMailServer(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.MailServerCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	ms, err := h.svc.Create(c.Request().Context(), &body)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create mail server",
		})
	}
	return c.JSON(http.StatusCreated, ms)
}

func (h *MailServerHandler) UpdateMailServer(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("mailServerId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidMailServerID,
		})
	}

	var body model.MailServerUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	ms, err := h.svc.Update(c.Request().Context(), id, &body)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update mail server",
		})
	}
	return c.JSON(http.StatusOK, ms)
}

func (h *MailServerHandler) DeleteMailServer(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role != "root" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("mailServerId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidMailServerID,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), id, forever, ctxUser.ID); err != nil {
		if errors.Is(err, service.ErrDeleteDefaultMailServer) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeMailServerCannotDelete,
				Status: http.StatusConflict,
				Detail: "Cannot delete the default mail server",
			})
		}
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeMailServerNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

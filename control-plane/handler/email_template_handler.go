package handler

import (
	"context"
	"net/http"
	"net/url"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgInvalidEmailTemplateID = "Param 'emailTemplateId' is invalid"
	msgEmailTplNoPermission   = "No permission"
)

type emailTemplateService interface {
	List(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error)
	Create(ctx context.Context, body *model.EmailTemplateCreate) (*model.EmailTemplate, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.EmailTemplateUpdate) (*model.EmailTemplate, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error
}

type EmailTemplateHandler struct {
	svc emailTemplateService
}

func NewEmailTemplateHandler(svc emailTemplateService) *EmailTemplateHandler {
	return &EmailTemplateHandler{svc: svc}
}

func (h *EmailTemplateHandler) GetEmailTemplates(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || !isRootOrAdmin(ctxUser.Role) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgEmailTplNoPermission,
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
			Detail: "Could not get email templates",
		})
	}

	if list == nil {
		list = []*model.EmailTemplate{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.EmailTemplate]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *EmailTemplateHandler) GetEmailTemplate(c echo.Context) error {
	if ctxUser, _ := c.Get("user").(*model.User); ctxUser == nil || !isRootOrAdmin(ctxUser.Role) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgEmailTplNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("emailTemplateId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidEmailTemplateID,
		})
	}

	tmpl, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeEmailTemplateNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}
	return c.JSON(http.StatusOK, tmpl)
}

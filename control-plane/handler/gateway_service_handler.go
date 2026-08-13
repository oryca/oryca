package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgGatewayServiceNotFound = "Not found"
	msgBasePathRequired       = "Body 'basePath' is required"
)

type gatewayServiceService interface {
	ListWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error)
	GetByIDWithSource(ctx context.Context, id string) (*model.GatewayServiceWithSource, error)
	Create(ctx context.Context, body *model.GatewayServiceCreate, ctxUser *model.User) (*model.GatewayServiceWithSource, error)
	Update(ctx context.Context, id string, body *model.GatewayServiceUpdate, ctxUser *model.User) (*model.GatewayServiceWithSource, error)
	Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error
	CheckPaths(ctx context.Context, basePath string, resourcePaths []string, excludeID *bson.ObjectID) (*model.GatewayServiceCheckPathsResponse, error)
}

type GatewayServiceHandler struct {
	svc gatewayServiceService
}

func NewGatewayServiceHandler(svc gatewayServiceService) *GatewayServiceHandler {
	return &GatewayServiceHandler{svc: svc}
}

func (h *GatewayServiceHandler) hasWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *GatewayServiceHandler) GetServices(c echo.Context) error {
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

	if ctxUser.Role == "user" {
		params.Set("enabled", "true")
	}

	list, total, err := h.svc.ListWithSource(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get services",
		})
	}

	if list == nil {
		list = []*model.GatewayServiceWithSource{}
	}

	if !isPrivilegedRole(ctxUser.Role) {
		for _, gs := range list {
			redactSourceFields(gs.ResourcePaths)
		}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.GatewayServiceWithSource]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *GatewayServiceHandler) GetService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	gs, err := h.svc.GetByIDWithSource(c.Request().Context(), c.Param("serviceId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewayServiceNotFound,
			Status: http.StatusNotFound,
			Detail: msgGatewayServiceNotFound,
		})
	}

	if ctxUser.Role == "user" && (gs.Enabled == nil || !*gs.Enabled) {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewayServiceNotFound,
			Status: http.StatusNotFound,
			Detail: msgGatewayServiceNotFound,
		})
	}

	if !isPrivilegedRole(ctxUser.Role) {
		redactSourceFields(gs.ResourcePaths)
	}
	return c.JSON(http.StatusOK, gs)
}

var validServiceTypes = map[string]bool{
	"General":              true,
	"OGC_API_Features":     true,
	"OGC_API_STAC":         true,
	"OGC_API_Styles":       true,
	"OGC_API_SensorThings": true,
	"OGC_API_Tiles":        true,
}

func validateServiceBodyFields(name, svcType, basePath string) string {
	switch {
	case name == "":
		return "Body 'name' is required"
	case svcType == "":
		return "Body 'type' is required"
	case !validServiceTypes[svcType]:
		return "Body 'type' must be one of [General, OGC_API_Features, OGC_API_STAC, OGC_API_Styles, OGC_API_SensorThings, OGC_API_Tiles]"
	case basePath == "":
		return msgBasePathRequired
	}
	return ""
}

func (h *GatewayServiceHandler) CreateService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewayServiceCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if detail := validateServiceBodyFields(body.Name, body.Type, body.BasePath); detail != "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: detail,
		})
	}

	svc, err := h.svc.Create(c.Request().Context(), &body, ctxUser)
	if err != nil {
		return gatewayServiceWriteError(c, err, "Could not create service")
	}

	return c.JSON(http.StatusCreated, svc)
}

func (h *GatewayServiceHandler) UpdateService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewayServiceUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if detail := validateServiceBodyFields(body.Name, body.Type, body.BasePath); detail != "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: detail,
		})
	}

	svc, err := h.svc.Update(c.Request().Context(), c.Param("serviceId"), &body, ctxUser)
	if err != nil {
		return gatewayServiceWriteError(c, err, "Could not update service")
	}

	return c.JSON(http.StatusOK, svc)
}

func (h *GatewayServiceHandler) DeleteService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), c.Param("serviceId"), forever, ctxUser); err != nil {
		if errors.Is(err, service.ErrGatewayServiceNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGatewayServiceNotFound,
				Status: http.StatusNotFound,
				Detail: msgGatewayServiceNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete service",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *GatewayServiceHandler) CheckResourcePaths(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role == "user" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewayServiceCheckPathsRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.BasePath == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgBasePathRequired,
		})
	}
	if len(body.ResourcePaths) == 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'resourcePaths' is required",
		})
	}

	for _, rp := range body.ResourcePaths {
		if strings.Contains(rp, "*") && !strings.HasSuffix(rp, "/*") {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeBodyInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: "Wildcard '*' must appear only as the last segment (e.g. /path/*)",
			})
		}
	}

	var excludeID *bson.ObjectID
	if body.ExcludeServiceID != "" {
		oid, err := bson.ObjectIDFromHex(body.ExcludeServiceID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeBodyInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: "Body 'excludeServiceId' is invalid",
			})
		}
		excludeID = &oid
	}

	resp, err := h.svc.CheckPaths(c.Request().Context(), body.BasePath, body.ResourcePaths, excludeID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not check resource paths",
		})
	}

	return c.JSON(http.StatusOK, resp)
}

func isPrivilegedRole(role string) bool {
	return isRootOrAdmin(role)
}

// redactSourceFields replaces embedded source with public-only fields for user role
func redactSourceFields(rps []*model.ResourcePathWithSource) {
	for _, rp := range rps {
		if rp.Source == nil {
			continue
		}
		rp.Source = &model.GatewaySource{
			ID:          rp.Source.ID,
			Alias:       rp.Source.Alias,
			Name:        rp.Source.Name,
			Description: rp.Source.Description,
			Type:        rp.Source.Type,
			Protocol:    rp.Source.Protocol,
		}
	}
}

func gatewayServiceWriteError(c echo.Context, err error, fallbackMsg string) error {
	// structured exception from service layer (e.g. source alias validation with field details)
	var exc *model.Exception
	if errors.As(err, &exc) {
		return c.JSON(exc.Status, exc)
	}
	switch {
	case errors.Is(err, service.ErrGatewayServiceNotFound):
		return c.JSON(http.StatusNotFound, &model.Exception{Code: tool.CodeGatewayServiceNotFound, Status: http.StatusNotFound, Detail: msgGatewayServiceNotFound})
	case errors.Is(err, service.ErrGatewayServiceBasePathDuplicate):
		return c.JSON(http.StatusConflict, &model.Exception{Code: tool.CodeGatewayServiceBasePathDuplicate, Status: http.StatusConflict, Detail: "BasePath already exists"})
	case errors.Is(err, service.ErrGatewayServiceResourcePathConflict):
		return c.JSON(http.StatusConflict, &model.Exception{Code: tool.CodeGatewayServiceResourcePathConflict, Status: http.StatusConflict, Detail: "One or more resource paths conflict internally"})
	case errors.Is(err, service.ErrGatewayServiceSourceAliasInvalid):
		return c.JSON(http.StatusBadRequest, &model.Exception{Code: tool.CodeGatewayServiceSourceAliasInvalid, Status: http.StatusBadRequest, Detail: "One or more 'sourceAlias' values do not exist"})
	}
	return c.JSON(http.StatusUnprocessableEntity, &model.Exception{Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: fallbackMsg})
}

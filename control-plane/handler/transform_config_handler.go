package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgResponseTransformNotFound         = "Response transform not found"
	msgInvalidResponseTransformID        = "Param 'transformConfigId' is invalid"
	msgInvalidResponseTransformServiceID = "Query param 'serviceId' is invalid"
)

type transformConfigService interface {
	List(ctx context.Context, serviceID *bson.ObjectID, limit, offset int64) ([]*model.TransformConfig, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.TransformConfig, error)
	Create(ctx context.Context, req *model.TransformConfigRequest, adminID bson.ObjectID) (*model.TransformConfig, error)
	Update(ctx context.Context, id bson.ObjectID, req *model.TransformConfigRequest, adminID bson.ObjectID) error
	Delete(ctx context.Context, id bson.ObjectID, adminID bson.ObjectID) error
}

type TransformConfigHandler struct {
	svc transformConfigService
}

func NewTransformConfigHandler(svc transformConfigService) *TransformConfigHandler {
	return &TransformConfigHandler{svc: svc}
}

func (h *TransformConfigHandler) canManage(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *TransformConfigHandler) List(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.canManage(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code: tool.CodeUnauthorizedAccess, Status: http.StatusForbidden, Detail: msgNoPermission,
		})
	}

	var serviceID *bson.ObjectID
	if s := c.QueryParam("serviceId"); s != "" {
		id, err := bson.ObjectIDFromHex(s)
		if err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code: tool.CodeQueryParamInvalidFormat, Status: http.StatusBadRequest, Detail: msgInvalidResponseTransformServiceID,
			})
		}
		serviceID = &id
	}

	limit, offset, err := parseLimitOffset(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{Code: tool.CodeQueryParamInvalidFormat, Status: http.StatusBadRequest, Detail: err.Error()})
	}
	cfgs, total, err := h.svc.List(c.Request().Context(), serviceID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: "Failed to list response transforms",
		})
	}
	if cfgs == nil {
		cfgs = []*model.TransformConfig{}
	}
	return c.JSON(http.StatusOK, &model.ListResponse[*model.TransformConfig]{
		NumberMatched: int(total), NumberReturned: len(cfgs), Items: cfgs,
	})
}

func (h *TransformConfigHandler) Get(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.canManage(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code: tool.CodeUnauthorizedAccess, Status: http.StatusForbidden, Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("transformConfigId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeParamInvalidFormat, Status: http.StatusBadRequest, Detail: msgInvalidResponseTransformID,
		})
	}

	cfg, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: "Failed to get response transform",
		})
	}
	if cfg == nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code: tool.CodeNotFound, Status: http.StatusNotFound, Detail: msgResponseTransformNotFound,
		})
	}
	return c.JSON(http.StatusOK, cfg)
}

func (h *TransformConfigHandler) Create(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.canManage(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code: tool.CodeUnauthorizedAccess, Status: http.StatusForbidden, Detail: msgNoPermission,
		})
	}

	var req model.TransformConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeBodyInvalidFormat, Status: http.StatusBadRequest, Detail: "Request body is invalid",
		})
	}

	if errs := validateTransformConfigRequest(&req); len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeBodyInvalidFormat, Status: http.StatusBadRequest, Detail: strings.Join(errs, ", "),
		})
	}

	cfg, err := h.svc.Create(c.Request().Context(), &req, ctxUser.ID)
	if err != nil {
		return responseTransformWriteError(c, err, "Failed to create response transform")
	}
	return c.JSON(http.StatusCreated, cfg)
}

func (h *TransformConfigHandler) Update(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.canManage(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code: tool.CodeUnauthorizedAccess, Status: http.StatusForbidden, Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("transformConfigId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeParamInvalidFormat, Status: http.StatusBadRequest, Detail: msgInvalidResponseTransformID,
		})
	}

	var req model.TransformConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeBodyInvalidFormat, Status: http.StatusBadRequest, Detail: "Request body is invalid",
		})
	}

	if errs := validateTransformConfigRequest(&req); len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeBodyInvalidFormat, Status: http.StatusBadRequest, Detail: strings.Join(errs, ", "),
		})
	}

	if err := h.svc.Update(c.Request().Context(), id, &req, ctxUser.ID); err != nil {
		return responseTransformWriteError(c, err, "Failed to update response transform")
	}
	updated, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil || updated == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *TransformConfigHandler) Delete(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.canManage(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code: tool.CodeUnauthorizedAccess, Status: http.StatusForbidden, Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("transformConfigId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code: tool.CodeParamInvalidFormat, Status: http.StatusBadRequest, Detail: msgInvalidResponseTransformID,
		})
	}

	if err := h.svc.Delete(c.Request().Context(), id, ctxUser.ID); err != nil {
		return responseTransformWriteError(c, err, "Failed to delete response transform")
	}
	return c.NoContent(http.StatusNoContent)
}

type transformVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

var transformVariables = []transformVariable{
	{
		Name:        "{{oryca_gateway_url}}",
		Description: "URL ของ gateway รวม base path ของ service derive จาก request อัตโนมัติ",
		Example:     "https://gateway.example.com/gateway/api/resources/my-service",
	},
	{
		Name:        "{{oryca_auth}}",
		Description: "Auth query param ของ user ที่เรียก gateway — api_key=xxx ถ้า auth ด้วย API key, token=xxx ถ้า auth ด้วย Bearer token",
		Example:     "api_key=abc123 หรือ token=eyJhbGci...",
	},
}

func (h *TransformConfigHandler) Variables(c echo.Context) error {
	search := strings.ToLower(c.QueryParam("search"))
	nameFilter := strings.ToLower(c.QueryParam("name"))

	result := make([]transformVariable, 0, len(transformVariables))
	for _, v := range transformVariables {
		nameLower := strings.ToLower(v.Name)
		descLower := strings.ToLower(v.Description)

		if search != "" && !matchWildcard(search, nameLower) && !matchWildcard(search, descLower) {
			continue
		}
		if nameFilter != "" && !matchWildcard(nameFilter, nameLower) {
			continue
		}
		result = append(result, v)
	}

	return c.JSON(http.StatusOK, &model.ListResponse[transformVariable]{
		NumberMatched:  len(result),
		NumberReturned: len(result),
		Items:          result,
	})
}

// matchWildcard รองรับ * เป็น wildcard เช่น "oryca_*" หรือ "*auth*"
func matchWildcard(pattern, s string) bool {
	if !strings.Contains(pattern, "*") {
		return strings.Contains(s, pattern)
	}
	return matchParts(strings.Split(pattern, "*"), strings.HasSuffix(pattern, "*"), s)
}

func matchParts(parts []string, trailingStar bool, s string) bool {
	pos := 0
	for i, p := range parts {
		if p == "" {
			continue
		}
		idx := strings.Index(s[pos:], p)
		if idx == -1 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		pos += idx + len(p)
	}
	return trailingStar || pos == len(s)
}

func responseTransformWriteError(c echo.Context, err error, fallbackMsg string) error {
	switch {
	case errors.Is(err, service.ErrTransformConfigNotFound):
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code: tool.CodeNotFound, Status: http.StatusNotFound, Detail: msgResponseTransformNotFound,
		})
	}
	return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
		Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: fallbackMsg,
	})
}

func validateTransformConfigRequest(req *model.TransformConfigRequest) []string {
	var errs []string
	if strings.TrimSpace(req.Name) == "" {
		errs = append(errs, "name is required")
	}
	if strings.TrimSpace(req.ServiceID) == "" {
		errs = append(errs, "serviceId is required")
	} else if _, err := bson.ObjectIDFromHex(req.ServiceID); err != nil {
		errs = append(errs, "serviceId is invalid")
	}
	if strings.TrimSpace(req.Match.Path) == "" {
		errs = append(errs, "match.path is required")
	}
	if len(req.Rules) == 0 {
		errs = append(errs, "rules must not be empty")
	}
	for i, rule := range req.Rules {
		if rule.Target != "body" && rule.Target != "headers" {
			errs = append(errs, fmt.Sprintf("rules[%d].target must be 'body' or 'headers'", i))
		}
		if rule.Target == "body" && rule.Type != "json" && rule.Type != "xml" {
			errs = append(errs, fmt.Sprintf("rules[%d].type must be 'json' or 'xml'", i))
		}
		validActions := map[string]bool{"replace": true, "add": true, "append": true, "remove": true, "rename": true}
		if !validActions[rule.Action] {
			errs = append(errs, fmt.Sprintf("rules[%d].action is invalid", i))
		}
	}
	return errs
}

func parseLimitOffset(c echo.Context) (int64, int64, error) {
	var limit, offset int64 = tool.Limit, tool.Offset
	if v := c.QueryParam("limit"); v != "" {
		n, err := tool.ParseLimit(v)
		if err != nil {
			return 0, 0, err
		}
		limit = int64(n)
	}
	if v := c.QueryParam("offset"); v != "" {
		n, err := tool.ParseOffset(v)
		if err != nil {
			return 0, 0, err
		}
		offset = int64(n)
	}
	return limit, offset, nil
}

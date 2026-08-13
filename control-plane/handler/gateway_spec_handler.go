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
)

type gatewaySpecService interface {
	GetByServiceID(ctx context.Context, serviceID string) (*model.GatewaySpec, error)
	Upsert(ctx context.Context, serviceID string, title string, description string, body *model.GatewaySpecUpsert, ctxUser *model.User) (*model.GatewaySpec, error)
	Delete(ctx context.Context, serviceID string) error
}

type specServiceGetter interface {
	GetByID(ctx context.Context, id string) (*model.GatewayService, error)
}

type GatewaySpecHandler struct {
	svc        gatewaySpecService
	serviceSvc specServiceGetter
}

func NewGatewaySpecHandler(svc gatewaySpecService, serviceSvc specServiceGetter) *GatewaySpecHandler {
	return &GatewaySpecHandler{svc: svc, serviceSvc: serviceSvc}
}

func (h *GatewaySpecHandler) hasWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *GatewaySpecHandler) GetSpec(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || ctxUser.Role == "user" {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	spec, err := h.svc.GetByServiceID(c.Request().Context(), c.Param("serviceId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewaySpecNotFound,
			Status: http.StatusNotFound,
			Detail: msgNotFound,
		})
	}

	return c.JSON(http.StatusOK, spec)
}

func (h *GatewaySpecHandler) UpsertSpec(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GatewaySpecUpsert
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	serviceID := c.Param("serviceId")
	gatewaySvc, err := h.serviceSvc.GetByID(c.Request().Context(), serviceID)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewayServiceNotFound,
			Status: http.StatusNotFound,
			Detail: "Service not found",
		})
	}
	spec, err := h.svc.Upsert(c.Request().Context(), serviceID, gatewaySvc.Name, gatewaySvc.Description, &body, ctxUser)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not save spec",
		})
	}

	return c.JSON(http.StatusOK, spec)
}

func (h *GatewaySpecHandler) DeleteSpec(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if err := h.svc.Delete(c.Request().Context(), c.Param("serviceId")); err != nil {
		if errors.Is(err, service.ErrGatewaySpecNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGatewaySpecNotFound,
				Status: http.StatusNotFound,
				Detail: msgNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete spec",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *GatewaySpecHandler) GetOpenAPI(c echo.Context) error {
	serviceID := c.Param("serviceId")

	spec, err := h.svc.GetByServiceID(c.Request().Context(), serviceID)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewaySpecNotFound,
			Status: http.StatusNotFound,
			Detail: msgNotFound,
		})
	}

	svc, err := h.serviceSvc.GetByID(c.Request().Context(), serviceID)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGatewayServiceNotFound,
			Status: http.StatusNotFound,
			Detail: msgNotFound,
		})
	}

	scheme := "https"
	if c.Request().TLS == nil {
		scheme = "http"
	}
	baseURL := scheme + "://" + c.Request().Host

	doc := h.buildOpenAPIDoc(spec, svc, baseURL)
	return c.JSON(http.StatusOK, doc)
}

func (h *GatewaySpecHandler) buildOpenAPIDoc(spec *model.GatewaySpec, svc *model.GatewayService, baseURL string) *model.OpenAPIDoc {
	version := spec.Version
	if version == "" {
		version = "1.0.0"
	}

	serverURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(svc.BasePath, "/")

	doc := &model.OpenAPIDoc{
		OpenAPI: "3.0.0",
		Info: model.OpenAPIInfo{
			Title:       spec.Title,
			Description: spec.Description,
			Version:     version,
		},
		Servers: []model.OpenAPIServer{
			{
				URL:         serverURL,
				Description: spec.ServerDescription,
			},
		},
		Paths: make(map[string]model.OpenAPIPathItem),
	}

	for _, p := range spec.Paths {
		if _, ok := doc.Paths[p.Path]; !ok {
			doc.Paths[p.Path] = make(model.OpenAPIPathItem)
		}

		params := make([]*model.OpenAPIParameter, 0, len(p.Parameters))
		for _, param := range p.Parameters {
			schema := toOpenAPISchema(param.Schema)
			if schema == nil && param.Type != "" {
				schema = &model.OpenAPISchema{Type: param.Type}
			}
			params = append(params, &model.OpenAPIParameter{
				Name:        param.Name,
				In:          param.In,
				Description: param.Description,
				Required:    param.Required,
				Schema:      schema,
			})
		}

		responses := make(map[string]*model.OpenAPIResponse, len(p.Responses))
		for _, r := range p.Responses {
			responses[fmt.Sprintf("%d", r.Status)] = &model.OpenAPIResponse{
				Description: r.Description,
				Content:     toOpenAPIContent(r.Content),
			}
		}
		if len(responses) == 0 {
			responses["200"] = &model.OpenAPIResponse{Description: "Success"}
		}

		doc.Paths[p.Path][strings.ToLower(p.Method)] = &model.OpenAPIOperation{
			OperationId: p.OperationId,
			Tags:        p.Tags,
			Summary:     p.Summary,
			Description: p.Description,
			Parameters:  params,
			RequestBody: toOpenAPIRequestBody(p.RequestBody),
			Responses:   responses,
		}
	}

	return doc
}

func toOpenAPISchema(s *model.SpecSchema) *model.OpenAPISchema {
	if s == nil {
		return nil
	}
	out := &model.OpenAPISchema{
		Type:        s.Type,
		Format:      s.Format,
		Description: s.Description,
		Example:     s.Example,
		Enum:        s.Enum,
		Required:    s.Required,
		Items:       toOpenAPISchema(s.Items),
	}
	if len(s.Properties) > 0 {
		out.Properties = make(map[string]*model.OpenAPISchema, len(s.Properties))
		for k, v := range s.Properties {
			out.Properties[k] = toOpenAPISchema(v)
		}
	}
	return out
}

func toOpenAPIContent(content map[string]*model.SpecMediaType) map[string]*model.OpenAPIMediaType {
	if len(content) == 0 {
		return nil
	}
	out := make(map[string]*model.OpenAPIMediaType, len(content))
	for mediaType, mt := range content {
		out[mediaType] = &model.OpenAPIMediaType{Schema: toOpenAPISchema(mt.Schema)}
	}
	return out
}

func toOpenAPIRequestBody(rb *model.SpecRequestBody) *model.OpenAPIRequestBody {
	if rb == nil {
		return nil
	}
	return &model.OpenAPIRequestBody{
		Description: rb.Description,
		Required:    rb.Required,
		Content:     toOpenAPIContent(rb.Content),
	}
}

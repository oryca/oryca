package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"
	"net/http"
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type apiDocService interface {
	List(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID, params url.Values) (*model.ApiDoc, error)
	Create(ctx context.Context, body *model.ApiDocCreate, ctxUser *model.User) (*model.ApiDoc, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.ApiDocUpdate, ctxUser *model.User) (*model.ApiDoc, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error
	GetPathsByID(ctx context.Context, id bson.ObjectID, params url.Values) ([]*model.ApiDocPath, int64, error)
}

type ApiDocHandler struct {
	svc     apiDocService
	apiHost string
}

func NewApiDocHandler(svc apiDocService, apiHost string) *ApiDocHandler {
	return &ApiDocHandler{svc: svc, apiHost: apiHost}
}

func (h *ApiDocHandler) hasServiceWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if ctxUser == nil {
		return false
	}
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *ApiDocHandler) applyReadScope(c echo.Context, params *url.Values) {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		params.Set("authenRequired", "none")
	}
	if !h.hasServiceWriteAccess(c, ctxUser) {
		params.Set("publishStatus", "published")
	}
}

func (h *ApiDocHandler) GetApiDocs(c echo.Context) error {
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

	h.applyReadScope(c, &params)

	list, total, err := h.svc.List(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not list api docs",
		})
	}

	if list == nil {
		list = []*model.ApiDoc{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.ApiDoc]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *ApiDocHandler) GetApiDoc(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("apiDocId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}

	params := url.Values{}
	h.applyReadScope(c, &params)

	doc, err := h.svc.GetByID(c.Request().Context(), id, params)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}
	if v := c.QueryParam("format"); v == "json" {
		openapiDoc := h.buildOpenAPIDocJson(doc)
		return c.JSON(http.StatusOK, openapiDoc)
	}

	return c.JSON(http.StatusOK, doc)
}

func (h *ApiDocHandler) CreateApiDoc(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if !h.hasServiceWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.ApiDocCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	h.autoFillApiDocCreate(&body)

	if body.AuthenRequired != model.ApiDocAuthenRequiredNone && body.AuthenRequired != model.ApiDocAuthenRequiredRequired {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'authenRequired' must be 'none' or 'required'",
		})
	}
	if body.PublishStatus != model.ApiDocPublishStatusDraft && body.PublishStatus != model.ApiDocPublishStatusPublished {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'publishStatus' must be 'draft' or 'published'",
		})
	}

	apidoc := h.convertApiDocCreateToApiDoc(&body)

	if issues := validateApiDocKeyField(apidoc); len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: buildApiDocErrorDetail(issues),
		})
	}

	openapi := h.buildOpenAPIDocJson(apidoc)
	openapiJson, _ := json.Marshal(openapi)
	err := validateOpenAPI(c.Request().Context(), openapiJson)
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: buildApiDocErrorDetail(parserKinErrorMessages(err)),
		})
	}

	doc, err := h.svc.Create(c.Request().Context(), &body, ctxUser)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create api doc",
		})
	}

	return c.JSON(http.StatusCreated, doc)
}

func (h *ApiDocHandler) UpdateApiDoc(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if !h.hasServiceWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiDocId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}

	var body model.ApiDocUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.AuthenRequired != model.ApiDocAuthenRequiredNone && body.AuthenRequired != model.ApiDocAuthenRequiredRequired {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'authenRequired' must be 'none' or 'required'",
		})
	}
	if body.PublishStatus != model.ApiDocPublishStatusDraft && body.PublishStatus != model.ApiDocPublishStatusPublished {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'publishStatus' must be 'draft' or 'published'",
		})
	}

	h.autoFillApiDocUpdate(&body)

	apidoc := h.convertApiDocUpdateToApiDoc(&body)

	if issues := validateApiDocKeyField(apidoc); len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: buildApiDocErrorDetail(issues),
		})
	}

	openapi := h.buildOpenAPIDocJson(apidoc)
	openapiJson, _ := json.Marshal(openapi)
	err = validateOpenAPI(c.Request().Context(), openapiJson)
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: buildApiDocErrorDetail(parserKinErrorMessages(err)),
		})
	}

	doc, err := h.svc.Update(c.Request().Context(), id, &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiDocNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: "Not found",
			})
		}
		if errors.Is(err, service.ErrApiDocFailedReFetch) {
			return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
				Code:   tool.CodeOperationFailed,
				Detail: "Updated api doc successfully, but could not retrieve the latest version",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update api doc",
		})
	}

	return c.JSON(http.StatusOK, doc)
}

func (h *ApiDocHandler) DeleteApiDoc(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if !h.hasServiceWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("apiDocId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}

	forever := c.QueryParam("forever") == "true"

	err = h.svc.Delete(c.Request().Context(), id, forever, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrApiDocNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: "Not found",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete api doc",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *ApiDocHandler) GetApiDocPaths(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("apiDocId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
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

	h.applyReadScope(c, &params)

	list, total, err := h.svc.GetPathsByID(c.Request().Context(), id, params)
	if err != nil {
		if errors.Is(err, service.ErrApiDocNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: "Not found",
			})
		}

		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not list paths",
		})
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.ApiDocPath]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})

}

func (h *ApiDocHandler) convertApiDocCreateToApiDoc(doc *model.ApiDocCreate) *model.ApiDoc {
	if doc == nil {
		return nil
	}

	return &model.ApiDoc{
		OpenAPI:        doc.OpenAPI,
		AuthenRequired: doc.AuthenRequired,
		PublishStatus:  doc.PublishStatus,
		Info:           doc.Info,
		Servers:        doc.Servers,
		Tags:           doc.Tags,
		Paths:          doc.Paths,
	}
}

func (h *ApiDocHandler) convertApiDocUpdateToApiDoc(doc *model.ApiDocUpdate) *model.ApiDoc {
	if doc == nil {
		return nil
	}

	return &model.ApiDoc{
		OpenAPI:        doc.OpenAPI,
		AuthenRequired: doc.AuthenRequired,
		PublishStatus:  doc.PublishStatus,
		Info:           doc.Info,
		Servers:        doc.Servers,
		Tags:           doc.Tags,
		Paths:          doc.Paths,
	}
}

func (h *ApiDocHandler) buildOpenAPIDocJson(doc *model.ApiDoc) *model.OpenAPIDoc {
	if doc == nil {
		return nil
	}

	output := &model.OpenAPIDoc{
		OpenAPI: doc.OpenAPI,
		Info: model.OpenAPIInfo{
			Title:       doc.Info.Title,
			Description: derefStr(doc.Info.Description),
			Version:     doc.Info.Version,
		},
		Servers: make([]model.OpenAPIServer, 0, len(doc.Servers)),
		Paths:   make(map[string]model.OpenAPIPathItem),
	}

	for _, s := range doc.Servers {
		if s == nil {
			continue
		}
		output.Servers = append(output.Servers, model.OpenAPIServer{
			URL:         s.URL,
			Description: derefStr(s.Description),
		})
	}

	for _, p := range doc.Paths {
		if p == nil {
			continue
		}
		if _, ok := output.Paths[p.Path]; !ok {
			output.Paths[p.Path] = make(model.OpenAPIPathItem)
		}
		output.Paths[p.Path][strings.ToLower(p.Method)] = apiDocPathToOperation(p)
	}

	return output
}

func apiDocPathToOperation(p *model.ApiDocPath) *model.OpenAPIOperation {
	op := &model.OpenAPIOperation{
		OperationId: derefStr(p.OperationID),
		Tags:        p.Tags,
		Summary:     derefStr(p.Summary),
		Description: derefStr(p.Description),
		Responses:   make(map[string]*model.OpenAPIResponse),
	}

	params := make([]*model.OpenAPIParameter, 0, len(p.Parameters))
	for _, param := range p.Parameters {
		if param == nil {
			continue
		}
		params = append(params, &model.OpenAPIParameter{
			Name:        param.Name,
			In:          param.In,
			Description: derefStr(param.Description),
			Required:    derefBool(param.Required, false),
			Schema:      apiDocToSchema(param.Schema),
		})
	}
	op.Parameters = params

	if p.RequestBody != nil {
		op.RequestBody = &model.OpenAPIRequestBody{
			Description: derefStr(p.RequestBody.Description),
			Required:    derefBool(p.RequestBody.Required, false),
			Content:     apiDocContentToMap(p.RequestBody.Content),
		}
	}

	for _, r := range p.Responses {
		if r == nil || r.Status == "" {
			continue
		}
		op.Responses[r.Status] = &model.OpenAPIResponse{
			Description: r.Description,
			Content:     apiDocContentsToMap(r.Contents),
		}
	}

	return op
}

func apiDocToSchema(s *model.ApiDocSchema) *model.OpenAPISchema {
	if s == nil {
		return nil
	}
	out := &model.OpenAPISchema{
		Type:     derefStr(s.Type),
		Required: s.Required,
	}
	if s.Type != nil && *s.Type == "array" {
		out.Items = apiDocToSchema(s.Items)
	}
	if len(s.Properties) > 0 {
		out.Properties = make(map[string]*model.OpenAPISchema, len(s.Properties))
		for _, prop := range s.Properties {
			if prop == nil {
				continue
			}
			out.Properties[prop.Property] = &model.OpenAPISchema{
				Description: derefStr(prop.Description),
				Type:        derefStr(prop.Type),
				Example:     prop.Example,
			}
			if prop.Type != nil && *prop.Type == "array" {
				out.Properties[prop.Property].Items = apiDocToSchema(prop.Items)
			}
		}
	}
	return out
}

// single ApiDocContent → map keyed by its media type
func apiDocContentToMap(c *model.ApiDocContent) map[string]*model.OpenAPIMediaType {
	out := make(map[string]*model.OpenAPIMediaType)
	if c == nil || c.MediaType == "" {
		return out
	}
	out[c.MediaType] = &model.OpenAPIMediaType{Schema: apiDocToSchema(c.Schema)}
	return out
}

// slice of ApiDocContent → map keyed by media type
func apiDocContentsToMap(contents []*model.ApiDocContent) map[string]*model.OpenAPIMediaType {
	out := make(map[string]*model.OpenAPIMediaType)
	for _, c := range contents {
		if c == nil || c.MediaType == "" {
			continue
		}
		out[c.MediaType] = &model.OpenAPIMediaType{Schema: apiDocToSchema(c.Schema)}
	}
	return out
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func validateOpenAPI(ctx context.Context, docJson []byte) error {
	loader := openapi3.NewLoader()
	loader.IncludeOrigin = true

	doc, err := loader.LoadFromData(docJson)
	if err != nil {
		return err
	}
	if err := doc.Validate(ctx, openapi3.EnableMultiError()); err != nil {
		return err
	}
	return nil
}

func validateApiDocKeyField(doc *model.ApiDoc) []string {
	if doc == nil {
		return nil
	}

	var issues []string
	type pathMethod struct {
		path   string
		method string
	}
	seenPathMethod := make(map[pathMethod]int)

	for i, p := range doc.Paths {
		if p == nil {
			continue
		}
		pathWhere := fmt.Sprintf("paths[%d]", i)

		if p.Path == "" {
			issues = append(issues, pathWhere+".path:")
			continue
		}
		if p.Method == "" {
			issues = append(issues, pathWhere+".method:can not be empty")
			continue
		}

		key := pathMethod{path: p.Path, method: strings.ToLower(p.Method)}
		if first, exists := seenPathMethod[key]; exists {
			issues = append(issues, fmt.Sprintf(
				"%s.method:duplicate path+method (first defined at paths[%d])", pathWhere, first))
		} else {
			seenPathMethod[key] = i
		}

		if p.RequestBody != nil && p.RequestBody.Content != nil {
			if p.RequestBody.Content.MediaType == "" {
				issues = append(issues, pathWhere+".requestBody.content.mediaType:can not be empty")
			}
		}

		issues = append(issues, vaildateApiDocResponseKeyField(pathWhere, p.Responses)...)
	}

	return issues
}

func vaildateApiDocResponseKeyField(pathWhere string, responses []*model.ApiDocResponse) []string {
	var issues []string
	seenStatus := make(map[string]int)

	for j, r := range responses {
		if r == nil {
			continue
		}
		responseWhere := fmt.Sprintf("%s.responses[%d]", pathWhere, j)

		if r.Status == "" {
			issues = append(issues, responseWhere+".status:can not be empty")
			continue
		}
		if first, exists := seenStatus[r.Status]; exists {
			issues = append(issues, fmt.Sprintf(
				"%s.status:duplicate status %q (first defined at responses[%d])",
				responseWhere, r.Status, first))
		} else {
			seenStatus[r.Status] = j
		}

		issues = append(issues, validateApiDocCotentKeyField(responseWhere, r.Contents)...)
	}

	return issues
}

func validateApiDocCotentKeyField(responseWhere string, contents []*model.ApiDocContent) []string {
	var issues []string
	seenMediaType := make(map[string]int)

	for k, c := range contents {
		if c == nil {
			continue
		}
		contentWhere := fmt.Sprintf("%s.contents[%d]", responseWhere, k)

		if c.MediaType == "" {
			issues = append(issues, contentWhere+".mediaType:can not be empty")
			continue
		}
		if first, exists := seenMediaType[c.MediaType]; exists {
			issues = append(issues, fmt.Sprintf(
				"%s.mediaType:duplicate media type %q (first defined at contents[%d])",
				contentWhere, c.MediaType, first))
		} else {
			seenMediaType[c.MediaType] = k
		}
	}

	return issues
}

func buildApiDocErrorDetail(parts []string) string {
	wrapped := make([]string, len(parts))
	for i, part := range parts {
		wrapped[i] = "[" + part + "]"
	}
	return fmt.Sprintf("Body validation failed (%d error(s)):%s",
		len(parts), strings.Join(wrapped, ","))
}

func parserKinErrorMessages(err error) []string {
	if err == nil {
		return nil
	}
	if multi, ok := err.(openapi3.MultiError); ok {
		out := make([]string, 0, len(multi))
		for _, e := range multi {
			out = append(out, kinMessage(e))
		}
		return out
	}
	return []string{kinMessage(err)}
}

func kinMessage(err error) string {
	return kinWhere(err) + ": " + kinWhat(err)
}

func kinWhere(err error) string {
	var parts []string
	if e := of[*openapi3.SectionValidationError](err); e != nil {
		parts = append(parts, e.Section)
	}
	if e := of[*openapi3.PathValidationError](err); e != nil {
		parts = append(parts, e.Path)
	}
	if e := of[*openapi3.OperationValidationError](err); e != nil {
		parts = append(parts, e.Method)
	}
	if e := of[*openapi3.ParameterFieldValidationError](err); e != nil {
		parts = append(parts, "parameters["+e.ParameterName+"]."+e.Field)
	}
	if e := of[*openapi3.RequiredFieldError](err); e != nil {
		parts = append(parts, e.Field)
	}
	if se := of[*openapi3.SchemaError](err); se != nil {
		if ptr := se.JSONPointer(); len(ptr) > 0 {
			parts = append(parts, "/"+strings.Join(ptr, "/"))
		}
	}
	if len(parts) == 0 {
		return "<root>"
	}
	return strings.Join(parts, ".")
}

func kinWhat(err error) string {
	if ve := of[*openapi3.ValidationError](err); ve != nil && ve.Message != "" {
		return ve.Message
	}
	if se := of[*openapi3.SchemaError](err); se != nil && se.Reason != "" {
		return se.Reason
	}
	return err.Error()
}

func of[T any](err error) T { var t T; _ = errors.As(err, &t); return t }

func (h *ApiDocHandler) autoFillApiDocCreate(doc *model.ApiDocCreate) {
	doc.OpenAPI = "3.0.4"

	host := h.apiHost + "/gateway/api/resources"
	server := &model.ApiDocServer{
		URL: host,
	}
	if doc.Servers == nil {
		doc.Servers = make([]*model.ApiDocServer, 0)
		doc.Servers = append(doc.Servers, server)
	} else {
		for _, s := range doc.Servers {
			if s.URL == host {
				return
			}
		}
		doc.Servers = append(doc.Servers, server)
	}
}

func (h *ApiDocHandler) autoFillApiDocUpdate(doc *model.ApiDocUpdate) {
	doc.OpenAPI = "3.0.4"

	host := h.apiHost + "/gateway/api/resources"
	server := &model.ApiDocServer{
		URL: host,
	}
	if doc.Servers == nil {
		doc.Servers = make([]*model.ApiDocServer, 0)
		doc.Servers = append(doc.Servers, server)
	} else {
		for _, s := range doc.Servers {
			if s.URL == host {
				return
			}
		}
		doc.Servers = append(doc.Servers, server)
	}
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- constants ---
const (
	etVerifyEmail  = "Verify Email"
	etUpdatedName  = "Updated Template"
	etErrDB        = "db error"
	etSubSvcError  = "service error ต้องได้ status 500 และ error code"
	etSubBadID     = "id ผิดรูปแบบ ต้องได้ status 400 และ error code"
	etSubForbidden = "ไม่มี user ใน context ต้องได้ status 403"
)

// --- mock ---
type mockEmailTemplateService struct {
	mock.Mock
}

func (m *mockEmailTemplateService) List(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.EmailTemplate), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockEmailTemplateService) GetByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.EmailTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockEmailTemplateService) Create(ctx context.Context, body *model.EmailTemplateCreate) (*model.EmailTemplate, error) {
	args := m.Called(ctx, body)
	if v := args.Get(0); v != nil {
		return v.(*model.EmailTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockEmailTemplateService) Update(ctx context.Context, id bson.ObjectID, body *model.EmailTemplateUpdate) (*model.EmailTemplate, error) {
	args := m.Called(ctx, id, body)
	if v := args.Get(0); v != nil {
		return v.(*model.EmailTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockEmailTemplateService) Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, forever).Error(0)
}

const emailTemplateBasePath = "/control-plane/api/v1/email-templates"

// --- helpers ---
func newEmailTemplateListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, emailTemplateBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newEmailTemplateListCtxWithQuery(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, emailTemplateBasePath+"?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newEmailTemplateGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, emailTemplateBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("emailTemplateId")
	c.SetParamValues(id)
	return c, rec
}

func newEmailTemplatePostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, emailTemplateBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newEmailTemplatePutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, emailTemplateBasePath+"/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("emailTemplateId")
	c.SetParamValues(id)
	return c, rec
}

func newEmailTemplateDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := emailTemplateBasePath + "/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("emailTemplateId")
	c.SetParamValues(id)
	return c, rec
}

// --- GetEmailTemplates ---
func TestGetEmailTemplates(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run(etSubForbidden, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplateListCtx(e)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("มีข้อมูล ต้องได้ status 200 และ list response", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		list := []*model.EmailTemplate{{Name: etVerifyEmail}, {Name: "Reset Password"}}
		svc.On("List", mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newEmailTemplateListCtx(e)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.EmailTemplate]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
		assert.Len(t, resp.Items, 2)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newEmailTemplateListCtx(e)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.EmailTemplate]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotNil(t, resp.Items)
		assert.Equal(t, 0, resp.NumberMatched)
	})

	t.Run(etSubSvcError, func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), errors.New(etErrDB))

		c, rec := newEmailTemplateListCtx(e)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})

	t.Run("limit=0 คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "limit=0")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("limit ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "limit=-1")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit เกิน 10000 คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "limit=10001")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "limit=abc")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("offset ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "offset=-1")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("offset ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		c, rec := newEmailTemplateListCtxWithQuery(e, "offset=abc")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplates(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- GetEmailTemplate ---

func TestGetEmailTemplate(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run(etSubForbidden, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplateGetCtx(e, validID.Hex())
		err := NewEmailTemplateHandler(svc).GetEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("พบข้อมูล ต้องได้ status 200 และ template", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		tmpl := &model.EmailTemplate{ID: validID, Name: etVerifyEmail}
		svc.On("GetByID", mock.Anything, validID).Return(tmpl, nil)

		c, rec := newEmailTemplateGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.EmailTemplate
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, etVerifyEmail, resp.Name)
	})

	t.Run(etSubBadID, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplateGetCtx(e, "invalid-id")
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ไม่พบ template ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, errors.New("not found"))

		c, rec := newEmailTemplateGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).GetEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailTemplateNotFound, resp.Code)
	})
}

// --- CreateEmailTemplate ---

func TestCreateEmailTemplate(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run(etSubForbidden, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplatePostCtx(e, map[string]any{"name": "Welcome"})
		err := NewEmailTemplateHandler(svc).CreateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ status 201 และ template", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		tmpl := &model.EmailTemplate{Name: "Welcome"}
		svc.On("Create", mock.Anything, mock.Anything).Return(tmpl, nil)

		c, rec := newEmailTemplatePostCtx(e, map[string]any{"name": "Welcome", "alias": "welcome"})
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).CreateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.EmailTemplate
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Welcome", resp.Name)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", rootUser)

		err := NewEmailTemplateHandler(svc).CreateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run(etSubSvcError, func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New(etErrDB))

		c, rec := newEmailTemplatePostCtx(e, map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).CreateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- UpdateEmailTemplate ---

func TestUpdateEmailTemplate(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run(etSubForbidden, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplatePutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		err := NewEmailTemplateHandler(svc).UpdateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("อัปเดตสำเร็จ ต้องได้ status 200 และ template ที่อัปเดตแล้ว", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		tmpl := &model.EmailTemplate{ID: validID, Name: etUpdatedName}
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(tmpl, nil)

		c, rec := newEmailTemplatePutCtx(e, validID.Hex(), map[string]any{"name": etUpdatedName})
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).UpdateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.EmailTemplate
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, etUpdatedName, resp.Name)
	})

	t.Run(etSubBadID, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplatePutCtx(e, "bad-id", map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).UpdateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run(etSubSvcError, func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(nil, errors.New(etErrDB))

		c, rec := newEmailTemplatePutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).UpdateEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- DeleteEmailTemplate ---
func TestDeleteEmailTemplate(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run(etSubForbidden, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplateDeleteCtx(e, validID.Hex(), false)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ลบสำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Delete", mock.Anything, validID, false).Return(nil)

		c, rec := newEmailTemplateDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ลบแบบ hard delete (forever=true) ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Delete", mock.Anything, validID, true).Return(nil)

		c, rec := newEmailTemplateDeleteCtx(e, validID.Hex(), true)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run(etSubBadID, func(t *testing.T) {
		svc := new(mockEmailTemplateService)

		c, rec := newEmailTemplateDeleteCtx(e, "bad-id", false)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ลบ system template ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Delete", mock.Anything, validID, false).Return(service.ErrDeleteSystemEmailTemplate)

		c, rec := newEmailTemplateDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailTemplateCannotDelete, resp.Code)
	})

	t.Run("ไม่พบ template ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockEmailTemplateService)
		svc.On("Delete", mock.Anything, validID, false).Return(errors.New("not found"))

		c, rec := newEmailTemplateDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := NewEmailTemplateHandler(svc).DeleteEmailTemplate(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeEmailTemplateNotFound, resp.Code)
	})
}

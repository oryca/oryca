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

// --- mock ---
type mockMailServerService struct {
	mock.Mock
}

func (m *mockMailServerService) List(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.MailServer), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockMailServerService) GetByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerService) Create(ctx context.Context, body *model.MailServerCreate) (*model.MailServer, error) {
	args := m.Called(ctx, body)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerService) Update(ctx context.Context, id bson.ObjectID, body *model.MailServerUpdate) (*model.MailServer, error) {
	args := m.Called(ctx, id, body)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerService) Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, forever).Error(0)
}

const (
	mailServerBasePath = "/control-plane/api/v1/mail-servers"

	msNameSmtp1    = "smtp-1"
	msNameSmtpNew  = "smtp-new"
	msNameUpdated  = "smtp-updated"
	msErrDB        = "db error"
	msSubBadID     = "id ผิดรูปแบบ ต้องได้ status 400 และ error code"
	msSubSvcError  = "service error ต้องได้ status 422 และ error code"
	msSubForbidden = "ไม่มี user ใน context ต้องได้ status 403"
)

// --- helpers ---
func newMailServerHandler(svc mailServerService) *MailServerHandler {
	return NewMailServerHandler(svc)
}

func newMailServerListCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, mailServerBasePath, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newMailServerListCtxWithQuery(e *echo.Echo, query string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, mailServerBasePath+"?"+query, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newMailServerGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, mailServerBasePath+"/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("mailServerId")
	c.SetParamValues(id)
	return c, rec
}

func newMailServerPostCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, mailServerBasePath, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newMailServerPutCtx(e *echo.Echo, id string, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/control-plane/api/v1/mail-servers/"+id, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("mailServerId")
	c.SetParamValues(id)
	return c, rec
}

func newMailServerDeleteCtx(e *echo.Echo, id string, forever bool) (echo.Context, *httptest.ResponseRecorder) {
	target := "/control-plane/api/v1/mail-servers/" + id
	if forever {
		target += "?forever=true"
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("mailServerId")
	c.SetParamValues(id)
	return c, rec
}

// --- GetMailServers ---

func TestGetMailServers(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run(msSubForbidden, func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerListCtx(e)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("มีข้อมูล ต้องได้ status 200 และ list response", func(t *testing.T) {
		svc := new(mockMailServerService)
		list := []*model.MailServer{{Name: msNameSmtp1}, {Name: "smtp-2"}}
		svc.On("List", mock.Anything, mock.Anything).Return(list, int64(2), nil)

		c, rec := newMailServerListCtx(e)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.MailServer]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 2, resp.NumberMatched)
		assert.Equal(t, 2, resp.NumberReturned)
		assert.Len(t, resp.Items, 2)
	})

	t.Run("ไม่มีข้อมูล ต้องได้ items เป็น array ว่าง", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), nil)

		c, rec := newMailServerListCtx(e)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ListResponse[*model.MailServer]
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, 0, resp.NumberMatched)
		assert.NotNil(t, resp.Items)
	})

	t.Run(msSubSvcError, func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("List", mock.Anything, mock.Anything).Return(nil, int64(0), errors.New(msErrDB))

		c, rec := newMailServerListCtx(e)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})

	t.Run("limit=0 คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "limit=0")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("limit ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "limit=-1")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit เกิน 10000 คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "limit=10001")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("limit ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "limit=abc")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("offset ติดลบ คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "offset=-1")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var ex model.Exception
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ex))
		assert.Equal(t, tool.CodeQueryParamInvalidFormat, ex.Code)
	})

	t.Run("offset ไม่ใช่ตัวเลข คืน 400", func(t *testing.T) {
		svc := new(mockMailServerService)
		c, rec := newMailServerListCtxWithQuery(e, "offset=abc")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServers(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- GetMailServer ---
func TestGetMailServer(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run(msSubForbidden, func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerGetCtx(e, validID.Hex())
		err := newMailServerHandler(svc).GetMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("พบข้อมูล ต้องได้ status 200 และ mail server", func(t *testing.T) {
		svc := new(mockMailServerService)
		ms := &model.MailServer{ID: validID, Name: msNameSmtp1}
		svc.On("GetByID", mock.Anything, validID).Return(ms, nil)

		c, rec := newMailServerGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.MailServer
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, msNameSmtp1, resp.Name)
	})

	t.Run(msSubBadID, func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerGetCtx(e, "invalid-id")
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ไม่พบ mail server ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("GetByID", mock.Anything, validID).Return(nil, errors.New("not found"))

		c, rec := newMailServerGetCtx(e, validID.Hex())
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).GetMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeMailServerNotFound, resp.Code)
	})
}

// --- CreateMailServer ---
func TestCreateMailServer(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run("ไม่มี user ใน context ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerPostCtx(e, map[string]any{"name": msNameSmtpNew})
		err := newMailServerHandler(svc).CreateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("สร้างสำเร็จ ต้องได้ status 201 และ mail server", func(t *testing.T) {
		svc := new(mockMailServerService)
		ms := &model.MailServer{Name: msNameSmtpNew}
		svc.On("Create", mock.Anything, mock.Anything).Return(ms, nil)

		c, rec := newMailServerPostCtx(e, map[string]any{"name": msNameSmtpNew})
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).CreateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp model.MailServer
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, msNameSmtpNew, resp.Name)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockMailServerService)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", rootUser)

		err := newMailServerHandler(svc).CreateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run(msSubSvcError, func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New(msErrDB))

		c, rec := newMailServerPostCtx(e, map[string]any{"name": "smtp-x"})
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).CreateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- UpdateMailServer ---
func TestUpdateMailServer(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run("อัปเดตสำเร็จ ต้องได้ status 200 และ mail server ที่อัปเดตแล้ว", func(t *testing.T) {
		svc := new(mockMailServerService)
		ms := &model.MailServer{ID: validID, Name: msNameUpdated}
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(ms, nil)

		c, rec := newMailServerPutCtx(e, validID.Hex(), map[string]any{"name": msNameUpdated})
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).UpdateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.MailServer
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, msNameUpdated, resp.Name)
	})

	t.Run(msSubBadID, func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerPutCtx(e, "bad-id", map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).UpdateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run(msSubSvcError, func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Update", mock.Anything, validID, mock.Anything).Return(nil, errors.New(msErrDB))

		c, rec := newMailServerPutCtx(e, validID.Hex(), map[string]any{"name": "x"})
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).UpdateMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

// --- DeleteMailServer ---
func TestDeleteMailServer(t *testing.T) {
	e := echo.New()
	validID := bson.NewObjectID()
	rootUser := &model.User{Role: "root"}

	t.Run("ลบสำเร็จ ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Delete", mock.Anything, validID, false).Return(nil)

		c, rec := newMailServerDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).DeleteMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("ลบแบบ hard delete (forever=true) ต้องได้ status 204", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Delete", mock.Anything, validID, true).Return(nil)

		c, rec := newMailServerDeleteCtx(e, validID.Hex(), true)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).DeleteMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run(msSubBadID, func(t *testing.T) {
		svc := new(mockMailServerService)

		c, rec := newMailServerDeleteCtx(e, "bad-id", false)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).DeleteMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeParamInvalidFormat, resp.Code)
	})

	t.Run("ลบ default mail server ต้องได้ status 409 และ error code", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Delete", mock.Anything, validID, false).Return(service.ErrDeleteDefaultMailServer)

		c, rec := newMailServerDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).DeleteMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeMailServerCannotDelete, resp.Code)
	})

	t.Run("ไม่พบ mail server ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockMailServerService)
		svc.On("Delete", mock.Anything, validID, false).Return(errors.New("not found"))

		c, rec := newMailServerDeleteCtx(e, validID.Hex(), false)
		c.Set("user", rootUser)
		err := newMailServerHandler(svc).DeleteMailServer(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeMailServerNotFound, resp.Code)
	})
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- mock ---
type mockConfigurationService struct {
	mock.Mock
}

func (m *mockConfigurationService) Get(ctx context.Context) (*model.Configuration, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.Configuration), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConfigurationService) Upsert(ctx context.Context, body *model.ConfigurationUpsert) (*model.Configuration, error) {
	args := m.Called(ctx, body)
	if v := args.Get(0); v != nil {
		return v.(*model.Configuration), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- helpers ---

func newConfigHandler(svc configurationService) *ConfigurationHandler {
	return NewConfigurationHandler(svc)
}

func newGetCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/control-plane/api/v1/configuration", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPatchCtx(e *echo.Echo, body any) (echo.Context, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/control-plane/api/v1/configuration", bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- GetConfiguration ---
func TestGetConfiguration(t *testing.T) {
	e := echo.New()

	t.Run("พบข้อมูล (ไม่ auth) ต้องได้ status 200 และ public configuration", func(t *testing.T) {
		svc := new(mockConfigurationService)
		svc.On("Get", mock.Anything).Return(&model.Configuration{Terms: "v1"}, nil)

		c, rec := newGetCtx(e)
		err := newConfigHandler(svc).GetConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.ConfigurationPublic
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "v1", resp.Terms)
	})

	t.Run("พบข้อมูล (root) ต้องได้ status 200 และ full configuration", func(t *testing.T) {
		svc := new(mockConfigurationService)
		svc.On("Get", mock.Anything).Return(&model.Configuration{Terms: "v1"}, nil)

		c, rec := newGetCtx(e)
		c.Set("user", &model.User{Role: "root"})
		err := newConfigHandler(svc).GetConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.Configuration
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "v1", resp.Terms)
	})

	t.Run("ไม่พบข้อมูล ต้องได้ status 404 และ error code", func(t *testing.T) {
		svc := new(mockConfigurationService)
		svc.On("Get", mock.Anything).Return(nil, errors.New("not found"))

		c, rec := newGetCtx(e)
		err := newConfigHandler(svc).GetConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeConfigNotFound, resp.Code)
	})
}

// --- UpdateConfiguration ---
func TestUpdateConfiguration(t *testing.T) {
	e := echo.New()
	rootUser := &model.User{Role: "root"}

	t.Run("บันทึกสำเร็จ ต้องได้ status 200 และ document จาก DB", func(t *testing.T) {
		saved := &model.Configuration{Terms: "updated"}
		svc := new(mockConfigurationService)
		svc.On("Upsert", mock.Anything, mock.Anything).Return(saved, nil)

		c, rec := newPatchCtx(e, map[string]string{"terms": "updated"})
		c.Set("user", rootUser)
		err := newConfigHandler(svc).UpdateConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp model.Configuration
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "updated", resp.Terms)
	})

	t.Run("ไม่มี user ใน context ต้องได้ status 403", func(t *testing.T) {
		svc := new(mockConfigurationService)

		c, rec := newPatchCtx(e, map[string]string{"terms": "x"})
		err := newConfigHandler(svc).UpdateConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeUnauthorizedAccess, resp.Code)
	})

	t.Run("body ผิดรูปแบบ ต้องได้ status 400 และ error code", func(t *testing.T) {
		svc := new(mockConfigurationService)

		req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader([]byte("not-json")))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user", rootUser)

		err := newConfigHandler(svc).UpdateConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeBodyInvalidFormat, resp.Code)
	})

	t.Run("service error ต้องได้ status 500 และ error code", func(t *testing.T) {
		svc := new(mockConfigurationService)
		svc.On("Upsert", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

		c, rec := newPatchCtx(e, map[string]string{"terms": "x"})
		c.Set("user", rootUser)
		err := newConfigHandler(svc).UpdateConfiguration(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

		var resp model.Exception
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, tool.CodeOperationFailed, resp.Code)
	})
}

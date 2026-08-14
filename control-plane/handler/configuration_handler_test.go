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

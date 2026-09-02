package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ---
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error {
	return m.err
}

func okPinger() *mockPinger  { return &mockPinger{} }
func errPinger() *mockPinger { return &mockPinger{err: errors.New("connection error")} }

// --- helper ---
func newHealthHandler(mongo, redis pinger) *HealthHandler {
	return &HealthHandler{mongo: mongo, redis: redis}
}

func newHealthCtx(e *echo.Echo) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// --- tests ---
func TestHealthCheck(t *testing.T) {
	e := echo.New()

	t.Run("ทุก dependency พร้อม ต้องได้ status 200", func(t *testing.T) {
		c, rec := newHealthCtx(e)
		err := newHealthHandler(okPinger(), okPinger()).Check(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("ทุก dependency พร้อม ต้องได้ response ถูกต้อง", func(t *testing.T) {
		c, rec := newHealthCtx(e)
		newHealthHandler(okPinger(), okPinger()).Check(c)

		var body model.HealthStatus
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, http.StatusOK, body.Database.Default)
		assert.Equal(t, http.StatusOK, body.Redis.Default)
	})

	t.Run("เรียก health check ต้องได้ Content-Type เป็น JSON", func(t *testing.T) {
		c, rec := newHealthCtx(e)
		newHealthHandler(okPinger(), okPinger()).Check(c)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("MongoDB error ต้องได้ status 503 และ database default เป็น 503", func(t *testing.T) {
		c, rec := newHealthCtx(e)
		newHealthHandler(errPinger(), okPinger()).Check(c)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		var body model.HealthStatus
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, http.StatusServiceUnavailable, body.Database.Default)
		assert.Equal(t, http.StatusOK, body.Redis.Default)
	})

	t.Run("Redis error ต้องได้ status 503 และ redis default เป็น 503", func(t *testing.T) {
		c, rec := newHealthCtx(e)
		newHealthHandler(okPinger(), errPinger()).Check(c)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		var body model.HealthStatus
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, http.StatusOK, body.Database.Default)
		assert.Equal(t, http.StatusServiceUnavailable, body.Redis.Default)
	})
}

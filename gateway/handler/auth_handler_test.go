package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/oryca/oryca/gateway/model"
)

func newTestContext() echo.Context {
	e := echo.New()
	rec := httptest.NewRecorder()
	return e.NewContext(httptest.NewRequest("GET", "/", nil), rec)
}

func testEnabledUser(packageID string) *model.User {
	enabled, verified := true, true
	return &model.User{ID: "user-1", PackageID: packageID, Enabled: &enabled, Verified: &verified}
}

// TestCheckUserStatus_PackageAccess covers the fail-closed policy: a service must be
// linked to at least one package to be callable. An empty PackageIDs list denies
// everyone rather than allowing everyone. Regression guard for the PACKAGE_ACCESS_DENIED
// bypass found in production (services with zero package links were reachable by any
// authenticated user regardless of their actual package).
func TestCheckUserStatus_PackageAccess(t *testing.T) {
	h := &Handler{}

	t.Run("empty PackageIDs denies every package, including no package at all", func(t *testing.T) {
		svc := &model.GatewayService{ID: "svc-1", PackageIDs: nil}
		err := h.checkUserStatus(newTestContext(), testEnabledUser("pkg-a"), svc)
		assert.Equal(t, errAuthWritten, err)
	})

	t.Run("user's package not in the list is denied", func(t *testing.T) {
		svc := &model.GatewayService{ID: "svc-1", PackageIDs: []string{"pkg-a", "pkg-b"}}
		err := h.checkUserStatus(newTestContext(), testEnabledUser("pkg-c"), svc)
		assert.Equal(t, errAuthWritten, err)
	})

	t.Run("user's package in the list is allowed", func(t *testing.T) {
		svc := &model.GatewayService{ID: "svc-1", PackageIDs: []string{"pkg-a", "pkg-b"}}
		err := h.checkUserStatus(newTestContext(), testEnabledUser("pkg-b"), svc)
		assert.NoError(t, err)
	})
}

func TestCheckUserStatus_AccountState(t *testing.T) {
	h := &Handler{}
	svc := &model.GatewayService{ID: "svc-1", PackageIDs: []string{"pkg-a"}}

	t.Run("disabled user is denied", func(t *testing.T) {
		disabled := false
		user := testEnabledUser("pkg-a")
		user.Enabled = &disabled
		err := h.checkUserStatus(newTestContext(), user, svc)
		assert.Equal(t, errAuthWritten, err)
	})

	t.Run("unverified user is denied", func(t *testing.T) {
		unverified := false
		user := testEnabledUser("pkg-a")
		user.Verified = &unverified
		err := h.checkUserStatus(newTestContext(), user, svc)
		assert.Equal(t, errAuthWritten, err)
	})

	t.Run("expired user is denied", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		user := testEnabledUser("pkg-a")
		user.ExpiredAt = &past
		err := h.checkUserStatus(newTestContext(), user, svc)
		assert.Equal(t, errAuthWritten, err)
	})

	t.Run("healthy user in the right package passes", func(t *testing.T) {
		err := h.checkUserStatus(newTestContext(), testEnabledUser("pkg-a"), svc)
		assert.NoError(t, err)
	})
}

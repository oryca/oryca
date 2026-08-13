package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func ptr[T any](v T) *T { return &v }

// ---- GetUser tests ----

type mockInternalUserRepo struct {
	mock.Mock
}

func (m *mockInternalUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockInternalUserRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.User, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func newInternalUserGetCtx(e *echo.Echo, id string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/control-plane/api/v1/internal/users/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestInternalHandlerGetUser(t *testing.T) {
	e := echo.New()

	t.Run("พบ user คืน 200 พร้อม freshness snapshot", func(t *testing.T) {
		userRepo := new(mockInternalUserRepo)
		id := bson.NewObjectID()
		pid := bson.NewObjectID()
		verified := true
		userRepo.On("FindByID", mock.Anything, id).Return(&model.User{ID: id, PackageID: &pid, Verified: &verified}, nil)

		h := NewInternalHandler(InternalHandlerConfig{UserRepo: userRepo})
		c, rec := newInternalUserGetCtx(e, id.Hex())

		require.NoError(t, h.GetUser(c))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got model.UserFreshnessCache
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, id.Hex(), got.ID)
		assert.Equal(t, pid.Hex(), got.PackageID)
		assert.True(t, *got.Verified)
	})

	t.Run("id ผิดรูปแบบ คืน 400", func(t *testing.T) {
		userRepo := new(mockInternalUserRepo)
		h := NewInternalHandler(InternalHandlerConfig{UserRepo: userRepo})
		c, rec := newInternalUserGetCtx(e, "not-a-valid-id")

		require.NoError(t, h.GetUser(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		userRepo.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
	})

	t.Run("ไม่พบ user คืน 404", func(t *testing.T) {
		userRepo := new(mockInternalUserRepo)
		id := bson.NewObjectID()
		userRepo.On("FindByID", mock.Anything, id).Return(nil, assert.AnError)

		h := NewInternalHandler(InternalHandlerConfig{UserRepo: userRepo})
		c, rec := newInternalUserGetCtx(e, id.Hex())

		require.NoError(t, h.GetUser(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

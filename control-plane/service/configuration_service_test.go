package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

// --- mock repo ---

type mockConfigRepo struct {
	mock.Mock
}

func (m *mockConfigRepo) Get(ctx context.Context) (*model.Configuration, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.Configuration), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConfigRepo) Upsert(ctx context.Context, doc *model.Configuration) error {
	return m.Called(ctx, doc).Error(0)
}

// --- mock cache ---

type mockConfigCache struct {
	mock.Mock
	sf singleflight.Group
}

func (m *mockConfigCache) Get(ctx context.Context) (*model.Configuration, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.Configuration), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConfigCache) Set(ctx context.Context, cfg *model.Configuration) error {
	return m.Called(ctx, cfg).Error(0)
}

func (m *mockConfigCache) SF() *singleflight.Group {
	return &m.sf
}

// --- Get ---

func TestConfigurationService_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit ต้องไม่ยิง DB", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)
		cached := &model.Configuration{Terms: "cached"}

		cache.On("Get", ctx).Return(cached, nil)
		// repo.Get ต้องไม่ถูกเรียก

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Get(ctx)

		require.NoError(t, err)
		assert.Equal(t, "cached", result.Terms)
		repo.AssertNotCalled(t, "Get", mock.Anything)
	})

	t.Run("cache miss ต้องยิง DB และ set cache", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)
		fromDB := &model.Configuration{Terms: "from_db"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("Get", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(nil)

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Get(ctx)

		require.NoError(t, err)
		assert.Equal(t, "from_db", result.Terms)
		cache.AssertCalled(t, "Set", ctx, fromDB)
	})

	t.Run("cache miss และ DB error ต้อง return error", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("Get", ctx).Return(nil, errors.New("db error"))

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Get(ctx)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("cache Set error ต้องไม่กระทบ result", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)
		fromDB := &model.Configuration{Terms: "ok"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("Get", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(errors.New("redis down"))

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Get(ctx)

		require.NoError(t, err)
		assert.Equal(t, "ok", result.Terms)
	})
}

// --- Upsert ---

func TestConfigurationService_Upsert(t *testing.T) {
	ctx := context.Background()
	body := &model.ConfigurationUpsert{Terms: "new terms"}

	t.Run("บันทึกสำเร็จ ต้องได้ document จาก DB และ set cache", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)
		fromDB := &model.Configuration{Terms: "new terms"}

		repo.On("Upsert", ctx, mock.Anything).Return(nil)
		repo.On("Get", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(nil)

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Upsert(ctx, body)

		require.NoError(t, err)
		assert.Equal(t, "new terms", result.Terms)
		cache.AssertCalled(t, "Set", ctx, fromDB)
	})

	t.Run("repo.Upsert error ต้อง return error", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)

		repo.On("Upsert", ctx, mock.Anything).Return(errors.New("write failed"))

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Upsert(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
		repo.AssertNotCalled(t, "Get", mock.Anything)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})

	t.Run("repo.Get หลัง upsert error ต้อง return error", func(t *testing.T) {
		repo := new(mockConfigRepo)
		cache := new(mockConfigCache)

		repo.On("Upsert", ctx, mock.Anything).Return(nil)
		repo.On("Get", ctx).Return(nil, errors.New("read failed"))

		svc := NewConfigurationService(repo, cache)
		result, err := svc.Upsert(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})
}

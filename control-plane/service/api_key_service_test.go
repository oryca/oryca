package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mock repo ---

type mockApiKeyRepo struct {
	mock.Mock
}

func (m *mockApiKeyRepo) FindAll(ctx context.Context, params url.Values, ownerIDs []bson.ObjectID, excludeOwnerIDs []bson.ObjectID) ([]*model.ApiKey, int64, error) {
	args := m.Called(ctx, params, ownerIDs, excludeOwnerIDs)
	if v := args.Get(0); v != nil {
		return v.([]*model.ApiKey), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockApiKeyRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.ApiKey, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyRepo) FindByKey(ctx context.Context, key string) (*model.ApiKey, error) {
	args := m.Called(ctx, key)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyRepo) Insert(ctx context.Context, doc *model.ApiKey) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockApiKeyRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockApiKeyRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}

func (m *mockApiKeyRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *mockApiKeyRepo) BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, ids, deletedAt, deletedBy).Error(0)
}

func (m *mockApiKeyRepo) BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error {
	return m.Called(ctx, ids).Error(0)
}

// --- mock cache ---

type mockApiKeyCache struct {
	mock.Mock
}

func (m *mockApiKeyCache) Get(ctx context.Context, keyValue string) (*model.ApiKey, error) {
	args := m.Called(ctx, keyValue)
	if v := args.Get(0); v != nil {
		return v.(*model.ApiKey), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockApiKeyCache) Set(ctx context.Context, ak *model.ApiKey, owner *model.User) error {
	return m.Called(ctx, ak, owner).Error(0)
}

func (m *mockApiKeyCache) Delete(ctx context.Context, keyValues []string) error {
	return m.Called(ctx, keyValues).Error(0)
}

// --- mock userFinder ---

type mockApiKeyUserFinder struct{}

func (m *mockApiKeyUserFinder) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	return nil, nil
}

// --- mock notifier ---

type mockApiKeyNotifier struct{}

func (m *mockApiKeyNotifier) Create(ctx context.Context, in *model.NotificationCreate) error {
	return nil
}

// --- mock gateway event publisher ---

type mockGatewayPublisher struct {
	mock.Mock
}

func (m *mockGatewayPublisher) Publish(eventType string, payload any) {
	m.Called(eventType, payload)
}

// --- helpers ---

func newApiKeyService(repo *mockApiKeyRepo, cache *mockApiKeyCache) *ApiKeyService {
	return NewApiKeyService(repo, cache, new(mockApiKeyUserFinder), new(mockApiKeyNotifier), nil)
}

// --- List ---

func TestApiKeyServiceList(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	rootUser := &model.User{ID: bson.NewObjectID(), Role: "root"}
	adminUser := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	normalUser := &model.User{ID: ownerID, Role: "user"}

	t.Run("user ปกติดูเฉพาะ key ของตัวเอง", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		list := []*model.ApiKey{{ID: bson.NewObjectID(), Name: "my key"}}
		repo.On("FindAll", ctx, mock.Anything, []bson.ObjectID{ownerID}, ([]bson.ObjectID)(nil)).Return(list, int64(1), nil)

		svc := newApiKeyService(repo, cache)
		result, total, err := svc.List(ctx, url.Values{}, normalUser)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("root ไม่ระบุ ownerBy ดูทุก key", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		list := []*model.ApiKey{{Name: "key1"}, {Name: "key2"}}
		repo.On("FindAll", ctx, mock.Anything, ([]bson.ObjectID)(nil), ([]bson.ObjectID)(nil)).Return(list, int64(2), nil)

		svc := newApiKeyService(repo, cache)
		result, total, err := svc.List(ctx, url.Values{}, rootUser)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), total)
	})

	t.Run("admin ไม่ระบุ ownerBy ดูทุก key", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindAll", ctx, mock.Anything, ([]bson.ObjectID)(nil), ([]bson.ObjectID)(nil)).Return([]*model.ApiKey{}, int64(0), nil)

		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, url.Values{}, adminUser)

		require.NoError(t, err)
	})

	t.Run("user ระบุ ownerBy ที่ไม่ใช่ id ตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		otherID := bson.NewObjectID()

		params := url.Values{"ownerBy": {otherID.Hex()}}
		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, params, normalUser)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("root ระบุ ownerBy=!id ยกเว้น key ของ id นั้น", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		excludeID := bson.NewObjectID()
		repo.On("FindAll", ctx, mock.Anything, ([]bson.ObjectID)(nil), []bson.ObjectID{excludeID}).Return([]*model.ApiKey{}, int64(0), nil)

		params := url.Values{"ownerBy": {"!" + excludeID.Hex()}}
		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, params, rootUser)

		require.NoError(t, err)
	})

	t.Run("root ระบุ userId เฉพาะดูได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		targetID := bson.NewObjectID()
		repo.On("FindAll", ctx, mock.Anything, []bson.ObjectID{targetID}, ([]bson.ObjectID)(nil)).Return([]*model.ApiKey{}, int64(0), nil)

		params := url.Values{"ownerBy": {targetID.Hex()}}
		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, params, rootUser)

		require.NoError(t, err)
	})

	t.Run("user ระบุ userId เฉพาะคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		targetID := bson.NewObjectID()

		params := url.Values{"ownerBy": {targetID.Hex()}}
		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, params, normalUser)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("repo error คืน error", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindAll", ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), errors.New("db error"))

		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, url.Values{}, normalUser)

		assert.Error(t, err)
	})

	t.Run("userId ไม่ถูกรูปแบบ hex คืน error", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)

		params := url.Values{"ownerBy": {"not-valid-hex"}}
		svc := newApiKeyService(repo, cache)
		_, _, err := svc.List(ctx, params, rootUser)

		assert.Error(t, err)
	})
}

// --- GetByID ---

func TestApiKeyServiceGetByID(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	otherID := bson.NewObjectID()
	keyID := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	rootUser := &model.User{ID: otherID, Role: "root"}
	normalOther := &model.User{ID: otherID, Role: "user"}
	ak := &model.ApiKey{ID: keyID, OwnerBy: &ownerID, CreatedBy: &ownerID}

	t.Run("เจ้าของดูได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.GetByID(ctx, keyID, owner)

		require.NoError(t, err)
		assert.Equal(t, keyID, result.ID)
	})

	t.Run("root ดู key ของคนอื่นได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.GetByID(ctx, keyID, rootUser)

		require.NoError(t, err)
		assert.Equal(t, keyID, result.ID)
	})

	t.Run("user อื่นดู key ไม่ใช่ของตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		_, err := svc.GetByID(ctx, keyID, normalOther)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("ไม่พบ key คืน ErrApiKeyNotFound", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(nil, errors.New("not found"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.GetByID(ctx, keyID, owner)

		assert.ErrorIs(t, err, ErrApiKeyNotFound)
	})
}

// --- GetByKey (cache) ---

func TestApiKeyServiceGetByKey(t *testing.T) {
	ctx := context.Background()
	keyValue := "abc123"
	ak := &model.ApiKey{ApiKey: keyValue}

	t.Run("cache hit คืน key ทันที", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		cache.On("Get", ctx, keyValue).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.GetByKey(ctx, keyValue)

		require.NoError(t, err)
		assert.Equal(t, keyValue, result.ApiKey)
		repo.AssertNotCalled(t, "FindByKey")
	})

	t.Run("cache miss ดึงจาก DB แล้ว set cache", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		cache.On("Get", ctx, keyValue).Return(nil, errors.New("miss"))
		repo.On("FindByKey", ctx, keyValue).Return(ak, nil)
		cache.On("Set", ctx, ak, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.GetByKey(ctx, keyValue)

		require.NoError(t, err)
		assert.Equal(t, keyValue, result.ApiKey)
	})

	t.Run("ไม่พบใน DB คืน ErrApiKeyNotFound", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		cache.On("Get", ctx, keyValue).Return(nil, errors.New("miss"))
		repo.On("FindByKey", ctx, keyValue).Return(nil, errors.New("not found"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.GetByKey(ctx, keyValue)

		assert.ErrorIs(t, err, ErrApiKeyNotFound)
	})
}

// --- Create ---

func TestApiKeyServiceCreate(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	adminID := bson.NewObjectID()
	otherID := bson.NewObjectID()
	rootID := bson.NewObjectID()

	ownerUser := &model.User{ID: ownerID, Role: "user"}
	adminUser := &model.User{ID: adminID, Role: "admin"}
	otherUser := &model.User{ID: otherID, Role: "user"}
	rootUser := &model.User{ID: rootID, Role: "root"}

	t.Run("user สร้าง key ของตัวเองสำเร็จ", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		cache.On("Set", ctx, mock.Anything, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		ak, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "my key", Description: "for testing"}, ownerUser, nil)

		require.NoError(t, err)
		assert.Equal(t, "my key", ak.Name)
		assert.Equal(t, "for testing", ak.Description)
		assert.Equal(t, ownerID, *ak.CreatedBy)
		assert.NotEmpty(t, ak.ApiKey)
		assert.Len(t, ak.ApiKey, 64)
	})

	t.Run("root สร้าง key ให้ root ได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		cache.On("Set", ctx, mock.Anything, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		targetRoot := &model.User{ID: bson.NewObjectID(), Role: "root"}
		ak, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "root key"}, rootUser, targetRoot)

		require.NoError(t, err)
		assert.NotNil(t, ak)
	})

	t.Run("admin สร้าง key ให้ admin คืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		targetAdmin := &model.User{ID: bson.NewObjectID(), Role: "admin"}

		svc := newApiKeyService(repo, cache)
		_, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "key"}, adminUser, targetAdmin)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("user สร้าง key ให้คนอื่นคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)

		svc := newApiKeyService(repo, cache)
		_, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "key"}, ownerUser, otherUser)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("enabled default เป็น true ถ้าไม่ส่งมา", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		cache.On("Set", ctx, mock.Anything, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		ak, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "key"}, ownerUser, nil)

		require.NoError(t, err)
		require.NotNil(t, ak.Enabled)
		assert.True(t, *ak.Enabled)
	})

	t.Run("repo Insert error คืน error", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("Insert", ctx, mock.Anything).Return(errors.New("insert failed"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.Create(ctx, &model.ApiKeyCreate{Name: "key"}, ownerUser, nil)

		assert.Error(t, err)
		assert.Equal(t, "insert failed", err.Error())
	})
}

// --- Update ---

func TestApiKeyServiceUpdate(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	otherID := bson.NewObjectID()
	keyID := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	other := &model.User{ID: otherID, Role: "user"}
	ak := &model.ApiKey{ID: keyID, ApiKey: "oldkey", OwnerBy: &ownerID, CreatedBy: &ownerID}

	t.Run("เจ้าของอัปเดตสำเร็จ", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		updated := &model.ApiKey{ID: keyID, Name: "updated", ApiKey: "oldkey", OwnerBy: &ownerID, CreatedBy: &ownerID}
		repo.On("FindByID", ctx, keyID).Return(ak, nil).Once()
		repo.On("Update", ctx, keyID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, []string{"oldkey"}).Return(nil)
		repo.On("FindByID", ctx, keyID).Return(updated, nil).Once()
		cache.On("Set", ctx, updated, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{Name: "updated"}, owner)

		require.NoError(t, err)
		assert.Equal(t, "updated", result.Name)
	})

	t.Run("ไม่พบ key คืน ErrApiKeyNotFound", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(nil, errors.New("not found"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{}, owner)

		assert.ErrorIs(t, err, ErrApiKeyNotFound)
	})

	t.Run("user อื่นอัปเดต key ไม่ใช่ของตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		_, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{}, other)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("repo Update error คืน error", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)
		repo.On("Update", ctx, keyID, mock.Anything).Return(errors.New("update failed"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{Name: "x"}, owner)

		assert.Error(t, err)
		assert.Equal(t, "update failed", err.Error())
	})

	t.Run("admin แก้ key ของ user อื่นได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		adminID := bson.NewObjectID()
		admin := &model.User{ID: adminID, Role: "admin"}
		userID := bson.NewObjectID()
		otherKey := &model.ApiKey{ID: keyID, ApiKey: "oldkey", OwnerBy: &userID, CreatedBy: &userID}
		updatedKey := &model.ApiKey{ID: keyID, Name: "updated", ApiKey: "oldkey", OwnerBy: &userID, CreatedBy: &userID}
		repo.On("FindByID", ctx, keyID).Return(otherKey, nil).Once()
		repo.On("Update", ctx, keyID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, []string{"oldkey"}).Return(nil)
		repo.On("FindByID", ctx, keyID).Return(updatedKey, nil).Once()
		cache.On("Set", ctx, updatedKey, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{Name: "updated"}, admin)

		require.NoError(t, err)
		assert.Equal(t, "updated", result.Name)
	})
}

func TestApiKeyServiceUpdate_PublishesGatewayEvent(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	keyID := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	ak := &model.ApiKey{ID: keyID, ApiKey: "oldkey", OwnerBy: &ownerID, CreatedBy: &ownerID}
	enabled := false
	updated := &model.ApiKey{ID: keyID, Name: "updated", ApiKey: "oldkey", Enabled: &enabled, OwnerBy: &ownerID, CreatedBy: &ownerID}

	repo := new(mockApiKeyRepo)
	cache := new(mockApiKeyCache)
	pub := new(mockGatewayPublisher)

	repo.On("FindByID", ctx, keyID).Return(ak, nil).Once()
	repo.On("Update", ctx, keyID, mock.Anything).Return(nil)
	cache.On("Delete", ctx, []string{"oldkey"}).Return(nil)
	repo.On("FindByID", ctx, keyID).Return(updated, nil).Once()
	cache.On("Set", ctx, updated, mock.Anything).Return(nil)
	// mockApiKeyUserFinder is a fixed stub that always returns (nil, nil). Owner ends
	// up nil here, so the payload matcher below only checks the ApiKey fields.
	pub.On("Publish", SyncEventTypeApiKey, mock.MatchedBy(func(p any) bool {
		payload, ok := p.(*model.ApiKeyCache)
		return ok && payload.ID == keyID.Hex() && payload.ApiKey == "oldkey" &&
			payload.Enabled != nil && !*payload.Enabled
	})).Return()

	svc := NewApiKeyService(repo, cache, new(mockApiKeyUserFinder), new(mockApiKeyNotifier), pub)
	_, err := svc.Update(ctx, keyID, &model.ApiKeyUpdate{Name: "updated", Enabled: &enabled}, owner)

	require.NoError(t, err)
	pub.AssertExpectations(t)
}

// --- Regenerate ---

func TestApiKeyServiceRegenerate(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	keyID := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	ak := &model.ApiKey{ID: keyID, ApiKey: "oldkey64chars000000000000000000000000000000000000000000000000000", OwnerBy: &ownerID, CreatedBy: &ownerID}

	t.Run("regenerate สำเร็จ key ใหม่ไม่ซ้ำกับเก่า", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil).Once()
		repo.On("Update", ctx, keyID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, []string{ak.ApiKey}).Return(nil)
		newAk := &model.ApiKey{ID: keyID, ApiKey: "newkey64chars000000000000000000000000000000000000000000000000000", OwnerBy: &ownerID, CreatedBy: &ownerID}
		repo.On("FindByID", ctx, keyID).Return(newAk, nil).Once()
		cache.On("Set", ctx, newAk, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		result, err := svc.Regenerate(ctx, keyID, owner)

		require.NoError(t, err)
		assert.NotEqual(t, ak.ApiKey, result.ApiKey)
	})

	t.Run("ไม่พบ key คืน ErrApiKeyNotFound", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(nil, errors.New("not found"))

		svc := newApiKeyService(repo, cache)
		_, err := svc.Regenerate(ctx, keyID, owner)

		assert.ErrorIs(t, err, ErrApiKeyNotFound)
	})

	t.Run("user อื่น regenerate key ไม่ใช่ของตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		otherUser := &model.User{ID: bson.NewObjectID(), Role: "user"}
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		_, err := svc.Regenerate(ctx, keyID, otherUser)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})
}

// --- Delete ---

func TestApiKeyServiceDelete(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	otherID := bson.NewObjectID()
	keyID := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	other := &model.User{ID: otherID, Role: "user"}
	ak := &model.ApiKey{ID: keyID, ApiKey: "somekey", OwnerBy: &ownerID, CreatedBy: &ownerID}

	t.Run("soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)
		cache.On("Delete", ctx, []string{"somekey"}).Return(nil)
		repo.On("SoftDelete", ctx, keyID, mock.Anything, ownerID).Return(nil)

		svc := newApiKeyService(repo, cache)
		err := svc.Delete(ctx, keyID, false, owner)

		require.NoError(t, err)
	})

	t.Run("hard delete สำเร็จ (?forever=true)", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)
		cache.On("Delete", ctx, []string{"somekey"}).Return(nil)
		repo.On("HardDelete", ctx, keyID).Return(nil)

		svc := newApiKeyService(repo, cache)
		err := svc.Delete(ctx, keyID, true, owner)

		require.NoError(t, err)
	})

	t.Run("ไม่พบ key คืน ErrApiKeyNotFound", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(nil, errors.New("not found"))

		svc := newApiKeyService(repo, cache)
		err := svc.Delete(ctx, keyID, false, owner)

		assert.ErrorIs(t, err, ErrApiKeyNotFound)
	})

	t.Run("user อื่นลบ key ไม่ใช่ของตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID).Return(ak, nil)

		svc := newApiKeyService(repo, cache)
		err := svc.Delete(ctx, keyID, false, other)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})

	t.Run("admin ลบ key ของ user อื่นได้", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		adminID := bson.NewObjectID()
		admin := &model.User{ID: adminID, Role: "admin"}
		userID := bson.NewObjectID()
		otherKey := &model.ApiKey{ID: keyID, ApiKey: "somekey", OwnerBy: &userID, CreatedBy: &userID}
		repo.On("FindByID", ctx, keyID).Return(otherKey, nil)
		cache.On("Delete", ctx, []string{"somekey"}).Return(nil)
		repo.On("SoftDelete", ctx, keyID, mock.Anything, adminID).Return(nil)

		svc := newApiKeyService(repo, cache)
		err := svc.Delete(ctx, keyID, false, admin)

		require.NoError(t, err)
	})
}

// --- BulkDelete ---

func TestApiKeyServiceBulkDelete(t *testing.T) {
	ctx := context.Background()
	ownerID := bson.NewObjectID()
	otherID := bson.NewObjectID()
	keyID1 := bson.NewObjectID()
	keyID2 := bson.NewObjectID()
	owner := &model.User{ID: ownerID, Role: "user"}
	other := &model.User{ID: otherID, Role: "user"}
	ak1 := &model.ApiKey{ID: keyID1, ApiKey: "key1", OwnerBy: &ownerID, CreatedBy: &ownerID}
	ak2 := &model.ApiKey{ID: keyID2, ApiKey: "key2", OwnerBy: &ownerID, CreatedBy: &ownerID}

	t.Run("bulk soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID1).Return(ak1, nil)
		repo.On("FindByID", ctx, keyID2).Return(ak2, nil)
		cache.On("Delete", ctx, mock.Anything).Return(nil)
		repo.On("BulkSoftDelete", ctx, mock.Anything, mock.Anything, ownerID).Return(nil)

		svc := newApiKeyService(repo, cache)
		err := svc.BulkDelete(ctx, &model.ApiKeyBulkDelete{
			Forever:   false,
			ApiKeyIDs: []bson.ObjectID{keyID1, keyID2},
		}, owner)

		require.NoError(t, err)
	})

	t.Run("bulk hard delete สำเร็จ", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID1).Return(ak1, nil)
		cache.On("Delete", ctx, mock.Anything).Return(nil)
		repo.On("BulkHardDelete", ctx, mock.Anything).Return(nil)

		svc := newApiKeyService(repo, cache)
		err := svc.BulkDelete(ctx, &model.ApiKeyBulkDelete{
			Forever:   true,
			ApiKeyIDs: []bson.ObjectID{keyID1},
		}, owner)

		require.NoError(t, err)
	})

	t.Run("มี key ที่ไม่ใช่ของตัวเองคืน ErrApiKeyForbidden", func(t *testing.T) {
		repo := new(mockApiKeyRepo)
		cache := new(mockApiKeyCache)
		repo.On("FindByID", ctx, keyID1).Return(ak1, nil)

		svc := newApiKeyService(repo, cache)
		err := svc.BulkDelete(ctx, &model.ApiKeyBulkDelete{
			Forever:   false,
			ApiKeyIDs: []bson.ObjectID{keyID1},
		}, other)

		assert.ErrorIs(t, err, ErrApiKeyForbidden)
	})
}

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
	"golang.org/x/sync/singleflight"
)

const mailSvcErrDB = "db error"

// --- mock repo ---

type mockMailServerRepo struct {
	mock.Mock
}

func (m *mockMailServerRepo) FindAll(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.MailServer), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockMailServerRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerRepo) FindDefault(ctx context.Context) (*model.MailServer, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerRepo) Insert(ctx context.Context, doc *model.MailServer) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockMailServerRepo) Update(ctx context.Context, id bson.ObjectID, doc *model.MailServer) error {
	return m.Called(ctx, id, doc).Error(0)
}

func (m *mockMailServerRepo) UnsetDefault(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockMailServerRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt).Error(0)
}

func (m *mockMailServerRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

// --- mock cache ---

type mockMailServerCache struct {
	mock.Mock
	sf singleflight.Group
}

func (m *mockMailServerCache) Get(ctx context.Context) (*model.MailServer, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.MailServer), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockMailServerCache) Set(ctx context.Context, ms *model.MailServer) error {
	return m.Called(ctx, ms).Error(0)
}

func (m *mockMailServerCache) Delete(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockMailServerCache) SF() *singleflight.Group {
	return &m.sf
}

// --- GetByID ---

func TestMailServerService_GetByID(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("พบ mail server คืน mail server", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		ms := &model.MailServer{ID: id, Name: "smtp-1"}
		repo.On("FindByID", ctx, id).Return(ms, nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, id, result.ID)
	})

	t.Run("ไม่พบ mail server คืน error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetByID(ctx, id)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- List ---

func TestMailServerService_List(t *testing.T) {
	ctx := context.Background()
	params := url.Values{}

	t.Run("มีข้อมูล ต้องได้ list จาก repo", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		list := []*model.MailServer{{Name: "smtp-1"}}
		repo.On("FindAll", ctx, params).Return(list, int64(1), nil)

		svc := NewMailServerService(repo, cache)
		result, total, err := svc.List(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("repo error ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		repo.On("FindAll", ctx, params).Return(nil, int64(0), errors.New(mailSvcErrDB))

		svc := NewMailServerService(repo, cache)
		result, _, err := svc.List(ctx, params)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetDefault ---

func TestMailServerService_GetDefault(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit ต้องไม่ยิง DB", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		cached := &model.MailServer{Name: "cached"}
		cache.On("Get", ctx).Return(cached, nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "cached", result.Name)
		repo.AssertNotCalled(t, "FindDefault", mock.Anything)
	})

	t.Run("cache miss ต้องยิง DB และ set cache", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		fromDB := &model.MailServer{Name: "from_db"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "from_db", result.Name)
		cache.AssertCalled(t, "Set", ctx, fromDB)
	})

	t.Run("cache miss และ DB error ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(nil, errors.New(mailSvcErrDB))

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetDefault(ctx)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("cache Set error ต้องไม่กระทบ result", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		fromDB := &model.MailServer{Name: "ok"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(errors.New("redis down"))

		svc := NewMailServerService(repo, cache)
		result, err := svc.GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "ok", result.Name)
	})
}

// --- Create ---

func TestMailServerService_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("สร้าง non-default สำเร็จ ต้องไม่แตะ unsetDefault และ cache", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerCreate{Name: "smtp-1", Default: false}
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.Create(ctx, body)

		require.NoError(t, err)
		assert.Equal(t, "smtp-1", result.Name)
		repo.AssertNotCalled(t, "UnsetDefault", mock.Anything)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})

	t.Run("สร้าง default ต้อง UnsetDefault และ set cache", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerCreate{Name: "smtp-default", Default: true}

		repo.On("UnsetDefault", ctx).Return(nil)
		cache.On("Delete", ctx).Return(nil)
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		cache.On("Set", ctx, mock.Anything).Return(nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.Create(ctx, body)

		require.NoError(t, err)
		assert.Equal(t, "smtp-default", result.Name)
		assert.True(t, result.Default)
		repo.AssertCalled(t, "UnsetDefault", ctx)
		cache.AssertCalled(t, "Set", ctx, mock.Anything)
	})

	t.Run("UnsetDefault error ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerCreate{Name: "smtp-x", Default: true}

		repo.On("UnsetDefault", ctx).Return(errors.New(mailSvcErrDB))

		svc := NewMailServerService(repo, cache)
		result, err := svc.Create(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
		repo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	})

	t.Run("Insert error ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerCreate{Name: "smtp-x", Default: false}

		repo.On("Insert", ctx, mock.Anything).Return(errors.New("write failed"))

		svc := NewMailServerService(repo, cache)
		result, err := svc.Create(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Update ---

func TestMailServerService_Update(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("อัปเดต non-default สำเร็จ ต้องได้ document ที่อัปเดตแล้ว", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerUpdate{Name: "updated", Default: false}
		updated := &model.MailServer{ID: id, Name: "updated"}

		cache.On("Delete", ctx).Return(nil)
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.Update(ctx, id, body)

		require.NoError(t, err)
		assert.Equal(t, "updated", result.Name)
		repo.AssertNotCalled(t, "UnsetDefault", mock.Anything)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})

	t.Run("อัปเดตเป็น default ต้อง UnsetDefault และ set cache", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerUpdate{Name: "new-default", Default: true}
		updated := &model.MailServer{ID: id, Name: "new-default", Default: true}

		repo.On("UnsetDefault", ctx).Return(nil)
		cache.On("Delete", ctx).Return(nil)
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)
		cache.On("Set", ctx, updated).Return(nil)

		svc := NewMailServerService(repo, cache)
		result, err := svc.Update(ctx, id, body)

		require.NoError(t, err)
		assert.True(t, result.Default)
		repo.AssertCalled(t, "UnsetDefault", ctx)
		cache.AssertCalled(t, "Set", ctx, updated)
	})

	t.Run("repo.Update error ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		body := &model.MailServerUpdate{Name: "x", Default: false}

		cache.On("Delete", ctx).Return(nil)
		repo.On("Update", ctx, id, mock.Anything).Return(errors.New("write failed"))

		svc := NewMailServerService(repo, cache)
		result, err := svc.Update(ctx, id, body)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Delete ---

func TestMailServerService_Delete(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("soft delete สำเร็จ ต้องไม่เรียก HardDelete", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		ms := &model.MailServer{ID: id, Name: "smtp-1", Default: false}

		repo.On("FindByID", ctx, id).Return(ms, nil)
		repo.On("SoftDelete", ctx, id, mock.AnythingOfType("time.Time")).Return(nil)

		svc := NewMailServerService(repo, cache)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		require.NoError(t, err)
		repo.AssertNotCalled(t, "HardDelete", mock.Anything, mock.Anything)
	})

	t.Run("hard delete (forever=true) ต้องเรียก HardDelete", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		ms := &model.MailServer{ID: id, Name: "smtp-1", Default: false}

		repo.On("FindByID", ctx, id).Return(ms, nil)
		repo.On("HardDelete", ctx, id).Return(nil)

		svc := NewMailServerService(repo, cache)
		err := svc.Delete(ctx, id, true, bson.NilObjectID)

		require.NoError(t, err)
		repo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("ลบ default mail server ต้อง return ErrDeleteDefaultMailServer", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)
		ms := &model.MailServer{ID: id, Name: "smtp-default", Default: true}

		repo.On("FindByID", ctx, id).Return(ms, nil)

		svc := NewMailServerService(repo, cache)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		assert.ErrorIs(t, err, ErrDeleteDefaultMailServer)
		repo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
		repo.AssertNotCalled(t, "HardDelete", mock.Anything, mock.Anything)
	})

	t.Run("ไม่พบ mail server ต้อง return error", func(t *testing.T) {
		repo := new(mockMailServerRepo)
		cache := new(mockMailServerCache)

		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := NewMailServerService(repo, cache)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		assert.Error(t, err)
	})
}

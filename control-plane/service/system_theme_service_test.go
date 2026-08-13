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

const themeSvcErrDB = "db error"

// --- mock repo ---

type mockSystemThemeRepo struct {
	mock.Mock
}

func (m *mockSystemThemeRepo) FindAll(ctx context.Context, params url.Values) ([]*model.SystemTheme, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.SystemTheme), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockSystemThemeRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.SystemTheme, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.SystemTheme), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSystemThemeRepo) FindDefault(ctx context.Context) (*model.SystemTheme, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.SystemTheme), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSystemThemeRepo) Insert(ctx context.Context, doc *model.SystemTheme) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockSystemThemeRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockSystemThemeRepo) UnsetDefault(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockSystemThemeRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}

// --- mock cache ---

type mockSystemThemeCache struct {
	mock.Mock
	sf singleflight.Group
}

func (m *mockSystemThemeCache) Get(ctx context.Context) (*model.SystemTheme, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.(*model.SystemTheme), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSystemThemeCache) Set(ctx context.Context, theme *model.SystemTheme) error {
	return m.Called(ctx, theme).Error(0)
}

func (m *mockSystemThemeCache) Delete(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockSystemThemeCache) SF() *singleflight.Group {
	return &m.sf
}

// --- helpers ---

func newThemeSvc(repo *mockSystemThemeRepo, cache *mockSystemThemeCache) *SystemThemeService {
	return NewSystemThemeService(repo, cache)
}

// --- List ---

func TestSystemThemeService_List(t *testing.T) {
	ctx := context.Background()
	params := url.Values{}

	t.Run("มีข้อมูล ต้องได้ list จาก repo", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		list := []*model.SystemTheme{{Name: "theme-1"}}
		repo.On("FindAll", ctx, params).Return(list, int64(1), nil)

		result, total, err := newThemeSvc(repo, cache).List(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("repo error ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		repo.On("FindAll", ctx, params).Return(nil, int64(0), errors.New(themeSvcErrDB))

		result, _, err := newThemeSvc(repo, cache).List(ctx, params)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetByID ---

func TestSystemThemeService_GetByID(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("พบ theme ต้อง return theme", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		theme := &model.SystemTheme{ID: id, Name: "dark"}
		repo.On("FindByID", ctx, id).Return(theme, nil)

		result, err := newThemeSvc(repo, cache).GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, id, result.ID)
	})

	t.Run("ไม่พบ theme ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		result, err := newThemeSvc(repo, cache).GetByID(ctx, id)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetDefault ---

func TestSystemThemeService_GetDefault(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit ต้องไม่ยิง DB", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		cached := &model.SystemTheme{Name: "cached"}
		cache.On("Get", ctx).Return(cached, nil)

		result, err := newThemeSvc(repo, cache).GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "cached", result.Name)
		repo.AssertNotCalled(t, "FindDefault", mock.Anything)
	})

	t.Run("cache miss ต้องยิง DB และ set cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		fromDB := &model.SystemTheme{Name: "from_db"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(nil)

		result, err := newThemeSvc(repo, cache).GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "from_db", result.Name)
		cache.AssertCalled(t, "Set", ctx, fromDB)
	})

	t.Run("cache miss และ DB error ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(nil, errors.New(themeSvcErrDB))

		result, err := newThemeSvc(repo, cache).GetDefault(ctx)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("cache Set error ต้องไม่กระทบ result", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		fromDB := &model.SystemTheme{Name: "ok"}

		cache.On("Get", ctx).Return(nil, errors.New("miss"))
		repo.On("FindDefault", ctx).Return(fromDB, nil)
		cache.On("Set", ctx, fromDB).Return(errors.New("redis down"))

		result, err := newThemeSvc(repo, cache).GetDefault(ctx)

		require.NoError(t, err)
		assert.Equal(t, "ok", result.Name)
	})
}

// --- Create ---

func TestSystemThemeService_Create(t *testing.T) {
	ctx := context.Background()
	createdBy := bson.NewObjectID()

	t.Run("สร้าง non-default สำเร็จ ต้องไม่ตรวจ FindDefault และไม่ set cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeCreate{Name: "light", Default: false}
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		result, err := newThemeSvc(repo, cache).Create(ctx, body, createdBy)

		require.NoError(t, err)
		assert.Equal(t, "light", result.Name)
		assert.Equal(t, createdBy, *result.CreatedBy)
		repo.AssertNotCalled(t, "FindDefault", mock.Anything)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})

	t.Run("สร้าง default เมื่อยังไม่มี default ต้อง Insert และ set cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeCreate{Name: "dark", Default: true}

		repo.On("FindDefault", ctx).Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		cache.On("Set", ctx, mock.Anything).Return(nil)

		result, err := newThemeSvc(repo, cache).Create(ctx, body, createdBy)

		require.NoError(t, err)
		assert.True(t, result.Default)
		cache.AssertCalled(t, "Set", ctx, mock.Anything)
	})

	t.Run("สร้าง default เมื่อมี default อยู่แล้ว ต้อง return ErrDefaultSystemThemeExists", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeCreate{Name: "another", Default: true}
		existing := &model.SystemTheme{Name: "current-default", Default: true}

		repo.On("FindDefault", ctx).Return(existing, nil)

		result, err := newThemeSvc(repo, cache).Create(ctx, body, createdBy)

		assert.ErrorIs(t, err, ErrDefaultSystemThemeExists)
		assert.Nil(t, result)
		repo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	})

	t.Run("Insert error ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeCreate{Name: "x", Default: false}

		repo.On("Insert", ctx, mock.Anything).Return(errors.New("write failed"))

		result, err := newThemeSvc(repo, cache).Create(ctx, body, createdBy)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Update ---

func TestSystemThemeService_Update(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()
	updatedBy := bson.NewObjectID()

	t.Run("อัปเดต non-default สำเร็จ ต้องไม่ตรวจ FindDefault และ delete cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeUpdate{Name: "updated", Default: false}
		updated := &model.SystemTheme{ID: id, Name: "updated", Default: false}

		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)
		cache.On("Delete", ctx).Return(nil)

		result, err := newThemeSvc(repo, cache).Update(ctx, id, body, updatedBy)

		require.NoError(t, err)
		assert.Equal(t, "updated", result.Name)
		repo.AssertNotCalled(t, "FindDefault", mock.Anything)
		cache.AssertCalled(t, "Delete", ctx)
		cache.AssertNotCalled(t, "Set", mock.Anything, mock.Anything)
	})

	t.Run("อัปเดตตัวเองให้เป็น default ได้ (ยกเว้นตัวเอง)", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeUpdate{Name: "dark", Default: true}
		// FindDefault คืน theme ตัวเดียวกัน (ID เดียวกัน) → ไม่ block
		existing := &model.SystemTheme{ID: id, Name: "dark", Default: true}
		updated := &model.SystemTheme{ID: id, Name: "dark", Default: true}

		repo.On("FindDefault", ctx).Return(existing, nil)
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)
		cache.On("Set", ctx, updated).Return(nil)

		result, err := newThemeSvc(repo, cache).Update(ctx, id, body, updatedBy)

		require.NoError(t, err)
		assert.True(t, result.Default)
	})

	t.Run("อัปเดต default เมื่อมี default อื่นอยู่แล้ว ต้อง return ErrDefaultSystemThemeExists", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		otherId := bson.NewObjectID()
		body := &model.SystemThemeUpdate{Name: "new", Default: true}
		existing := &model.SystemTheme{ID: otherId, Default: true} // คนละ ID

		repo.On("FindDefault", ctx).Return(existing, nil)

		result, err := newThemeSvc(repo, cache).Update(ctx, id, body, updatedBy)

		assert.ErrorIs(t, err, ErrDefaultSystemThemeExists)
		assert.Nil(t, result)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("repo.Update error ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeUpdate{Name: "x", Default: false}

		repo.On("Update", ctx, id, mock.Anything).Return(errors.New("write failed"))

		result, err := newThemeSvc(repo, cache).Update(ctx, id, body, updatedBy)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("FindByID หลัง update error ต้อง return error", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		body := &model.SystemThemeUpdate{Name: "x", Default: false}

		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(nil, errors.New(themeSvcErrDB))

		result, err := newThemeSvc(repo, cache).Update(ctx, id, body, updatedBy)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Delete ---

func TestSystemThemeService_Delete(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()
	deletedBy := bson.NewObjectID()

	t.Run("soft delete สำเร็จ ต้อง delete cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		theme := &model.SystemTheme{ID: id, Name: "light", Default: false}

		repo.On("FindByID", ctx, id).Return(theme, nil)
		repo.On("SoftDelete", ctx, id, mock.AnythingOfType("time.Time"), deletedBy).Return(nil)
		cache.On("Delete", ctx).Return(nil)

		err := newThemeSvc(repo, cache).Delete(ctx, id, deletedBy)

		require.NoError(t, err)
		cache.AssertCalled(t, "Delete", ctx)
	})

	t.Run("ลบ default theme ต้อง return ErrDeleteDefaultSystemTheme", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		theme := &model.SystemTheme{ID: id, Name: "dark", Default: true}

		repo.On("FindByID", ctx, id).Return(theme, nil)

		err := newThemeSvc(repo, cache).Delete(ctx, id, deletedBy)

		assert.ErrorIs(t, err, ErrDeleteDefaultSystemTheme)
		repo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		cache.AssertNotCalled(t, "Delete", mock.Anything)
	})

	t.Run("ไม่พบ theme ต้อง return ErrSystemThemeNotFound", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)

		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		err := newThemeSvc(repo, cache).Delete(ctx, id, deletedBy)

		assert.ErrorIs(t, err, ErrSystemThemeNotFound)
	})

	t.Run("SoftDelete error ต้อง return error และไม่ delete cache", func(t *testing.T) {
		repo := new(mockSystemThemeRepo)
		cache := new(mockSystemThemeCache)
		theme := &model.SystemTheme{ID: id, Default: false}

		repo.On("FindByID", ctx, id).Return(theme, nil)
		repo.On("SoftDelete", ctx, id, mock.AnythingOfType("time.Time"), deletedBy).Return(errors.New("write failed"))

		err := newThemeSvc(repo, cache).Delete(ctx, id, deletedBy)

		assert.Error(t, err)
		cache.AssertNotCalled(t, "Delete", mock.Anything)
	})
}

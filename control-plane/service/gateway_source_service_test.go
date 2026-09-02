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

// --- mocks ---

type mockGatewaySourceRepo struct{ mock.Mock }

func (m *mockGatewaySourceRepo) FindAll(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewaySource), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockGatewaySourceRepo) FindAllActive(ctx context.Context) ([]*model.GatewaySource, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewaySource, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceRepo) FindByAlias(ctx context.Context, alias string) (*model.GatewaySource, error) {
	args := m.Called(ctx, alias)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceRepo) FindByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error) {
	args := m.Called(ctx, idOrAlias)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewaySourceRepo) Insert(ctx context.Context, doc *model.GatewaySource) error {
	return m.Called(ctx, doc).Error(0)
}
func (m *mockGatewaySourceRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}
func (m *mockGatewaySourceRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}
func (m *mockGatewaySourceRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

type mockGatewaySourceInUseChecker struct{ mock.Mock }

func (m *mockGatewaySourceInUseChecker) CountBySourceAlias(ctx context.Context, alias string) (int64, error) {
	args := m.Called(ctx, alias)
	return args.Get(0).(int64), args.Error(1)
}

func newGatewaySourceSvc(repo *mockGatewaySourceRepo, checker *mockGatewaySourceInUseChecker) *GatewaySourceService {
	return NewGatewaySourceService(repo, checker, NewGatewayRedisSync(nil, 0, nil))
}

// --- Create ---

func TestGatewaySourceServiceCreate(t *testing.T) {
	ctx := context.Background()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}

	t.Run("สร้างสำเร็จ ต้องได้ doc พร้อม createdAt และ updatedAt", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByAlias", ctx, "my-alias").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := newGatewaySourceSvc(repo, checker)
		result, err := svc.Create(ctx, &model.GatewaySourceCreate{
			Alias: "my-alias",
			Name:  "My Source",
			Type:  "upstream",
		}, admin)

		require.NoError(t, err)
		assert.Equal(t, "my-alias", result.Alias)
		assert.Equal(t, admin.ID, *result.CreatedBy)
		assert.Equal(t, admin.ID, *result.UpdatedBy)
		assert.NotNil(t, result.CreatedAt)
		assert.NotNil(t, result.UpdatedAt)
	})

	t.Run("alias ซ้ำ คืน ErrGatewaySourceAliasDuplicate", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		existing := &model.GatewaySource{ID: bson.NewObjectID(), Alias: "my-alias"}
		repo.On("FindByAlias", ctx, "my-alias").Return(existing, nil)

		svc := newGatewaySourceSvc(repo, checker)
		_, err := svc.Create(ctx, &model.GatewaySourceCreate{Alias: "my-alias", Name: "x", Type: "upstream"}, admin)

		assert.ErrorIs(t, err, ErrGatewaySourceAliasDuplicate)
	})

	t.Run("repo Insert error คืน error", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByAlias", ctx, "my-alias").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(errors.New("insert failed"))

		svc := newGatewaySourceSvc(repo, checker)
		_, err := svc.Create(ctx, &model.GatewaySourceCreate{Alias: "my-alias", Name: "x", Type: "upstream"}, admin)

		assert.Error(t, err)
	})
}

// --- Update ---

func TestGatewaySourceServiceUpdate(t *testing.T) {
	ctx := context.Background()
	srcID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	existing := &model.GatewaySource{ID: srcID, Alias: "old-alias", Name: "Old", Type: "upstream"}

	t.Run("อัปเดตสำเร็จ alias เดิมไม่เช็ค duplicate", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		updated := &model.GatewaySource{ID: srcID, Alias: "old-alias", Name: "New Name"}
		repo.On("FindByIDOrAlias", ctx, "old-alias").Return(existing, nil)
		repo.On("Update", ctx, srcID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, srcID).Return(updated, nil)

		svc := newGatewaySourceSvc(repo, checker)
		result, err := svc.Update(ctx, "old-alias", &model.GatewaySourceUpdate{Alias: "old-alias", Name: "New Name", Type: "upstream"}, admin)

		require.NoError(t, err)
		assert.Equal(t, "New Name", result.Name)
		repo.AssertNotCalled(t, "FindByAlias")
	})

	t.Run("เปลี่ยน alias ซ้ำ คืน ErrGatewaySourceAliasDuplicate", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		dup := &model.GatewaySource{ID: bson.NewObjectID(), Alias: "new-alias"}
		repo.On("FindByIDOrAlias", ctx, "old-alias").Return(existing, nil)
		repo.On("FindByAlias", ctx, "new-alias").Return(dup, nil)

		svc := newGatewaySourceSvc(repo, checker)
		_, err := svc.Update(ctx, "old-alias", &model.GatewaySourceUpdate{Alias: "new-alias", Name: "x", Type: "upstream"}, admin)

		assert.ErrorIs(t, err, ErrGatewaySourceAliasDuplicate)
	})

	t.Run("ไม่พบ source คืน ErrGatewaySourceNotFound", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByIDOrAlias", ctx, "unknown").Return(nil, errors.New("not found"))

		svc := newGatewaySourceSvc(repo, checker)
		_, err := svc.Update(ctx, "unknown", &model.GatewaySourceUpdate{Alias: "x", Name: "x", Type: "upstream"}, admin)

		assert.ErrorIs(t, err, ErrGatewaySourceNotFound)
	})
}

// --- Delete ---

func TestGatewaySourceServiceDelete(t *testing.T) {
	ctx := context.Background()
	srcID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	existing := &model.GatewaySource{ID: srcID, Alias: "my-alias"}

	t.Run("soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByIDOrAlias", ctx, "my-alias").Return(existing, nil)
		checker.On("CountBySourceAlias", ctx, "my-alias").Return(int64(0), nil)
		repo.On("SoftDelete", ctx, srcID, mock.Anything, admin.ID).Return(nil)

		svc := newGatewaySourceSvc(repo, checker)
		err := svc.Delete(ctx, "my-alias", false, admin)

		require.NoError(t, err)
		repo.AssertCalled(t, "SoftDelete", ctx, srcID, mock.Anything, admin.ID)
	})

	t.Run("hard delete สำเร็จ", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByIDOrAlias", ctx, "my-alias").Return(existing, nil)
		checker.On("CountBySourceAlias", ctx, "my-alias").Return(int64(0), nil)
		repo.On("HardDelete", ctx, srcID).Return(nil)

		svc := newGatewaySourceSvc(repo, checker)
		err := svc.Delete(ctx, "my-alias", true, admin)

		require.NoError(t, err)
		repo.AssertCalled(t, "HardDelete", ctx, srcID)
	})

	t.Run("alias ถูกใช้งานอยู่ คืน ErrGatewaySourceAliasInUse", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByIDOrAlias", ctx, "my-alias").Return(existing, nil)
		checker.On("CountBySourceAlias", ctx, "my-alias").Return(int64(2), nil)

		svc := newGatewaySourceSvc(repo, checker)
		err := svc.Delete(ctx, "my-alias", false, admin)

		assert.ErrorIs(t, err, ErrGatewaySourceAliasInUse)
		repo.AssertNotCalled(t, "SoftDelete")
		repo.AssertNotCalled(t, "HardDelete")
	})

	t.Run("ไม่พบ source คืน ErrGatewaySourceNotFound", func(t *testing.T) {
		repo := new(mockGatewaySourceRepo)
		checker := new(mockGatewaySourceInUseChecker)
		repo.On("FindByIDOrAlias", ctx, "unknown").Return(nil, errors.New("not found"))

		svc := newGatewaySourceSvc(repo, checker)
		err := svc.Delete(ctx, "unknown", false, admin)

		assert.ErrorIs(t, err, ErrGatewaySourceNotFound)
	})
}

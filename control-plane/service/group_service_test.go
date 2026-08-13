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

// --- mock groupRepo ---

type mockGroupRepo struct {
	mock.Mock
}

func (m *mockGroupRepo) FindAll(ctx context.Context, params url.Values) ([]*model.Group, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.Group), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockGroupRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.Group, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.Group), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupRepo) FindByAlias(ctx context.Context, alias string) (*model.Group, error) {
	args := m.Called(ctx, alias)
	if v := args.Get(0); v != nil {
		return v.(*model.Group), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupRepo) Insert(ctx context.Context, doc *model.Group) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockGroupRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockGroupRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}

func (m *mockGroupRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

// --- mock groupUserRepo ---

type mockGroupUserRepo struct {
	mock.Mock
}

func (m *mockGroupUserRepo) FindByGroup(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error) {
	args := m.Called(ctx, groupID, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GroupUserDetail), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockGroupUserRepo) FindByUserAndGroup(ctx context.Context, userID, groupID bson.ObjectID) (*model.GroupUser, error) {
	args := m.Called(ctx, userID, groupID)
	if v := args.Get(0); v != nil {
		return v.(*model.GroupUser), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockGroupUserRepo) CountByGroup(ctx context.Context, groupID bson.ObjectID) (int, error) {
	args := m.Called(ctx, groupID)
	return args.Int(0), args.Error(1)
}

func (m *mockGroupUserRepo) InsertMany(ctx context.Context, docs []interface{}) error {
	return m.Called(ctx, docs).Error(0)
}

func (m *mockGroupUserRepo) SoftDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, groupID, userIDs, deletedAt, deletedBy).Error(0)
}

func (m *mockGroupUserRepo) HardDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID) error {
	return m.Called(ctx, groupID, userIDs).Error(0)
}

func (m *mockGroupUserRepo) SoftDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, groupID, deletedAt, deletedBy).Error(0)
}

func (m *mockGroupUserRepo) HardDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID) error {
	return m.Called(ctx, groupID).Error(0)
}

// --- helpers ---

func newGroupService(repo *mockGroupRepo, guRepo *mockGroupUserRepo) *GroupService {
	return NewGroupService(repo, guRepo, new(mockApiKeyNotifier))
}

// --- List ---

func TestGroupServiceList(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()

	t.Run("คืน list พร้อม userCount แต่ละ group", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)

		groups := []*model.Group{{ID: groupID, Name: "Engineering"}}
		repo.On("FindAll", ctx, mock.Anything).Return(groups, int64(1), nil)
		guRepo.On("CountByGroup", ctx, groupID).Return(5, nil)

		svc := newGroupService(repo, guRepo)
		result, total, err := svc.List(ctx, url.Values{})

		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, 5, result[0].UserCount)
	})

	t.Run("repo error คืน error", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindAll", ctx, mock.Anything).Return(nil, int64(0), errors.New("db error"))

		svc := newGroupService(repo, guRepo)
		_, _, err := svc.List(ctx, url.Values{})

		assert.Error(t, err)
	})
}

// --- GetByID ---

func TestGroupServiceGetByID(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	group := &model.Group{ID: groupID, Name: "Engineering"}

	t.Run("พบ group คืน group พร้อม userCount", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("CountByGroup", ctx, groupID).Return(3, nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.GetByID(ctx, groupID)

		require.NoError(t, err)
		assert.Equal(t, groupID, result.ID)
		assert.Equal(t, 3, result.UserCount)
	})

	t.Run("ไม่พบ group คืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.GetByID(ctx, groupID)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})
}

// --- Create ---

func TestGroupServiceCreate(t *testing.T) {
	ctx := context.Background()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}

	t.Run("สร้างสำเร็จไม่มี alias", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByAlias", ctx, mock.AnythingOfType("string")).Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.Create(ctx, &model.GroupCreate{Name: "Engineering"}, admin)

		require.NoError(t, err)
		assert.Equal(t, "Engineering", result.Name)
		assert.Equal(t, admin.ID, *result.CreatedBy)
	})

	t.Run("สร้างสำเร็จมี alias ไม่ซ้ำ", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByAlias", ctx, "eng").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.Create(ctx, &model.GroupCreate{Name: "Engineering", Alias: "eng"}, admin)

		require.NoError(t, err)
		assert.Equal(t, "eng", result.Alias)
	})

	t.Run("alias ซ้ำคืน ErrGroupAliasDuplicate", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		existing := &model.Group{ID: bson.NewObjectID(), Alias: "eng"}
		repo.On("FindByAlias", ctx, "eng").Return(existing, nil)

		svc := newGroupService(repo, guRepo)
		_, err := svc.Create(ctx, &model.GroupCreate{Name: "Engineering", Alias: "eng"}, admin)

		assert.ErrorIs(t, err, ErrGroupAliasDuplicate)
	})

	t.Run("repo Insert error คืน error", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByAlias", ctx, mock.AnythingOfType("string")).Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(errors.New("insert failed"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.Create(ctx, &model.GroupCreate{Name: "Engineering"}, admin)

		assert.Error(t, err)
	})
}

// --- Update ---

func TestGroupServiceUpdate(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	existing := &model.Group{ID: groupID, Name: "Old Name", Alias: "old-alias"}

	t.Run("อัปเดตสำเร็จ", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		updated := &model.Group{ID: groupID, Name: "New Name", Alias: "old-alias"}
		repo.On("FindByID", ctx, groupID).Return(existing, nil).Once()
		repo.On("Update", ctx, groupID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, groupID).Return(updated, nil).Once()

		svc := newGroupService(repo, guRepo)
		result, err := svc.Update(ctx, groupID, &model.GroupUpdate{Name: "New Name", Alias: "old-alias"}, admin)

		require.NoError(t, err)
		assert.Equal(t, "New Name", result.Name)
	})

	t.Run("ไม่พบ group คืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.Update(ctx, groupID, &model.GroupUpdate{Name: "x"}, admin)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})

	t.Run("เปลี่ยน alias ซ้ำคืน ErrGroupAliasDuplicate", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		dup := &model.Group{ID: bson.NewObjectID(), Alias: "new-alias"}
		repo.On("FindByID", ctx, groupID).Return(existing, nil)
		repo.On("FindByAlias", ctx, "new-alias").Return(dup, nil)

		svc := newGroupService(repo, guRepo)
		_, err := svc.Update(ctx, groupID, &model.GroupUpdate{Name: "x", Alias: "new-alias"}, admin)

		assert.ErrorIs(t, err, ErrGroupAliasDuplicate)
	})

	t.Run("alias เดิมไม่เช็ค duplicate", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		updated := &model.Group{ID: groupID, Name: "New Name", Alias: "old-alias"}
		repo.On("FindByID", ctx, groupID).Return(existing, nil).Once()
		repo.On("Update", ctx, groupID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, groupID).Return(updated, nil).Once()

		svc := newGroupService(repo, guRepo)
		_, err := svc.Update(ctx, groupID, &model.GroupUpdate{Name: "New Name", Alias: "old-alias"}, admin)

		require.NoError(t, err)
		repo.AssertNotCalled(t, "FindByAlias")
	})

	t.Run("repo Update error คืน error", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(existing, nil)
		repo.On("Update", ctx, groupID, mock.Anything).Return(errors.New("update failed"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.Update(ctx, groupID, &model.GroupUpdate{Name: "x"}, admin)

		assert.Error(t, err)
	})
}

// --- Delete ---

func TestGroupServiceDelete(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	group := &model.Group{ID: groupID, Name: "Engineering"}

	t.Run("soft delete สำเร็จ cascade group_users", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("SoftDeleteAllByGroup", ctx, groupID, mock.Anything, admin.ID).Return(nil)
		repo.On("SoftDelete", ctx, groupID, mock.Anything, admin.ID).Return(nil)

		svc := newGroupService(repo, guRepo)
		err := svc.Delete(ctx, groupID, false, admin)

		require.NoError(t, err)
		guRepo.AssertCalled(t, "SoftDeleteAllByGroup", ctx, groupID, mock.Anything, admin.ID)
	})

	t.Run("hard delete สำเร็จ cascade group_users", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("HardDeleteAllByGroup", ctx, groupID).Return(nil)
		repo.On("HardDelete", ctx, groupID).Return(nil)

		svc := newGroupService(repo, guRepo)
		err := svc.Delete(ctx, groupID, true, admin)

		require.NoError(t, err)
		guRepo.AssertCalled(t, "HardDeleteAllByGroup", ctx, groupID)
	})

	t.Run("ไม่พบ group คืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		err := svc.Delete(ctx, groupID, false, admin)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})

	t.Run("SoftDeleteAllByGroup error คืน error ไม่ลบ group", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("SoftDeleteAllByGroup", ctx, groupID, mock.Anything, admin.ID).Return(errors.New("cascade error"))

		svc := newGroupService(repo, guRepo)
		err := svc.Delete(ctx, groupID, false, admin)

		assert.Error(t, err)
		repo.AssertNotCalled(t, "SoftDelete")
	})

	t.Run("HardDeleteAllByGroup error คืน error ไม่ลบ group", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("HardDeleteAllByGroup", ctx, groupID).Return(errors.New("cascade error"))

		svc := newGroupService(repo, guRepo)
		err := svc.Delete(ctx, groupID, true, admin)

		assert.Error(t, err)
		repo.AssertNotCalled(t, "HardDelete")
	})
}

// --- GetGroupUsers ---

func TestGroupServiceGetGroupUsers(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	group := &model.Group{ID: groupID}
	params := url.Values{}

	t.Run("คืน list users ใน group", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		list := []*model.GroupUserDetail{{ID: bson.NewObjectID()}}
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("FindByGroup", ctx, groupID, params).Return(list, int64(1), nil)

		svc := newGroupService(repo, guRepo)
		result, total, err := svc.GetGroupUsers(ctx, groupID, params)

		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, result, 1)
	})

	t.Run("group ไม่พบคืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		_, _, err := svc.GetGroupUsers(ctx, groupID, params)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})
}

// --- AddGroupUsers ---

func TestGroupServiceAddGroupUsers(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	group := &model.Group{ID: groupID}
	uid1 := bson.NewObjectID()
	uid2 := bson.NewObjectID()

	t.Run("เพิ่มสำเร็จคืน added/skipped", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("FindByUserAndGroup", ctx, uid1, groupID).Return(nil, errors.New("not found"))
		guRepo.On("FindByUserAndGroup", ctx, uid2, groupID).Return(nil, errors.New("not found"))
		guRepo.On("InsertMany", ctx, mock.Anything).Return(nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.AddGroupUsers(ctx, groupID, []bson.ObjectID{uid1, uid2}, admin)

		require.NoError(t, err)
		assert.Equal(t, 2, result.Added)
		assert.Equal(t, 0, result.Skipped)
	})

	t.Run("user ที่อยู่ใน group แล้วถูก skip", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		existing := &model.GroupUser{ID: bson.NewObjectID()}
		guRepo.On("FindByUserAndGroup", ctx, uid1, groupID).Return(existing, nil)
		guRepo.On("FindByUserAndGroup", ctx, uid2, groupID).Return(nil, errors.New("not found"))
		guRepo.On("InsertMany", ctx, mock.Anything).Return(nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.AddGroupUsers(ctx, groupID, []bson.ObjectID{uid1, uid2}, admin)

		require.NoError(t, err)
		assert.Equal(t, 1, result.Added)
		assert.Equal(t, 1, result.Skipped)
	})

	t.Run("ทุก user ซ้ำหมด added=0 skipped=N ไม่เรียก InsertMany", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		existing := &model.GroupUser{ID: bson.NewObjectID()}
		guRepo.On("FindByUserAndGroup", ctx, uid1, groupID).Return(existing, nil)

		svc := newGroupService(repo, guRepo)
		result, err := svc.AddGroupUsers(ctx, groupID, []bson.ObjectID{uid1}, admin)

		require.NoError(t, err)
		assert.Equal(t, 0, result.Added)
		assert.Equal(t, 1, result.Skipped)
		guRepo.AssertNotCalled(t, "InsertMany")
	})

	t.Run("group ไม่พบคืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.AddGroupUsers(ctx, groupID, []bson.ObjectID{uid1}, admin)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})

	t.Run("InsertMany error คืน error", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("FindByUserAndGroup", ctx, uid1, groupID).Return(nil, errors.New("not found"))
		guRepo.On("InsertMany", ctx, mock.Anything).Return(errors.New("insert failed"))

		svc := newGroupService(repo, guRepo)
		_, err := svc.AddGroupUsers(ctx, groupID, []bson.ObjectID{uid1}, admin)

		assert.Error(t, err)
	})
}

// --- RemoveGroupUsers ---

func TestGroupServiceRemoveGroupUsers(t *testing.T) {
	ctx := context.Background()
	groupID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	group := &model.Group{ID: groupID}
	uid1 := bson.NewObjectID()

	t.Run("soft delete users ออกจาก group สำเร็จ", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("SoftDeleteByGroupAndUsers", ctx, groupID, []bson.ObjectID{uid1}, mock.Anything, admin.ID).Return(nil)

		svc := newGroupService(repo, guRepo)
		err := svc.RemoveGroupUsers(ctx, groupID, &model.GroupUserRemove{UserIDs: []bson.ObjectID{uid1}, Forever: false}, admin)

		require.NoError(t, err)
	})

	t.Run("hard delete users ออกจาก group สำเร็จ", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(group, nil)
		guRepo.On("HardDeleteByGroupAndUsers", ctx, groupID, []bson.ObjectID{uid1}).Return(nil)

		svc := newGroupService(repo, guRepo)
		err := svc.RemoveGroupUsers(ctx, groupID, &model.GroupUserRemove{UserIDs: []bson.ObjectID{uid1}, Forever: true}, admin)

		require.NoError(t, err)
	})

	t.Run("group ไม่พบคืน ErrGroupNotFound", func(t *testing.T) {
		repo := new(mockGroupRepo)
		guRepo := new(mockGroupUserRepo)
		repo.On("FindByID", ctx, groupID).Return(nil, errors.New("not found"))

		svc := newGroupService(repo, guRepo)
		err := svc.RemoveGroupUsers(ctx, groupID, &model.GroupUserRemove{UserIDs: []bson.ObjectID{uid1}}, admin)

		assert.ErrorIs(t, err, ErrGroupNotFound)
	})
}

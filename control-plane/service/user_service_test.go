package service

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mock repo ---

type mockUserRepo struct {
	mock.Mock
}

func (m *mockUserRepo) FindAll(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error) {
	args := m.Called(ctx, params, excludeRoot)
	if v := args.Get(0); v != nil {
		return v.([]*model.User), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserRepo) FindByPhone(ctx context.Context, phone string) (*model.User, error) {
	args := m.Called(ctx, phone)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserRepo) FindByUsernameExact(ctx context.Context, username string) (*model.User, error) {
	args := m.Called(ctx, username)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserRepo) Insert(ctx context.Context, doc *model.User) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockUserRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockUserRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt).Error(0)
}

func (m *mockUserRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *mockUserRepo) BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, ids, deletedAt).Error(0)
}

func (m *mockUserRepo) BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error {
	return m.Called(ctx, ids).Error(0)
}

// --- mock setPasswordSender ---

type mockSetPasswordSender struct {
	mock.Mock
}

func (m *mockSetPasswordSender) Send(ctx context.Context, user *model.User, callbackUrl string) (*model.SendEmailResult, error) {
	args := m.Called(ctx, user, callbackUrl)
	if v := args.Get(0); v != nil {
		return v.(*model.SendEmailResult), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- mock gateway publisher ---

type mockUserGatewayPublisher struct {
	mock.Mock
}

func (m *mockUserGatewayPublisher) Publish(eventType string, payload any) {
	m.Called(eventType, payload)
}

// --- helpers ---

func newUserService(repo *mockUserRepo, spSvc *mockSetPasswordSender) *UserService {
	if spSvc != nil {
		return NewUserService(repo, spSvc, nil)
	}
	return NewUserService(repo, nil, nil)
}

func newUserServiceWithPublisher(repo *mockUserRepo, pub *mockUserGatewayPublisher) *UserService {
	return NewUserService(repo, nil, pub)
}

// --- List ---

func TestUserServiceList(t *testing.T) {
	ctx := context.Background()

	t.Run("คืน list user สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		list := []*model.User{{FirstName: "John"}, {FirstName: "Jane"}}
		repo.On("FindAll", ctx, mock.Anything, false).Return(list, int64(2), nil)

		svc := newUserService(repo, nil)
		result, total, err := svc.List(ctx, url.Values{}, false)

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), total)
	})

	t.Run("repo error คืน error", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindAll", ctx, mock.Anything, false).Return(nil, int64(0), errors.New("db error"))

		svc := newUserService(repo, nil)
		result, _, err := svc.List(ctx, url.Values{}, false)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetByID ---

func TestUserServiceGetByID(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("พบ user คืน user", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id, FirstName: "John"}, nil)

		svc := newUserService(repo, nil)
		u, err := svc.GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, "John", u.FirstName)
	})

	t.Run("ไม่พบ user คืน ErrUserNotFound", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := newUserService(repo, nil)
		_, err := svc.GetByID(ctx, id)

		assert.ErrorIs(t, err, ErrUserNotFound)
	})
}

// --- Create ---

func TestUserServiceCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("สร้าง user สำเร็จ role default เป็น user", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "john@example.com").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := newUserService(repo, nil)
		u, err := svc.Create(ctx, &model.UserCreate{
			Email:     "john@example.com",
			FirstName: "John",
		})

		require.NoError(t, err)
		assert.Equal(t, "user", u.Role)
	})

	t.Run("role ไม่ถูกต้อง คืน ErrInvalidRole", func(t *testing.T) {
		repo := new(mockUserRepo)

		svc := newUserService(repo, nil)
		_, err := svc.Create(ctx, &model.UserCreate{Role: "root"})

		assert.ErrorIs(t, err, ErrInvalidRole)
	})

	t.Run("email ซ้ำ คืน ErrEmailDuplicate", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "dup@example.com").Return(&model.User{}, nil)

		svc := newUserService(repo, nil)
		_, err := svc.Create(ctx, &model.UserCreate{Email: "dup@example.com"})

		assert.ErrorIs(t, err, ErrEmailDuplicate)
	})

	t.Run("username ซ้ำ คืน ErrUsernameDuplicate", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "new@example.com").Return(nil, errors.New("not found"))
		repo.On("FindByUsernameExact", ctx, "dupuser").Return(&model.User{}, nil)

		svc := newUserService(repo, nil)
		_, err := svc.Create(ctx, &model.UserCreate{Email: "new@example.com", Username: "dupuser"})

		assert.ErrorIs(t, err, ErrUsernameDuplicate)
	})

	t.Run("phone ซ้ำ คืน ErrPhoneDuplicate", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "new@example.com").Return(nil, errors.New("not found"))
		repo.On("FindByUsernameExact", ctx, "newuser").Return(nil, errors.New("not found"))
		repo.On("FindByPhone", ctx, "0891234567").Return(&model.User{}, nil)

		svc := newUserService(repo, nil)
		_, err := svc.Create(ctx, &model.UserCreate{Email: "new@example.com", Username: "newuser", Phone: "0891234567"})

		assert.ErrorIs(t, err, ErrPhoneDuplicate)
	})

	t.Run("มี callbackUrl ส่ง email สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "john@example.com").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		spSvc := new(mockSetPasswordSender)
		spSvc.On("Send", ctx, mock.Anything, "https://app.example.com/set-password").Return(nil, nil)

		svc := newUserService(repo, spSvc)
		_, err := svc.Create(ctx, &model.UserCreate{
			Email:       "john@example.com",
			CallbackUrl: "https://app.example.com/set-password",
		})

		require.NoError(t, err)
		spSvc.AssertCalled(t, "Send", ctx, mock.Anything, "https://app.example.com/set-password")
	})

	t.Run("ส่ง email fail ไม่ทำให้ create fail", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "john@example.com").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		spSvc := new(mockSetPasswordSender)
		spSvc.On("Send", ctx, mock.Anything, "https://app.example.com/set-password").Return(nil, errors.New("smtp error"))

		svc := newUserService(repo, spSvc)
		u, err := svc.Create(ctx, &model.UserCreate{
			Email:       "john@example.com",
			CallbackUrl: "https://app.example.com/set-password",
		})

		require.NoError(t, err)
		assert.NotNil(t, u)
	})

}

// --- Update ---

func TestUserServiceUpdate(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("อัปเดตสำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		updated := &model.User{ID: id, FirstName: "Updated"}
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil).Once()

		svc := newUserService(repo, nil)
		u, err := svc.Update(ctx, id, &model.UserUpdate{FirstName: "Updated"})

		require.NoError(t, err)
		assert.Equal(t, "Updated", u.FirstName)
	})

	t.Run("ไม่พบ user คืน ErrUserNotFound", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := newUserService(repo, nil)
		_, err := svc.Update(ctx, id, &model.UserUpdate{})

		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("role ไม่ถูกต้อง คืน ErrInvalidRole", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil)

		svc := newUserService(repo, nil)
		_, err := svc.Update(ctx, id, &model.UserUpdate{Role: "superadmin"})

		assert.ErrorIs(t, err, ErrInvalidRole)
	})

	t.Run("ไม่ส่ง role ใช้ default user", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("Update", ctx, id, mock.MatchedBy(func(fields bson.M) bool {
			return fields["role"] == "user"
		})).Return(nil)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()

		svc := newUserService(repo, nil)
		_, err := svc.Update(ctx, id, &model.UserUpdate{})

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("ไม่ส่ง verified/enabled ใช้ default false/true", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("Update", ctx, id, mock.MatchedBy(func(fields bson.M) bool {
			return fields["verified"] == false && fields["enabled"] == true
		})).Return(nil)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()

		svc := newUserService(repo, nil)
		_, err := svc.Update(ctx, id, &model.UserUpdate{})

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("ไม่ส่ง expiredAt/properties ถูก set เป็น nil (clear)", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("Update", ctx, id, mock.MatchedBy(func(fields bson.M) bool {
			expiredAt, hasExpiredAt := fields["expiredAt"]
			properties, hasProperties := fields["properties"]
			expiredAtNil := expiredAt == nil || expiredAt == (*time.Time)(nil)
			propertiesNil := properties == nil || reflect.ValueOf(properties).IsNil()
			return hasExpiredAt && expiredAtNil && hasProperties && propertiesNil
		})).Return(nil)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()

		svc := newUserService(repo, nil)
		_, err := svc.Update(ctx, id, &model.UserUpdate{})

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

// --- Delete ---

func TestUserServiceDelete(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()
	ctxUserID := bson.NewObjectID()

	t.Run("soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil)
		repo.On("SoftDelete", ctx, id, mock.Anything).Return(nil)

		svc := newUserService(repo, nil)
		err := svc.Delete(ctx, id, false, ctxUserID)

		require.NoError(t, err)
		repo.AssertCalled(t, "SoftDelete", ctx, id, mock.Anything)
	})

	t.Run("hard delete (forever=true) สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil)
		repo.On("HardDelete", ctx, id).Return(nil)

		svc := newUserService(repo, nil)
		err := svc.Delete(ctx, id, true, ctxUserID)

		require.NoError(t, err)
		repo.AssertCalled(t, "HardDelete", ctx, id)
	})

	t.Run("ไม่พบ user คืน ErrUserNotFound", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := newUserService(repo, nil)
		err := svc.Delete(ctx, id, false, ctxUserID)

		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("ลบตัวเองไม่ได้ คืน ErrCannotDeleteSelf", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil)

		svc := newUserService(repo, nil)
		err := svc.Delete(ctx, id, false, id) // ctxUserID == id

		assert.ErrorIs(t, err, ErrCannotDeleteSelf)
	})
}

// --- BulkDelete ---

func TestUserServiceBulkDelete(t *testing.T) {
	ctx := context.Background()
	id1 := bson.NewObjectID()
	id2 := bson.NewObjectID()
	ctxUserID := bson.NewObjectID()

	t.Run("bulk soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("BulkSoftDelete", ctx, []bson.ObjectID{id1, id2}, mock.Anything).Return(nil)

		svc := newUserService(repo, nil)
		err := svc.BulkDelete(ctx, &model.UserBulkDelete{
			UserIDs: []bson.ObjectID{id1, id2},
			Forever: false,
		}, ctxUserID)

		require.NoError(t, err)
	})

	t.Run("bulk hard delete สำเร็จ", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("BulkHardDelete", ctx, []bson.ObjectID{id1, id2}).Return(nil)

		svc := newUserService(repo, nil)
		err := svc.BulkDelete(ctx, &model.UserBulkDelete{
			UserIDs: []bson.ObjectID{id1, id2},
			Forever: true,
		}, ctxUserID)

		require.NoError(t, err)
	})

	t.Run("มี ctxUserID อยู่ใน list คืน ErrCannotDeleteSelf", func(t *testing.T) {
		repo := new(mockUserRepo)

		svc := newUserService(repo, nil)
		err := svc.BulkDelete(ctx, &model.UserBulkDelete{
			UserIDs: []bson.ObjectID{id1, ctxUserID},
		}, ctxUserID)

		assert.ErrorIs(t, err, ErrCannotDeleteSelf)
	})
}

// --- gateway event publishing ---

func TestUserServicePublishesGatewayEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("Create publishes SyncEventTypeUser", func(t *testing.T) {
		repo := new(mockUserRepo)
		repo.On("FindByEmail", ctx, "john@example.com").Return(nil, errors.New("not found"))
		repo.On("Insert", ctx, mock.Anything).Return(nil)
		pub := new(mockUserGatewayPublisher)
		pub.On("Publish", SyncEventTypeUser, mock.Anything).Return()

		svc := newUserServiceWithPublisher(repo, pub)
		_, err := svc.Create(ctx, &model.UserCreate{Email: "john@example.com"})

		require.NoError(t, err)
		pub.AssertCalled(t, "Publish", SyncEventTypeUser, mock.Anything)
	})

	t.Run("Update publishes SyncEventTypeUser with the refetched user", func(t *testing.T) {
		id := bson.NewObjectID()
		repo := new(mockUserRepo)
		updated := &model.User{ID: id, FirstName: "Updated"}
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil).Once()
		pub := new(mockUserGatewayPublisher)
		pub.On("Publish", SyncEventTypeUser, model.ToUserFreshnessCache(updated)).Return()

		svc := newUserServiceWithPublisher(repo, pub)
		_, err := svc.Update(ctx, id, &model.UserUpdate{FirstName: "Updated"})

		require.NoError(t, err)
		pub.AssertCalled(t, "Publish", SyncEventTypeUser, model.ToUserFreshnessCache(updated))
	})

	t.Run("Delete publishes a synthesized Enabled=false payload, not a DB refetch", func(t *testing.T) {
		// Regression: SoftDelete never touches `enabled`, and a deleted user is
		// filtered out of FindByID entirely. So this must NOT come from a refetch.
		id := bson.NewObjectID()
		ctxUserID := bson.NewObjectID()
		repo := new(mockUserRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id}, nil).Once()
		repo.On("SoftDelete", ctx, id, mock.Anything).Return(nil)
		pub := new(mockUserGatewayPublisher)
		disabled := false
		pub.On("Publish", SyncEventTypeUser, &model.UserFreshnessCache{ID: id.Hex(), Enabled: &disabled}).Return()

		svc := newUserServiceWithPublisher(repo, pub)
		err := svc.Delete(ctx, id, false, ctxUserID)

		require.NoError(t, err)
		pub.AssertCalled(t, "Publish", SyncEventTypeUser, &model.UserFreshnessCache{ID: id.Hex(), Enabled: &disabled})
		repo.AssertNumberOfCalls(t, "FindByID", 1) // only the existence check — no refetch after delete
	})

	t.Run("BulkDelete publishes exactly one SyncEventTypeUserInvalidate event regardless of N users", func(t *testing.T) {
		// Regression test for the event-storm class of bug fixed for reload_services —
		// a bulk Mongo call must never fan out into one event per affected user.
		id1, id2, id3 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
		ctxUserID := bson.NewObjectID()
		ids := []bson.ObjectID{id1, id2, id3}
		repo := new(mockUserRepo)
		repo.On("BulkSoftDelete", ctx, ids, mock.Anything).Return(nil)
		pub := new(mockUserGatewayPublisher)
		hexIDs := []string{id1.Hex(), id2.Hex(), id3.Hex()}
		pub.On("Publish", SyncEventTypeUserInvalidate, &model.UserIDsPayload{UserIDs: hexIDs}).Return()

		svc := newUserServiceWithPublisher(repo, pub)
		err := svc.BulkDelete(ctx, &model.UserBulkDelete{UserIDs: ids}, ctxUserID)

		require.NoError(t, err)
		pub.AssertNumberOfCalls(t, "Publish", 1)
	})
}

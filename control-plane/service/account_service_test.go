package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

const (
	acctErrDB      = "db error"
	acctErrRedis   = "redis error"
	acctKeySession = "session:"
	acctKeyUser    = ",user:"
)

// --- mocks ---

type mockAccountUserRepo struct{ mock.Mock }

func (m *mockAccountUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountUserRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockAccountUserRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt).Error(0)
}

type mockAccountSessionCache struct{ mock.Mock }

func (m *mockAccountSessionCache) ScanByUser(ctx context.Context, userID string) ([]string, error) {
	args := m.Called(ctx, userID)
	if v := args.Get(0); v != nil {
		return v.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountSessionCache) Delete(ctx context.Context, keys []string) error {
	return m.Called(ctx, keys).Error(0)
}

type mockAccountUserSessionRepo struct{ mock.Mock }

func (m *mockAccountUserSessionRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.UserSession, error) {
	args := m.Called(ctx, ids)
	if v := args.Get(0); v != nil {
		return v.([]*model.UserSession), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountUserSessionRepo) FindHistoryByUserID(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error) {
	args := m.Called(ctx, userID)
	if v := args.Get(0); v != nil {
		return v.([]*model.UserSession), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockAccountUserSessionRepo) UpdateLogout(ctx context.Context, sessionID bson.ObjectID, logoutAt time.Time, reason string) error {
	return m.Called(ctx, sessionID, logoutAt, reason).Error(0)
}

func (m *mockAccountUserSessionRepo) BulkUpdateLogout(ctx context.Context, sessionIDs []bson.ObjectID, logoutAt time.Time, reason string) error {
	return m.Called(ctx, sessionIDs, logoutAt, reason).Error(0)
}

func newAccountService(repo *mockAccountUserRepo, sc *mockAccountSessionCache, sr *mockAccountUserSessionRepo) *AccountService {
	return NewAccountService(repo, sc, sr)
}

// --- UpdateProfile ---

func TestAccountServiceUpdateProfile(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("อัปเดตสำเร็จคืน user ใหม่", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		updated := &model.User{ID: id, FirstName: "New"}
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)

		svc := newAccountService(repo, sc, sr)
		u, err := svc.UpdateProfile(ctx, id, &model.AccountProfileUpdate{FirstName: "New"})

		require.NoError(t, err)
		assert.Equal(t, "New", u.FirstName)
	})

	t.Run("repo update error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("Update", ctx, id, mock.Anything).Return(errors.New(acctErrDB))

		svc := newAccountService(repo, sc, sr)
		_, err := svc.UpdateProfile(ctx, id, &model.AccountProfileUpdate{})

		assert.Error(t, err)
	})
}

// --- ChangePassword ---

func TestAccountServiceChangePassword(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()
	oldPlain := "old-secret-1"
	newPlain := "new-secret-2"
	hashed, errHash := bcrypt.GenerateFromPassword([]byte(oldPlain), bcrypt.DefaultCost)
	require.NoError(t, errHash)

	t.Run("เปลี่ยนรหัสสำเร็จ", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		updated := &model.User{ID: id, FirstName: "After"}
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id, Password: string(hashed)}, nil).Once()
		repo.On("Update", ctx, id, mock.MatchedBy(func(m bson.M) bool {
			pw, ok := m["password"].(string)
			return ok && pw != "" && pw != newPlain
		})).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil).Once()

		svc := newAccountService(repo, sc, sr)
		u, err := svc.ChangePassword(ctx, id, oldPlain, newPlain)
		require.NoError(t, err)
		require.NotNil(t, u)
		assert.Equal(t, "After", u.FirstName)
	})

	t.Run("รหัสเดิมผิด", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id, Password: string(hashed)}, nil)

		svc := newAccountService(repo, sc, sr)
		u, err := svc.ChangePassword(ctx, id, "wrong", newPlain)
		assert.Nil(t, u)
		assert.ErrorIs(t, err, ErrChangePasswordWrongCurrent)
	})

	t.Run("ไม่มี local password", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id, Password: ""}, nil)

		svc := newAccountService(repo, sc, sr)
		u, err := svc.ChangePassword(ctx, id, oldPlain, newPlain)
		assert.Nil(t, u)
		assert.ErrorIs(t, err, ErrChangePasswordNoLocal)
	})

	t.Run("รหัสใหม่เหมือนเดิม", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("FindByID", ctx, id).Return(&model.User{ID: id, Password: string(hashed)}, nil)

		svc := newAccountService(repo, sc, sr)
		u, err := svc.ChangePassword(ctx, id, oldPlain, oldPlain)
		assert.Nil(t, u)
		assert.ErrorIs(t, err, ErrChangePasswordUnchanged)
	})

	t.Run("ไม่พบ user", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("FindByID", ctx, id).Return(nil, mongo.ErrNoDocuments)

		svc := newAccountService(repo, sc, sr)
		u, err := svc.ChangePassword(ctx, id, oldPlain, newPlain)
		assert.Nil(t, u)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})
}

// --- DeleteProfile ---

func TestAccountServiceDeleteProfile(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("soft delete สำเร็จ", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("SoftDelete", ctx, id, mock.Anything).Return(nil)

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteProfile(ctx, id)

		require.NoError(t, err)
		repo.AssertCalled(t, "SoftDelete", ctx, id, mock.Anything)
	})

	t.Run("repo error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		repo.On("SoftDelete", ctx, id, mock.Anything).Return(errors.New(acctErrDB))

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteProfile(ctx, id)

		assert.Error(t, err)
	})
}

// --- GetSessions ---

func TestAccountServiceGetSessions(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()
	sessionID := bson.NewObjectID()
	currentSessionID := sessionID
	loginAt := time.Now().UTC().Truncate(time.Second)
	redisKey := acctKeySession + sessionID.Hex() + acctKeyUser + userID.Hex()

	t.Run("คืน sessions สำเร็จ พร้อม current flag และ metadata จาก MongoDB", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{redisKey}, nil)
		sr.On("FindByIDs", ctx, []bson.ObjectID{sessionID}).Return([]*model.UserSession{{
			ID:      sessionID,
			Browser: "Chrome",
			LoginAt: loginAt,
		}}, nil)

		svc := newAccountService(repo, sc, sr)
		sessions, err := svc.GetSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, sessionID, sessions[0].ID)
		assert.True(t, sessions[0].Current)
		assert.Equal(t, "Chrome", sessions[0].Browser)
		assert.Equal(t, loginAt, sessions[0].LoginAt)
	})

	t.Run("session ที่ไม่ใช่ current ค่า current=false", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		otherID := bson.NewObjectID()
		otherKey := acctKeySession + otherID.Hex() + acctKeyUser + userID.Hex()
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{otherKey}, nil)
		sr.On("FindByIDs", ctx, []bson.ObjectID{otherID}).Return([]*model.UserSession{{ID: otherID, LoginAt: loginAt}}, nil)

		svc := newAccountService(repo, sc, sr)
		sessions, err := svc.GetSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.False(t, sessions[0].Current)
	})

	t.Run("Redis scan error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("ScanByUser", ctx, userID.Hex()).Return(nil, errors.New(acctErrRedis))

		svc := newAccountService(repo, sc, sr)
		_, err := svc.GetSessions(ctx, userID, currentSessionID)

		assert.Error(t, err)
	})

	t.Run("ไม่มี session ใน Redis คืน empty slice", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{}, nil)

		svc := newAccountService(repo, sc, sr)
		sessions, err := svc.GetSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		assert.Empty(t, sessions)
	})
}

// --- DeleteSession ---

func TestAccountServiceDeleteSession(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()
	sessionID := bson.NewObjectID()

	t.Run("ลบ session สำเร็จ และ update MongoDB", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("Delete", ctx, mock.Anything).Return(nil)
		sr.On("UpdateLogout", ctx, sessionID, mock.Anything, "logout").Return(nil)

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteSession(ctx, sessionID, userID)

		require.NoError(t, err)
		sc.AssertCalled(t, "Delete", ctx, mock.Anything)
		sr.AssertCalled(t, "UpdateLogout", ctx, sessionID, mock.Anything, "logout")
	})

	t.Run("cache error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("Delete", ctx, mock.Anything).Return(errors.New(acctErrRedis))

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteSession(ctx, sessionID, userID)

		assert.Error(t, err)
	})
}

// --- DeleteAllSessions ---

func TestAccountServiceDeleteAllSessions(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()
	currentSessionID := bson.NewObjectID()

	t.Run("ลบ session อื่นยกเว้น current สำเร็จ และ bulk update MongoDB", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		otherSessionID := bson.NewObjectID()
		currentKey := acctKeySession + currentSessionID.Hex() + acctKeyUser + userID.Hex()
		otherKey := acctKeySession + otherSessionID.Hex() + acctKeyUser + userID.Hex()
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{currentKey, otherKey}, nil)
		sc.On("Delete", ctx, []string{otherKey}).Return(nil)
		sr.On("BulkUpdateLogout", ctx, []bson.ObjectID{otherSessionID}, mock.Anything, "logout_all").Return(nil)

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteAllSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		sc.AssertCalled(t, "Delete", ctx, []string{otherKey})
		sr.AssertCalled(t, "BulkUpdateLogout", ctx, []bson.ObjectID{otherSessionID}, mock.Anything, "logout_all")
	})

	t.Run("มีแค่ current session ไม่เรียก Delete", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		currentKey := acctKeySession + currentSessionID.Hex() + acctKeyUser + userID.Hex()
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{currentKey}, nil)

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteAllSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		sc.AssertNotCalled(t, "Delete")
	})

	t.Run("ไม่มี session ไม่เรียก Delete", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("ScanByUser", ctx, userID.Hex()).Return([]string{}, nil)

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteAllSessions(ctx, userID, currentSessionID)

		require.NoError(t, err)
		sc.AssertNotCalled(t, "Delete")
	})

	t.Run("scan error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sc.On("ScanByUser", ctx, userID.Hex()).Return(nil, errors.New(acctErrRedis))

		svc := newAccountService(repo, sc, sr)
		err := svc.DeleteAllSessions(ctx, userID, currentSessionID)

		assert.Error(t, err)
	})
}

// --- GetSessionHistory ---

func TestAccountServiceGetSessionHistory(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()
	loginAt := time.Now().UTC().Truncate(time.Second)

	t.Run("คืน history สำเร็จ", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sessions := []*model.UserSession{
			{ID: bson.NewObjectID(), UserID: userID, Browser: "Chrome", LoginAt: loginAt},
			{ID: bson.NewObjectID(), UserID: userID, Browser: "Firefox", LoginAt: loginAt},
		}
		sr.On("FindHistoryByUserID", ctx, userID).Return(sessions, nil)

		svc := newAccountService(repo, sc, sr)
		result, err := svc.GetSessionHistory(ctx, userID)

		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("repo error คืน error", func(t *testing.T) {
		repo := new(mockAccountUserRepo)
		sc := new(mockAccountSessionCache)
		sr := new(mockAccountUserSessionRepo)
		sr.On("FindHistoryByUserID", ctx, userID).Return(nil, errors.New(acctErrDB))

		svc := newAccountService(repo, sc, sr)
		_, err := svc.GetSessionHistory(ctx, userID)

		assert.Error(t, err)
	})
}

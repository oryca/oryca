package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/oryca/oryca/control-plane/config"
	"github.com/oryca/oryca/control-plane/model"
)

// --- mock verify email cache ---

type mockVerifyEmailCache struct {
	mock.Mock
}

func (m *mockVerifyEmailCache) Set(ctx context.Context, token, userID string, ttl time.Duration) error {
	return m.Called(ctx, token, userID, ttl).Error(0)
}

func (m *mockVerifyEmailCache) Get(ctx context.Context, token string) (string, error) {
	args := m.Called(ctx, token)
	return args.String(0), args.Error(1)
}

func (m *mockVerifyEmailCache) Delete(ctx context.Context, token string) error {
	return m.Called(ctx, token).Error(0)
}

// --- mock verify email user repo ---

type mockVerifyEmailUserRepo struct {
	mock.Mock
}

func (m *mockVerifyEmailUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockVerifyEmailUserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockVerifyEmailUserRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

// --- mock gateway publisher ---

type mockVerifyEmailGatewayPublisher struct {
	mock.Mock
}

func (m *mockVerifyEmailGatewayPublisher) Publish(eventType string, payload any) {
	m.Called(eventType, payload)
}

func TestVerifyEmailServiceVerify(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()

	t.Run("verify สำเร็จ", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)

		cache.On("Get", ctx, "valid-token").Return(userID.Hex(), nil)
		userRepo.On("Update", ctx, userID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, "valid-token").Return(nil)

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo}
		err := svc.Verify(ctx, "valid-token")

		require.NoError(t, err)
		userRepo.AssertCalled(t, "Update", ctx, userID, mock.Anything)
		cache.AssertCalled(t, "Delete", ctx, "valid-token")
	})

	t.Run("token ไม่พบใน cache คืน ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)

		cache.On("Get", ctx, "bad-token").Return("", errors.New("not found"))

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo}
		err := svc.Verify(ctx, "bad-token")

		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("userID ใน cache ไม่ใช่ ObjectID ถูกต้อง คืน ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)

		cache.On("Get", ctx, "token-bad-id").Return("not-valid-hex", nil)

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo}
		err := svc.Verify(ctx, "token-bad-id")

		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("repo Update error คืน error", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)

		cache.On("Get", ctx, "token-update-fail").Return(userID.Hex(), nil)
		userRepo.On("Update", ctx, userID, mock.Anything).Return(errors.New("db error"))

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo}
		err := svc.Verify(ctx, "token-update-fail")

		assert.Error(t, err)
		assert.Equal(t, "db error", err.Error())
	})

	t.Run("verify สำเร็จ publish SyncEventTypeUser จาก refetch", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)
		pub := new(mockVerifyEmailGatewayPublisher)
		verified := true
		refetched := &model.User{ID: userID, Verified: &verified}

		cache.On("Get", ctx, "valid-token").Return(userID.Hex(), nil)
		userRepo.On("Update", ctx, userID, mock.Anything).Return(nil)
		userRepo.On("FindByID", ctx, userID).Return(refetched, nil)
		cache.On("Delete", ctx, "valid-token").Return(nil)
		pub.On("Publish", SyncEventTypeUser, model.ToUserFreshnessCache(refetched)).Return()

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo, publisher: pub}
		err := svc.Verify(ctx, "valid-token")

		require.NoError(t, err)
		pub.AssertCalled(t, "Publish", SyncEventTypeUser, model.ToUserFreshnessCache(refetched))
	})

	t.Run("publisher nil ไม่ panic (พฤติกรรมเดิม)", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userRepo := new(mockVerifyEmailUserRepo)

		cache.On("Get", ctx, "valid-token").Return(userID.Hex(), nil)
		userRepo.On("Update", ctx, userID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, "valid-token").Return(nil)

		svc := &VerifyEmailService{cache: cache, userRepo: userRepo}
		err := svc.Verify(ctx, "valid-token")

		require.NoError(t, err)
		userRepo.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
	})
}

func TestVerifyEmailServiceSendByEmail(t *testing.T) {
	ctx := context.Background()
	userID := bson.NewObjectID()
	callbackUrl := "https://app.example.com/verify"
	verified := true
	notVerified := false
	disabled := false

	t.Run("email ไม่พบในระบบ คืน ErrUserNotFound", func(t *testing.T) {
		userRepo := new(mockVerifyEmailUserRepo)
		userRepo.On("FindByEmail", ctx, "unknown@test.com").Return(nil, errors.New("not found"))

		svc := &VerifyEmailService{userRepo: userRepo}
		_, err := svc.SendByEmail(ctx, "unknown@test.com", callbackUrl)

		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("user verified แล้ว คืน ErrUserAlreadyVerified", func(t *testing.T) {
		userRepo := new(mockVerifyEmailUserRepo)
		user := &model.User{ID: userID, Email: "user@test.com", Verified: &verified}
		userRepo.On("FindByEmail", ctx, "user@test.com").Return(user, nil)

		svc := &VerifyEmailService{userRepo: userRepo}
		_, err := svc.SendByEmail(ctx, "user@test.com", callbackUrl)

		assert.ErrorIs(t, err, ErrUserAlreadyVerified)
	})

	t.Run("user ถูก disable คืน ErrUserNotEnabledAuth", func(t *testing.T) {
		userRepo := new(mockVerifyEmailUserRepo)
		user := &model.User{ID: userID, Email: "user@test.com", Verified: &notVerified, Enabled: &disabled}
		userRepo.On("FindByEmail", ctx, "user@test.com").Return(user, nil)

		svc := &VerifyEmailService{userRepo: userRepo}
		_, err := svc.SendByEmail(ctx, "user@test.com", callbackUrl)

		assert.ErrorIs(t, err, ErrUserNotEnabledAuth)
	})
}

func TestVerifyEmailServiceSend(t *testing.T) {
	ctx := context.Background()
	veTTL := 30 * time.Minute
	callbackUrl := "https://app.example.com/verify"

	t.Run("TTL ต้องมาจากค่าที่ inject เข้ามา ไม่ใช่ของ reset-password", func(t *testing.T) {
		cache := new(mockVerifyEmailCache)
		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: "user@test.com", FirstName: "John"}

		cache.On("Set", ctx, mock.Anything, userID.Hex(), veTTL).Return(nil)

		svc := &VerifyEmailService{cache: cache, mailSvc: NewMailService(&config.SMTPConfig{}), ttl: veTTL}
		_, err := svc.Send(ctx, user, callbackUrl)

		require.ErrorIs(t, err, ErrMailDisabled)
		cache.AssertCalled(t, "Set", ctx, mock.Anything, userID.Hex(), veTTL)
	})
}

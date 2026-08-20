package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mocks ---

type mockSetPasswordCache struct{ mock.Mock }

func (m *mockSetPasswordCache) Set(ctx context.Context, token, userID string, ttl time.Duration) error {
	return m.Called(ctx, token, userID, ttl).Error(0)
}

func (m *mockSetPasswordCache) Get(ctx context.Context, token string) (string, error) {
	args := m.Called(ctx, token)
	return args.String(0), args.Error(1)
}

func (m *mockSetPasswordCache) Delete(ctx context.Context, token string) error {
	return m.Called(ctx, token).Error(0)
}

type mockSetPasswordUserRepo struct{ mock.Mock }

func (m *mockSetPasswordUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSetPasswordUserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if v := args.Get(0); v != nil {
		return v.(*model.User), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSetPasswordUserRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

type mockSetPasswordMailSender struct{ mock.Mock }

func (m *mockSetPasswordMailSender) SendTemplate(ctx context.Context, toEmail, alias string, vars map[string]string) error {
	return m.Called(ctx, toEmail, alias, vars).Error(0)
}

// --- helpers ---

const (
	spEmail       = "user@example.com"
	spToken       = "valid-token"
	spCallbackUrl = "https://app.example.com/reset"
	spPassword    = "newpassword123"
	spErrDB       = "db error"
)

const spTTL = 3600 * time.Second

func newSetPasswordSvc(
	cache *mockSetPasswordCache,
	repo *mockSetPasswordUserRepo,
	mailSvc *mockSetPasswordMailSender,
) *SetPasswordService {
	return &SetPasswordService{
		cache:    cache,
		userRepo: repo,
		mailSvc:  mailSvc,
		ttl:      spTTL,
	}
}

func boolPtr(b bool) *bool { return &b }

// --- TestSetPasswordService_ForgotPassword ---

func TestSetPasswordService_ForgotPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("email ไม่มีในระบบ ต้องได้ ErrUserNotFound", func(t *testing.T) {
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(nil, repo, nil)

		repo.On("FindByEmail", ctx, spEmail).Return(nil, errors.New(spErrDB))

		_, err := svc.ForgotPassword(ctx, spEmail, spCallbackUrl)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("user ยังไม่ verify ต้องได้ ErrUserNotVerifiedAuth", func(t *testing.T) {
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(nil, repo, nil)

		user := &model.User{ID: bson.NewObjectID(), Email: spEmail, Verified: boolPtr(false)}
		repo.On("FindByEmail", ctx, spEmail).Return(user, nil)

		_, err := svc.ForgotPassword(ctx, spEmail, spCallbackUrl)
		assert.ErrorIs(t, err, ErrUserNotVerifiedAuth)
	})

	t.Run("verified เป็น nil ต้องได้ ErrUserNotVerifiedAuth", func(t *testing.T) {
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(nil, repo, nil)

		user := &model.User{ID: bson.NewObjectID(), Email: spEmail, Verified: nil}
		repo.On("FindByEmail", ctx, spEmail).Return(user, nil)

		_, err := svc.ForgotPassword(ctx, spEmail, spCallbackUrl)
		assert.ErrorIs(t, err, ErrUserNotVerifiedAuth)
	})

	t.Run("user ถูก disable ต้องได้ ErrUserNotEnabledAuth", func(t *testing.T) {
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(nil, repo, nil)

		user := &model.User{ID: bson.NewObjectID(), Email: spEmail, Verified: boolPtr(true), Enabled: boolPtr(false)}
		repo.On("FindByEmail", ctx, spEmail).Return(user, nil)

		_, err := svc.ForgotPassword(ctx, spEmail, spCallbackUrl)
		assert.ErrorIs(t, err, ErrUserNotEnabledAuth)
	})

	t.Run("ส่งสำเร็จ ต้องได้ result พร้อม email และ expiresIn", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John", Verified: boolPtr(true), Enabled: boolPtr(true)}
		repo.On("FindByEmail", ctx, spEmail).Return(user, nil)
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.Anything).Return(nil)

		result, err := svc.ForgotPassword(ctx, spEmail, spCallbackUrl)
		require.NoError(t, err)
		assert.Equal(t, spEmail, result.Email)
		assert.WithinDuration(t, time.Now().Add(spTTL), result.ExpiredAt, 5*time.Second)
	})
}

// --- TestSetPasswordService_Send ---

func TestSetPasswordService_Send(t *testing.T) {
	ctx := context.Background()

	t.Run("mail ปิดอยู่ (ErrMailDisabled) ต้องได้ error กลับไป", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.Anything).Return(ErrMailDisabled)

		_, err := svc.Send(ctx, user, spCallbackUrl)
		assert.ErrorIs(t, err, ErrMailDisabled)
	})

	t.Run("cache.Set error ต้องได้ error", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(errors.New(spErrDB))

		_, err := svc.Send(ctx, user, spCallbackUrl)
		assert.Error(t, err)
	})

	t.Run("ใช้ DisplayName เป็น name เมื่อมีค่า", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John", DisplayName: "Johnny"}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.MatchedBy(func(vars map[string]string) bool {
			return vars["name"] == "Johnny"
		})).Return(nil)

		result, err := svc.Send(ctx, user, spCallbackUrl)
		require.NoError(t, err)
		assert.Equal(t, spEmail, result.Email)
		assert.WithinDuration(t, time.Now().Add(spTTL), result.ExpiredAt, 5*time.Second)
		mail.AssertExpectations(t)
	})

	t.Run("ใช้ FirstName เมื่อ DisplayName ว่าง", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John", DisplayName: ""}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.MatchedBy(func(vars map[string]string) bool {
			return vars["name"] == "John"
		})).Return(nil)

		_, err := svc.Send(ctx, user, spCallbackUrl)
		require.NoError(t, err)
		mail.AssertExpectations(t)
	})

	t.Run("link ต้องมี ?token= ต่อท้าย callbackUrl ปกติ", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John"}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.MatchedBy(func(vars map[string]string) bool {
			link := vars["link"]
			u, err := url.Parse(link)
			if err != nil {
				return false
			}
			return u.Query().Get("token") != "" && strings.HasPrefix(link, spCallbackUrl)
		})).Return(nil)

		_, err := svc.Send(ctx, user, spCallbackUrl)
		require.NoError(t, err)
		mail.AssertExpectations(t)
	})

	t.Run("callbackUrl มี query param อื่นอยู่แล้ว ต้องแนบ &token= ได้ถูกต้อง", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		urlWithParam := "https://app.example.com/reset?lang=th&source=email"
		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John"}
		cache.On("Set", ctx, mock.Anything, userID.Hex(), spTTL).Return(nil)
		mail.On("SendTemplate", ctx, spEmail, aliasSetPassword, mock.MatchedBy(func(vars map[string]string) bool {
			u, err := url.Parse(vars["link"])
			if err != nil {
				return false
			}
			q := u.Query()
			return q.Get("token") != "" && q.Get("lang") == "th" && q.Get("source") == "email"
		})).Return(nil)

		_, err := svc.Send(ctx, user, urlWithParam)
		require.NoError(t, err)
		mail.AssertExpectations(t)
	})

	t.Run("callbackUrl มี token param อยู่แล้ว ต้องได้ ErrCallbackUrlInvalid", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		user := &model.User{ID: bson.NewObjectID(), Email: spEmail, FirstName: "John"}
		cache.On("Set", ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		_, err := svc.Send(ctx, user, "https://app.example.com/reset?token=garbage")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCallbackUrlInvalid)
	})

	t.Run("callbackUrl ไม่ใช่ URL ที่ถูกต้อง ต้องได้ ErrCallbackUrlInvalid", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		mail := new(mockSetPasswordMailSender)
		svc := newSetPasswordSvc(cache, repo, mail)

		user := &model.User{ID: bson.NewObjectID(), Email: spEmail, FirstName: "John"}
		cache.On("Set", ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		_, err := svc.Send(ctx, user, "not-a-valid-url")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCallbackUrlInvalid)
	})
}

// --- TestAppendTokenParam ---

func TestAppendTokenParam(t *testing.T) {
	t.Run("URL ปกติ ต้องได้ ?token=xxx", func(t *testing.T) {
		result, err := appendTokenParam("https://app.example.com/reset", "abc123")
		require.NoError(t, err)
		u, _ := url.Parse(result)
		assert.Equal(t, "abc123", u.Query().Get("token"))
	})

	t.Run("URL ที่มี query param อื่น ต้องได้ &token=xxx", func(t *testing.T) {
		result, err := appendTokenParam("https://app.example.com/reset?lang=th", "abc123")
		require.NoError(t, err)
		u, _ := url.Parse(result)
		assert.Equal(t, "abc123", u.Query().Get("token"))
		assert.Equal(t, "th", u.Query().Get("lang"))
		assert.NotContains(t, result, "?token")
		assert.Contains(t, result, "lang=th")
	})

	t.Run("URL มี token param อยู่แล้ว ต้องได้ ErrCallbackUrlInvalid", func(t *testing.T) {
		_, err := appendTokenParam("https://app.example.com/reset?token=old", "abc123")
		assert.ErrorIs(t, err, ErrCallbackUrlInvalid)
	})

	t.Run("URL ไม่มี scheme ต้องได้ ErrCallbackUrlInvalid", func(t *testing.T) {
		_, err := appendTokenParam("app.example.com/reset", "abc123")
		assert.ErrorIs(t, err, ErrCallbackUrlInvalid)
	})

	t.Run("string ว่าง ต้องได้ ErrCallbackUrlInvalid", func(t *testing.T) {
		_, err := appendTokenParam("", "abc123")
		assert.ErrorIs(t, err, ErrCallbackUrlInvalid)
	})
}

// --- TestSetPasswordService_Verify ---

func TestSetPasswordService_Verify(t *testing.T) {
	ctx := context.Background()

	t.Run("token ไม่มีใน cache ต้องได้ ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		svc := newSetPasswordSvc(cache, nil, nil)

		cache.On("Get", ctx, spToken).Return("", errors.New("not found"))

		_, err := svc.Verify(ctx, spToken)
		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("userID ใน cache ไม่ใช่ ObjectID ที่ถูกต้อง ต้องได้ ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		svc := newSetPasswordSvc(cache, nil, nil)

		cache.On("Get", ctx, spToken).Return("invalid-hex", nil)

		_, err := svc.Verify(ctx, spToken)
		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("ไม่พบ user ต้องได้ ErrUserNotFound", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(cache, repo, nil)

		userID := bson.NewObjectID()
		cache.On("Get", ctx, spToken).Return(userID.Hex(), nil)
		repo.On("FindByID", ctx, userID).Return(nil, errors.New(spErrDB))

		_, err := svc.Verify(ctx, spToken)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("verify สำเร็จ ต้องได้ token info", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(cache, repo, nil)

		userID := bson.NewObjectID()
		user := &model.User{ID: userID, Email: spEmail, FirstName: "John"}
		cache.On("Get", ctx, spToken).Return(userID.Hex(), nil)
		repo.On("FindByID", ctx, userID).Return(user, nil)

		info, err := svc.Verify(ctx, spToken)
		require.NoError(t, err)
		assert.Equal(t, userID, info.UserID)
		assert.Equal(t, spEmail, info.Email)
		assert.Equal(t, "John", info.FirstName)
	})
}

// --- TestSetPasswordService_SetPassword ---

func TestSetPasswordService_SetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("token ไม่มีใน cache ต้องได้ ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		svc := newSetPasswordSvc(cache, nil, nil)

		cache.On("Get", ctx, spToken).Return("", errors.New("not found"))

		err := svc.SetPassword(ctx, spToken, spPassword)
		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("userID ใน cache ไม่ถูกต้อง ต้องได้ ErrTokenInvalidOrExpired", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		svc := newSetPasswordSvc(cache, nil, nil)

		cache.On("Get", ctx, spToken).Return("invalid-hex", nil)

		err := svc.SetPassword(ctx, spToken, spPassword)
		assert.ErrorIs(t, err, ErrTokenInvalidOrExpired)
	})

	t.Run("repo.Update error ต้องได้ error", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(cache, repo, nil)

		userID := bson.NewObjectID()
		cache.On("Get", ctx, spToken).Return(userID.Hex(), nil)
		repo.On("Update", ctx, userID, mock.Anything).Return(errors.New(spErrDB))

		err := svc.SetPassword(ctx, spToken, spPassword)
		assert.Error(t, err)
	})

	t.Run("ตั้งรหัสผ่านสำเร็จ ต้องลบ token ออกจาก cache", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(cache, repo, nil)

		userID := bson.NewObjectID()
		cache.On("Get", ctx, spToken).Return(userID.Hex(), nil)
		repo.On("Update", ctx, userID, mock.Anything).Return(nil)
		cache.On("Delete", ctx, spToken).Return(nil)

		err := svc.SetPassword(ctx, spToken, spPassword)
		require.NoError(t, err)
		cache.AssertCalled(t, "Delete", ctx, spToken)
	})

	t.Run("password ต้องถูก hash ก่อน save", func(t *testing.T) {
		cache := new(mockSetPasswordCache)
		repo := new(mockSetPasswordUserRepo)
		svc := newSetPasswordSvc(cache, repo, nil)

		userID := bson.NewObjectID()
		cache.On("Get", ctx, spToken).Return(userID.Hex(), nil)
		repo.On("Update", ctx, userID, mock.MatchedBy(func(fields bson.M) bool {
			hashed, ok := fields["password"].(string)
			return ok && hashed != spPassword && len(hashed) > 0
		})).Return(nil)
		cache.On("Delete", ctx, spToken).Return(nil)

		err := svc.SetPassword(ctx, spToken, spPassword)
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

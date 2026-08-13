package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrTokenInvalidOrExpired = errors.New("token invalid or expired")
	ErrCallbackUrlInvalid    = errors.New("callback url invalid")
)

const aliasSetPassword = "oryca.set-password"

type setPasswordCacher interface {
	Set(ctx context.Context, token, userID string, ttl time.Duration) error
	Get(ctx context.Context, token string) (string, error)
	Delete(ctx context.Context, token string) error
}

type setPasswordUserRepo interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
}

type setPasswordMailServerSvc interface {
	GetDefault(ctx context.Context) (*model.MailServer, error)
}

type setPasswordMailSender interface {
	SendTemplate(ctx context.Context, toEmail, alias string, vars map[string]string) error
}

type SetPasswordService struct {
	cache         setPasswordCacher
	userRepo      setPasswordUserRepo
	mailSvc       setPasswordMailSender
	mailServerSvc setPasswordMailServerSvc
}

func NewSetPasswordService(
	cache setPasswordCacher,
	userRepo setPasswordUserRepo,
	mailSvc *MailService,
	mailServerSvc setPasswordMailServerSvc,
) *SetPasswordService {
	return &SetPasswordService{
		cache:         cache,
		userRepo:      userRepo,
		mailSvc:       mailSvc,
		mailServerSvc: mailServerSvc,
	}
}

// Send generates a one-time token, stores it in Redis with TTL from mail server config, and sends an email to the user with the token link.
func (s *SetPasswordService) Send(ctx context.Context, user *model.User, callbackUrl string) (*model.SendEmailResult, error) {
	ms, err := s.mailServerSvc.GetDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("set-password: get mail server: %w", err)
	}

	token, err := tool.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("set-password: generate token: %w", err)
	}

	ttl := time.Duration(ms.ResetPasswordExpired) * time.Second
	if err := s.cache.Set(ctx, token, user.ID.Hex(), ttl); err != nil {
		return nil, fmt.Errorf("set-password: store token: %w", err)
	}

	name := user.FirstName
	if user.DisplayName != "" {
		name = user.DisplayName
	}

	link, err := appendTokenParam(callbackUrl, token)
	if err != nil {
		return nil, fmt.Errorf("set-password: %w", ErrCallbackUrlInvalid)
	}

	if err := s.mailSvc.SendTemplate(ctx, user.Email, aliasSetPassword, map[string]string{
		"name": name,
		"link": link,
	}); err != nil {
		return nil, err
	}

	return &model.SendEmailResult{
		Email:     user.Email,
		ExpiredAt: tool.NowUTC().Add(ttl),
	}, nil
}

// SendByEmail finds a user by email and sends a set-password email.
func (s *SetPasswordService) SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.Enabled != nil && !*user.Enabled {
		return nil, ErrUserNotEnabledAuth
	}

	return s.Send(ctx, user, callbackUrl)
}

func (s *SetPasswordService) ForgotPassword(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.Verified == nil || !*user.Verified {
		return nil, ErrUserNotVerifiedAuth
	}

	if user.Enabled == nil || !*user.Enabled {
		return nil, ErrUserNotEnabledAuth
	}

	return s.Send(ctx, user, callbackUrl)
}

func (s *SetPasswordService) Verify(ctx context.Context, token string) (*model.SetPasswordTokenInfo, error) {
	userIDHex, err := s.cache.Get(ctx, token)
	if err != nil {
		return nil, ErrTokenInvalidOrExpired
	}

	id, err := bson.ObjectIDFromHex(userIDHex)
	if err != nil {
		return nil, ErrTokenInvalidOrExpired
	}

	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return &model.SetPasswordTokenInfo{
		UserID:    user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
	}, nil
}

// appendTokenParam parses callbackUrl, validates it, appends ?token= (or &token=),
// and returns the final link. Returns ErrCallbackUrlInvalid if the URL is malformed
// or already contains a "token" query param.
func appendTokenParam(callbackUrl, token string) (string, error) {
	u, err := url.Parse(callbackUrl)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrCallbackUrlInvalid
	}
	q := u.Query()
	if q.Has("token") {
		return "", ErrCallbackUrlInvalid
	}
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *SetPasswordService) SetPassword(ctx context.Context, token, password string) error {
	userIDHex, err := s.cache.Get(ctx, token)
	if err != nil {
		return ErrTokenInvalidOrExpired
	}

	id, err := bson.ObjectIDFromHex(userIDHex)
	if err != nil {
		return ErrTokenInvalidOrExpired
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	verified := true
	now := tool.NowUTC()
	if err := s.userRepo.Update(ctx, id, bson.M{
		"password":  string(hashed),
		"verified":  verified,
		"updatedAt": now,
	}); err != nil {
		return err
	}

	_ = s.cache.Delete(ctx, token)
	return nil
}

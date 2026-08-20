package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrUserAlreadyVerified = errors.New("user already verified")

const aliasVerifyEmail = "oryca.verify-email"

type verifyEmailCacher interface {
	Set(ctx context.Context, token, userID string, ttl time.Duration) error
	Get(ctx context.Context, token string) (string, error)
	Delete(ctx context.Context, token string) error
}

type verifyEmailUserRepo interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
}

// verifyEmailGatewayPublisher is the narrow slice of GatewayEventPublisher this service needs.
type verifyEmailGatewayPublisher interface {
	Publish(eventType string, payload any)
}

type VerifyEmailService struct {
	cache     verifyEmailCacher
	userRepo  verifyEmailUserRepo
	mailSvc   *MailService
	ttl       time.Duration
	publisher verifyEmailGatewayPublisher
}

func NewVerifyEmailService(
	cache verifyEmailCacher,
	userRepo verifyEmailUserRepo,
	mailSvc *MailService,
	publisher verifyEmailGatewayPublisher,
	ttl time.Duration,
) *VerifyEmailService {
	return &VerifyEmailService{
		cache:     cache,
		userRepo:  userRepo,
		mailSvc:   mailSvc,
		ttl:       ttl,
		publisher: publisher,
	}
}

// Send generates a one-time token, stores it in Redis, then sends the verify-email to the user.
func (s *VerifyEmailService) Send(ctx context.Context, user *model.User, callbackUrl string) (*model.SendEmailResult, error) {
	token, err := tool.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("verify-email: generate token: %w", err)
	}

	ttl := s.ttl
	if err := s.cache.Set(ctx, token, user.ID.Hex(), ttl); err != nil {
		return nil, fmt.Errorf("verify-email: store token: %w", err)
	}

	name := user.FirstName
	if user.DisplayName != "" {
		name = user.DisplayName
	}

	link, err := appendTokenParam(callbackUrl, token)
	if err != nil {
		return nil, fmt.Errorf("verify-email: %w", ErrCallbackUrlInvalid)
	}

	if err := s.mailSvc.SendTemplate(ctx, user.Email, aliasVerifyEmail, map[string]string{
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

// SendByEmail finds a user by email and sends a verification email.
func (s *VerifyEmailService) SendByEmail(ctx context.Context, email, callbackUrl string) (*model.SendEmailResult, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.Verified != nil && *user.Verified {
		return nil, ErrUserAlreadyVerified
	}

	if user.Enabled != nil && !*user.Enabled {
		return nil, ErrUserNotEnabledAuth
	}

	return s.Send(ctx, user, callbackUrl)
}

// Verify checks the token, marks user as verified, then invalidates the token.
func (s *VerifyEmailService) Verify(ctx context.Context, token string) error {
	userIDHex, err := s.cache.Get(ctx, token)
	if err != nil {
		return ErrTokenInvalidOrExpired
	}

	id, err := bson.ObjectIDFromHex(userIDHex)
	if err != nil {
		return ErrTokenInvalidOrExpired
	}

	verified := true
	now := tool.NowUTC()
	if err := s.userRepo.Update(ctx, id, bson.M{
		"verified":  verified,
		"updatedAt": now,
	}); err != nil {
		return err
	}

	if s.publisher != nil {
		if u, err := s.userRepo.FindByID(ctx, id); err == nil {
			s.publisher.Publish(SyncEventTypeUser, model.ToUserFreshnessCache(u))
		}
	}

	_ = s.cache.Delete(ctx, token)
	return nil
}

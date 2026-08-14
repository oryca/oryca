package service

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailDuplicate    = errors.New("email already exists")
	ErrUsernameDuplicate = errors.New("username already exists")
	ErrPhoneDuplicate    = errors.New("phone already exists")
	ErrCannotDeleteSelf  = errors.New("cannot delete your own account")
	ErrInvalidRole       = errors.New("invalid role")
)

var allowedRoles = map[string]bool{
	"admin": true,
	"user":  true,
}

type userRepo interface {
	FindAll(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByPhone(ctx context.Context, phone string) (*model.User, error)
	FindByUsernameExact(ctx context.Context, username string) (*model.User, error)
	Insert(ctx context.Context, doc *model.User) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
	BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error
}

type setPasswordSender interface {
	Send(ctx context.Context, user *model.User, callbackUrl string) (*model.SendEmailResult, error)
}

// userGatewayPublisher is the narrow slice of GatewayEventPublisher this service needs.
type userGatewayPublisher interface {
	Publish(eventType string, payload any)
}

type UserService struct {
	repo           userRepo
	setPasswordSvc setPasswordSender
	publisher      userGatewayPublisher
}

func NewUserService(repo userRepo, setPasswordSvc setPasswordSender, publisher userGatewayPublisher) *UserService {
	return &UserService{repo: repo, setPasswordSvc: setPasswordSvc, publisher: publisher}
}

func (s *UserService) publishUserEvent(u *model.User) {
	if s.publisher == nil {
		return
	}
	s.publisher.Publish(SyncEventTypeUser, model.ToUserFreshnessCache(u))
}

func (s *UserService) List(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error) {
	return s.repo.FindAll(ctx, params, excludeRoot)
}

func (s *UserService) GetByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (s *UserService) Create(ctx context.Context, body *model.UserCreate) (*model.User, error) {
	if body.Email != "" {
		if existing, err := s.repo.FindByEmail(ctx, body.Email); err == nil && existing != nil {
			return nil, ErrEmailDuplicate
		}
	}
	if body.Username != "" {
		if existing, err := s.repo.FindByUsernameExact(ctx, body.Username); err == nil && existing != nil {
			return nil, ErrUsernameDuplicate
		}
	}
	if body.Phone != "" {
		if existing, err := s.repo.FindByPhone(ctx, body.Phone); err == nil && existing != nil {
			return nil, ErrPhoneDuplicate
		}
	}

	doc, err := buildUserDoc(body)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	if body.CallbackUrl != "" && s.setPasswordSvc != nil {
		if _, err := s.setPasswordSvc.Send(ctx, doc, body.CallbackUrl); err != nil {
			logger.Error("set-password email failed for " + doc.Email + ": " + err.Error())
		}
	}

	s.publishUserEvent(doc)
	return doc, nil
}

func buildUserDoc(body *model.UserCreate) (*model.User, error) {
	role := body.Role
	if role == "" {
		role = "user"
	}
	if !allowedRoles[role] {
		return nil, ErrInvalidRole
	}

	hashedPassword := ""
	if body.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hashedPassword = string(h)
	}

	verified := false
	enabled := true
	if body.Verified != nil {
		verified = *body.Verified
	}
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	now := tool.NowUTC()
	return &model.User{
		ID:           bson.NewObjectID(),
		AccountType:  body.AccountType,
		Email:        body.Email,
		Phone:        body.Phone,
		CountryCode:  body.CountryCode,
		Username:     body.Username,
		Password:     hashedPassword,
		FirstName:    body.FirstName,
		LastName:     body.LastName,
		DisplayName:  body.DisplayName,
		Organization: body.Organization,
		Avatar:       body.Avatar,
		Role:         role,
		PackageID:    body.PackageID,
		Verified:     &verified,
		Enabled:      &enabled,
		ExpiredAt:    body.ExpiredAt,
		Properties:   body.Properties,
		CreatedAt:    &now,
	}, nil
}

func (s *UserService) Update(ctx context.Context, id bson.ObjectID, body *model.UserUpdate) (*model.User, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, ErrUserNotFound
	}

	fields, err := buildUserUpdateFields(body)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.publishUserEvent(updated)
	return updated, nil
}

func buildUserUpdateFields(body *model.UserUpdate) (bson.M, error) {
	role := body.Role
	if role == "" {
		role = "user"
	}
	if !allowedRoles[role] {
		return nil, ErrInvalidRole
	}

	verified := false
	if body.Verified != nil {
		verified = *body.Verified
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	fields := bson.M{
		"updatedAt":    tool.NowUTC(),
		"username":     body.Username,
		"firstName":    body.FirstName,
		"lastName":     body.LastName,
		"displayName":  body.DisplayName,
		"organization": body.Organization,
		"avatar":       body.Avatar,
		"role":         role,
		"packageId":    body.PackageID,
		"verified":     verified,
		"enabled":      enabled,
		"expiredAt":    body.ExpiredAt,
		"properties":   body.Properties,
	}

	if body.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		fields["password"] = string(h)
	}

	return fields, nil
}

func (s *UserService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUserID bson.ObjectID) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return ErrUserNotFound
	}

	if id == ctxUserID {
		return ErrCannotDeleteSelf
	}

	now := tool.NowUTC()
	var deleteErr error
	if forever {
		deleteErr = s.repo.HardDelete(ctx, id)
	} else {
		deleteErr = s.repo.SoftDelete(ctx, id, now, ctxUserID)
	}
	if deleteErr != nil {
		return deleteErr
	}

	// Synthesize the payload rather than refetching. A deleted user is filtered out of
	// FindByID entirely, and SoftDelete never touches the enabled field, so a refetch
	// would give the gateway stale "still enabled" data. This is what actually revokes
	// gateway access on delete.
	if s.publisher != nil {
		disabled := false
		s.publisher.Publish(SyncEventTypeUser, &model.UserFreshnessCache{ID: id.Hex(), Enabled: &disabled})
	}
	return nil
}

func (s *UserService) BulkDelete(ctx context.Context, body *model.UserBulkDelete, ctxUserID bson.ObjectID) error {
	for _, id := range body.UserIDs {
		if id == ctxUserID {
			return ErrCannotDeleteSelf
		}
	}

	now := tool.NowUTC()
	var deleteErr error
	if body.Forever {
		deleteErr = s.repo.BulkHardDelete(ctx, body.UserIDs)
	} else {
		deleteErr = s.repo.BulkSoftDelete(ctx, body.UserIDs, now, ctxUserID)
	}
	if deleteErr != nil {
		return deleteErr
	}

	// One thin invalidate event per call, never one per user. A group-driven bulk
	// delete can touch hundreds of ids in a single Mongo call, and publishing per-id
	// here would reproduce the event-storm bug already fixed for reload_services.
	if s.publisher != nil {
		hexIDs := make([]string, len(body.UserIDs))
		for i, id := range body.UserIDs {
			hexIDs[i] = id.Hex()
		}
		s.publisher.Publish(SyncEventTypeUserInvalidate, &model.UserIDsPayload{UserIDs: hexIDs})
	}
	return nil
}

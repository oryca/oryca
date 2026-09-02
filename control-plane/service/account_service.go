package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/cache"
	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrChangePasswordNoLocal means the user has no bcrypt password (e.g. IDP-only).
	ErrChangePasswordNoLocal = errors.New("change password: no local password set")
	// ErrChangePasswordWrongCurrent means old password did not match.
	ErrChangePasswordWrongCurrent = errors.New("change password: current password incorrect")
	// ErrChangePasswordUnchanged means new password equals the current one.
	ErrChangePasswordUnchanged = errors.New("change password: new password must differ from current")
	// ErrChangePasswordHashFailed means bcrypt hashing the new password failed.
	ErrChangePasswordHashFailed = errors.New("change password: hash failed")
)

type accountUserRepo interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
}

type accountSessionCache interface {
	ScanByUser(ctx context.Context, userID string) ([]string, error)
	Delete(ctx context.Context, keys []string) error
}

type accountUserSessionRepo interface {
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.UserSession, error)
	FindHistoryByUserID(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error)
	UpdateLogout(ctx context.Context, sessionID bson.ObjectID, logoutAt time.Time, reason string) error
	BulkUpdateLogout(ctx context.Context, sessionIDs []bson.ObjectID, logoutAt time.Time, reason string) error
}

type AccountService struct {
	userRepo        accountUserRepo
	sessionCache    accountSessionCache
	userSessionRepo accountUserSessionRepo
}

func NewAccountService(userRepo accountUserRepo, sessionCache accountSessionCache, userSessionRepo accountUserSessionRepo) *AccountService {
	return &AccountService{
		userRepo:        userRepo,
		sessionCache:    sessionCache,
		userSessionRepo: userSessionRepo,
	}
}

// ChangePassword updates the user's bcrypt password after verifying the current password.
func (s *AccountService) ChangePassword(ctx context.Context, userID bson.ObjectID, oldPassword, newPassword string) (*model.User, error) {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if u.Password == "" {
		return nil, ErrChangePasswordNoLocal
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(oldPassword)); err != nil {
		return nil, ErrChangePasswordWrongCurrent
	}
	if oldPassword == newPassword {
		return nil, ErrChangePasswordUnchanged
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrChangePasswordHashFailed
	}
	if err := s.userRepo.Update(ctx, userID, bson.M{
		"password":  string(hashed),
		"updatedAt": tool.NowUTC(),
	}); err != nil {
		return nil, err
	}
	return s.userRepo.FindByID(ctx, userID)
}

func (s *AccountService) UpdateProfile(ctx context.Context, userID bson.ObjectID, body *model.AccountProfileUpdate) (*model.User, error) {
	fields := bson.M{
		"updatedAt":    tool.NowUTC(),
		"username":     body.Username,
		"firstName":    body.FirstName,
		"lastName":     body.LastName,
		"displayName":  body.DisplayName,
		"avatar":       body.Avatar,
		"organization": body.Organization,
	}
	if body.PasswordSkipped != nil {
		fields["passwordSkipped"] = *body.PasswordSkipped
	}

	if err := s.userRepo.Update(ctx, userID, fields); err != nil {
		return nil, err
	}

	return s.userRepo.FindByID(ctx, userID)
}

func (s *AccountService) DeleteProfile(ctx context.Context, userID bson.ObjectID) error {
	return s.userRepo.SoftDelete(ctx, userID, tool.NowUTC(), userID)
}

func (s *AccountService) GetSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) ([]*model.Session, error) {
	// Redis is source of truth for active sessions
	keys, err := s.sessionCache.ScanByUser(ctx, userID.Hex())
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return []*model.Session{}, nil
	}

	// parse session IDs from Redis keys
	activeIDs := make([]bson.ObjectID, 0, len(keys))
	for _, k := range keys {
		if id, err := parseSessionIDFromKey(k); err == nil {
			activeIDs = append(activeIDs, id)
		}
	}

	// enrich with MongoDB metadata (browser, loginAt)
	metaMap := make(map[bson.ObjectID]*model.UserSession)
	if len(activeIDs) > 0 {
		docs, err := s.userSessionRepo.FindByIDs(ctx, activeIDs)
		if err == nil {
			for _, d := range docs {
				metaMap[d.ID] = d
			}
		}
	}

	sessions := make([]*model.Session, 0, len(activeIDs))
	for _, id := range activeIDs {
		s := &model.Session{
			ID:      id,
			Current: id == currentSessionID,
		}
		if meta, ok := metaMap[id]; ok {
			s.Browser = meta.Browser
			s.OS = meta.OS
			s.IPAddress = meta.IPAddress
			s.LoginAt = meta.LoginAt
		}
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LoginAt.After(sessions[j].LoginAt)
	})
	return sessions, nil
}

func (s *AccountService) DeleteSession(ctx context.Context, sessionID, userID bson.ObjectID) error {
	key := cache.SessionKey(sessionID.Hex(), userID.Hex())
	if err := s.sessionCache.Delete(ctx, []string{key}); err != nil {
		return err
	}

	now := tool.NowUTC()
	if err := s.userSessionRepo.UpdateLogout(ctx, sessionID, now, "logout"); err != nil {
		logger.Error("failed to update logout for session " + sessionID.Hex() + ": " + err.Error())
	}

	return nil
}

func (s *AccountService) DeleteAllSessions(ctx context.Context, userID bson.ObjectID, currentSessionID bson.ObjectID) error {
	keys, err := s.sessionCache.ScanByUser(ctx, userID.Hex())
	if err != nil {
		return err
	}

	currentKey := cache.SessionKey(currentSessionID.Hex(), userID.Hex())
	var otherKeys []string
	var otherSessionIDs []bson.ObjectID
	for _, k := range keys {
		if k != currentKey {
			otherKeys = append(otherKeys, k)
			if id, err := parseSessionIDFromKey(k); err == nil {
				otherSessionIDs = append(otherSessionIDs, id)
			}
		}
	}

	if len(otherKeys) == 0 {
		return nil
	}

	if err := s.sessionCache.Delete(ctx, otherKeys); err != nil {
		return err
	}

	if len(otherSessionIDs) > 0 {
		now := tool.NowUTC()
		if err := s.userSessionRepo.BulkUpdateLogout(ctx, otherSessionIDs, now, "logout_all"); err != nil {
			logger.Error("failed to bulk update logout for user " + userID.Hex() + ": " + err.Error())
		}
	}

	return nil
}

func (s *AccountService) GetSessionHistory(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error) {
	return s.userSessionRepo.FindHistoryByUserID(ctx, userID)
}

// parseSessionIDFromKey แยก sessionID จาก key รูปแบบ "session:{id},user:{uid}"
func parseSessionIDFromKey(key string) (bson.ObjectID, error) {
	parts := strings.SplitN(key, ",", 2)
	if len(parts) == 0 {
		return bson.NilObjectID, ErrUserNotFound
	}
	hex := strings.TrimPrefix(parts[0], "session:")
	return bson.ObjectIDFromHex(hex)
}

package service

import (
	"context"
	"errors"
	"time"

	"github.com/oryca/oryca/control-plane/cache"
	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrIncorrectPassword   = errors.New("incorrect email or password")
	ErrUserNotVerifiedAuth = errors.New("user not verified")
	ErrUserNotEnabledAuth  = errors.New("user not enabled")
	ErrRegisterNotEnabled  = errors.New("registration is not enabled")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
)

type authConfigSvc interface {
	Get(ctx context.Context) (*model.Configuration, error)
}

type authSessionCache interface {
	Set(ctx context.Context, key string, ttl time.Duration) error
	Get(ctx context.Context, key string) error
	Delete(ctx context.Context, keys []string) error
	ScanByUser(ctx context.Context, userID string) ([]string, error)
}

type authUserRepo interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByPhone(ctx context.Context, phone string) (*model.User, error)
	FindByUsernameExact(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	Insert(ctx context.Context, doc *model.User) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
}

type authUserSessionRepo interface {
	Insert(ctx context.Context, doc *model.UserSession) error
	UpdateLogout(ctx context.Context, sessionID bson.ObjectID, logoutAt time.Time, reason string) error
	BulkUpdateLogout(ctx context.Context, sessionIDs []bson.ObjectID, logoutAt time.Time, reason string) error
}

type authUserCache interface {
	SetUser(ctx context.Context, user *model.User, ttl time.Duration) error
	DeleteUser(ctx context.Context, userID string) error
}

// authPackageFinder หา package ที่ user สมัครเองจะได้รับ (ตาม config register.defaultPackageAlias)
type authPackageFinder interface {
	FindByAlias(ctx context.Context, alias string) (*model.Package, error)
}

type AuthService struct {
	configSvc      authConfigSvc
	packageRepo    authPackageFinder
	userRepo       authUserRepo
	sessionCache   authSessionCache
	sessionRepo    authUserSessionRepo
	userCache      authUserCache
	verifyEmailSvc *VerifyEmailService
}

func NewAuthService(configSvc authConfigSvc, userRepo authUserRepo, packageRepo authPackageFinder, sessionCache authSessionCache, sessionRepo authUserSessionRepo, userCache authUserCache, verifyEmailSvc *VerifyEmailService) *AuthService {
	return &AuthService{
		configSvc:      configSvc,
		packageRepo:    packageRepo,
		userRepo:       userRepo,
		sessionCache:   sessionCache,
		sessionRepo:    sessionRepo,
		userCache:      userCache,
		verifyEmailSvc: verifyEmailSvc,
	}
}

func (s *AuthService) Login(ctx context.Context, username, password, deviceInfo, ipAddress string) (*model.Auth, error) {
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil || user == nil {
		return nil, ErrIncorrectPassword
	}

	if user.Verified != nil && !*user.Verified {
		return nil, ErrUserNotVerifiedAuth
	}

	if user.Enabled != nil && !*user.Enabled {
		return nil, ErrUserNotEnabledAuth
	}

	if user.Password == "" {
		return nil, ErrIncorrectPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrIncorrectPassword
	}

	cfg, err := s.configSvc.Get(ctx)
	if err != nil || cfg == nil || cfg.Token == nil {
		return nil, errors.New("configuration not found")
	}

	auth, err := s.generateTokens(ctx, user, cfg.Token, deviceInfo, ipAddress)
	if err != nil {
		return nil, err
	}

	// update lastLogin best-effort
	now := tool.NowUTC()
	if updateErr := s.userRepo.Update(ctx, user.ID, bson.M{"lastLogin": now}); updateErr != nil {
		logger.Error("failed to update lastLogin for user " + user.ID.Hex() + ": " + updateErr.Error())
	}

	return auth, nil
}

func (s *AuthService) Register(ctx context.Context, body *model.RegisterRequest) (*model.User, error) {
	cfg, err := s.configSvc.Get(ctx)
	if err != nil {
		return nil, errors.New("configuration not found")
	}

	if cfg.Register == nil || !cfg.Register.Enabled {
		return nil, ErrRegisterNotEnabled
	}

	if existing, err := s.userRepo.FindByEmail(ctx, body.Email); err == nil && existing != nil {
		return nil, ErrEmailDuplicate
	}
	if body.Username != "" {
		if existing, err := s.userRepo.FindByUsernameExact(ctx, body.Username); err == nil && existing != nil {
			return nil, ErrUsernameDuplicate
		}
	}
	if body.Phone != "" {
		if existing, err := s.userRepo.FindByPhone(ctx, body.Phone); err == nil && existing != nil {
			return nil, ErrPhoneDuplicate
		}
	}

	createBody := &model.UserCreate{
		AccountType: "email",
		Email:       body.Email,
		Username:    body.Username,
		Phone:       body.Phone,
		CountryCode: body.CountryCode,
		Password:    body.Password,
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		DisplayName: body.DisplayName,
		Role:        "user", // force role on self-register
	}

	// package ตั้งต้นของคนสมัครเอง. หาไม่เจอก็ปล่อยว่าง (admin ผูกให้ทีหลังได้) ไม่ block การสมัคร
	if alias := cfg.Register.DefaultPackageAlias; alias != "" && s.packageRepo != nil {
		if pkg, err := s.packageRepo.FindByAlias(ctx, alias); err == nil && pkg != nil {
			createBody.PackageID = &pkg.ID
		} else {
			logger.Error("register: default package " + alias + " not found, user created without package")
		}
	}

	// handle trial expiry
	if cfg.Register.TrialExpiresIn != nil && *cfg.Register.TrialExpiresIn > 0 {
		expiredAt := tool.NowUTC().Add(time.Second * time.Duration(*cfg.Register.TrialExpiresIn))
		createBody.ExpiredAt = &expiredAt
	}

	doc, err := buildUserDoc(createBody)
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	// ส่ง verify email best-effort (non-blocking)
	if body.CallbackUrl != "" && s.verifyEmailSvc != nil {
		go func() {
			if _, err := s.verifyEmailSvc.Send(context.WithoutCancel(ctx), doc, body.CallbackUrl); err != nil {
				logger.Error("verify-email send failed for " + doc.Email + ": " + err.Error())
			}
		}()
	}

	return doc, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.Auth, error) {
	claims, err := tool.ValidateJWT(refreshToken)
	if err != nil || claims == nil {
		return nil, ErrInvalidRefreshToken
	}

	if claims.GrantType != tool.GrantTypeRefreshToken {
		return nil, ErrInvalidRefreshToken
	}

	// verify session still exists
	sessionKey := cache.SessionKey(claims.Session.Hex(), claims.UserID.Hex())
	if err := s.sessionCache.Get(ctx, sessionKey); err != nil {
		return nil, ErrInvalidRefreshToken
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil || user == nil {
		return nil, ErrInvalidRefreshToken
	}

	cfg, err := s.configSvc.Get(ctx)
	if err != nil || cfg == nil || cfg.Token == nil {
		return nil, errors.New("configuration not found")
	}

	// delete old session, create new tokens with same sessionId
	_ = s.sessionCache.Delete(ctx, []string{sessionKey})

	return s.generateTokensWithSession(ctx, user, cfg.Token, claims.Session, "")
}

func (s *AuthService) Logout(ctx context.Context, sessionID, userID bson.ObjectID) error {
	key := cache.SessionKey(sessionID.Hex(), userID.Hex())
	if err := s.sessionCache.Delete(ctx, []string{key}); err != nil {
		return err
	}

	// update logout info in MongoDB best-effort
	if updateErr := s.sessionRepo.UpdateLogout(ctx, sessionID, tool.NowUTC(), "logout"); updateErr != nil {
		logger.Error("failed to update user_session logout for " + sessionID.Hex() + ": " + updateErr.Error())
	}

	return nil
}

func (s *AuthService) generateTokens(ctx context.Context, user *model.User, cfg *model.ConfigToken, deviceInfo, ipAddress string) (*model.Auth, error) {
	if cfg.MaxSessions > 0 {
		enforceMaxSessions(ctx, s.sessionCache, s.sessionRepo, user.ID.Hex(), cfg.MaxSessions)
	}
	sessionID := bson.NewObjectID()
	auth, err := s.generateTokensWithSession(ctx, user, cfg, sessionID, deviceInfo)
	if err != nil {
		return nil, err
	}

	// persist new session to MongoDB best-effort
	doc := &model.UserSession{
		ID:         sessionID,
		UserID:     user.ID,
		DeviceInfo: deviceInfo,
		Browser:    tool.ParseBrowser(deviceInfo),
		OS:         tool.ParseOS(deviceInfo),
		IPAddress:  ipAddress,
		LoginAt:    tool.NowUTC(),
	}
	if insertErr := s.sessionRepo.Insert(ctx, doc); insertErr != nil {
		logger.Error("failed to insert user_session for " + user.ID.Hex() + ": " + insertErr.Error())
	}

	return auth, nil
}

func (s *AuthService) generateTokensWithSession(ctx context.Context, user *model.User, cfg *model.ConfigToken, sessionID bson.ObjectID, deviceInfo string) (*model.Auth, error) {
	now := tool.NowUTC()

	accessTTL := time.Second * time.Duration(cfg.AccessTokenExpired)
	refreshTTL := time.Second * time.Duration(cfg.RefreshTokenExpired)

	accessTokenClaims := &tool.JWTCustomClaims{
		Session:   sessionID,
		UserID:    user.ID,
		GrantType: tool.GrantTypeAccessToken,
		Role:      user.Role,
		PackageID: tool.ObjectIDHex(user.PackageID),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	accessToken, err := tool.GenerateJWT(accessTokenClaims)
	if err != nil {
		return nil, err
	}

	refreshTokenClaims := &tool.JWTCustomClaims{
		Session:   sessionID,
		UserID:    user.ID,
		GrantType: tool.GrantTypeRefreshToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(refreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	refreshToken, err := tool.GenerateJWT(refreshTokenClaims)
	if err != nil {
		return nil, err
	}

	// store session in Redis (TTL = refresh token lifetime)
	sessionKey := cache.SessionKey(sessionID.Hex(), user.ID.Hex())
	if err := s.sessionCache.Set(ctx, sessionKey, refreshTTL); err != nil {
		return nil, err
	}

	// cache user สำหรับ gateway (TTL = refresh token lifetime)
	if err := s.userCache.SetUser(ctx, user, refreshTTL); err != nil {
		logger.Error("auth: cache user failed: " + err.Error())
	}

	expiredAt := now.Add(accessTTL)
	expiresIn := int64(accessTTL.Seconds())

	return &model.Auth{
		User:         user,
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		ExpiredAt:    &expiredAt,
		RefreshToken: refreshToken,
	}, nil
}

type maxSessionCache interface {
	ScanByUser(ctx context.Context, userID string) ([]string, error)
	Delete(ctx context.Context, keys []string) error
}

type maxSessionRepo interface {
	BulkUpdateLogout(ctx context.Context, sessionIDs []bson.ObjectID, logoutAt time.Time, reason string) error
}

// enforceMaxSessions revokes the oldest sessions if the user exceeds max active sessions.
func enforceMaxSessions(ctx context.Context, sc maxSessionCache, repo maxSessionRepo, userID string, max int) {
	keys, err := sc.ScanByUser(ctx, userID)
	if err != nil || len(keys) < max {
		return
	}
	toRevoke := keys[:len(keys)-max+1]
	_ = sc.Delete(ctx, toRevoke)

	var ids []bson.ObjectID
	for _, k := range toRevoke {
		if hex := cache.ParseSessionID(k); hex != "" {
			if oid, err := bson.ObjectIDFromHex(hex); err == nil {
				ids = append(ids, oid)
			}
		}
	}
	if len(ids) > 0 {
		if err := repo.BulkUpdateLogout(ctx, ids, tool.NowUTC(), "max_sessions"); err != nil {
			logger.Error("enforceMaxSessions: BulkUpdateLogout failed: " + err.Error())
		}
	}
}

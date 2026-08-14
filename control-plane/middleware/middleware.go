package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/cache"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const msgTokenInvalidOrExpired = "Access token is invalid or expired"

type sessionCache interface {
	Get(ctx context.Context, key string) error
}

type userFinder interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
}

type JWTMiddleware struct {
	sessionCache sessionCache
	userRepo     userFinder
}

func NewJWTMiddleware(sessionCache sessionCache, userRepo userFinder) *JWTMiddleware {
	return &JWTMiddleware{sessionCache: sessionCache, userRepo: userRepo}
}

func (m *JWTMiddleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := extractBearerToken(c)
		if token == "" {
			return c.JSON(http.StatusUnauthorized, &model.Exception{
				Code:   tool.CodeUnauthorizedAccess,
				Status: http.StatusUnauthorized,
				Detail: "Authentication required",
			})
		}

		user, sessionID, errResp := m.resolveToken(c.Request().Context(), token)
		if errResp != nil {
			return c.JSON(http.StatusUnauthorized, errResp)
		}

		if checkErr := checkUserStatus(c, user); checkErr != nil {
			return checkErr
		}

		c.Set("user", user)
		c.Set("session", sessionID)
		return next(c)
	}
}

// OptionalAuthenticate ตรวจ JWT ถ้ามี token แนบมา แต่ไม่บล็อกถ้าไม่มี
func (m *JWTMiddleware) OptionalAuthenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := extractBearerToken(c)
		if token == "" {
			return next(c)
		}

		user, sessionID, errResp := m.resolveToken(c.Request().Context(), token)
		if errResp != nil {
			return next(c)
		}

		c.Set("user", user)
		c.Set("session", sessionID)
		return next(c)
	}
}

// resolveToken validates token and returns user without writing to response.
func (m *JWTMiddleware) resolveToken(ctx context.Context, token string) (*model.User, bson.ObjectID, *model.Exception) {
	invalidErr := &model.Exception{
		Code:   tool.CodeInvalidToken,
		Status: http.StatusUnauthorized,
		Detail: msgTokenInvalidOrExpired,
	}

	claims, err := tool.ValidateJWT(token)
	if err != nil || claims == nil || claims.GrantType != tool.GrantTypeAccessToken {
		return nil, bson.NilObjectID, invalidErr
	}

	sessionKey := cache.SessionKey(claims.Session.Hex(), claims.UserID.Hex())
	if err := m.sessionCache.Get(ctx, sessionKey); err != nil {
		return nil, bson.NilObjectID, &model.Exception{
			Code:   tool.CodeSessionExpired,
			Status: http.StatusUnauthorized,
			Detail: "Session expired",
		}
	}

	user, err := m.userRepo.FindByID(ctx, claims.UserID)
	if err != nil || user == nil {
		return nil, bson.NilObjectID, invalidErr
	}

	return user, claims.Session, nil
}

func checkUserStatus(c echo.Context, user *model.User) error {
	if user.Verified != nil && !*user.Verified {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUserNotVerified,
			Status: http.StatusForbidden,
			Detail: "User not verified",
		})
	}

	if user.Enabled != nil && !*user.Enabled {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUserNotEnabled,
			Status: http.StatusForbidden,
			Detail: "User not enabled",
		})
	}

	now := tool.NowUTC()
	if user.ExpiredAt != nil && !user.ExpiredAt.After(now) {
		return c.JSON(http.StatusPaymentRequired, &model.Exception{
			Code:   tool.CodeTrialExpired,
			Status: http.StatusPaymentRequired,
			Detail: "Trial expired",
		})
	}

	return nil
}

func extractBearerToken(c echo.Context) string {
	auth := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// --- ApiKeyMiddleware ---

type apiKeyLookup interface {
	GetByKey(ctx context.Context, keyValue string) (*model.ApiKey, error)
}

type ApiKeyMiddleware struct {
	apiKeySvc apiKeyLookup
	userRepo  userFinder
}

func NewApiKeyMiddleware(apiKeySvc apiKeyLookup, userRepo userFinder) *ApiKeyMiddleware {
	return &ApiKeyMiddleware{apiKeySvc: apiKeySvc, userRepo: userRepo}
}

const msgApiKeyInvalid = "API key is invalid"

// Authenticate ตรวจสอบ API key จาก header X-Api-Key หรือ query param api_key
func (m *ApiKeyMiddleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		keyValue := extractApiKey(c)
		if keyValue == "" {
			return c.JSON(http.StatusUnauthorized, &model.Exception{
				Code:   tool.CodeUnauthorizedAccess,
				Status: http.StatusUnauthorized,
				Detail: "API key is required",
			})
		}

		ak, err := m.apiKeySvc.GetByKey(c.Request().Context(), keyValue)
		if err != nil || ak == nil {
			return c.JSON(http.StatusUnauthorized, &model.Exception{
				Code:   tool.CodeApiKeyNotFound,
				Status: http.StatusUnauthorized,
				Detail: msgApiKeyInvalid,
			})
		}

		if errResp := validateApiKey(c, ak); errResp != nil {
			return errResp
		}

		user, errResp := m.resolveApiKeyUser(c, ak)
		if errResp != nil {
			return errResp
		}

		if checkErr := checkUserStatus(c, user); checkErr != nil {
			return checkErr
		}

		c.Set("user", user)
		c.Set("authType", "apikey")
		return next(c)
	}
}

func validateApiKey(c echo.Context, ak *model.ApiKey) error {
	if ak.Enabled != nil && !*ak.Enabled {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeApiKeyDisabled,
			Status: http.StatusForbidden,
			Detail: "API key is disabled",
		})
	}

	if ak.ExpiredAt != nil && ak.ExpiredAt.Before(time.Now().UTC()) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeApiKeyExpired,
			Status: http.StatusForbidden,
			Detail: "API key has expired",
		})
	}

	return nil
}

func (m *ApiKeyMiddleware) resolveApiKeyUser(c echo.Context, ak *model.ApiKey) (*model.User, error) {
	if ak.CreatedBy == nil {
		return nil, c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeApiKeyNotFound,
			Status: http.StatusUnauthorized,
			Detail: msgApiKeyInvalid,
		})
	}

	user, err := m.userRepo.FindByID(c.Request().Context(), *ak.CreatedBy)
	if err != nil || user == nil {
		return nil, c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeApiKeyNotFound,
			Status: http.StatusUnauthorized,
			Detail: msgApiKeyInvalid,
		})
	}

	return user, nil
}

func extractApiKey(c echo.Context) string {
	if key := c.Request().Header.Get("X-Api-Key"); key != "" {
		return key
	}
	return c.QueryParam("api_key")
}

// secretEqual เทียบ secret แบบ constant-time กัน timing attack
func secretEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// InternalKeyMiddleware ตรวจ X-Internal-Key header สำหรับ gateway-to-controlplane calls
// รองรับ secret rotation ด้วย secretPrev (ถ้าว่างให้ skip)
func InternalKeyMiddleware(secret, secretPrev string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if secret == "" {
				return c.JSON(http.StatusServiceUnavailable, &model.Exception{
					Code:   tool.CodeUnauthorizedAccess,
					Status: http.StatusServiceUnavailable,
					Detail: "Internal secret not configured",
				})
			}
			key := c.Request().Header.Get("X-Internal-Key")
			if !secretEqual(key, secret) && (secretPrev == "" || !secretEqual(key, secretPrev)) {
				return c.JSON(http.StatusUnauthorized, &model.Exception{
					Code:   tool.CodeUnauthorizedAccess,
					Status: http.StatusUnauthorized,
					Detail: "Unauthorized",
				})
			}
			return next(c)
		}
	}
}

// AllowJWTOrApiKey ลอง JWT ก่อน ถ้าไม่มี Bearer token ค่อยลอง API key
func AllowJWTOrApiKey(jwtMW *JWTMiddleware, apiKeyMW *ApiKeyMiddleware) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if extractBearerToken(c) != "" {
				return jwtMW.Authenticate(next)(c)
			}
			return apiKeyMW.Authenticate(next)(c)
		}
	}
}

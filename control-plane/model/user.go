package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID              bson.ObjectID          `json:"id" bson:"_id,omitempty"`
	AccountType     string                 `json:"accountType" bson:"accountType"`
	Email           string                 `json:"email,omitempty" bson:"email,omitempty"`
	Phone           string                 `json:"phone,omitempty" bson:"phone,omitempty"`
	CountryCode     string                 `json:"countryCode,omitempty" bson:"countryCode,omitempty"`
	Username        string                 `json:"username,omitempty" bson:"username,omitempty"`
	Password        string                 `json:"-" bson:"password,omitempty"`
	PasswordSkipped bool                   `json:"-" bson:"passwordSkipped"`
	FirstName       string                 `json:"firstName,omitempty" bson:"firstName,omitempty"`
	LastName        string                 `json:"lastName,omitempty" bson:"lastName,omitempty"`
	DisplayName     string                 `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Organization    string                 `json:"organization,omitempty" bson:"organization,omitempty"`
	Avatar          string                 `json:"avatar,omitempty" bson:"avatar,omitempty"`
	Role            string                 `json:"role" bson:"role"`
	PackageID       *bson.ObjectID         `json:"packageId,omitempty" bson:"packageId,omitempty"`
	Verified        *bool                  `json:"verified,omitempty" bson:"verified,omitempty"`
	Enabled         *bool                  `json:"enabled,omitempty" bson:"enabled,omitempty"`
	LastLogin       *time.Time             `json:"lastLogin,omitempty" bson:"lastLogin,omitempty"`
	ExpiredAt       *time.Time             `json:"expiredAt,omitempty" bson:"expiredAt,omitempty"`
	Properties      map[string]interface{} `json:"properties,omitempty" bson:"properties,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type UserCreate struct {
	AccountType  string                 `json:"accountType"`
	Email        string                 `json:"email"`
	Phone        string                 `json:"phone"`
	CountryCode  string                 `json:"countryCode"`
	Username     string                 `json:"username"`
	Password     string                 `json:"password"`
	FirstName    string                 `json:"firstName"`
	LastName     string                 `json:"lastName"`
	DisplayName  string                 `json:"displayName"`
	Organization string                 `json:"organization"`
	Avatar       string                 `json:"avatar"`
	Role         string                 `json:"role"`
	PackageID    *bson.ObjectID         `json:"packageId"`
	Verified     *bool                  `json:"verified"`
	Enabled      *bool                  `json:"enabled"`
	ExpiredAt    *time.Time             `json:"expiredAt"`
	Properties   map[string]interface{} `json:"properties"`
	CallbackUrl  string                 `json:"callbackUrl"`
}

type SetPasswordTokenInfo struct {
	UserID    bson.ObjectID `json:"userId"`
	Email     string        `json:"email"`
	FirstName string        `json:"firstName"`
}

type SetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type UserUpdate struct {
	Username     string                 `json:"username"`
	Password     string                 `json:"password"`
	FirstName    string                 `json:"firstName"`
	LastName     string                 `json:"lastName"`
	DisplayName  string                 `json:"displayName"`
	Organization string                 `json:"organization"`
	Avatar       string                 `json:"avatar"`
	Role         string                 `json:"role"`
	PackageID    *bson.ObjectID         `json:"packageId"`
	Verified     *bool                  `json:"verified"`
	Enabled      *bool                  `json:"enabled"`
	ExpiredAt    *time.Time             `json:"expiredAt"`
	Properties   map[string]interface{} `json:"properties"`
}

type UserPublic struct {
	ID          bson.ObjectID `json:"id" bson:"_id"`
	FirstName   string        `json:"firstName,omitempty"`
	LastName    string        `json:"lastName,omitempty"`
	Phone       string        `json:"phone,omitempty"`
	Username    string        `json:"username,omitempty"`
	DisplayName string        `json:"displayName,omitempty"`
	Email       string        `json:"email,omitempty"`
	Avatar      string        `json:"avatar,omitempty"`
	Role        string        `json:"role,omitempty"`
}

type UserBulkDelete struct {
	UserIDs []bson.ObjectID `json:"userIds"`
	Forever bool            `json:"forever"`
}

type UserProfile struct {
	ID              bson.ObjectID          `json:"id"`
	AccountType     string                 `json:"accountType"`
	Email           string                 `json:"email,omitempty"`
	Phone           string                 `json:"phone,omitempty"`
	CountryCode     string                 `json:"countryCode,omitempty"`
	Username        string                 `json:"username,omitempty"`
	FirstName       string                 `json:"firstName,omitempty"`
	LastName        string                 `json:"lastName,omitempty"`
	DisplayName     string                 `json:"displayName,omitempty"`
	Organization    string                 `json:"organization,omitempty"`
	Avatar          string                 `json:"avatar,omitempty"`
	Role            string                 `json:"role"`
	PackageID       *bson.ObjectID         `json:"packageId,omitempty"`
	Verified        *bool                  `json:"verified,omitempty"`
	Enabled         *bool                  `json:"enabled,omitempty"`
	LastLogin       *time.Time             `json:"lastLogin,omitempty"`
	ExpiredAt       *time.Time             `json:"expiredAt,omitempty"`
	Properties      map[string]interface{} `json:"properties,omitempty"`
	HasPassword     bool                   `json:"hasPassword"`
	PasswordSkipped bool                   `json:"passwordSkipped"`
	CreatedAt       *time.Time             `json:"createdAt,omitempty"`
	UpdatedAt       *time.Time             `json:"updatedAt,omitempty"`
}

// UserFreshnessCache is the gateway-facing snapshot of the authorization-relevant
// fields that must never go stale. Used by both the GET /internal/users/:id
// on-demand lookup and the SyncEventTypeUser real-time event. Role stays
// out of scope here: it's baked into the JWT at login like PackageID always was,
// but only PackageID/Enabled/Verified/ExpiredAt are the ones gateway must re-check.
type UserFreshnessCache struct {
	ID        string     `json:"id"`
	PackageID string     `json:"packageId,omitempty"`
	Verified  *bool      `json:"verified,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiredAt *time.Time `json:"expiredAt,omitempty"`
}

// UserIDsPayload is the thin bulk-invalidation payload for AssignUsers/RemoveUsers/
// BulkDelete. Carries only affected ids, never full user data, since those operations
// touch many users in one Mongo call and a full payload per user would reproduce the
// event-storm bug already fixed for reload_services.
type UserIDsPayload struct {
	UserIDs []string `json:"userIds"`
}

// ToUserFreshnessCache builds the gateway-facing snapshot from a full user document.
func ToUserFreshnessCache(u *User) *UserFreshnessCache {
	packageID := ""
	if u.PackageID != nil {
		packageID = u.PackageID.Hex()
	}
	return &UserFreshnessCache{
		ID:        u.ID.Hex(),
		PackageID: packageID,
		Verified:  u.Verified,
		Enabled:   u.Enabled,
		ExpiredAt: u.ExpiredAt,
	}
}

func ToUserProfile(u *User) *UserProfile {
	return &UserProfile{
		ID:              u.ID,
		AccountType:     u.AccountType,
		Email:           u.Email,
		Phone:           u.Phone,
		CountryCode:     u.CountryCode,
		Username:        u.Username,
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		DisplayName:     u.DisplayName,
		Organization:    u.Organization,
		Avatar:          u.Avatar,
		Role:            u.Role,
		PackageID:       u.PackageID,
		Verified:        u.Verified,
		Enabled:         u.Enabled,
		LastLogin:       u.LastLogin,
		ExpiredAt:       u.ExpiredAt,
		Properties:      u.Properties,
		HasPassword:     u.Password != "",
		PasswordSkipped: u.PasswordSkipped,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}

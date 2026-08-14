package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const ()

type ApiKey struct {
	ID            bson.ObjectID  `json:"id" bson:"_id"`
	Name          string         `json:"name" bson:"name"`
	Description   string         `json:"description,omitempty" bson:"description,omitempty"`
	ApiKey        string         `json:"apiKey,omitempty" bson:"apiKey"`
	Enabled       *bool          `json:"enabled,omitempty" bson:"enabled,omitempty"`
	ExpiredAt     *time.Time     `json:"expiredAt,omitempty" bson:"expiredAt,omitempty"`
	OwnerBy       *bson.ObjectID `json:"ownerBy,omitempty" bson:"ownerBy,omitempty"`
	CreatedAt     *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy     *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt     *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy     *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt     *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy     *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
	OwnerByUser   *UserPublic    `json:"ownerByUser,omitempty" bson:"ownerByUser,omitempty"`
	CreatedByUser *UserPublic    `json:"createdByUser,omitempty" bson:"createdByUser,omitempty"`
}

// ApiKeyOwnerCache is embedded in the Redis apikey payload so the gateway
// can validate the owner without a separate user lookup.
type ApiKeyOwnerCache struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	PackageID string     `json:"packageId,omitempty"`
	Verified  *bool      `json:"verified,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiredAt *time.Time `json:"expiredAt,omitempty"`
}

// ApiKeyCache is what gets written to Redis (gateway-facing payload).
type ApiKeyCache struct {
	ID        string            `json:"id"`
	ApiKey    string            `json:"apiKey"`
	Enabled   *bool             `json:"enabled,omitempty"`
	ExpiredAt *time.Time        `json:"expiredAt,omitempty"`
	Owner     *ApiKeyOwnerCache `json:"owner,omitempty"`
}

type ApiKeyCreate struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Enabled     *bool          `json:"enabled,omitempty"`
	ExpiredAt   *time.Time     `json:"expiredAt,omitempty"`
	UserID      *bson.ObjectID `json:"userId,omitempty"` // root/admin ระบุ owner ได้
}

type ApiKeyUpdate struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Enabled     *bool      `json:"enabled,omitempty"`
	ExpiredAt   *time.Time `json:"expiredAt,omitempty"`
	Reason      *string    `json:"reason,omitempty"` // optional, admin กรอกเหตุผลตอน suspend/enable กลับ (ไม่บังคับ)
}

type ApiKeyBulkDelete struct {
	Forever   bool            `json:"forever"`
	ApiKeyIDs []bson.ObjectID `json:"apiKeyIds"`
}

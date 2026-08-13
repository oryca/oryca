package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Configuration struct {
	ID            bson.ObjectID   `json:"id" bson:"_id,omitempty"`
	Terms         string          `json:"terms,omitempty" bson:"terms,omitempty"`
	PrivacyPolicy string          `json:"privacyPolicy,omitempty" bson:"privacyPolicy,omitempty"`
	Token         *ConfigToken    `json:"token,omitempty" bson:"token,omitempty"`
	Register      *ConfigRegister `json:"register,omitempty" bson:"register,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
}

type ConfigToken struct {
	AccessTokenExpired  int64 `json:"accessTokenExpired" bson:"accessTokenExpired"`
	RefreshTokenExpired int64 `json:"refreshTokenExpired" bson:"refreshTokenExpired"`
	MaxSessions         int   `json:"maxSessions" bson:"maxSessions"` // 0 = unlimited
}

type ConfigRegister struct {
	Enabled        bool   `json:"enabled" bson:"enabled"`
	TrialExpiresIn *int64 `json:"trialExpiresIn,omitempty" bson:"trialExpiresIn,omitempty"`
	// DefaultPackageAlias คือ package ที่ user สมัครเองจะได้รับ — ว่าง = ไม่ผูก package
	DefaultPackageAlias string `json:"defaultPackageAlias,omitempty" bson:"defaultPackageAlias,omitempty"`
}

type ConfigurationPublic struct {
	ID            bson.ObjectID   `json:"id,omitempty"`
	Terms         string          `json:"terms,omitempty"`
	PrivacyPolicy string          `json:"privacyPolicy,omitempty"`
	Register      *ConfigRegister `json:"register,omitempty"`
	UpdatedAt     *time.Time      `json:"updatedAt,omitempty"`
}

type ConfigurationUpsert struct {
	Terms         string          `json:"terms,omitempty"`
	PrivacyPolicy string          `json:"privacyPolicy,omitempty"`
	Token         *ConfigToken    `json:"token,omitempty"`
	Register      *ConfigRegister `json:"register,omitempty"`
}

package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GroupUserRef — stored format in group_users collection
type GroupUserRef struct {
	ID bson.ObjectID `json:"id" bson:"_id"`
}

// GroupUserUserRef — enriched user fields after $lookup
type GroupUserUserRef struct {
	ID           bson.ObjectID          `json:"id" bson:"_id"`
	AccountType  string                 `json:"accountType,omitempty" bson:"accountType,omitempty"`
	Email        string                 `json:"email,omitempty" bson:"email,omitempty"`
	Phone        string                 `json:"phone,omitempty" bson:"phone,omitempty"`
	CountryCode  string                 `json:"countryCode,omitempty" bson:"countryCode,omitempty"`
	Username     string                 `json:"username,omitempty" bson:"username,omitempty"`
	FirstName    string                 `json:"firstName,omitempty" bson:"firstName,omitempty"`
	LastName     string                 `json:"lastName,omitempty" bson:"lastName,omitempty"`
	DisplayName  string                 `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Organization string                 `json:"organization,omitempty" bson:"organization,omitempty"`
	Avatar       string                 `json:"avatar,omitempty" bson:"avatar,omitempty"`
	Role         string                 `json:"role,omitempty" bson:"role,omitempty"`
	Verified     *bool                  `json:"verified,omitempty" bson:"verified,omitempty"`
	Enabled      *bool                  `json:"enabled,omitempty" bson:"enabled,omitempty"`
	LastLogin    *time.Time             `json:"lastLogin,omitempty" bson:"lastLogin,omitempty"`
	ExpiredAt    *time.Time             `json:"expiredAt,omitempty" bson:"expiredAt,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty" bson:"properties,omitempty"`
}

// GroupUserGroupRef — enriched group fields after $lookup
type GroupUserGroupRef struct {
	ID    bson.ObjectID `json:"id" bson:"_id"`
	Name  string        `json:"name,omitempty" bson:"name,omitempty"`
	Alias string        `json:"alias,omitempty" bson:"alias,omitempty"`
}

// GroupUser — stored document in group_users collection
type GroupUser struct {
	ID    bson.ObjectID `json:"id" bson:"_id,omitempty"`
	User  GroupUserRef  `json:"user" bson:"user"`
	Group GroupUserRef  `json:"group" bson:"group"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

// GroupUserDetail — enriched response with joined user and group data
type GroupUserDetail struct {
	ID    bson.ObjectID     `json:"id" bson:"_id,omitempty"`
	User  GroupUserUserRef  `json:"user" bson:"user"`
	Group GroupUserGroupRef `json:"group" bson:"group"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

// GroupUserPublicDetail — same as GroupUserDetail but user field is masked to public fields only
type GroupUserPublicDetail struct {
	ID    bson.ObjectID     `json:"id"`
	User  UserPublic        `json:"user"`
	Group GroupUserGroupRef `json:"group"`

	CreatedAt *time.Time     `json:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty"`
}

type GroupUserAdd struct {
	UserIDs []bson.ObjectID `json:"userIds"`
}

type GroupUserAddResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

type GroupUserRemove struct {
	UserIDs []bson.ObjectID `json:"userIds"`
	Forever bool            `json:"forever"`
}

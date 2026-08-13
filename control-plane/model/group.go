package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Group struct {
	ID          bson.ObjectID          `json:"id" bson:"_id,omitempty"`
	Alias       string                 `json:"alias,omitempty" bson:"alias,omitempty"`
	Name        string                 `json:"name" bson:"name"`
	Description string                 `json:"description,omitempty" bson:"description,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty" bson:"properties,omitempty"`
	ParentID    *bson.ObjectID         `json:"parentId,omitempty" bson:"parentId,omitempty"`
	UserCount   int                    `json:"userCount" bson:"-"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type GroupCreate struct {
	Alias       string                 `json:"alias"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Properties  map[string]interface{} `json:"properties"`
	ParentID    *bson.ObjectID         `json:"parentId"`
}

type GroupUpdate struct {
	Alias       string                 `json:"alias"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Properties  map[string]interface{} `json:"properties"`
	ParentID    *bson.ObjectID         `json:"parentId"`
}

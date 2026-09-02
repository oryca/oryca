package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type GatewaySource struct {
	ID          bson.ObjectID     `json:"id" bson:"_id,omitempty"`
	Alias       string            `json:"alias,omitempty" bson:"alias,omitempty"`
	Name        string            `json:"name" bson:"name"`
	Description string            `json:"description,omitempty" bson:"description,omitempty"`
	Type        string            `json:"type" bson:"type"`                             // upstream | static
	Protocol    string            `json:"protocol,omitempty" bson:"protocol,omitempty"` // http | https
	URL         string            `json:"url,omitempty" bson:"url,omitempty"`
	Headers     []*SourceKeyValue `json:"headers,omitempty" bson:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty" bson:"contentType,omitempty"`
	Body        string            `json:"body,omitempty" bson:"body,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

// GatewaySourcePublic is returned to user role. Sensitive fields (URL, headers, body, contentType) are omitted
type GatewaySourcePublic struct {
	ID          bson.ObjectID `json:"id"`
	Alias       string        `json:"alias,omitempty"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Type        string        `json:"type"`
	Protocol    string        `json:"protocol,omitempty"`
}

type SourceKeyValue struct {
	Key   string `json:"key" bson:"key"`
	Value string `json:"value" bson:"value"`
}

type GatewaySourceCreate struct {
	Alias       string            `json:"alias"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Protocol    string            `json:"protocol"`
	URL         string            `json:"url"`
	Headers     []*SourceKeyValue `json:"headers"`
	ContentType string            `json:"contentType"`
	Body        string            `json:"body,omitempty"`
}

type GatewaySourceUpdate struct {
	Alias       string            `json:"alias"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Protocol    string            `json:"protocol"`
	URL         string            `json:"url"`
	Headers     []*SourceKeyValue `json:"headers"`
	ContentType string            `json:"contentType"`
	Body        string            `json:"body,omitempty"`
}

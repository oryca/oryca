package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type GatewaySpec struct {
	ID                bson.ObjectID `json:"id" bson:"_id,omitempty"`
	ServiceID         bson.ObjectID `json:"serviceId" bson:"serviceId"`
	Title             string        `json:"title" bson:"title"`
	Description       string        `json:"description,omitempty" bson:"description,omitempty"`
	Version           string        `json:"version,omitempty" bson:"version,omitempty"`
	ServerDescription string        `json:"serverDescription,omitempty" bson:"serverDescription,omitempty"`
	Paths             []*SpecPath   `json:"paths,omitempty" bson:"paths,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
}

type SpecPath struct {
	Path        string           `json:"path" bson:"path"`
	Method      string           `json:"method" bson:"method"` // GET | POST | PUT | DELETE | PATCH
	OperationId string           `json:"operationId,omitempty" bson:"operationId,omitempty"`
	Tags        []string         `json:"tags,omitempty" bson:"tags,omitempty"`
	Summary     string           `json:"summary,omitempty" bson:"summary,omitempty"`
	Description string           `json:"description,omitempty" bson:"description,omitempty"`
	Parameters  []*SpecParameter `json:"parameters,omitempty" bson:"parameters,omitempty"`
	RequestBody *SpecRequestBody `json:"requestBody,omitempty" bson:"requestBody,omitempty"`
	Responses   []*SpecResponse  `json:"responses,omitempty" bson:"responses,omitempty"`
}

type SpecSchema struct {
	Type        string                 `json:"type,omitempty" bson:"type,omitempty"`     // string | integer | number | boolean | object | array
	Format      string                 `json:"format,omitempty" bson:"format,omitempty"` // int32 | int64 | float | double | date | date-time | uuid | ...
	Description string                 `json:"description,omitempty" bson:"description,omitempty"`
	Example     any                    `json:"example,omitempty" bson:"example,omitempty"`
	Enum        []any                  `json:"enum,omitempty" bson:"enum,omitempty"`
	Required    []string               `json:"required,omitempty" bson:"required,omitempty"`
	Properties  map[string]*SpecSchema `json:"properties,omitempty" bson:"properties,omitempty"`
	Items       *SpecSchema            `json:"items,omitempty" bson:"items,omitempty"`
}

type SpecMediaType struct {
	Schema *SpecSchema `json:"schema,omitempty" bson:"schema,omitempty"`
}

type SpecRequestBody struct {
	Description string                    `json:"description,omitempty" bson:"description,omitempty"`
	Required    bool                      `json:"required,omitempty" bson:"required,omitempty"`
	Content     map[string]*SpecMediaType `json:"content,omitempty" bson:"content,omitempty"`
}

type SpecParameter struct {
	Name        string      `json:"name" bson:"name"`
	In          string      `json:"in" bson:"in"` // query | path | header
	Description string      `json:"description,omitempty" bson:"description,omitempty"`
	Required    bool        `json:"required" bson:"required"`
	Type        string      `json:"type,omitempty" bson:"type,omitempty"` // string | integer | boolean (legacy)
	Schema      *SpecSchema `json:"schema,omitempty" bson:"schema,omitempty"`
}

type SpecResponse struct {
	Status      int                       `json:"status" bson:"status"`
	Description string                    `json:"description,omitempty" bson:"description,omitempty"`
	Content     map[string]*SpecMediaType `json:"content,omitempty" bson:"content,omitempty"`
}

type GatewaySpecUpsert struct {
	Version           string      `json:"version"`
	ServerDescription string      `json:"serverDescription"`
	Paths             []*SpecPath `json:"paths"`
}

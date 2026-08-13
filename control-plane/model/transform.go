package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type TransformConfig struct {
	ID          bson.ObjectID   `bson:"_id,omitempty"            json:"id,omitempty"`
	Name        string          `bson:"name"                     json:"name"`
	Description string          `bson:"description,omitempty"    json:"description,omitempty"`
	ServiceID   bson.ObjectID   `bson:"serviceId"                json:"serviceId"`
	Match       TransformMatch  `bson:"match"                    json:"match"`
	Enabled     bool            `bson:"enabled"                  json:"enabled"`
	Rules       []TransformRule `bson:"rules"                    json:"rules"`
	CreatedAt   time.Time       `bson:"createdAt"                json:"createdAt"`
	CreatedBy   *bson.ObjectID  `bson:"createdBy,omitempty"      json:"createdBy,omitempty"`
	UpdatedAt   *time.Time      `bson:"updatedAt,omitempty"      json:"updatedAt,omitempty"`
	UpdatedBy   *bson.ObjectID  `bson:"updatedBy,omitempty"      json:"updatedBy,omitempty"`
	DeletedAt   *time.Time      `bson:"deletedAt,omitempty"      json:"deletedAt,omitempty"`
	DeletedBy   *bson.ObjectID  `bson:"deletedBy,omitempty"      json:"deletedBy,omitempty"`
}

type TransformMatch struct {
	Path    string                 `bson:"path"               json:"path"`
	Methods []string               `bson:"methods,omitempty"  json:"methods,omitempty"`
	Options map[string]interface{} `bson:"options,omitempty"  json:"options,omitempty"`
}

type TransformRule struct {
	Type       string               `bson:"type"                  json:"type"`
	Target     string               `bson:"target"                json:"target"`
	Action     string               `bson:"action"                json:"action"`
	Path       string               `bson:"path,omitempty"        json:"path,omitempty"`
	XPath      string               `bson:"xpath,omitempty"       json:"xpath,omitempty"`
	HeaderName string               `bson:"headerName,omitempty"  json:"headerName,omitempty"`
	Conditions []TransformCondition `bson:"conditions,omitempty"  json:"conditions,omitempty"`
	Params     TransformParams      `bson:"params"                json:"params"`
}

type TransformCondition struct {
	Field     string        `bson:"field"                json:"field"`
	Equals    []interface{} `bson:"equals,omitempty"     json:"equals,omitempty"`
	NotEquals []interface{} `bson:"notEquals,omitempty"  json:"notEquals,omitempty"`
}

type TransformParams struct {
	Find      string      `bson:"find,omitempty"       json:"find,omitempty"`
	Replace   string      `bson:"replace,omitempty"    json:"replace,omitempty"`
	Regex     string      `bson:"regex,omitempty"      json:"regex,omitempty"`
	Value     interface{} `bson:"value,omitempty"      json:"value,omitempty"`
	Separator string      `bson:"separator,omitempty"  json:"separator,omitempty"`
	From      string      `bson:"from,omitempty"       json:"from,omitempty"`
	To        string      `bson:"to,omitempty"         json:"to,omitempty"`
	Field     string      `bson:"field,omitempty"      json:"field,omitempty"`
}

// Request/Response DTOs

type TransformConfigRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	ServiceID   string          `json:"serviceId"`
	Match       TransformMatch  `json:"match"`
	Enabled     bool            `json:"enabled"`
	Rules       []TransformRule `json:"rules"`
}

type TransformConfigs struct {
	NumberMatched  int                `json:"numberMatched"`
	NumberReturned int                `json:"numberReturned"`
	Items          []*TransformConfig `json:"items"`
}

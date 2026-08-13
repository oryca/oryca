package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type EmailTemplate struct {
	ID        bson.ObjectID  `json:"id" bson:"_id,omitempty"`
	Name      string         `json:"name" bson:"name"`
	Alias     string         `json:"alias" bson:"alias"`
	Type      string         `json:"type" bson:"type"`
	Default   bool           `json:"default" bson:"default"`
	System    bool           `json:"system" bson:"system"`
	Subject   string         `json:"subject" bson:"subject"`
	Variables []string       `json:"variables" bson:"variables"`
	HtmlBody  string         `json:"htmlBody" bson:"htmlBody"`
	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type EmailTemplateCreate struct {
	Name      string   `json:"name"`
	Alias     string   `json:"alias"`
	Type      string   `json:"type"`
	Default   bool     `json:"default"`
	Subject   string   `json:"subject"`
	Variables []string `json:"variables"`
	HtmlBody  string   `json:"htmlBody"`
}

type EmailTemplateUpdate struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Default   bool     `json:"default"`
	Subject   string   `json:"subject"`
	Variables []string `json:"variables"`
	HtmlBody  string   `json:"htmlBody"`
}

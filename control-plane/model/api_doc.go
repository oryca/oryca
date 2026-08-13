package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	ApiDocPublishStatusDraft     = "draft"
	ApiDocPublishStatusPublished = "published"
	ApiDocAuthenRequiredNone     = "none"
	ApiDocAuthenRequiredRequired = "required"
)

type ApiDoc struct {
	ID             bson.ObjectID   `json:"id" bson:"_id,omitempty"`
	OpenAPI        string          `json:"openapi" bson:"openapi"` // Required
	AuthenRequired string          `json:"authenRequired" bson:"authenRequired"`
	PublishStatus  string          `json:"publishStatus" bson:"publishStatus"`
	Info           ApiDocInfo      `json:"info" bson:"info"`
	Servers        []*ApiDocServer `json:"servers,omitempty" bson:"servers,omitempty"`
	Tags           []*ApiDocTag    `json:"tags,omitempty" bson:"tags,omitempty"`
	Paths          []*ApiDocPath   `json:"paths,omitempty" bson:"paths,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type ApiDocInfo struct {
	Title       string  `json:"title" bson:"title"`
	Version     string  `json:"version" bson:"version"`
	Description *string `json:"description,omitempty" bson:"description,omitempty"`
}

type ApiDocServer struct {
	URL         string  `json:"url" bson:"url"`
	Description *string `json:"description,omitempty" bson:"description,omitempty"`
}

type ApiDocTag struct {
	Name        string  `json:"name" bson:"name"`
	Description *string `json:"description,omitempty" bson:"description,omitempty"`
}

type ApiDocPath struct {
	ServiceID   bson.ObjectID      `json:"serviceId,omitempty" bson:"serviceId,omitempty"`
	Path        string             `json:"path" bson:"path"`
	Method      string             `json:"method" bson:"method"`
	Tags        []string           `json:"tags,omitempty" bson:"tags,omitempty"`
	Summary     *string            `json:"summary,omitempty" bson:"summary,omitempty"`
	Description *string            `json:"description,omitempty" bson:"description,omitempty"`
	OperationID *string            `json:"operationId,omitempty" bson:"operationId,omitempty"`
	Parameters  []*ApiDocParameter `json:"parameters,omitempty" bson:"parameters,omitempty"`
	RequestBody *ApiDocRequestBody `json:"requestBody,omitempty" bson:"requestBody,omitempty"`
	Responses   []*ApiDocResponse  `json:"responses" bson:"responses"`
}

type ApiDocParameter struct {
	Name        string        `json:"name" bson:"name"`
	In          string        `json:"in" bson:"in"`
	Description *string       `json:"description,omitempty" bson:"description,omitempty"`
	Required    *bool         `json:"required,omitempty" bson:"required,omitempty"`
	Schema      *ApiDocSchema `json:"schema,omitempty" bson:"schema,omitempty"`
}

type ApiDocRequestBody struct {
	Description *string        `json:"description,omitempty" bson:"description,omitempty"`
	Required    *bool          `json:"required,omitempty" bson:"required,omitempty"`
	Content     *ApiDocContent `json:"content,omitempty" bson:"content,omitempty"`
}

type ApiDocResponse struct {
	Status      string           `json:"status" bson:"status"`
	Description string           `json:"description" bson:"description"`
	Contents    []*ApiDocContent `json:"contents,omitempty" bson:"contents,omitempty"`
}

type ApiDocContent struct {
	MediaType string        `json:"mediaType" bson:"mediaType"`
	Schema    *ApiDocSchema `json:"schema,omitempty" bson:"schema,omitempty"`
	Example   any           `json:"example,omitempty" bson:"example,omitempty"`
}

type ApiDocSchema struct {
	Type       *string                 `json:"type,omitempty" bson:"type,omitempty"`
	Items      *ApiDocSchema           `json:"items,omitempty" bson:"items,omitempty"`
	Required   []string                `json:"required,omitempty" bson:"required,omitempty"`
	Properties []*ApiDocSchemaProperty `json:"properties,omitempty" bson:"properties,omitempty"`
}

type ApiDocSchemaProperty struct {
	Property    string        `json:"property" bson:"property"`
	Description *string       `json:"description,omitempty" bson:"description,omitempty"`
	Type        *string       `json:"type,omitempty" bson:"type,omitempty"`
	Items       *ApiDocSchema `json:"items,omitempty" bson:"items,omitempty"`
	Example     any           `json:"example,omitempty" bson:"example,omitempty"`
}

type ApiDocCreate struct {
	OpenAPI        string          `json:"openapi"`
	AuthenRequired string          `json:"authenRequired"`
	PublishStatus  string          `json:"publishStatus"`
	Info           ApiDocInfo      `json:"info" `
	Servers        []*ApiDocServer `json:"servers,omitempty"`
	Tags           []*ApiDocTag    `json:"tags,omitempty"`
	Paths          []*ApiDocPath   `json:"paths,omitempty"`
}

type ApiDocUpdate struct {
	OpenAPI        string          `json:"openapi"`
	AuthenRequired string          `json:"authenRequired"`
	PublishStatus  string          `json:"publishStatus"`
	Info           ApiDocInfo      `json:"info" `
	Servers        []*ApiDocServer `json:"servers,omitempty"`
	Tags           []*ApiDocTag    `json:"tags,omitempty"`
	Paths          []*ApiDocPath   `json:"paths,omitempty"`
}

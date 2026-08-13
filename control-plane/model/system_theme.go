package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type SystemTheme struct {
	ID          bson.ObjectID          `json:"id" bson:"_id,omitempty"`
	Name        string                 `json:"name" bson:"name"`
	Description string                 `json:"description,omitempty" bson:"description,omitempty"`
	Default     bool                   `json:"default" bson:"default"`
	Logo        string                 `json:"logo,omitempty" bson:"logo,omitempty"`
	Title       string                 `json:"title,omitempty" bson:"title,omitempty"`
	ThemeConfig map[string]interface{} `json:"themeConfig,omitempty" bson:"themeConfig,omitempty"`
	CreatedAt   *time.Time             `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy   *bson.ObjectID         `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt   *time.Time             `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy   *bson.ObjectID         `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt   *time.Time             `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy   *bson.ObjectID         `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type SystemThemeCreate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Default     bool                   `json:"default"`
	Logo        string                 `json:"logo"`
	Title       string                 `json:"title"`
	ThemeConfig map[string]interface{} `json:"themeConfig"`
}

type SystemThemeUpdate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Default     bool                   `json:"default"`
	Logo        string                 `json:"logo"`
	Title       string                 `json:"title"`
	ThemeConfig map[string]interface{} `json:"themeConfig"`
}

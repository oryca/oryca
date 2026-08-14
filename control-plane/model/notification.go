package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	NotificationLevelInfo    = "info"
	NotificationLevelWarning = "warning"
	NotificationLevelError   = "error"
)

const (
	NotificationTopicApiKeySuspended        = "apikey_suspended"
	NotificationTopicPackageChanged         = "package_changed"
	NotificationTopicPackageServicesUpdated = "package_services_updated"
	NotificationTopicApiKeyExpiring         = "apikey_expiring"
)

// SystemObjectID แทนผู้สร้างจากระบบ (batch/cron). ใช้ "fff...f" กันชนกับ zero-value ของ ObjectID
var SystemObjectID bson.ObjectID

func init() {
	id, err := bson.ObjectIDFromHex("ffffffffffffffffffffffff")
	if err != nil {
		panic(err)
	}
	SystemObjectID = id
}

type Notification struct {
	ID        bson.ObjectID  `json:"id" bson:"_id,omitempty"`
	UserID    bson.ObjectID  `json:"userId" bson:"userId"`
	Topic     string         `json:"topic" bson:"topic"`
	Level     string         `json:"level" bson:"level"`
	Title     string         `json:"title" bson:"title"`
	Message   string         `json:"message" bson:"message"`
	Read      bool           `json:"read" bson:"read"`
	ReadAt    *time.Time     `json:"readAt,omitempty" bson:"readAt,omitempty"`
	DedupKey  string         `json:"dedupKey,omitempty" bson:"dedupKey,omitempty"`
	Detail    map[string]any `json:"detail,omitempty" bson:"detail,omitempty"`
	CreatedAt time.Time      `json:"createdAt" bson:"createdAt"`
	CreatedBy bson.ObjectID  `json:"createdBy" bson:"createdBy"`
}

// NotificationCreate คือ input สำหรับสร้างแจ้งเตือนใหม่ (ใช้ภายใน service เท่านั้น)
type NotificationCreate struct {
	UserID    bson.ObjectID
	Topic     string
	Level     string
	Title     string
	Message   string
	DedupKey  string
	Detail    map[string]any
	CreatedBy bson.ObjectID
}

type NotificationUnreadCount struct {
	Count int64 `json:"count"`
}

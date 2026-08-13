package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type UserSession struct {
	ID           bson.ObjectID  `bson:"_id"                    json:"id"`
	UserID       bson.ObjectID  `bson:"userId"                 json:"userId"`
	AuthMethodID *bson.ObjectID `bson:"authMethodId,omitempty" json:"authMethodId,omitempty"`
	DeviceInfo   string         `bson:"deviceInfo,omitempty"   json:"deviceInfo,omitempty"`
	Browser      string         `bson:"browser,omitempty"      json:"browser,omitempty"`
	OS           string         `bson:"os,omitempty"           json:"os,omitempty"`
	IPAddress    string         `bson:"ipAddress,omitempty"    json:"ipAddress,omitempty"`
	Location     string         `bson:"location,omitempty"     json:"location,omitempty"`
	LoginAt      time.Time      `bson:"loginAt"                json:"loginAt"`
	LogoutAt     *time.Time     `bson:"logoutAt,omitempty"     json:"logoutAt,omitempty"`
	LogoutReason string         `bson:"logoutReason,omitempty" json:"logoutReason,omitempty"`
}

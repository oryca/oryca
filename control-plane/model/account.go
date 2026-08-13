package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type AccountProfileUpdate struct {
	Username        string `json:"username"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	DisplayName     string `json:"displayName"`
	Avatar          string `json:"avatar"`
	Organization    string `json:"organization"`
	PasswordSkipped *bool  `json:"passwordSkipped,omitempty"`
}

// AccountChangePasswordRequest is the body for POST /account/change-password (local password accounts).
type AccountChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type Session struct {
	ID        bson.ObjectID `json:"id"`
	Current   bool          `json:"current"`
	Browser   string        `json:"browser,omitempty"`
	OS        string        `json:"os,omitempty"`
	IPAddress string        `json:"ipAddress,omitempty"`
	LoginAt   time.Time     `json:"loginAt"`
}

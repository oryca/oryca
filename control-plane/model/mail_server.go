package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type MailServer struct {
	ID                   bson.ObjectID   `json:"id" bson:"_id,omitempty"`
	Name                 string          `json:"name" bson:"name"`
	Default              bool            `json:"default" bson:"default"`
	Sender               string          `json:"sender" bson:"sender"`
	SenderEmail          string          `json:"senderEmail,omitempty" bson:"senderEmail,omitempty"`
	Auth                 *bool           `json:"auth,omitempty" bson:"auth,omitempty"`
	Smtp                 *MailServerSmtp `json:"smtp,omitempty" bson:"smtp,omitempty"`
	ResetPasswordExpired int64           `json:"resetPasswordExpired" bson:"resetPasswordExpired"`
	VerifyEmailExpired   int64           `json:"verifyEmailExpired" bson:"verifyEmailExpired"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type MailServerSmtp struct {
	Host          string `json:"host" bson:"host"`
	Port          string `json:"port" bson:"port"`
	User          string `json:"user" bson:"user"`
	Password      string `json:"password" bson:"password"`
	TlsSkipVerify *bool  `json:"tlsSkipVerify,omitempty" bson:"tlsSkipVerify,omitempty"`
}

type MailServerCreate struct {
	Name                 string          `json:"name"`
	Default              bool            `json:"default"`
	Sender               string          `json:"sender"`
	SenderEmail          string          `json:"senderEmail"`
	Auth                 *bool           `json:"auth"`
	Smtp                 *MailServerSmtp `json:"smtp"`
	ResetPasswordExpired int64           `json:"resetPasswordExpired"`
	VerifyEmailExpired   int64           `json:"verifyEmailExpired"`
}

type MailServerUpdate struct {
	Name                 string          `json:"name"`
	Default              bool            `json:"default"`
	Sender               string          `json:"sender"`
	SenderEmail          string          `json:"senderEmail"`
	Auth                 *bool           `json:"auth"`
	Smtp                 *MailServerSmtp `json:"smtp"`
	ResetPasswordExpired int64           `json:"resetPasswordExpired"`
	VerifyEmailExpired   int64           `json:"verifyEmailExpired"`
}

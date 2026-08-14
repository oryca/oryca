package model

import "time"

type User struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	PackageID string     `json:"packageId,omitempty"`
	Verified  *bool      `json:"verified,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiredAt *time.Time `json:"expiredAt,omitempty"`
}

type ApiKey struct {
	ID        string     `json:"id"`
	ApiKey    string     `json:"apiKey"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiredAt *time.Time `json:"expiredAt,omitempty"`
	Owner     *User      `json:"owner,omitempty"`
}

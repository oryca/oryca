package model

import "encoding/json"

type Exception struct {
	Code       string                 `json:"code,omitempty"`
	Type       string                 `json:"type,omitempty"`
	Title      string                 `json:"title,omitempty"`
	Status     int                    `json:"status,omitempty"`
	Detail     string                 `json:"detail,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

func (e Exception) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

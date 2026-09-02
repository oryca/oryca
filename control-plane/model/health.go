package model

type HealthStatus struct {
	Database HealthDetail `json:"database"`
	Redis    HealthDetail `json:"redis"`
}

type HealthDetail struct {
	Default int `json:"default"`
}

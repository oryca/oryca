package model

type ListResponse[T any] struct {
	NumberMatched  int `json:"numberMatched"`
	NumberReturned int `json:"numberReturned"`
	Items          []T `json:"items"`
}

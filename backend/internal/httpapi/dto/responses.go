package dto

type ListResponse[T any] struct {
	Data []T `json:"data"`
}

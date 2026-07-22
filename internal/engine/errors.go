package engine

import "errors"

var (
	ErrInvalidQuantity = errors.New("quantity must be greater than zero")
	ErrSymbolMismatch  = errors.New("order symbol does not match this book's symbol")
	ErrOrderNotFound   = errors.New("order is not currently resting on this book")
)

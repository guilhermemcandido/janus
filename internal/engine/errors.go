package engine

import "errors"

var (
	ErrInvalidQuantity = errors.New("quantity must be greater than zero")
	ErrInvalidPrice    = errors.New("price must be greater than zero for limit orders")
	ErrSymbolMismatch  = errors.New("order symbol does not match this book's symbol")
	ErrOrderNotFound   = errors.New("order is not currently resting on this book")
	ErrEngineStopped   = errors.New("engine has stopped")
)

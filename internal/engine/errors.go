package engine

import "errors"

var (
	ErrDuplicateOrderID = errors.New("order id already used on this book")
	ErrInvalidQuantity  = errors.New("quantity must be greater than zero")
	ErrSymbolMismatch   = errors.New("order symbol does not match this book's symbol")
)

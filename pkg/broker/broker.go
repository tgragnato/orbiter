package broker

import (
	"errors"
)

type BuyDirection int
type OrderType int

const (
	BuyDirectionLong BuyDirection = iota
	BuyDirectionShort

	OrderTypeMarket OrderType = iota
	OrderTypeLimit
)

var (
	ErrPositionNotFound    = errors.New("position not found")
	ErrUnknownBuyDirection = errors.New("unknown buy direction")
)

func (bd BuyDirection) String() string {
	return [...]string{"Long", "Short"}[bd]
}

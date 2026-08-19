package broker

import (
	"errors"
)

// BuyDirection indicates whether an order is long or short.
type BuyDirection int

// OrderType specifies how an order should be filled.
type OrderType int

// BuyDirection and OrderType constants.
const (
	// BuyDirectionLong is a buy (long) direction.
	BuyDirectionLong BuyDirection = iota
	// BuyDirectionShort is a sell (short) direction.
	BuyDirectionShort

	// OrderTypeMarket executes at the current market price.
	OrderTypeMarket OrderType = iota
	// OrderTypeLimit executes only at a specified price or better.
	OrderTypeLimit
)

// Sentinel errors returned by the broker package.
var (
	// ErrPositionNotFound is returned when a position cannot be located.
	ErrPositionNotFound = errors.New("position not found")
	// ErrUnknownBuyDirection is returned for an unrecognised direction value.
	ErrUnknownBuyDirection = errors.New("unknown buy direction")
)

func (bd BuyDirection) String() string {
	return [...]string{"Long", "Short"}[bd]
}

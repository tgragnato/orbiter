package broker

import (
	"fmt"
	"time"
)

// Order is a request to open a new position
type Order struct {
	ID            string // set by broker
	Type          OrderType
	Direction     BuyDirection
	Size          float64
	Instrument    string
	CurrencyCode  string
	TargetPrice   float64 // optional
	StopLossPrice float64 // optional
	Limit         float64 // required when Type=OrderTypeLimit
	CandleStart   time.Time
}

// NewMarketOrder creates a new order from given parameters
func NewMarketOrder(direction BuyDirection, size float64, instrument string, targetPrice, stopLossPrice float64) Order {
	return newOrderImpl(OrderTypeMarket, direction, size, instrument, targetPrice, stopLossPrice, 0)
}

func newOrderImpl(orderType OrderType, direction BuyDirection, size float64, instrument string, targetPrice, stopLossPrice, limitPrice float64) Order {
	return Order{
		Type:          orderType,
		Direction:     direction,
		Size:          size,
		Instrument:    instrument,
		TargetPrice:   targetPrice,
		StopLossPrice: stopLossPrice,
		Limit:         limitPrice,
	}
}

// HasTargetPrice checks if the optional target price has been set
func (order *Order) HasTargetPrice() bool {
	return order.TargetPrice != 0
}

// Valid checks if the given order contains malformed data
func (order *Order) Valid() error {
	if order.Type != OrderTypeMarket && order.Type != OrderTypeLimit {
		return fmt.Errorf("unknown order type %d", order.Type)
	}
	if order.Direction != BuyDirectionShort && order.Direction != BuyDirectionLong {
		return fmt.Errorf("unknown order direction %v", order.Direction)
	}
	if order.Size <= 0 {
		return fmt.Errorf("cannot be <= 0")
	}
	return nil
}

func type2String(t OrderType) string {
	switch t {
	case OrderTypeLimit:
		return "Limit"
	case OrderTypeMarket:
		return "Market"
	default:
		return "Unknown"
	}
}

func (order *Order) String() string {
	return fmt.Sprintf("{OrderID=%q Type=%s BuyDirection=%q Size=%f Target=%g Limit=%g StopLoss=%g}",
		order.ID, type2String(order.Type), order.Direction, order.Size, order.TargetPrice, order.Limit, order.StopLossPrice)
}

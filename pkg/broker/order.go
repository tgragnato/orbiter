// Copyright (c) 2019 Simon Klinkert
// Copyright (c) 2026 Tommaso Gragnato
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package broker

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnknownOrderType is returned when an order has an unrecognized type.
var ErrUnknownOrderType = errors.New("unknown order type")

// ErrUnknownOrderDirection is returned when an order has an unrecognized direction.
var ErrUnknownOrderDirection = errors.New("unknown order direction")

// ErrInvalidSize is returned when an order size is zero or negative.
var ErrInvalidSize = errors.New("size cannot be <= 0")

// Order is a request to open a new position.
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

// NewMarketOrder creates a new order from given parameters.
func NewMarketOrder(direction BuyDirection, size float64, instrument string, targetPrice, stopLossPrice float64) Order {
	return newOrderImpl(OrderTypeMarket, direction, size, instrument, targetPrice, stopLossPrice, 0)
}

func newOrderImpl(
	orderType OrderType,
	direction BuyDirection,
	size float64,
	instrument string,
	targetPrice, stopLossPrice, limitPrice float64,
) Order {
	return Order{
		ID:            "",
		Type:          orderType,
		Direction:     direction,
		Size:          size,
		Instrument:    instrument,
		CurrencyCode:  "",
		TargetPrice:   targetPrice,
		StopLossPrice: stopLossPrice,
		Limit:         limitPrice,
		CandleStart:   time.Time{},
	}
}

// HasTargetPrice checks if the optional target price has been set.
func (order *Order) HasTargetPrice() bool {
	return order.TargetPrice != 0
}

// Valid checks if the given order contains malformed data.
func (order *Order) Valid() error {
	if order.Type != OrderTypeMarket && order.Type != OrderTypeLimit {
		return fmt.Errorf("order type %d: %w", order.Type, ErrUnknownOrderType)
	}

	if order.Direction != BuyDirectionShort && order.Direction != BuyDirectionLong {
		return fmt.Errorf("order direction %v: %w", order.Direction, ErrUnknownOrderDirection)
	}

	if order.Size <= 0 {
		return ErrInvalidSize
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

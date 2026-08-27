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

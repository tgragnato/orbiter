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

// Package tick provides market tick data structures and utilities.
package tick

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Sentinel errors for tick validation.
var (
	ErrEmptyInstrument = errors.New("empty instrument")
	ErrEmptyDatetime   = errors.New("empty datetime")
	ErrEmptyBid        = errors.New("empty bid")
	ErrEmptyAsk        = errors.New("empty ask")
	ErrAskLessThanBid  = errors.New("ask is less than bid")
)

// Tick represents a market data tick with bid and ask prices.
type Tick struct {
	// ID is for keeping the order from reading CSV file in DB because the timestamp is not precise enough.
	// We have to deal with multiple ticks at the same time.
	ID         uint
	Datetime   time.Time
	Instrument string
	Bid        float64
	Ask        float64
	price      float64
}

// var maxSpread = 0.00025 // 2.5 pips.
const (
	dec2            = 2.0
	spreadPrecision = 1e10
	percentFactor   = 100
)

// New creates a new Tick with the given instrument, datetime, bid and ask prices.
func New(instrument string, datetime time.Time, bid, ask float64) Tick {
	return Tick{
		0,
		datetime,
		instrument,
		bid,
		ask,
		(bid + ask) / dec2,
	}
}

// Spread returns the absolute difference between Ask and Bid prices.
func (t *Tick) Spread() float64 {
	return math.Abs(t.Ask - t.Bid)
}

// SpreadInPercent returns the spread as a percentage of the bid price.
func (t *Tick) SpreadInPercent() float64 {
	n := (t.Ask - t.Bid) / t.Bid

	return math.Round(math.Abs(n*percentFactor)*spreadPrecision) / spreadPrecision
}

// String returns a string representation of the tick.
func (t *Tick) String() string {
	return fmt.Sprintf("{Datetime=%s Bid=%g Ask=%g}",
		t.Datetime.String(), t.Bid, t.Ask)
}

// Price returns the mid price of the tick.
func (t *Tick) Price() float64 {
	return t.price
}

// Validate checks that the tick has valid data.
func (t *Tick) Validate() error {
	if t.Instrument == "" {
		return ErrEmptyInstrument
	}

	if t.Datetime.IsZero() {
		return ErrEmptyDatetime
	}

	if t.Bid == 0 {
		return ErrEmptyBid
	}

	if t.Ask == 0 {
		return ErrEmptyAsk
	}

	if t.Ask < t.Bid {
		return ErrAskLessThanBid
	}

	return nil
}

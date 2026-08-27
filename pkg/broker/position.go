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
	"fmt"
	"math"
	"time"
)

const percentageMultiplier = 100

// Position represents an open or closed trading position.
type Position struct {
	PerformanceRecordID uint // foreign key
	Reference           string
	Instrument          string
	BuyPrice            float64
	BuyTime             time.Time
	BuyDirection        BuyDirection
	SellPrice           float64
	SellTime            time.Time
	TargetPrice         float64
	StopLossPrice       float64
	Size                float64
	OHLCAgeOnBuy        time.Duration
	CandleBuyTime       time.Time
	CandleSellTime      time.Time

	// Backtesting
	MaxSurge                  float64 // Pips
	MaxDrawdown               float64 // Pips
	TodayPerformanceInPercent float64
	GapToSMA                  float64
}

// Duration returns duration of position.
func (p *Position) Duration() time.Duration {
	return p.SellTime.Sub(p.BuyTime)
}

// PerformanceAbsolute returns the absolute performance of the position in currency units.
func (p *Position) PerformanceAbsolute(bid, ask float64) float64 {
	var abs float64

	if p.SellPrice == 0 {
		switch p.BuyDirection {
		case BuyDirectionLong:
			abs = bid - p.BuyPrice
		case BuyDirectionShort:
			abs = p.BuyPrice - ask
		}
	} else {
		switch p.BuyDirection {
		case BuyDirectionLong:
			abs = p.SellPrice - p.BuyPrice
		case BuyDirectionShort:
			abs = p.BuyPrice - p.SellPrice
		}
	}

	abs *= p.Size

	return abs
}

// PerformanceInPercentagePretty returns performance for closed positions.
func (p *Position) PerformanceInPercentagePretty() float64 {
	perf := p.PerformanceInPercentage(0, 0)

	return math.Round(perf*percentageMultiplier) / percentageMultiplier
}

// PerformanceInPercentage returns the performance of the position as a percentage.
func (p *Position) PerformanceInPercentage(bid, ask float64) float64 {
	var percentage float64

	if p.SellPrice == 0 {
		switch p.BuyDirection {
		case BuyDirectionLong:
			percentage = (bid - p.BuyPrice) / p.BuyPrice
		case BuyDirectionShort:
			if ask == 0 {
				return 0
			}

			percentage = (p.BuyPrice - ask) / ask
		}

		percentage *= percentageMultiplier
	} else {
		switch p.BuyDirection {
		case BuyDirectionLong:
			percentage = (p.SellPrice - p.BuyPrice) / p.BuyPrice
		case BuyDirectionShort:
			percentage = (p.BuyPrice - p.SellPrice) / p.SellPrice
		}

		percentage *= percentageMultiplier
	}

	return percentage
}

// Age returns how long ago the position was opened relative to now.
func (p *Position) Age(now time.Time) time.Duration {
	return now.Sub(p.BuyTime)
}

func (p *Position) String() string {
	return fmt.Sprintf("%s/%s: Direction=%s BuyLevel=%g BuyTime=%s Size=%.2f",
		p.Instrument, p.Reference, p.BuyDirection, p.BuyPrice, p.BuyTime, p.Size)
}

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

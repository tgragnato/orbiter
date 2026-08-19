// Package ohlc provides OHLC (Open High Low Close) candle data structures.
package ohlc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tgragnato/orbiter/pkg/tick"
)

const maxGapBetweenTicksInSeconds = 60

const (
	percentMultiplier    = 100
	heikinAshiOpenDiv   = 2
	heikinAshiCloseDiv  = 4
)

var (
	// ErrNoData is returned when no price data has been received.
	ErrNoData = errors.New("no data received")
	// ErrLowHigherThanHigh is returned when low is higher than high.
	ErrLowHigherThanHigh = errors.New("low is higher than High")
	// ErrOpenHigherThanHigh is returned when open is higher than high.
	ErrOpenHigherThanHigh = errors.New("open is higher than High")
	// ErrOpenLowerThanLow is returned when open is lower than low.
	ErrOpenLowerThanLow = errors.New("open is lower than Low")
	// ErrCloseHigherThanHigh is returned when close is higher than high.
	ErrCloseHigherThanHigh = errors.New("close is higher than High")
	// ErrCloseLowerThanLow is returned when close is lower than low.
	ErrCloseLowerThanLow = errors.New("close is lower than Low")
	// ErrEndBeforeStart is returned when end is before start.
	ErrEndBeforeStart = errors.New("end is before start")
	// ErrMissingInstrument is returned when instrument name is missing.
	ErrMissingInstrument = errors.New("instrument name is missing")
)

// OHLC represents a full candle.
type OHLC struct {
	Instrument        string
	Open              float64
	High              float64
	HighTime          time.Time
	Low               float64
	LowTime           time.Time
	Close             float64
	Start             time.Time
	End               time.Time
	Duration          time.Duration
	Gaps              bool
	priceDataSeen     bool
	closed            bool
	lastReceivedPrice time.Time
}

// New creates a new OHLC candle for the given instrument.
func New(instrument string, now time.Time, duration time.Duration, round bool) *OHLC {
	var start = now
	if round {
		start = smoothCandleStart(start, duration)
	}

	return &OHLC{
		Instrument:        instrument,
		Open:              0,
		High:              0,
		HighTime:          time.Time{},
		Low:               0,
		LowTime:           time.Time{},
		Close:             0,
		Start:             start,
		End:               start.Add(duration),
		Duration:          duration,
		Gaps:              false,
		priceDataSeen:     false,
		closed:            false,
		lastReceivedPrice: time.Time{},
	}
}

// Age returns the age of the candle relative to the given time.
func (o *OHLC) Age(now time.Time) time.Duration {
	return now.Sub(o.Start)
}

// Closed return true if candle is already closed (by time) or false if not.
func (o *OHLC) Closed() bool {
	return o.closed
}

// CloseTick returns a tick representing the close price.
func (o *OHLC) CloseTick() tick.Tick {
	return tick.New(o.Instrument, o.End, o.Close, o.Close)
}

// ForceClose closes the candle immediately, setting End to the last received price time.
func (o *OHLC) ForceClose() {
	o.closed = true
	o.End = o.lastReceivedPrice
}

// HasGaps returns true if the candle has gaps in price data.
func (o *OHLC) HasGaps() bool {
	return o.Gaps
}

// HasPriceData returns true if the candle has received at least one price.
func (o *OHLC) HasPriceData() bool {
	return o.priceDataSeen
}

// HighTick returns a tick representing the high price.
func (o *OHLC) HighTick() tick.Tick {
	return tick.New(o.Instrument, o.HighTime, o.High, o.High)
}

// LowTick returns a tick representing the low price.
func (o *OHLC) LowTick() tick.Tick {
	return tick.New(o.Instrument, o.LowTime, o.Low, o.Low)
}

// NewPrice handles new price data.
// Returns true if data was considered.
// Returns false if the candle is already closed.
func (o *OHLC) NewPrice(price float64, now time.Time) bool {
	if o.Closed() {
		return false
	}

	if now.After(o.End) || now.Equal(o.End) {
		o.closed = true

		return false
	}

	if price > o.High {
		o.High = price
		o.HighTime = now
	} else if price < o.Low {
		o.Low = price
		o.LowTime = now
	}

	if !o.priceDataSeen {
		o.Open = price
		o.Low = price
		o.LowTime = now
	}

	if o.priceDataSeen {
		diffLastReceivedValidPrice := now.Sub(o.lastReceivedPrice)
		if diffLastReceivedValidPrice.Seconds() > maxGapBetweenTicksInSeconds {
			o.Gaps = true
		}
	}

	o.lastReceivedPrice = now
	o.Close = price
	o.priceDataSeen = true

	return true
}

// OpenTick returns a tick representing the open price.
func (o *OHLC) OpenTick() tick.Tick {
	return tick.New(o.Instrument, o.Start, o.Open, o.Open)
}

// PerformanceFromOpenToHighAbsolute returns the percentage performance from open to high.
func (o *OHLC) PerformanceFromOpenToHighAbsolute() float64 {
	if o.Open == 0 {
		return 0
	}

	return ((o.High - o.Open) / o.Open * percentMultiplier)
}

// PerformanceFromOpenToLowAbsolute returns the percentage performance from open to low.
func (o *OHLC) PerformanceFromOpenToLowAbsolute() float64 {
	if o.Open == 0 {
		return 0
	}

	return ((o.Low - o.Open) / o.Open * percentMultiplier)
}

// PerformanceInPercentage returns the percentage change from open to close.
func (o *OHLC) PerformanceInPercentage() float64 {
	if o.Open == 0 {
		return 0
	}

	return ((o.Close - o.Open) / o.Open * percentMultiplier)
}

// ReversionPerformanceFromHighAbsolute returns the percentage reversion from high to close.
func (o *OHLC) ReversionPerformanceFromHighAbsolute() float64 {
	if o.High == 0 {
		return 0
	}

	return ((o.Close - o.High) / o.High * percentMultiplier)
}

// Store persists the OHLC candle to the database.
func (o *OHLC) Store(ctx context.Context, db *sql.DB) error {
	ohlcCopy := *o
	ohlcCopy.Start = ohlcCopy.Start.In(time.UTC)
	ohlcCopy.End = ohlcCopy.End.In(time.UTC)

	_, err := db.ExecContext(ctx, `
		INSERT INTO ohlcs (
			instrument, open, high, high_time, low, low_time, close, start, end_time, duration_ns, gaps
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		ohlcCopy.Instrument,
		ohlcCopy.Open,
		ohlcCopy.High,
		ohlcCopy.HighTime,
		ohlcCopy.Low,
		ohlcCopy.LowTime,
		ohlcCopy.Close,
		ohlcCopy.Start,
		ohlcCopy.End,
		int64(ohlcCopy.Duration),
		ohlcCopy.Gaps,
	)
	if err != nil {
		return fmt.Errorf("ohlc store: %w", err)
	}

	return nil
}

// String returns a human-readable representation of the candle.
func (o *OHLC) String() string {
	return fmt.Sprintf("OHLC(%s, Open=%g High=%g Low=%g Close=%g, Start=%s End=%s)",
		o.Instrument, o.Open, o.High, o.Low, o.Close, o.Start, o.End)
}

// ToTicks converts the OHLC candle to 4 ticks. It ensures the correct
// chronological order of high and low.
func (o *OHLC) ToTicks() []tick.Tick {
	var ticks []tick.Tick

	var high = o.HighTick()

	var low = o.LowTick()

	ticks = append(ticks, o.OpenTick())
	if high.Datetime.After(low.Datetime) {
		ticks = append(ticks, low, high)
	} else {
		ticks = append(ticks, high, low)
	}

	ticks = append(ticks, o.CloseTick())

	return ticks
}

// Validate returns an error if the candle is invalid.
func (o *OHLC) Validate() error {
	if !o.priceDataSeen {
		return ErrNoData
	}

	if o.Low > o.High {
		return ErrLowHigherThanHigh
	}

	if o.Open > o.High {
		return ErrOpenHigherThanHigh
	}

	if o.Open < o.Low {
		return ErrOpenLowerThanLow
	}

	if o.Close > o.High {
		return ErrCloseHigherThanHigh
	}

	if o.Close < o.Low {
		return ErrCloseLowerThanLow
	}

	if o.End.Before(o.Start) {
		return ErrEndBeforeStart
	}

	if o.Instrument == "" {
		return ErrMissingInstrument
	}

	return nil
}

// VolatilityInPercentage returns the volatility of the candle as a percentage of open.
func (o *OHLC) VolatilityInPercentage() float64 {
	if o.Open == 0 {
		return 0
	}

	return ((o.High - o.Low) / o.Open * percentMultiplier)
}

// smoothCandleStart rounds ts to the closest period.
func smoothCandleStart(ts time.Time, period time.Duration) time.Time {
	minuteSlot := ts.Minute() / int(period.Minutes()) * int(period.Minutes())

	return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), minuteSlot, 0, 0, ts.Location())
}

// ToHeikinAshi calculates a Heikin Ashi candle from two OHLC candles.
func ToHeikinAshi(previous, now *OHLC) *OHLC {
	heikinAshi := *now
	heikinAshi.Open = (previous.Open + previous.Close) / heikinAshiOpenDiv
	heikinAshi.Close = (now.Open + now.Close + now.High + now.Low) / heikinAshiCloseDiv
	heikinAshi.High = max(now.High, heikinAshi.Open, heikinAshi.Close)
	heikinAshi.Low = min(now.Low, heikinAshi.Open, heikinAshi.Close)

	return &heikinAshi
}

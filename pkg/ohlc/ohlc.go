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

// OHLC represents a full candle
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

func New(instrument string, now time.Time, duration time.Duration, round bool) *OHLC {
	var start = now
	if round {
		start = smoothCandleStart(start, duration)
	}
	return &OHLC{
		Instrument: instrument,
		Start:      start,
		End:        start.Add(duration),
		Duration:   duration,
	}
}

func (o *OHLC) ForceClose() {
	o.closed = true
	o.End = o.lastReceivedPrice
}

func (o *OHLC) String() string {
	return fmt.Sprintf("OHLC(%s, Open=%g High=%g Low=%g Close=%g, Start=%s End=%s)",
		o.Instrument, o.Open, o.High, o.Low, o.Close, o.Start, o.End)
}

// Closed return true if candle is already closed (by time) or false if not.
func (o *OHLC) Closed() bool {
	return o.closed
}

// NewPrice handles new price data
// Returns true if data was considered
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

func (o *OHLC) HasGaps() bool {
	return o.Gaps
}

func (o *OHLC) HasPriceData() bool {
	return o.priceDataSeen
}

func (o *OHLC) Validate() error {
	if !o.priceDataSeen {
		return errors.New("no data received")
	}
	if o.Low > o.High {
		return errors.New("low is higher than High")
	}
	if o.Open > o.High {
		return errors.New("open is higher than High")
	}
	if o.Open < o.Low {
		return errors.New("open is lower than Low")
	}
	if o.Close > o.High {
		return errors.New("close is higher than High")
	}
	if o.Close < o.Low {
		return errors.New("close is lower than Low")
	}
	if o.End.Before(o.Start) {
		return errors.New("end is before start")
	}
	if o.Instrument == "" {
		return errors.New("instrument name is missing")
	}
	return nil
}

func (o *OHLC) PerformanceFromOpenToHighAbsolute() float64 {
	if o.Open == 0 {
		return 0
	}
	return ((o.High - o.Open) / o.Open * 100)
}

func (o *OHLC) PerformanceFromOpenToLowAbsolute() float64 {
	if o.Open == 0 {
		return 0
	}
	return ((o.Low - o.Open) / o.Open * 100)
}

func (o *OHLC) ReversionPerformanceFromHighAbsolute() float64 {
	if o.High == 0 {
		return 0
	}
	return ((o.Close - o.High) / o.High * 100)
}

func (o *OHLC) PerformanceInPercentage() float64 {
	if o.Open == 0 {
		return 0
	}
	return ((o.Close - o.Open) / o.Open * 100)
}

func (o *OHLC) VolatilityInPercentage() float64 {
	if o.Open == 0 {
		return 0
	}
	return ((o.High - o.Low) / o.Open * 100)
}

func (o *OHLC) Age(now time.Time) time.Duration {
	return now.Sub(o.Start)
}

func (o *OHLC) Store(ctx context.Context, db *sql.DB) error {
	var oc = *o
	oc.Start = oc.Start.In(time.UTC)
	oc.End = oc.End.In(time.UTC)
	_, err := db.ExecContext(ctx, `
		INSERT INTO ohlcs (
			instrument, open, high, high_time, low, low_time, close, start, end_time, duration_ns, gaps
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		oc.Instrument,
		oc.Open,
		oc.High,
		oc.HighTime,
		oc.Low,
		oc.LowTime,
		oc.Close,
		oc.Start,
		oc.End,
		int64(oc.Duration),
		oc.Gaps,
	)
	if err != nil {
		return err
	}
	return nil
}

func (o *OHLC) OpenTick() tick.Tick {
	return tick.New(o.Instrument, o.Start, o.Open, o.Open)
}

func (o *OHLC) CloseTick() tick.Tick {
	return tick.New(o.Instrument, o.End, o.Close, o.Close)
}

func (o *OHLC) HighTick() tick.Tick {
	return tick.New(o.Instrument, o.HighTime, o.High, o.High)
}

func (o *OHLC) LowTick() tick.Tick {
	return tick.New(o.Instrument, o.LowTime, o.Low, o.Low)
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

// round ts to the closest period
func smoothCandleStart(ts time.Time, period time.Duration) time.Time {
	return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute()/int(period.Minutes())*int(period.Minutes()), 0, 0, ts.Location())
}

// ToHeikinAshi calculates a Heikin Ashi candle from two OHLC candles
func ToHeikinAshi(previous, now *OHLC) *OHLC {
	ha := *now
	ha.Open = (previous.Open + previous.Close) / 2
	ha.Close = (now.Open + now.Close + now.High + now.Low) / 4
	ha.High = max(now.High, ha.Open, ha.Close)
	ha.Low = min(now.Low, ha.Open, ha.Close)
	return &ha
}

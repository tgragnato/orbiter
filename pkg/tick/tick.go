package tick

import (
	"errors"
	"fmt"
	"math"
	"time"
)

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

// var maxSpread = 0.00025) // 2.5 pips
const dec2 = 2.0

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

func (t *Tick) Spread() float64 {
	return math.Abs(t.Ask - t.Bid)
}

func (t *Tick) SpreadInPercent() float64 {
	var n = (t.Ask - t.Bid) / t.Bid
	return math.Round(math.Abs(n*100)*1e10) / 1e10
}

func (t *Tick) String() string {
	return fmt.Sprintf("{Datetime=%s Bid=%g Ask=%g}",
		t.Datetime.String(), t.Bid, t.Ask)
}

func (t *Tick) Price() float64 {
	return t.price
}

func (t *Tick) Validate() error {
	if t.Instrument == "" {
		return errors.New("empty instrument")
	}
	if t.Datetime.IsZero() {
		return errors.New("empty datetime")
	}
	if t.Bid == 0 {
		return errors.New("empty bid")
	}
	if t.Ask == 0 {
		return errors.New("empty ask")
	}
	if t.Ask < t.Bid {
		return errors.New("ask is less than bid")
	}
	return nil
}

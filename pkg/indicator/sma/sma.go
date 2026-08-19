package sma

import (
	"fmt"

	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Value is the key used to store the SMA value in the result map.
const Value = "SMA_VALUE"

// SMA computes a simple moving average over a fixed window of OHLC close prices.
type SMA struct {
	cb *circularbuffer.CircularBuffer
}

// New creates a new SMA indicator with the given window size.
func New(size int) *SMA {
	return &SMA{
		cb: circularbuffer.New(size, size),
	}
}

// Insert adds the close price of a closed OHLC bar to the moving average window.
func (v *SMA) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.Insert(o.Close)
}

// Value returns a map containing the current SMA value.
func (v *SMA) Value() (map[string]float64, error) {
	result := map[string]float64{}

	avg, avgErr := v.cb.Average()
	if avgErr != nil {
		return result, fmt.Errorf("sma average: %w", avgErr)
	}

	result[Value] = avg

	return result, nil
}

// ValueResultKeys returns the list of keys present in the Value result map.
func (v *SMA) ValueResultKeys() []string {
	return []string{Value}
}

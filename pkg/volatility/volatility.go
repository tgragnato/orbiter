// Package volatility computes price volatility metrics using a circular buffer of OHLC samples.
package volatility

import (
	"fmt"

	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Volatility tracks price volatility using a circular buffer of historical OHLC data.
type Volatility struct {
	cb *circularbuffer.CircularBuffer
}

// New creates a new Volatility tracker with the given minimum and maximum buffer sizes.
func New(minSize, maxSize int) *Volatility {
	return &Volatility{
		cb: circularbuffer.New(minSize, maxSize),
	}
}

// AddOHLC records the volatility of a closed OHLC bar into the circular buffer.
func (vol *Volatility) AddOHLC(bar *ohlc.OHLC) {
	if !bar.Closed() {
		return
	}

	vol.cb.Insert(bar.VolatilityInPercentage())
}

// MedianVolatilityInPercentage returns the median volatility across all buffered samples.
func (vol *Volatility) MedianVolatilityInPercentage() (float64, error) {
	median, err := vol.cb.Median()
	if err != nil {
		return 0, fmt.Errorf("median volatility: %w", err)
	}

	return median, nil
}

// VolatilityInPercentageQuantile returns the volatility at the given quantile (0–1).
func (vol *Volatility) VolatilityInPercentageQuantile(quantile float64) (float64, error) {
	result, err := vol.cb.Quantile(quantile)
	if err != nil {
		return 0, fmt.Errorf("volatility quantile: %w", err)
	}

	return result, nil
}

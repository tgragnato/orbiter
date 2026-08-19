// Package stoch provides a Stochastic oscillator indicator.
package stoch

import (
	"fmt"

	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// ValueK is the map key for the Stochastic %K value.
const ValueK = "Stoch_VALUE_K"

// ValueD is the map key for the Stochastic %D value.
const ValueD = "Stoch_VALUE_D"

// Stoch computes the Stochastic oscillator over a circular buffer of OHLC data.
type Stoch struct {
	closePrices *circularbuffer.CircularBuffer
	highPrices  *circularbuffer.CircularBuffer
	lowPrices   *circularbuffer.CircularBuffer
	fastKPeriod int
	fastDPeriod int
}

// New creates a new Stoch indicator with the given fast K and fast D periods.
func New(fastKPeriod, fastDPeriod int) *Stoch {
	return &Stoch{
		closePrices: circularbuffer.New(1, fastKPeriod*fastDPeriod),
		highPrices:  circularbuffer.New(1, fastKPeriod*fastDPeriod),
		lowPrices:   circularbuffer.New(1, fastKPeriod*fastDPeriod),
		fastKPeriod: fastKPeriod,
		fastDPeriod: fastDPeriod,
	}
}

// Insert adds a closed OHLC bar to the indicator buffers.
func (v *Stoch) Insert(bar *ohlc.OHLC) {
	if !bar.Closed() {
		return
	}

	v.closePrices.Insert(bar.Close)
	v.highPrices.Insert(bar.High)
	v.lowPrices.Insert(bar.Low)
}

// Value returns the current %K and %D values of the Stochastic oscillator.
func (v *Stoch) Value() (map[string]float64, error) {
	result := map[string]float64{}

	closePrices, err := v.closePrices.GetAll()
	if err != nil {
		return nil, fmt.Errorf("stoch close prices: %w", err)
	}

	highPrices, err := v.highPrices.GetAll()
	if err != nil {
		return nil, fmt.Errorf("stoch high prices: %w", err)
	}

	lowPrices, err := v.lowPrices.GetAll()
	if err != nil {
		return nil, fmt.Errorf("stoch low prices: %w", err)
	}

	stochK, stochD := talib.StochF(highPrices, lowPrices, closePrices, v.fastKPeriod, v.fastDPeriod, talib.SMA)

	if len(stochK) > 0 {
		result[ValueK] = stochK[len(stochK)-1]
	}

	if len(stochD) > 0 {
		result[ValueD] = stochK[len(stochD)-1]
	}

	return result, nil
}

// Package stochrsi implements the Stochastic RSI technical indicator.
package stochrsi

import (
	"fmt"

	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// ValueK is the map key for the StochRSI %K line value.
const ValueK = "StochRSI_VALUE_K"

// ValueD is the map key for the StochRSI %D line value.
const ValueD = "StochRSI_VALUE_D"

const bufferMultiplier = 3

// StochRSI computes the Stochastic RSI indicator over a circular buffer of close prices.
type StochRSI struct {
	cb           *circularbuffer.CircularBuffer
	fastKPeriod  int
	fastDPeriod  int
	inTimePeriod int
}

// New creates a new StochRSI indicator with the given fast K period, fast D period, and lookback size.
func New(fastKPeriod, fastDPeriod, size int) *StochRSI {
	return &StochRSI{
		cb:           circularbuffer.New(size*bufferMultiplier, size*bufferMultiplier),
		inTimePeriod: size,
		fastKPeriod:  fastKPeriod,
		fastDPeriod:  fastDPeriod,
	}
}

// Insert adds a closed OHLC bar's close price to the internal buffer.
func (v *StochRSI) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.Insert(o.Close)
}

// Value computes and returns the current StochRSI %K and %D values.
func (v *StochRSI) Value() (map[string]float64, error) {
	result := map[string]float64{}

	closePrices, err := v.cb.GetAll()
	if err != nil {
		return nil, fmt.Errorf("stochrsi get all: %w", err)
	}

	fastK, fastD := talib.StochRsi(closePrices, v.inTimePeriod, v.fastKPeriod, v.fastDPeriod, talib.SMA)

	if len(fastK) > 0 {
		result[ValueK] = fastK[len(fastK)-1]
	}

	if len(fastD) > 0 {
		result[ValueD] = fastK[len(fastD)-1]
	}

	return result, nil
}

// ValueResultKeys returns the map keys produced by Value.
func (v *StochRSI) ValueResultKeys() []string {
	return []string{ValueK, ValueD}
}

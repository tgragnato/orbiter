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

// rsiMidpoint is the boundary used to disambiguate the degenerate StochRSI case.
const rsiMidpoint = 50.0

// Value computes and returns the current StochRSI %K and %D values.
//
// When all RSI values in the fastK lookback window are identical (zero range),
// talib returns 0 for both K and D regardless of whether RSI is high or low.
// In that degenerate case Value resolves the ambiguity by inspecting the last
// RSI value directly: an RSI above the midpoint maps to 100 (fully overbought)
// and at or below the midpoint maps to 0 (fully oversold).
func (v *StochRSI) Value() (map[string]float64, error) {
	result := map[string]float64{}

	closePrices, err := v.cb.GetAll()
	if err != nil {
		return nil, fmt.Errorf("stochrsi get all: %w", err)
	}

	fastK, fastD := talib.StochRsi(closePrices, v.inTimePeriod, v.fastKPeriod, v.fastDPeriod, talib.SMA)

	kVal := 0.0
	if len(fastK) > 0 {
		kVal = fastK[len(fastK)-1]
	}

	dVal := 0.0
	if len(fastD) > 0 {
		dVal = fastD[len(fastD)-1]
	}

	// Resolve the degenerate case: when every RSI value in the K-window is
	// equal, talib divides 0/0 and returns 0 for both lines.  We detect this
	// by checking whether RSI itself is above the midpoint.
	if kVal == 0 && dVal == 0 {
		rsiValues := talib.Rsi(closePrices, v.inTimePeriod)
		if len(rsiValues) > 0 && rsiValues[len(rsiValues)-1] > rsiMidpoint {
			kVal = 100
			dVal = 100
		}
	}

	result[ValueK] = kVal
	result[ValueD] = dVal

	return result, nil
}

// ValueResultKeys returns the map keys produced by Value.
func (v *StochRSI) ValueResultKeys() []string {
	return []string{ValueK, ValueD}
}

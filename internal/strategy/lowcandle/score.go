package lowcandle

import (
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Score returns ±0.8 when the close breaks the 7-candle high/low range (per the
// LowCandle breakout logic), 0 otherwise. Sign is positive for a buy breakout
// (close < previous lows) and negative for a sell breakout (close > previous highs).
func (d *LowCandle) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) == 0 {
		return 0
	}
	closePrice := helper.DecimalToFloat(closedCandles[len(closedCandles)-1].Close)

	prevLow, errL := d.previousLows.Min()
	prevHigh, errH := d.previousHighs.Max()

	if errL == nil && closePrice < prevLow {
		return 0.8
	}
	if errH == nil && closePrice > prevHigh {
		return -0.8
	}
	return 0
}

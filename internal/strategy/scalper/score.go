package scalper

import (
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Score returns ±0.8 when a counter-trend scalp signal is detected (the last
// candle direction differs from all 9 prior candles), 0 otherwise.
// A bullish-momentum setup (all 9 prior long, last short) returns -0.8
// (fade the trend → short signal). A bearish setup returns +0.8.
func (mr *scalper) Score(closedCandles []*ohlc.OHLC) float64 {
	const candles = 10
	if len(closedCandles) < candles {
		return 0
	}

	last := closedCandles[len(closedCandles)-1]
	lastDir := getBuyDirection(last)

	for i := len(closedCandles) - candles; i < len(closedCandles)-1; i++ {
		if getBuyDirection(closedCandles[i]) == lastDir {
			return 0
		}
	}

	// Prior candles are all in the opposite direction: fade them.
	switch lastDir {
	case broker.BuyDirectionLong:
		// Last candle long, all prior short → counter long → buy signal
		return 0.8
	default:
		// Last candle short, all prior long → counter short → sell signal
		return -0.8
	}
}
